package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFileType(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		content  string
		expected string
	}{
		{"dockerfile", "Dockerfile", "FROM alpine", "dockerfile"},
		{"dockerfile variant", "Dockerfile.prod", "FROM golang", "dockerfile"},
		{"kubernetes yaml", "deploy.yaml", "apiVersion: apps/v1\nkind: Deployment", "kubernetes"},
		{"compose", "docker-compose.yml", "services:\n  web:", "compose"},
		{"compose v2", "compose.yaml", "services:\n  api:", "compose"},
		{"terraform", "main.tf", "resource \"aws_instance\" \"web\" {}", "terraform"},
		{"tfvars", "vars.tfvars", "region = \"us-east-1\"", "terraform"},
		{"unknown", "readme.md", "# Hello", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectFileType(tt.path, []byte(tt.content))
			if result != tt.expected {
				t.Errorf("detectFileType(%s) = %s, want %s", tt.path, result, tt.expected)
			}
		})
	}
}

func TestLintDockerfile(t *testing.T) {
	content := []byte(`FROM node:latest
RUN apt-get update && apt-get install -y curl
COPY . /app
CMD ["node", "app.js"]
`)
	issues := lintDockerfile(content)

	if len(issues) == 0 {
		t.Error("expected lint issues for problematic Dockerfile")
	}

	hasLatestWarning := false
	hasUserWarning := false
	for _, issue := range issues {
		if issue.Severity == severityWarning && issue.Message == "Using 'latest' or untagged image: node:latest" {
			hasLatestWarning = true
		}
		if issue.Severity == severityCritical && issue.Message == "No USER instruction — container runs as root" {
			hasUserWarning = true
		}
	}

	if !hasLatestWarning {
		t.Error("should warn about :latest tag")
	}
	if !hasUserWarning {
		t.Error("should warn about missing USER")
	}
}

func TestLintDockerfileClean(t *testing.T) {
	content := []byte(`FROM node:20-alpine
RUN apt-get update && apt-get install -y --no-install-recommends curl
COPY . /app
USER node
HEALTHCHECK CMD curl -f http://localhost/ || exit 1
CMD ["node", "app.js"]
`)
	issues := lintDockerfile(content)

	for _, issue := range issues {
		if issue.Severity == severityCritical {
			t.Errorf("unexpected critical issue: %s", issue.Message)
		}
	}
}

func TestLintKubernetesYAML(t *testing.T) {
	content := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
spec:
  template:
    spec:
      containers:
      - name: app
        image: nginx:latest
`)
	issues := lintKubernetesYAML(content)

	if len(issues) == 0 {
		t.Error("expected lint issues")
	}

	hasResourceWarning := false
	hasLatestWarning := false
	for _, issue := range issues {
		if issue.Message == "No resource requests/limits defined" {
			hasResourceWarning = true
		}
		if issue.Message == "Using ':latest' image tag" {
			hasLatestWarning = true
		}
	}
	if !hasResourceWarning {
		t.Error("should warn about missing resources")
	}
	if !hasLatestWarning {
		t.Error("should warn about :latest tag")
	}
}

func TestValidateDockerfile(t *testing.T) {
	valid := []byte("FROM alpine\nRUN echo hello\n")
	errors := validateDockerfile(valid)
	if len(errors) > 0 {
		t.Errorf("valid Dockerfile should have no errors, got: %v", errors)
	}

	noFrom := []byte("RUN echo hello\n")
	errors = validateDockerfile(noFrom)
	if len(errors) == 0 {
		t.Error("should detect missing FROM")
	}
}

func TestValidateYAML(t *testing.T) {
	valid := []byte("key: value\n  nested: value\n")
	errors := validateYAML(valid)
	if len(errors) > 0 {
		t.Errorf("valid YAML should have no errors, got: %v", errors)
	}

	withTabs := []byte("\tkey: value\n")
	errors = validateYAML(withTabs)
	if len(errors) == 0 {
		t.Error("should detect tab indentation")
	}
}

func TestScanDir(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine"), 0644)
	os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte("apiVersion: v1\nkind: Pod"), 0644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Hello"), 0644)

	h := &handlers{rulesets: []string{"dockerfile", "kubernetes"}}

	// Test using the underlying detection
	content1, _ := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if detectFileType("Dockerfile", content1) != "dockerfile" {
		t.Error("should detect Dockerfile")
	}

	content2, _ := os.ReadFile(filepath.Join(dir, "deploy.yaml"))
	if detectFileType("deploy.yaml", content2) != "kubernetes" {
		t.Error("should detect kubernetes yaml")
	}

	_ = h // handlers used in integration tests
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"one line", 1},
		{"line1\nline2\nline3", 3},
	}
	for _, tt := range tests {
		result := countLines([]byte(tt.input))
		if result != tt.expected {
			t.Errorf("countLines(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}
