package context

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/paffin/opsmate/internal/config"
	"github.com/charmbracelet/log"
)

// InfraContext holds pre-collected infrastructure information.
type InfraContext struct {
	KubernetesContext string
	KubernetesCluster string
	KubernetesNodes   int
	DockerVersion     string
	DockerContainers  int
	PrometheusURL     string
	PrometheusUp      bool
}

// Collect gathers infrastructure context before launching Claude.
func Collect(cfg *config.Config) *InfraContext {
	ctx := &InfraContext{}

	if cfg.Servers.Kubernetes.Enabled {
		collectKubernetes(cfg, ctx)
	}

	if cfg.Servers.Docker.Enabled {
		collectDocker(cfg, ctx)
	}

	if cfg.Servers.Prometheus.Enabled {
		ctx.PrometheusURL = cfg.Servers.Prometheus.URL
	}

	return ctx
}

func collectKubernetes(cfg *config.Config, ctx *InfraContext) {
	// Get current context
	out, err := exec.Command("kubectl", "config", "current-context").Output()
	if err != nil {
		log.Debug("Could not get kubectl context", "err", err)
		return
	}
	ctx.KubernetesContext = strings.TrimSpace(string(out))

	// Get cluster info
	out, err = exec.Command("kubectl", "config", "view", "--minify", "-o", "jsonpath={.clusters[0].name}").Output()
	if err == nil {
		ctx.KubernetesCluster = strings.TrimSpace(string(out))
	}

	// Get node count
	out, err = exec.Command("kubectl", "get", "nodes", "--no-headers").Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		ctx.KubernetesNodes = len(lines)
	}
}

func collectDocker(cfg *config.Config, ctx *InfraContext) {
	// Get docker version
	out, err := exec.Command("docker", "version", "--format", "{{.Server.Version}}").Output()
	if err != nil {
		log.Debug("Could not get docker version", "err", err)
		return
	}
	ctx.DockerVersion = strings.TrimSpace(string(out))

	// Get running container count
	out, err = exec.Command("docker", "ps", "-q").Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if lines[0] != "" {
			ctx.DockerContainers = len(lines)
		}
	}
}

// Summary returns a human-readable summary of collected context.
func (c *InfraContext) Summary() string {
	var parts []string

	if c.KubernetesContext != "" {
		parts = append(parts, fmt.Sprintf("K8s: %s (%d nodes)", c.KubernetesContext, c.KubernetesNodes))
	}
	if c.DockerVersion != "" {
		parts = append(parts, fmt.Sprintf("Docker: v%s (%d containers)", c.DockerVersion, c.DockerContainers))
	}
	if c.PrometheusURL != "" {
		parts = append(parts, fmt.Sprintf("Prometheus: %s", c.PrometheusURL))
	}

	if len(parts) == 0 {
		return "No infrastructure detected"
	}

	return strings.Join(parts, " | ")
}
