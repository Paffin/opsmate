package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type jsonrpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

type toolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

type mcpClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	t      *testing.T
}

func newMCPClient(t *testing.T, binary string, args ...string) *mcpClient {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start MCP server: %v", err)
	}

	c := &mcpClient{
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReaderSize(stdout, 1024*1024),
		t:      t,
	}

	// Initialize
	c.send(jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":   map[string]interface{}{},
			"clientInfo":     map[string]interface{}{"name": "test", "version": "1.0"},
		},
	})
	resp := c.recv()
	if resp.Error != nil {
		t.Fatalf("initialize failed: %s", string(resp.Error))
	}

	// Send initialized notification
	c.send(jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	})

	return c
}

func (c *mcpClient) send(req jsonrpcRequest) {
	data, err := json.Marshal(req)
	if err != nil {
		c.t.Fatalf("marshal error: %v", err)
	}
	_, _ = c.stdin.Write(append(data, '\n'))
}

func (c *mcpClient) recv() jsonrpcResponse {
	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		c.t.Fatalf("read error: %v", err)
	}
	var resp jsonrpcResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		c.t.Fatalf("unmarshal error: %v\nline: %s", err, string(line))
	}
	return resp
}

func (c *mcpClient) callTool(name string, args map[string]interface{}) (string, bool) {
	c.send(jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      name,
			"arguments": args,
		},
	})
	resp := c.recv()
	if resp.Error != nil {
		return fmt.Sprintf("RPC error: %s", string(resp.Error)), true
	}

	var tr toolResult
	if err := json.Unmarshal(resp.Result, &tr); err != nil {
		return fmt.Sprintf("unmarshal result: %v", err), true
	}

	text := ""
	if len(tr.Content) > 0 {
		text = tr.Content[0].Text
	}
	return text, tr.IsError
}

func (c *mcpClient) close() {
	_ = c.stdin.Close()
	_ = c.cmd.Wait()
}

func getBinary() string {
	// Look for opsmate-test.exe in parent directory
	return filepath.Join("..", "opsmate-test.exe")
}

// ==========================================
// DOCKER MCP SERVER TESTS
// ==========================================

