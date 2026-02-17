package files

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paffin/opsmate/pkg/mcputil"
	"github.com/mark3labs/mcp-go/mcp"
)

type handlers struct {
	rulesets []string
}

func (h *handlers) analyze(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := mcputil.RequireString(req, "path")
	if err != nil {
		return mcputil.ToolError("path is required")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return mcputil.ToolError("failed to read file: %v", err)
	}

	fileType := detectFileType(path, content)
	info, _ := os.Stat(path)

	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "File: %s\n", path)
	_, _ = fmt.Fprintf(&sb, "Type: %s\n", fileType)
	_, _ = fmt.Fprintf(&sb, "Size: %d bytes\n", info.Size())
	_, _ = fmt.Fprintf(&sb, "Lines: %d\n\n", countLines(content))

	switch fileType {
	case "dockerfile":
		sb.WriteString(analyzeDockerfile(content))
	case "kubernetes":
		sb.WriteString(analyzeKubernetesYAML(content))
	case "compose":
		sb.WriteString(analyzeComposeFile(content))
	case "terraform":
		sb.WriteString(analyzeTerraform(content))
	default:
		sb.WriteString("File type not recognized as a known infrastructure file.\n")
		sb.WriteString("Supported types: Dockerfile, Kubernetes YAML, Docker Compose, Terraform\n")
	}

	return mcputil.ToolText("%s", sb.String())
}

func (h *handlers) lint(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := mcputil.RequireString(req, "path")
	if err != nil {
		return mcputil.ToolError("path is required")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return mcputil.ToolError("failed to read file: %v", err)
	}

	ruleset := mcputil.OptionalString(req, "ruleset", "")
	if ruleset == "" {
		ruleset = detectFileType(path, content)
	}

	var issues []lintIssue
	switch ruleset {
	case "dockerfile":
		issues = lintDockerfile(content)
	case "kubernetes":
		issues = lintKubernetesYAML(content)
	case "compose":
		issues = lintComposeFile(content)
	case "terraform":
		issues = lintTerraform(content)
	default:
		return mcputil.ToolText("No lint rules available for file type: %s", ruleset)
	}

	if len(issues) == 0 {
		return mcputil.ToolText("No issues found in %s", path)
	}

	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "Lint results for %s (%d issues):\n\n", path, len(issues))
	for _, issue := range issues {
		_, _ = fmt.Fprintf(&sb, "%s %s: %s\n", issue.Severity.Icon(), issue.Severity, issue.Message)
		if issue.Line > 0 {
			_, _ = fmt.Fprintf(&sb, "  Line %d\n", issue.Line)
		}
		if issue.Fix != "" {
			_, _ = fmt.Fprintf(&sb, "  Fix: %s\n", issue.Fix)
		}
		sb.WriteString("\n")
	}

	return mcputil.ToolText("%s", sb.String())
}

func (h *handlers) validate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path, err := mcputil.RequireString(req, "path")
	if err != nil {
		return mcputil.ToolError("path is required")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return mcputil.ToolError("failed to read file: %v", err)
	}

	fileType := detectFileType(path, content)

	var validationErrors []string
	switch fileType {
	case "dockerfile":
		validationErrors = validateDockerfile(content)
	case "kubernetes", "compose":
		validationErrors = validateYAML(content)
	case "terraform":
		validationErrors = validateTerraform(content)
	default:
		return mcputil.ToolText("Cannot validate unknown file type. Supported: Dockerfile, Kubernetes YAML, Compose, Terraform")
	}

	if len(validationErrors) == 0 {
		return mcputil.ToolText("Validation passed for %s (type: %s)", path, fileType)
	}

	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "Validation errors for %s:\n\n", path)
	for _, e := range validationErrors {
		_, _ = fmt.Fprintf(&sb, "  - %s\n", e)
	}
	return mcputil.ToolText("%s", sb.String())
}

func (h *handlers) scanDir(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dir := mcputil.OptionalString(req, "dir", ".")
	recursive := mcputil.OptionalBool(req, "recursive", true)

	var files []fileInfo
	walkFn := func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if !recursive && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		ft := detectFileType(path, content)
		if ft != "unknown" {
			finfo, _ := info.Info()
			files = append(files, fileInfo{
				Path: path,
				Type: ft,
				Size: finfo.Size(),
			})
		}
		return nil
	}

	if err := filepath.WalkDir(dir, walkFn); err != nil {
		return mcputil.ToolError("failed to scan directory: %v", err)
	}

	if len(files) == 0 {
		return mcputil.ToolText("No infrastructure files found in %s", dir)
	}

	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "Found %d infrastructure files in %s:\n\n", len(files), dir)
	for _, f := range files {
		_, _ = fmt.Fprintf(&sb, "  [%s] %s (%d bytes)\n", f.Type, f.Path, f.Size)
	}

	return mcputil.ToolText("%s", sb.String())
}

type fileInfo struct {
	Path string
	Type string
	Size int64
}

type severity string

const (
	severityCritical severity = "CRITICAL"
	severityWarning  severity = "WARNING"
	severityInfo     severity = "INFO"
)

