package kubernetes

import "github.com/mark3labs/mcp-go/mcp"

func toolGetPods() mcp.Tool {
	return mcp.NewTool("k8s_get_pods",
		mcp.WithDescription("List pods with their status in a namespace"),
		mcp.WithString("namespace",
			mcp.Description("Kubernetes namespace (default: all)"),
		),
		mcp.WithString("label_selector",
			mcp.Description("Label selector to filter pods (e.g. app=nginx)"),
		),
	)
}

func toolGetPodLogs() mcp.Tool {
	return mcp.NewTool("k8s_get_pod_logs",
		mcp.WithDescription("Get logs from a pod"),
		mcp.WithString("pod",
			mcp.Required(),
			mcp.Description("Pod name"),
		),
		mcp.WithString("namespace",
			mcp.Description("Kubernetes namespace (default: default)"),
		),
		mcp.WithString("container",
			mcp.Description("Container name (for multi-container pods)"),
		),
		mcp.WithNumber("tail_lines",
			mcp.Description("Number of lines from the end (default: 100)"),
		),
	)
}

func toolDescribe() mcp.Tool {
	return mcp.NewTool("k8s_describe",
		mcp.WithDescription("Describe a Kubernetes resource with detailed info"),
		mcp.WithString("kind",
			mcp.Required(),
			mcp.Description("Resource kind (pod, deployment, service, node, etc.)"),
		),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Resource name"),
		),
		mcp.WithString("namespace",
			mcp.Description("Kubernetes namespace (default: default)"),
		),
	)
}

func toolGetEvents() mcp.Tool {
	return mcp.NewTool("k8s_get_events",
		mcp.WithDescription("Get cluster events, useful for debugging"),
		mcp.WithString("namespace",
			mcp.Description("Kubernetes namespace (default: all)"),
		),
		mcp.WithString("field_selector",
			mcp.Description("Field selector (e.g. involvedObject.name=nginx)"),
		),
	)
}

func toolGetNodes() mcp.Tool {
	return mcp.NewTool("k8s_get_nodes",
		mcp.WithDescription("Get cluster nodes with status and resource capacity"),
	)
}

func toolGetDeployments() mcp.Tool {
	return mcp.NewTool("k8s_get_deployments",
		mcp.WithDescription("List deployments with their status"),
		mcp.WithString("namespace",
			mcp.Description("Kubernetes namespace (default: all)"),
		),
	)
}

func toolGetServices() mcp.Tool {
	return mcp.NewTool("k8s_get_services",
		mcp.WithDescription("List services with endpoints"),
		mcp.WithString("namespace",
			mcp.Description("Kubernetes namespace (default: all)"),
		),
	)
}

func toolApply() mcp.Tool {
	return mcp.NewTool("k8s_apply",
		mcp.WithDescription("Apply a YAML manifest to the cluster (DESTRUCTIVE - requires confirmation)"),
		mcp.WithString("yaml_content",
			mcp.Required(),
			mcp.Description("YAML manifest content to apply"),
		),
	)
}

func toolScale() mcp.Tool {
	return mcp.NewTool("k8s_scale",
		mcp.WithDescription("Scale a deployment (DESTRUCTIVE - requires confirmation)"),
		mcp.WithString("deployment",
			mcp.Required(),
			mcp.Description("Deployment name"),
		),
		mcp.WithNumber("replicas",
			mcp.Required(),
			mcp.Description("Desired number of replicas"),
		),
		mcp.WithString("namespace",
			mcp.Description("Kubernetes namespace (default: default)"),
		),
	)
}

func toolRolloutStatus() mcp.Tool {
	return mcp.NewTool("k8s_rollout_status",
		mcp.WithDescription("Check rollout status of a deployment"),
		mcp.WithString("deployment",
			mcp.Required(),
			mcp.Description("Deployment name"),
		),
		mcp.WithString("namespace",
			mcp.Description("Kubernetes namespace (default: default)"),
		),
	)
}

func toolTop() mcp.Tool {
	return mcp.NewTool("k8s_top",
		mcp.WithDescription("Show resource usage (CPU/memory) for pods or nodes"),
		mcp.WithString("resource_type",
			mcp.Required(),
			mcp.Description("Resource type: pods or nodes"),
			mcp.Enum("pods", "nodes"),
		),
		mcp.WithString("namespace",
			mcp.Description("Kubernetes namespace for pods (default: all)"),
		),
	)
}
