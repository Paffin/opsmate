package kubernetes

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/paffin/opsmate/internal/config"
	"github.com/paffin/opsmate/pkg/mcputil"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Run starts the Kubernetes MCP server on stdio.
func Run(cfg config.KubernetesConfig, maxLogLines int) error {
	k8sClient, dynClient, err := buildClients(cfg)
	if err != nil {
		return fmt.Errorf("kubernetes client: %w", err)
	}

	h := &handlers{
		client:   k8sClient,
		dynamic:  dynClient,
		readonly: cfg.ReadOnly,
		maxLines: maxLogLines,
	}

	s := mcputil.NewServer("kubernetes", "0.1.0")

	// Read-only tools
	s.AddTool(toolGetPods(), mcputil.SafeHandler(h.getPods))
	s.AddTool(toolGetPodLogs(), mcputil.SafeHandler(h.getPodLogs))
	s.AddTool(toolDescribe(), mcputil.SafeHandler(h.describe))
	s.AddTool(toolGetEvents(), mcputil.SafeHandler(h.getEvents))
	s.AddTool(toolGetNodes(), mcputil.SafeHandler(h.getNodes))
	s.AddTool(toolGetDeployments(), mcputil.SafeHandler(h.getDeployments))
	s.AddTool(toolGetServices(), mcputil.SafeHandler(h.getServices))
	s.AddTool(toolRolloutStatus(), mcputil.SafeHandler(h.rolloutStatus))
	s.AddTool(toolTop(), mcputil.SafeHandler(h.top))

	// Write tools (only if not readonly)
	if !cfg.ReadOnly {
		s.AddTool(toolApply(), mcputil.SafeHandler(h.apply))
		s.AddTool(toolScale(), mcputil.SafeHandler(h.scale))
	}

	return mcputil.Serve(s)
}

func buildClients(cfg config.KubernetesConfig) (kubernetes.Interface, dynamic.Interface, error) {
	var restConfig *rest.Config
	var err error

	// Try in-cluster config first
	restConfig, err = rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig
		kubeconfig := cfg.Kubeconfig
		if kubeconfig == "" {
			home, _ := os.UserHomeDir()
			kubeconfig = filepath.Join(home, ".kube", "config")
		}

		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
		configOverrides := &clientcmd.ConfigOverrides{}
		if cfg.Context != "" {
			configOverrides.CurrentContext = cfg.Context
		}

		restConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules, configOverrides).ClientConfig()
		if err != nil {
			return nil, nil, fmt.Errorf("kubeconfig: %w", err)
		}
	}

	k8sClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("kubernetes client: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("dynamic client: %w", err)
	}

	return k8sClient, dynClient, nil
}
