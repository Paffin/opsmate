package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/paffin/opsmate/pkg/mcputil"
	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type handlers struct {
	client    kubernetes.Interface
	dynamic   dynamic.Interface
	readonly  bool
	maxLines  int
}

func (h *handlers) getPods(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ns := mcputil.OptionalString(req, "namespace", "")
	labelSelector := mcputil.OptionalString(req, "label_selector", "")

	opts := metav1.ListOptions{LabelSelector: labelSelector}
	pods, err := h.client.CoreV1().Pods(ns).List(ctx, opts)
	if err != nil {
		return mcputil.ToolError("failed to list pods: %v", err)
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "NAMESPACE\tNAME\tSTATUS\tRESTARTS\tAGE\n")

	for _, pod := range pods.Items {
		restarts := int32(0)
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += cs.RestartCount
		}
		age := formatAge(pod.CreationTimestamp.Time)
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
			pod.Namespace, pod.Name, string(pod.Status.Phase), restarts, age)
	}
	w.Flush()

	return mcputil.ToolText("%d pods found:\n\n%s", len(pods.Items), buf.String())
}

func (h *handlers) getPodLogs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pod, err := mcputil.RequireString(req, "pod")
	if err != nil {
		return mcputil.ToolError("pod name is required")
	}

	ns := mcputil.OptionalString(req, "namespace", "default")
	container := mcputil.OptionalString(req, "container", "")
	tailLines := int64(mcputil.OptionalInt(req, "tail_lines", 100))

	if h.maxLines > 0 && tailLines > int64(h.maxLines) {
		tailLines = int64(h.maxLines)
	}

	opts := &corev1.PodLogOptions{
		TailLines: &tailLines,
	}
	if container != "" {
		opts.Container = container
	}

	stream, err := h.client.CoreV1().Pods(ns).GetLogs(pod, opts).Stream(ctx)
	if err != nil {
		return mcputil.ToolError("failed to get logs: %v", err)
	}
	defer stream.Close()

	logs, err := io.ReadAll(stream)
	if err != nil {
		return mcputil.ToolError("failed to read logs: %v", err)
	}

	return mcputil.ToolText("Logs for %s/%s (last %d lines):\n\n%s", ns, pod, tailLines, string(logs))
}

func (h *handlers) describe(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	kind, err := mcputil.RequireString(req, "kind")
	if err != nil {
		return mcputil.ToolError("kind is required")
	}
	name, err := mcputil.RequireString(req, "name")
	if err != nil {
		return mcputil.ToolError("name is required")
	}
	ns := mcputil.OptionalString(req, "namespace", "default")

	var result string
	switch strings.ToLower(kind) {
	case "pod":
		pod, err := h.client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcputil.ToolError("failed to get pod: %v", err)
		}
		result = formatPodDescription(pod)
	case "deployment":
		dep, err := h.client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcputil.ToolError("failed to get deployment: %v", err)
		}
		result = fmt.Sprintf("Deployment: %s/%s\nReplicas: %d desired, %d available, %d ready\nStrategy: %s\nSelector: %v\nTemplate Labels: %v",
			dep.Namespace, dep.Name,
			*dep.Spec.Replicas, dep.Status.AvailableReplicas, dep.Status.ReadyReplicas,
			dep.Spec.Strategy.Type, dep.Spec.Selector.MatchLabels, dep.Spec.Template.Labels)
	case "service":
		svc, err := h.client.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcputil.ToolError("failed to get service: %v", err)
		}
		ports := []string{}
		for _, p := range svc.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%s:%d→%d", p.Protocol, p.Port, p.TargetPort.IntValue()))
		}
		result = fmt.Sprintf("Service: %s/%s\nType: %s\nClusterIP: %s\nPorts: %s\nSelector: %v",
			svc.Namespace, svc.Name, svc.Spec.Type, svc.Spec.ClusterIP, strings.Join(ports, ", "), svc.Spec.Selector)
	case "node":
		node, err := h.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcputil.ToolError("failed to get node: %v", err)
		}
		conditions := []string{}
		for _, c := range node.Status.Conditions {
			conditions = append(conditions, fmt.Sprintf("%s=%s", c.Type, c.Status))
		}
		result = fmt.Sprintf("Node: %s\nConditions: %s\nCapacity: CPU=%s, Memory=%s\nAllocatable: CPU=%s, Memory=%s",
			node.Name, strings.Join(conditions, ", "),
			node.Status.Capacity.Cpu().String(), node.Status.Capacity.Memory().String(),
			node.Status.Allocatable.Cpu().String(), node.Status.Allocatable.Memory().String())
	default:
		return mcputil.ToolError("unsupported resource kind: %s (supported: pod, deployment, service, node)", kind)
	}

	return mcputil.ToolText("%s", result)
}