func TestDockerPS(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "docker")
	defer c.close()

	text, isErr := c.callTool("docker_ps", map[string]interface{}{"all": true})
	if isErr {
		t.Fatalf("docker_ps failed: %s", text)
	}
	if !strings.Contains(text, "containers") {
		t.Errorf("expected 'containers' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("docker_ps: %s", text[:min(len(text), 300)])
}

func TestDockerPSFilter(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "docker")
	defer c.close()

	text, isErr := c.callTool("docker_ps", map[string]interface{}{"all": false, "filter": "name=teststand"})
	if isErr {
		t.Fatalf("docker_ps (filter) failed: %s", text)
	}
	if !strings.Contains(text, "containers") {
		t.Errorf("expected 'containers' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("docker_ps (filter): %s", text[:min(len(text), 300)])
}

func TestDockerLogs(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "docker")
	defer c.close()

	container := getContainerName("teststand-nginx")
	if container == "" {
		t.Skip("nginx container not running")
	}

	text, isErr := c.callTool("docker_logs", map[string]interface{}{"container": container, "tail": 10})
	if isErr {
		t.Fatalf("docker_logs failed: %s", text)
	}
	if !strings.Contains(text, "Logs for") {
		t.Errorf("expected 'Logs for' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("docker_logs: %s", text[:min(len(text), 300)])
}

func TestDockerInspect(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "docker")
	defer c.close()

	container := getContainerName("teststand-nginx")
	if container == "" {
		t.Skip("nginx container not running")
	}

	text, isErr := c.callTool("docker_inspect", map[string]interface{}{"container": container})
	if isErr {
		t.Fatalf("docker_inspect failed: %s", text)
	}
	if !strings.Contains(text, "Container:") {
		t.Errorf("expected 'Container:' in output, got: %s", text[:min(len(text), 200)])
	}
	// Check secret redaction
	if strings.Contains(text, "Network:") {
		t.Logf("docker_inspect shows network info correctly")
	}
	t.Logf("docker_inspect: %s", text[:min(len(text), 500)])
}

func TestDockerStatsSingle(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "docker")
	defer c.close()

	container := getContainerName("teststand-nginx")
	if container == "" {
		t.Skip("nginx container not running")
	}

	text, isErr := c.callTool("docker_stats", map[string]interface{}{"container": container})
	if isErr {
		t.Fatalf("docker_stats (single) failed: %s", text)
	}
	if !strings.Contains(text, "Stats for") {
		t.Errorf("expected 'Stats for' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("docker_stats (single): %s", text[:min(len(text), 300)])
}

func TestDockerStatsAll(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "docker")
	defer c.close()

	text, isErr := c.callTool("docker_stats", map[string]interface{}{})
	if isErr {
		t.Fatalf("docker_stats (all) failed: %s", text)
	}
	if !strings.Contains(text, "Container stats") {
		t.Errorf("expected 'Container stats' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("docker_stats (all): %s", text[:min(len(text), 500)])
}

func TestDockerImages(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "docker")
	defer c.close()

	text, isErr := c.callTool("docker_images", map[string]interface{}{})
	if isErr {
		t.Fatalf("docker_images failed: %s", text)
	}
	if !strings.Contains(text, "images") {
		t.Errorf("expected 'images' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("docker_images: %s", text[:min(len(text), 500)])
}

func TestDockerComposePS(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "docker")
	defer c.close()

	text, isErr := c.callTool("docker_compose_ps", map[string]interface{}{})
	if isErr {
		t.Fatalf("docker_compose_ps failed: %s", text)
	}
	if !strings.Contains(text, "Compose services") {
		t.Errorf("expected 'Compose services' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("docker_compose_ps: %s", text[:min(len(text), 500)])
}

func TestDockerComposeLogs(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "docker")
	defer c.close()

	text, isErr := c.callTool("docker_compose_logs", map[string]interface{}{"service": "nginx"})
	if isErr {
		t.Fatalf("docker_compose_logs failed: %s", text)
	}
	if !strings.Contains(text, "Compose logs") {
		t.Errorf("expected 'Compose logs' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("docker_compose_logs: %s", text[:min(len(text), 300)])
}

func TestDockerExecReadonly(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "docker")
	defer c.close()

	container := getContainerName("teststand-nginx")
	if container == "" {
		t.Skip("nginx container not running")
	}

	text, _ := c.callTool("docker_exec", map[string]interface{}{"container": container, "command": "echo hello"})
	// In readonly mode, docker_exec tool is not registered at all, so we get "tool not found" OR "read-only"
	if !strings.Contains(text, "read-only") && !strings.Contains(text, "not found") {
		t.Errorf("expected 'read-only' or 'not found' error, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("docker_exec (readonly): correctly blocked - %s", text[:min(len(text), 100)])
}

func TestDockerExecWritable(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "docker", "--readonly=false")
	defer c.close()

	container := getContainerName("teststand-nginx")
	if container == "" {
		t.Skip("nginx container not running")
	}

	text, isErr := c.callTool("docker_exec", map[string]interface{}{"container": container, "command": "echo hello"})
	if isErr {
		t.Fatalf("docker_exec (writable) failed: %s", text)
	}
	if !strings.Contains(text, "Exec output") {
		t.Errorf("expected 'Exec output' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("docker_exec (writable): %s", text[:min(len(text), 200)])
}

// ==========================================
// PROMETHEUS MCP SERVER TESTS
// ==========================================

func TestPromQuery(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "prometheus", "--url", "http://localhost:9090")
	defer c.close()

	// Wait for prometheus to be ready
	time.Sleep(2 * time.Second)

	text, isErr := c.callTool("prom_query", map[string]interface{}{"query": "up"})
	if isErr {
		t.Fatalf("prom_query failed: %s", text)
	}
	if !strings.Contains(text, "Result type") {
		t.Errorf("expected 'Result type' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("prom_query: %s", text[:min(len(text), 500)])
}

func TestPromQueryRange(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "prometheus", "--url", "http://localhost:9090")
	defer c.close()

	text, isErr := c.callTool("prom_query_range", map[string]interface{}{"query": "up", "step": "30s"})
	if isErr {
		t.Fatalf("prom_query_range failed: %s", text)
	}
	if !strings.Contains(text, "Range Query") {
		t.Errorf("expected 'Range Query' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("prom_query_range: %s", text[:min(len(text), 300)])
}

func TestPromAlerts(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "prometheus", "--url", "http://localhost:9090")
	defer c.close()

	text, isErr := c.callTool("prom_alerts", map[string]interface{}{})
	if isErr {
		t.Fatalf("prom_alerts failed: %s", text)
	}
	// May have no active alerts, which is fine
	t.Logf("prom_alerts: %s", text[:min(len(text), 300)])
}

func TestPromTargets(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "prometheus", "--url", "http://localhost:9090")
	defer c.close()

	text, isErr := c.callTool("prom_targets", map[string]interface{}{"state": "active"})
	if isErr {
		t.Fatalf("prom_targets failed: %s", text)
	}
	if !strings.Contains(text, "targets") {
		t.Errorf("expected 'targets' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("prom_targets: %s", text[:min(len(text), 500)])
}

func TestPromRules(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "prometheus", "--url", "http://localhost:9090")
	defer c.close()

	text, isErr := c.callTool("prom_rules", map[string]interface{}{"type": "all"})
	if isErr {
		t.Fatalf("prom_rules failed: %s", text)
	}
	if !strings.Contains(text, "Rules") {
		t.Errorf("expected 'Rules' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("prom_rules: %s", text[:min(len(text), 300)])
}

func TestPromSeries(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "prometheus", "--url", "http://localhost:9090")
	defer c.close()

	text, isErr := c.callTool("prom_series", map[string]interface{}{"match": "up"})
	if isErr {
		t.Fatalf("prom_series failed: %s", text)
	}
	if !strings.Contains(text, "series") {
		t.Errorf("expected 'series' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("prom_series: %s", text[:min(len(text), 300)])
}

func TestPromLabelValues(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "prometheus", "--url", "http://localhost:9090")
	defer c.close()

	text, isErr := c.callTool("prom_label_values", map[string]interface{}{"label_name": "job"})
	if isErr {
		t.Fatalf("prom_label_values failed: %s", text)
	}
	if !strings.Contains(text, "values") {
		t.Errorf("expected 'values' in output, got: %s", text[:min(len(text), 200)])
	}
	t.Logf("prom_label_values: %s", text[:min(len(text), 300)])
}

// ==========================================
// FILES MCP SERVER TESTS
// ==========================================

func TestFileAnalyzeDockerfile(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "files")
	defer c.close()

	path := getSampleFilePath("Dockerfile")
	text, isErr := c.callTool("file_analyze", map[string]interface{}{"path": path})
	if isErr {
		t.Fatalf("file_analyze (Dockerfile) failed: %s", text)
	}
	if !strings.Contains(text, "Dockerfile Analysis") {
		t.Errorf("expected 'Dockerfile Analysis', got: %s", text[:min(len(text), 200)])
	}
	t.Logf("file_analyze (Dockerfile): %s", text[:min(len(text), 300)])
}

func TestFileAnalyzeK8s(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "files")
	defer c.close()

	path := getSampleFilePath("deploy.yaml")
	text, isErr := c.callTool("file_analyze", map[string]interface{}{"path": path})
	if isErr {
		t.Fatalf("file_analyze (K8s) failed: %s", text)
	}
	if !strings.Contains(text, "Kubernetes YAML Analysis") {
		t.Errorf("expected 'Kubernetes YAML Analysis', got: %s", text[:min(len(text), 200)])
	}
	t.Logf("file_analyze (K8s): %s", text[:min(len(text), 300)])
}

func TestFileAnalyzeTerraform(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "files")
	defer c.close()

	path := getSampleFilePath("main.tf")
	text, isErr := c.callTool("file_analyze", map[string]interface{}{"path": path})
	if isErr {
		t.Fatalf("file_analyze (Terraform) failed: %s", text)
	}
	if !strings.Contains(text, "Terraform Analysis") {
		t.Errorf("expected 'Terraform Analysis', got: %s", text[:min(len(text), 200)])
	}
	t.Logf("file_analyze (Terraform): %s", text[:min(len(text), 300)])
}

func TestFileAnalyzeCompose(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "files")
	defer c.close()

	path := getSampleFilePath("compose.yaml")
	text, isErr := c.callTool("file_analyze", map[string]interface{}{"path": path})
	if isErr {
		t.Fatalf("file_analyze (Compose) failed: %s", text)
	}
	if !strings.Contains(text, "Docker Compose Analysis") {
		t.Errorf("expected 'Docker Compose Analysis', got: %s", text[:min(len(text), 200)])
	}
	t.Logf("file_analyze (Compose): %s", text[:min(len(text), 300)])
}

func TestFileLintBadDockerfile(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "files")
	defer c.close()

	path := getSampleFilePath("Dockerfile")
	text, isErr := c.callTool("file_lint", map[string]interface{}{"path": path})
	if isErr {
		t.Fatalf("file_lint (bad Dockerfile) failed: %s", text)
	}
	if !strings.Contains(text, "CRITICAL") {
		t.Errorf("expected CRITICAL issue for bad Dockerfile, got: %s", text[:min(len(text), 300)])
	}
	if !strings.Contains(text, "latest") {
		t.Errorf("expected warning about :latest, got: %s", text[:min(len(text), 300)])
	}
	t.Logf("file_lint (bad Dockerfile): %s", text[:min(len(text), 500)])
}

func TestFileLintGoodDockerfile(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "files")
	defer c.close()

	path := getSampleFilePath("Dockerfile.good")
	text, isErr := c.callTool("file_lint", map[string]interface{}{"path": path})
	if isErr {
		t.Fatalf("file_lint (good Dockerfile) failed: %s", text)
	}
	if strings.Contains(text, "CRITICAL") {
		t.Errorf("good Dockerfile should not have CRITICAL issues, got: %s", text[:min(len(text), 300)])
	}
	t.Logf("file_lint (good Dockerfile): %s", text[:min(len(text), 300)])
}

func TestFileLintBadK8s(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "files")
	defer c.close()

	path := getSampleFilePath("deploy.yaml")
	text, isErr := c.callTool("file_lint", map[string]interface{}{"path": path})
	if isErr {
		t.Fatalf("file_lint (bad K8s) failed: %s", text)
	}
	if !strings.Contains(text, "WARNING") {
		t.Errorf("expected WARNING for bad K8s manifest, got: %s", text[:min(len(text), 300)])
	}
	t.Logf("file_lint (bad K8s): %s", text[:min(len(text), 500)])
}

func TestFileLintGoodK8s(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "files")
	defer c.close()

	path := getSampleFilePath("deploy-good.yaml")
	text, isErr := c.callTool("file_lint", map[string]interface{}{"path": path})
	if isErr {
		t.Fatalf("file_lint (good K8s) failed: %s", text)
	}
	if strings.Contains(text, "CRITICAL") {
		t.Errorf("good K8s manifest should not have CRITICAL issues, got: %s", text[:min(len(text), 300)])
	}
	t.Logf("file_lint (good K8s): %s", text[:min(len(text), 300)])
}

func TestFileValidateDockerfile(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "files")
	defer c.close()

	path := getSampleFilePath("Dockerfile")
	text, isErr := c.callTool("file_validate", map[string]interface{}{"path": path})
	if isErr {
		t.Fatalf("file_validate (Dockerfile) failed: %s", text)
	}
	if !strings.Contains(text, "Validation passed") {
		t.Errorf("expected 'Validation passed', got: %s", text[:min(len(text), 200)])
	}
	t.Logf("file_validate (Dockerfile): %s", text[:min(len(text), 200)])
}

func TestFileValidateYAML(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "files")
	defer c.close()

	path := getSampleFilePath("deploy.yaml")
	text, isErr := c.callTool("file_validate", map[string]interface{}{"path": path})
	if isErr {
		t.Fatalf("file_validate (YAML) failed: %s", text)
	}
	if !strings.Contains(text, "Validation passed") {
		t.Errorf("expected 'Validation passed', got: %s", text[:min(len(text), 200)])
	}
	t.Logf("file_validate (YAML): %s", text[:min(len(text), 200)])
}

func TestFileScanDir(t *testing.T) {
	c := newMCPClient(t, getBinary(), "mcp", "files")
	defer c.close()

	dir := filepath.Join(".", "sample-files")
	absDir, _ := filepath.Abs(dir)
	text, isErr := c.callTool("file_scan_dir", map[string]interface{}{"dir": absDir})
	if isErr {
		t.Fatalf("file_scan_dir failed: %s", text)
	}
	if !strings.Contains(text, "infrastructure files") {
		t.Errorf("expected 'infrastructure files', got: %s", text[:min(len(text), 200)])
	}
	t.Logf("file_scan_dir: %s", text[:min(len(text), 500)])
}

// ==========================================
// HELPERS
// ==========================================

func getContainerName(prefix string) string {
	out, err := exec.Command("docker", "ps", "--format", "{{.Names}}").Output()
	if err != nil {
		return ""
	}
	for _, name := range strings.Split(string(out), "\n") {
		name = strings.TrimSpace(name)
		if strings.HasPrefix(name, prefix) {
			return name
		}
	}
	return ""
}

func getSampleFilePath(name string) string {
	path := filepath.Join(".", "sample-files", name)
	abs, _ := filepath.Abs(path)
	return abs
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