func (s severity) Icon() string {
	switch s {
	case severityCritical:
		return "[!]"
	case severityWarning:
		return "[~]"
	case severityInfo:
		return "[i]"
	default:
		return "[?]"
	}
}

type lintIssue struct {
	Severity severity
	Message  string
	Line     int
	Fix      string
}

func detectFileType(path string, content []byte) string {
	base := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))

	if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") {
		return "dockerfile"
	}
	if base == "docker-compose.yml" || base == "docker-compose.yaml" || base == "compose.yml" || base == "compose.yaml" {
		return "compose"
	}
	if ext == ".tf" || ext == ".tfvars" {
		return "terraform"
	}
	if ext == ".yaml" || ext == ".yml" {
		s := string(content)
		if strings.Contains(s, "apiVersion:") && strings.Contains(s, "kind:") {
			return "kubernetes"
		}
		if strings.Contains(s, "services:") {
			return "compose"
		}
	}
	return "unknown"
}

func countLines(content []byte) int {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	count := 0
	for scanner.Scan() {
		count++
	}
	return count
}

func analyzeDockerfile(content []byte) string {
	var sb strings.Builder
	sb.WriteString("Dockerfile Analysis:\n")

	lines := strings.Split(string(content), "\n")
	hasUser := false
	hasHealthcheck := false
	fromCount := 0
	baseImages := []string{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "FROM ") {
			fromCount++
			baseImages = append(baseImages, strings.Fields(trimmed)[1])
		}
		if strings.HasPrefix(trimmed, "USER ") {
			hasUser = true
		}
		if strings.HasPrefix(trimmed, "HEALTHCHECK ") {
			hasHealthcheck = true
		}
	}

	_, _ = fmt.Fprintf(&sb, "  Base images: %s\n", strings.Join(baseImages, " -> "))
	_, _ = fmt.Fprintf(&sb, "  Multi-stage: %v (%d stages)\n", fromCount > 1, fromCount)
	_, _ = fmt.Fprintf(&sb, "  Has USER instruction: %v\n", hasUser)
	_, _ = fmt.Fprintf(&sb, "  Has HEALTHCHECK: %v\n", hasHealthcheck)

	return sb.String()
}

func analyzeKubernetesYAML(content []byte) string {
	var sb strings.Builder
	sb.WriteString("Kubernetes YAML Analysis:\n")

	s := string(content)
	if strings.Contains(s, "kind:") {
		for _, line := range strings.Split(s, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "kind:") {
				_, _ = fmt.Fprintf(&sb, "  Kind: %s\n", strings.TrimPrefix(trimmed, "kind:"))
			}
			if strings.HasPrefix(trimmed, "apiVersion:") {
				_, _ = fmt.Fprintf(&sb, "  API Version: %s\n", strings.TrimPrefix(trimmed, "apiVersion:"))
			}
		}
	}

	return sb.String()
}

func analyzeComposeFile(content []byte) string {
	var sb strings.Builder
	sb.WriteString("Docker Compose Analysis:\n")

	lines := strings.Split(string(content), "\n")
	serviceCount := 0
	inServices := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "services:" {
			inServices = true
			continue
		}
		if inServices && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
			inServices = false
		}
		if inServices && len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			if indent <= 4 && strings.HasSuffix(trimmed, ":") {
				serviceCount++
				_, _ = fmt.Fprintf(&sb, "  Service: %s\n", strings.TrimSuffix(trimmed, ":"))
			}
		}
	}
	_, _ = fmt.Fprintf(&sb, "  Total services: %d\n", serviceCount)

	return sb.String()
}

func analyzeTerraform(content []byte) string {
	var sb strings.Builder
	sb.WriteString("Terraform Analysis:\n")

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "resource ") || strings.HasPrefix(trimmed, "data ") ||
			strings.HasPrefix(trimmed, "module ") || strings.HasPrefix(trimmed, "provider ") {
			_, _ = fmt.Fprintf(&sb, "  %s\n", trimmed)
		}
	}

	return sb.String()
}

// Lint rules

func lintDockerfile(content []byte) []lintIssue {
	var issues []lintIssue
	lines := strings.Split(string(content), "\n")

	hasUser := false
	hasHealthcheck := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		lineNum := i + 1

		if strings.HasPrefix(trimmed, "FROM ") {
			img := strings.Fields(trimmed)[1]
			if strings.HasSuffix(img, ":latest") || !strings.Contains(img, ":") {
				issues = append(issues, lintIssue{
					Severity: severityWarning, Line: lineNum,
					Message: fmt.Sprintf("Using 'latest' or untagged image: %s", img),
					Fix:     "Pin to a specific version tag",
				})
			}
		}
		if strings.HasPrefix(trimmed, "USER ") {
			hasUser = true
		}
		if strings.HasPrefix(trimmed, "HEALTHCHECK ") {
			hasHealthcheck = true
		}
		if strings.HasPrefix(trimmed, "RUN ") && strings.Contains(trimmed, "apt-get install") && !strings.Contains(trimmed, "--no-install-recommends") {
			issues = append(issues, lintIssue{
				Severity: severityInfo, Line: lineNum,
				Message: "apt-get install without --no-install-recommends",
				Fix:     "Add --no-install-recommends to reduce image size",
			})
		}
		if strings.HasPrefix(trimmed, "ADD ") && !strings.Contains(trimmed, ".tar") && !strings.Contains(trimmed, "http") {
			issues = append(issues, lintIssue{
				Severity: severityInfo, Line: lineNum,
				Message: "Use COPY instead of ADD for local files",
				Fix:     "Replace ADD with COPY unless extracting archives or fetching URLs",
			})
		}
	}

	if !hasUser {
		issues = append(issues, lintIssue{
			Severity: severityCritical,
			Message:  "No USER instruction — container runs as root",
			Fix:      "Add USER instruction to run as non-root user",
		})
	}
	if !hasHealthcheck {
		issues = append(issues, lintIssue{
			Severity: severityWarning,
			Message:  "No HEALTHCHECK instruction defined",
			Fix:      "Add HEALTHCHECK to enable container health monitoring",
		})
	}

	return issues
}

