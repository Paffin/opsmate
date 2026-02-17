package mcphost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/paffin/opsmate/internal/config"
	"github.com/charmbracelet/log"
)

// MCPServerConfig represents a single MCP server entry for .mcp.json.
type MCPServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// MCPConfig is the top-level .mcp.json structure.
type MCPConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// Manager manages MCP server configurations for Claude Code.
type Manager struct {
	cfg    *config.Config
	binary string // path to opsmate binary
	mu     sync.Mutex
}

// New creates a new MCP server manager.
func New(cfg *config.Config) *Manager {
	binary, _ := os.Executable()
	return &Manager{
		cfg:    cfg,
		binary: binary,
	}
}

// GenerateMCPConfig creates the .mcp.json file for Claude Code.
func (m *Manager) GenerateMCPConfig(dir string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	mcpCfg := MCPConfig{
		MCPServers: make(map[string]MCPServerConfig),
	}

	if m.cfg.Servers.Kubernetes.Enabled {
		args := []string{"mcp", "kubernetes"}
		if m.cfg.Servers.Kubernetes.Kubeconfig != "" {
			args = append(args, "--kubeconfig", m.cfg.Servers.Kubernetes.Kubeconfig)
		}
		if m.cfg.Servers.Kubernetes.Context != "" {
			args = append(args, "--context", m.cfg.Servers.Kubernetes.Context)
		}
		if m.cfg.Servers.Kubernetes.ReadOnly {
			args = append(args, "--readonly")
		}
		mcpCfg.MCPServers["kubernetes"] = MCPServerConfig{
			Command: m.binary,
			Args:    args,
		}
	}

	if m.cfg.Servers.Docker.Enabled {
		args := []string{"mcp", "docker"}
		if m.cfg.Servers.Docker.Host != "" {
			args = append(args, "--host", m.cfg.Servers.Docker.Host)
		}
		if m.cfg.Servers.Docker.ReadOnly {
			args = append(args, "--readonly")
		}
		mcpCfg.MCPServers["docker"] = MCPServerConfig{
			Command: m.binary,
			Args:    args,
		}
	}

	if m.cfg.Servers.Prometheus.Enabled {
		args := []string{"mcp", "prometheus", "--url", m.cfg.Servers.Prometheus.URL}
		if m.cfg.Servers.Prometheus.BasicAuth != nil {
			args = append(args, "--username", m.cfg.Servers.Prometheus.BasicAuth.Username)
			args = append(args, "--password", m.cfg.Servers.Prometheus.BasicAuth.Password)
		}
		mcpCfg.MCPServers["prometheus"] = MCPServerConfig{
			Command: m.binary,
			Args:    args,
		}
	}

	if m.cfg.Servers.Files.Enabled {
		args := []string{"mcp", "files"}
		mcpCfg.MCPServers["files"] = MCPServerConfig{
			Command: m.binary,
			Args:    args,
		}
	}

	// Write .mcp.json
	mcpPath := filepath.Join(dir, ".mcp.json")
	data, err := json.MarshalIndent(mcpCfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal mcp config: %w", err)
	}

	if err := os.WriteFile(mcpPath, data, 0644); err != nil {
		return "", fmt.Errorf("write %s: %w", mcpPath, err)
	}

	log.Info("Generated MCP config", "path", mcpPath, "servers", len(mcpCfg.MCPServers))
	return mcpPath, nil
}

// EnabledServers returns a list of enabled server names.
func (m *Manager) EnabledServers() []string {
	var servers []string
	if m.cfg.Servers.Kubernetes.Enabled {
		servers = append(servers, "kubernetes")
	}
	if m.cfg.Servers.Docker.Enabled {
		servers = append(servers, "docker")
	}
	if m.cfg.Servers.Prometheus.Enabled {
		servers = append(servers, "prometheus")
	}
	if m.cfg.Servers.Files.Enabled {
		servers = append(servers, "files")
	}
	return servers
}

// Cleanup removes generated config files.
func (m *Manager) Cleanup(dir string) {
	mcpPath := filepath.Join(dir, ".mcp.json")
	_ = os.Remove(mcpPath)
	log.Debug("Cleaned up MCP config", "path", mcpPath)
}
