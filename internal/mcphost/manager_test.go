package mcphost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/paffin/opsmate/internal/config"
)

func TestEnabledServers(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)

	servers := m.EnabledServers()

	// Default: kubernetes, docker, files enabled; prometheus disabled
	if len(servers) != 3 {
		t.Errorf("expected 3 enabled servers, got %d: %v", len(servers), servers)
	}

	hasK8s := false
	hasDocker := false
	hasFiles := false
	for _, s := range servers {
		switch s {
		case "kubernetes":
			hasK8s = true
		case "docker":
			hasDocker = true
		case "files":
			hasFiles = true
		}
	}
	if !hasK8s {
		t.Error("should have kubernetes")
	}
	if !hasDocker {
		t.Error("should have docker")
	}
	if !hasFiles {
		t.Error("should have files")
	}
}

func TestGenerateMCPConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Servers.Prometheus.Enabled = true
	cfg.Servers.Prometheus.URL = "http://test:9090"

	m := New(cfg)
	dir := t.TempDir()

	mcpPath, err := m.GenerateMCPConfig(dir)
	if err != nil {
		t.Fatalf("failed to generate mcp config: %v", err)
	}

	if mcpPath != filepath.Join(dir, ".mcp.json") {
		t.Errorf("unexpected path: %s", mcpPath)
	}

	// Read and parse the file
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("failed to read mcp config: %v", err)
	}

	var mcpCfg MCPConfig
	if err := json.Unmarshal(data, &mcpCfg); err != nil {
		t.Fatalf("failed to parse mcp config: %v", err)
	}

	if len(mcpCfg.MCPServers) != 4 {
		t.Errorf("expected 4 servers, got %d", len(mcpCfg.MCPServers))
	}

	if _, ok := mcpCfg.MCPServers["kubernetes"]; !ok {
		t.Error("should have kubernetes server")
	}
	if _, ok := mcpCfg.MCPServers["prometheus"]; !ok {
		t.Error("should have prometheus server")
	}
}

func TestCleanup(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)
	dir := t.TempDir()

	// Generate and then cleanup
	_, err := m.GenerateMCPConfig(dir)
	if err != nil {
		t.Fatal(err)
	}

	mcpPath := filepath.Join(dir, ".mcp.json")
	if _, err := os.Stat(mcpPath); os.IsNotExist(err) {
		t.Fatal("mcp config should exist before cleanup")
	}

	m.Cleanup(dir)

	if _, err := os.Stat(mcpPath); !os.IsNotExist(err) {
		t.Error("mcp config should be removed after cleanup")
	}
}