func (h *handlers) getEvents(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ns := mcputil.OptionalString(req, "namespace", "")
	fieldSelector := mcputil.OptionalString(req, "field_selector", "")

	opts := metav1.ListOptions{FieldSelector: fieldSelector}
	events, err := h.client.CoreV1().Events(ns).List(ctx, opts)
	if err != nil {
		return mcputil.ToolError("failed to list events: %v", err)
	}

	sort.Slice(events.Items, func(i, j int) bool {
		return events.Items[i].LastTimestamp.After(events.Items[j].LastTimestamp.Time)
	})

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "TIME\tTYPE\tREASON\tOBJECT\tMESSAGE\n")

	limit := 50
	for i, event := range events.Items {
		if i >= limit {
			break
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s/%s\t%s\n",
			formatAge(event.LastTimestamp.Time),
			event.Type, event.Reason,
			event.InvolvedObject.Kind, event.InvolvedObject.Name,
			truncate(event.Message, 80))
	}
	w.Flush()

	return mcputil.ToolText("%d events (showing last %d):\n\n%s", len(events.Items), min(limit, len(events.Items)), buf.String())
}

func (h *handlers) getNodes(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	nodes, err := h.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return mcputil.ToolError("failed to list nodes: %v", err)
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tSTATUS\tROLES\tAGE\tCPU\tMEMORY\n")

	for _, node := range nodes.Items {
		status := "NotReady"
		for _, c := range node.Status.Conditions {
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
				status = "Ready"
			}
		}
		roles := []string{}
		for label := range node.Labels {
			if strings.HasPrefix(label, "node-role.kubernetes.io/") {
				roles = append(roles, strings.TrimPrefix(label, "node-role.kubernetes.io/"))
			}
		}
		if len(roles) == 0 {
			roles = []string{"<none>"}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			node.Name, status, strings.Join(roles, ","),
			formatAge(node.CreationTimestamp.Time),
			node.Status.Capacity.Cpu().String(),
			node.Status.Capacity.Memory().String())
	}
	w.Flush()

	return mcputil.ToolText("%d nodes:\n\n%s", len(nodes.Items), buf.String())
}

func (h *handlers) getDeployments(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ns := mcputil.OptionalString(req, "namespace", "")

	deployments, err := h.client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return mcputil.ToolError("failed to list deployments: %v", err)
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "NAMESPACE\tNAME\tREADY\tUP-TO-DATE\tAVAILABLE\tAGE\n")

	for _, d := range deployments.Items {
		fmt.Fprintf(w, "%s\t%s\t%d/%d\t%d\t%d\t%s\n",
			d.Namespace, d.Name,
			d.Status.ReadyReplicas, *d.Spec.Replicas,
			d.Status.UpdatedReplicas, d.Status.AvailableReplicas,
			formatAge(d.CreationTimestamp.Time))
	}
	w.Flush()

	return mcputil.ToolText("%d deployments:\n\n%s", len(deployments.Items), buf.String())
}

