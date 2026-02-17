package context

import (
	"fmt"
	"strings"

	"github.com/paffin/opsmate/internal/config"
)

// GenerateSystemPrompt creates the CLAUDE.md content for DevOps context.
func GenerateSystemPrompt(cfg *config.Config, infraCtx *InfraContext) string {
	var sb strings.Builder

	sb.WriteString("# opsmate — DevOps AI Assistant\n\n")
	sb.WriteString("You are a DevOps expert assistant powered by opsmate. ")
	sb.WriteString("You have access to live infrastructure through MCP tools.\n\n")

	sb.WriteString("## Available Infrastructure\n\n")

	if cfg.Servers.Kubernetes.Enabled {
		sb.WriteString("### Kubernetes\n")
		if infraCtx.KubernetesContext != "" {
			fmt.Fprintf(&sb, "- Context: `%s`\n", infraCtx.KubernetesContext)
		}
		if infraCtx.KubernetesCluster != "" {
			fmt.Fprintf(&sb, "- Cluster: `%s`\n", infraCtx.KubernetesCluster)
		}
		if infraCtx.KubernetesNodes > 0 {
			fmt.Fprintf(&sb, "- Nodes: %d\n", infraCtx.KubernetesNodes)
		}
		sb.WriteString("- Tools: k8s_get_pods, k8s_get_pod_logs, k8s_describe, k8s_get_events, k8s_get_nodes, k8s_get_deployments, k8s_get_services, k8s_rollout_status, k8s_top")
		if !cfg.Servers.Kubernetes.ReadOnly {
			sb.WriteString(", k8s_apply, k8s_scale")
		}
		sb.WriteString("\n\n")
	}

	if cfg.Servers.Docker.Enabled {
		sb.WriteString("### Docker\n")
		if infraCtx.DockerVersion != "" {
			fmt.Fprintf(&sb, "- Version: %s\n", infraCtx.DockerVersion)
		}
		if infraCtx.DockerContainers > 0 {
			fmt.Fprintf(&sb, "- Running containers: %d\n", infraCtx.DockerContainers)
		}
		sb.WriteString("- Tools: docker_ps, docker_logs, docker_inspect, docker_stats, docker_images, docker_compose_ps, docker_compose_logs")
		if !cfg.Servers.Docker.ReadOnly {
			sb.WriteString(", docker_exec")
		}
		sb.WriteString("\n\n")
	}

	if cfg.Servers.Prometheus.Enabled {
		sb.WriteString("### Prometheus\n")
		fmt.Fprintf(&sb, "- URL: %s\n", cfg.Servers.Prometheus.URL)
		sb.WriteString("- Tools: prom_query, prom_query_range, prom_alerts, prom_targets, prom_rules, prom_series, prom_label_values\n\n")
	}

	if cfg.Servers.Files.Enabled {
		sb.WriteString("### File Analysis\n")
		sb.WriteString("- Tools: file_analyze, file_lint, file_validate, file_scan_dir\n")
		fmt.Fprintf(&sb, "- Rulesets: %s\n\n", strings.Join(cfg.Servers.Files.Rulesets, ", "))
	}

	sb.WriteString("## Guidelines\n\n")
	sb.WriteString("1. **Use the tools** — always query live infrastructure before making recommendations\n")
	sb.WriteString("2. **Be specific** — reference actual pod names, container IDs, metric values\n")
	sb.WriteString("3. **Explain reasoning** — show the data that led to your diagnosis\n")
	sb.WriteString("4. **Safety first** — warn before destructive operations (apply, scale, delete)\n")
	sb.WriteString("5. **Check metrics** — use Prometheus to correlate symptoms with resource usage\n")

	if cfg.Safety.ConfirmDestructive {
		sb.WriteString("\n## Safety Mode\n\n")
		sb.WriteString("Destructive operations require user confirmation. Always explain what will change before applying.\n")
	}

	if cfg.Claude.CustomPrompt != "" {
		sb.WriteString("\n## Custom Instructions\n\n")
		sb.WriteString(cfg.Claude.CustomPrompt)
		sb.WriteString("\n")
	}

	return sb.String()
}