func lintKubernetesYAML(content []byte) []lintIssue {
	var issues []lintIssue
	s := string(content)

	if !strings.Contains(s, "resources:") {
		issues = append(issues, lintIssue{
			Severity: severityWarning,
			Message:  "No resource requests/limits defined",
			Fix:      "Add resources.requests and resources.limits for CPU and memory",
		})
	}
	if !strings.Contains(s, "livenessProbe:") {
		issues = append(issues, lintIssue{
			Severity: severityWarning,
			Message:  "No livenessProbe defined",
			Fix:      "Add livenessProbe for automatic container restart on failure",
		})
	}
	if !strings.Contains(s, "readinessProbe:") {
		issues = append(issues, lintIssue{
			Severity: severityWarning,
			Message:  "No readinessProbe defined",
			Fix:      "Add readinessProbe to control traffic routing",
		})
	}
	if strings.Contains(s, ":latest") {
		issues = append(issues, lintIssue{
			Severity: severityWarning,
			Message:  "Using ':latest' image tag",
			Fix:      "Pin to a specific image version for reproducibility",
		})
	}
	if strings.Contains(s, "securityContext:") && strings.Contains(s, "privileged: true") {
		issues = append(issues, lintIssue{
			Severity: severityCritical,
			Message:  "Container running in privileged mode",
			Fix:      "Remove privileged: true unless absolutely necessary",
		})
	}

	return issues
}

func lintComposeFile(content []byte) []lintIssue {
	var issues []lintIssue
	s := string(content)

	if !strings.Contains(s, "healthcheck:") {
		issues = append(issues, lintIssue{
			Severity: severityWarning,
			Message:  "No healthcheck defined for services",
			Fix:      "Add healthcheck to services for monitoring",
		})
	}
	if !strings.Contains(s, "restart:") {
		issues = append(issues, lintIssue{
			Severity: severityInfo,
			Message:  "No restart policy defined",
			Fix:      "Add restart: unless-stopped or restart: always",
		})
	}
	if strings.Contains(s, ":latest") {
		issues = append(issues, lintIssue{
			Severity: severityWarning,
			Message:  "Using ':latest' image tag",
			Fix:      "Pin to specific image versions",
		})
	}

	return issues
}

func lintTerraform(content []byte) []lintIssue {
	var issues []lintIssue
	s := string(content)

	if !strings.Contains(s, "required_version") {
		issues = append(issues, lintIssue{
			Severity: severityWarning,
			Message:  "No required_version constraint for Terraform",
			Fix:      "Add required_version in terraform block",
		})
	}
	if strings.Contains(s, "\"*\"") {
		issues = append(issues, lintIssue{
			Severity: severityCritical,
			Message:  "Wildcard (*) permissions detected",
			Fix:      "Use least-privilege principle — specify exact permissions",
		})
	}

	return issues
}

func validateDockerfile(content []byte) []string {
	var errors []string
	lines := strings.Split(string(content), "\n")
	hasFrom := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !hasFrom && !strings.HasPrefix(strings.ToUpper(trimmed), "FROM ") && !strings.HasPrefix(strings.ToUpper(trimmed), "ARG ") {
			errors = append(errors, "First instruction must be FROM (or ARG before FROM)")
		}
		if strings.HasPrefix(strings.ToUpper(trimmed), "FROM ") {
			hasFrom = true
		}
	}
	if !hasFrom {
		errors = append(errors, "Missing FROM instruction")
	}
	return errors
}

func validateYAML(content []byte) []string {
	var errors []string
	s := string(content)
	// Basic YAML validation: check for tabs (YAML doesn't allow tabs for indentation)
	for i, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "\t") {
			errors = append(errors, fmt.Sprintf("Line %d: Tab character used for indentation (YAML requires spaces)", i+1))
		}
	}
	return errors
}

func validateTerraform(content []byte) []string {
	var errors []string
	s := string(content)
	// Basic brace matching
	open := strings.Count(s, "{")
	close := strings.Count(s, "}")
	if open != close {
		errors = append(errors, fmt.Sprintf("Mismatched braces: %d opening, %d closing", open, close))
	}
	return errors
}