func (h *handlers) getServices(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ns := mcputil.OptionalString(req, "namespace", "")

	services, err := h.client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return mcputil.ToolError("failed to list services: %v", err)
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	fmt.Fprintf(w, "NAMESPACE\tNAME\tTYPE\tCLUSTER-IP\tPORTS\tAGE\n")

	for _, svc := range services.Items {
		ports := []string{}
		for _, p := range svc.Spec.Ports {
			ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			svc.Namespace, svc.Name, svc.Spec.Type,
			svc.Spec.ClusterIP, strings.Join(ports, ","),
			formatAge(svc.CreationTimestamp.Time))
	}
	w.Flush()

	return mcputil.ToolText("%d services:\n\n%s", len(services.Items), buf.String())
}

func (h *handlers) apply(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if h.readonly {
		return mcputil.ToolError("cluster is in read-only mode, apply is disabled")
	}

	yamlContent, err := mcputil.RequireString(req, "yaml_content")
	if err != nil {
		return mcputil.ToolError("yaml_content is required")
	}

	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	_, gvk, err := decUnstructured.Decode([]byte(yamlContent), nil, obj)
	if err != nil {
		return mcputil.ToolError("failed to parse YAML: %v", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: strings.ToLower(gvk.Kind) + "s",
	}

	ns := obj.GetNamespace()
	if ns == "" {
		ns = "default"
	}

	_, err = h.dynamic.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return mcputil.ToolError("failed to apply: %v", err)
	}

	return mcputil.ToolText("Successfully applied %s/%s in namespace %s", gvk.Kind, obj.GetName(), ns)
}

func (h *handlers) scale(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if h.readonly {
		return mcputil.ToolError("cluster is in read-only mode, scale is disabled")
	}

	deployment, err := mcputil.RequireString(req, "deployment")
	if err != nil {
		return mcputil.ToolError("deployment name is required")
	}
	replicas := int32(mcputil.OptionalInt(req, "replicas", -1))
	if replicas < 0 {
		return mcputil.ToolError("replicas is required")
	}
	ns := mcputil.OptionalString(req, "namespace", "default")

	s, err := h.client.AppsV1().Deployments(ns).GetScale(ctx, deployment, metav1.GetOptions{})
	if err != nil {
		return mcputil.ToolError("failed to get scale: %v", err)
	}

	oldReplicas := s.Spec.Replicas
	s.Spec.Replicas = replicas

	_, err = h.client.AppsV1().Deployments(ns).UpdateScale(ctx, deployment, s, metav1.UpdateOptions{})
	if err != nil {
		return mcputil.ToolError("failed to scale: %v", err)
	}

	return mcputil.ToolText("Scaled deployment %s/%s from %d to %d replicas", ns, deployment, oldReplicas, replicas)
}

func (h *handlers) rolloutStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	deployment, err := mcputil.RequireString(req, "deployment")
	if err != nil {
		return mcputil.ToolError("deployment name is required")
	}
	ns := mcputil.OptionalString(req, "namespace", "default")

	dep, err := h.client.AppsV1().Deployments(ns).Get(ctx, deployment, metav1.GetOptions{})
	if err != nil {
		return mcputil.ToolError("failed to get deployment: %v", err)
	}

	var status string
	if dep.Status.UpdatedReplicas == *dep.Spec.Replicas &&
		dep.Status.ReadyReplicas == *dep.Spec.Replicas &&
		dep.Status.AvailableReplicas == *dep.Spec.Replicas {
		status = "✓ Successfully rolled out"
	} else {
		status = fmt.Sprintf("⟳ In progress: %d/%d replicas updated, %d available",
			dep.Status.UpdatedReplicas, *dep.Spec.Replicas, dep.Status.AvailableReplicas)
	}

	conditions := []string{}
	for _, c := range dep.Status.Conditions {
		conditions = append(conditions, fmt.Sprintf("  %s: %s (%s)", c.Type, c.Status, c.Message))
	}

	return mcputil.ToolText("Deployment %s/%s rollout status:\n%s\n\nConditions:\n%s",
		ns, deployment, status, strings.Join(conditions, "\n"))
}

