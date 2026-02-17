package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Servers.Kubernetes.Enabled {
		t.Error("kubernetes should be enabled by default")
	}
	if !cfg.Servers.Docker.Enabled {
		t.Error("docker should be enabled by default")
	}
	if cfg.Servers.Prometheus.Enabled {
		t.Error("prometheus should be disabled by default")
	}
	if !cfg.Servers.Files.Enabled {
		t.Error("files should be enabled by default")
	}
	if !cfg.Safety.ConfirmDestructive {
		t.Error("confirm_destructive should be true by default")
	}
	if cfg.Safety.MaxLogLines != 1000 {
		t.Errorf("max_log_lines should be 1000, got %d", cfg.Safety.MaxLogLines)
	}
	if !cfg.Safety.RedactSecrets {
		t.Error("redact_secrets should be true by default")
	}
	if cfg.Servers.Docker.ReadOnly != true {
		t.Error("docker should be readonly by default")
	}
}

func TestLoadMissingConfig(t *testing.T) {
	// When no config file is specified, should use defaults
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("should not error on missing config: %v", err)
	}
	if cfg == nil {
		t.Fatal("should return default config")
	}
	if !cfg.Servers.Kubernetes.Enabled {
		t.Error("should have defaults when config missing")
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
servers:
  kubernetes:
    enabled: false
  docker:
    enabled: true
    readonly: false
  prometheus:
    enabled: true
    url: http://prom:9090
safety:
  max_log_lines: 500
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Servers.Kubernetes.Enabled {
		t.Error("kubernetes should be disabled")
	}
	if !cfg.Servers.Prometheus.Enabled {
		t.Error("prometheus should be enabled")
	}
	if cfg.Servers.Prometheus.URL != "http://prom:9090" {
		t.Errorf("unexpected prometheus URL: %s", cfg.Servers.Prometheus.URL)
	}
	if cfg.Safety.MaxLogLines != 500 {
		t.Errorf("max_log_lines should be 500, got %d", cfg.Safety.MaxLogLines)
	}
}

func TestConfigDir(t *testing.T) {
	dir := ConfigDir()
	if dir == "" {
		t.Error("config dir should not be empty")
	}
	if !filepath.IsAbs(dir) {
		t.Error("config dir should be absolute")
	}
}