func (h *handlers) top(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resourceType, err := mcputil.RequireString(req, "resource_type")
	if err != nil {
		return mcputil.ToolError("resource_type is required (pods or nodes)")
	}

	switch resourceType {
	case "nodes":
		nodes, err := h.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcputil.ToolError("failed to list nodes: %v", err)
		}
		var buf bytes.Buffer
		w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
		fmt.Fprintf(w, "NAME\tCPU CAPACITY\tMEMORY CAPACITY\n")
		for _, node := range nodes.Items {
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				node.Name,
				node.Status.Capacity.Cpu().String(),
				node.Status.Capacity.Memory().String())
		}
		w.Flush()
		return mcputil.ToolText("Node resources:\n\n%s\nNote: For real-time usage, metrics-server must be installed.", buf.String())

	case "pods":
		ns := mcputil.OptionalString(req, "namespace", "")
		pods, err := h.client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcputil.ToolError("failed to list pods: %v", err)
		}
		var buf bytes.Buffer
		w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
		fmt.Fprintf(w, "NAMESPACE\tNAME\tCPU REQUEST\tMEMORY REQUEST\tCPU LIMIT\tMEMORY LIMIT\n")
		for _, pod := range pods.Items {
			cpuReq, memReq, cpuLim, memLim := "0", "0", "0", "0"
			for _, c := range pod.Spec.Containers {
				if r := c.Resources.Requests.Cpu(); r != nil {
					cpuReq = r.String()
				}
				if r := c.Resources.Requests.Memory(); r != nil {
					memReq = r.String()
				}
				if r := c.Resources.Limits.Cpu(); r != nil {
					cpuLim = r.String()
				}
				if r := c.Resources.Limits.Memory(); r != nil {
					memLim = r.String()
				}
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				pod.Namespace, pod.Name, cpuReq, memReq, cpuLim, memLim)
		}
		w.Flush()
		return mcputil.ToolText("Pod resources:\n\n%s", buf.String())

	default:
		return mcputil.ToolError("unsupported resource_type: %s (use 'pods' or 'nodes')", resourceType)
	}
}

func formatPodDescription(pod *corev1.Pod) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Pod: %s/%s\n", pod.Namespace, pod.Name)
	fmt.Fprintf(&buf, "Status: %s\n", pod.Status.Phase)
	fmt.Fprintf(&buf, "Node: %s\n", pod.Spec.NodeName)
	fmt.Fprintf(&buf, "IP: %s\n", pod.Status.PodIP)
	fmt.Fprintf(&buf, "Labels: %v\n\n", pod.Labels)

	fmt.Fprintf(&buf, "Containers:\n")
	for _, c := range pod.Spec.Containers {
		fmt.Fprintf(&buf, "  - %s (image: %s)\n", c.Name, c.Image)
		fmt.Fprintf(&buf, "    Resources: requests=%v limits=%v\n", c.Resources.Requests, c.Resources.Limits)
	}

	fmt.Fprintf(&buf, "\nContainer Statuses:\n")
	for _, cs := range pod.Status.ContainerStatuses {
		fmt.Fprintf(&buf, "  - %s: ready=%v, restarts=%d\n", cs.Name, cs.Ready, cs.RestartCount)
		if cs.State.Waiting != nil {
			fmt.Fprintf(&buf, "    Waiting: %s (%s)\n", cs.State.Waiting.Reason, cs.State.Waiting.Message)
		}
		if cs.State.Terminated != nil {
			fmt.Fprintf(&buf, "    Terminated: %s (exit=%d)\n", cs.State.Terminated.Reason, cs.State.Terminated.ExitCode)
		}
	}

	if len(pod.Status.Conditions) > 0 {
		fmt.Fprintf(&buf, "\nConditions:\n")
		for _, c := range pod.Status.Conditions {
			fmt.Fprintf(&buf, "  %s: %s\n", c.Type, c.Status)
		}
	}

	return buf.String()
}
