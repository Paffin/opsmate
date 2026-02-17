package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/paffin/opsmate/internal/config"
	infractx "github.com/paffin/opsmate/internal/context"
	"github.com/paffin/opsmate/internal/mcphost"
	"github.com/charmbracelet/log"
)

// Launcher manages the Claude Code process lifecycle.
type Launcher struct {
	cfg     *config.Config
	manager *mcphost.Manager
	workDir string
}

// New creates a new Claude Code launcher.
func New(cfg *config.Config, workDir string) *Launcher {
	return &Launcher{
		cfg:     cfg,
		manager: mcphost.New(cfg),
		workDir: workDir,
	}
}

// Launch starts Claude Code with MCP configuration.
func (l *Launcher) Launch() error {
	// 1. Collect infrastructure context
	log.Info("Collecting infrastructure context...")
	ctx := infractx.Collect(l.cfg)
	log.Info("Infrastructure", "summary", ctx.Summary())

	// 2. Generate .mcp.json
	log.Info("Generating MCP configuration...")
	mcpPath, err := l.manager.GenerateMCPConfig(l.workDir)
	if err != nil {
		return fmt.Errorf("generate mcp config: %w", err)
	}
	defer l.manager.Cleanup(l.workDir)

	// 3. Generate CLAUDE.md
	log.Info("Generating DevOps system prompt...")
	claudeMDPath, err := l.generateClaudeMD(ctx)
	if err != nil {
		return fmt.Errorf("generate CLAUDE.md: %w", err)
	}
	defer l.cleanupClaudeMD(claudeMDPath)

	// 4. Launch Claude Code
	servers := l.manager.EnabledServers()
	log.Info("Launching Claude Code with DevOps superpowers",
		"servers", servers,
		"mcp_config", mcpPath,
	)

	return l.runClaude()
}

func (l *Launcher) generateClaudeMD(ctx *infractx.InfraContext) (string, error) {
	content := infractx.GenerateSystemPrompt(l.cfg, ctx)

	claudeMDPath := filepath.Join(l.workDir, "CLAUDE.md")

	// Check if CLAUDE.md already exists
	existingContent := ""
	if data, err := os.ReadFile(claudeMDPath); err == nil {
		existingContent = string(data)
	}

	// Wrap opsmate content in markers so we can update without losing user content
	marker := "<!-- opsmate:start -->"
	markerEnd := "<!-- opsmate:end -->"
	opsmateBlock := fmt.Sprintf("%s\n%s\n%s", marker, content, markerEnd)

	var finalContent string
	if existingContent != "" {
		// Check if markers already exist
		if idx := indexOf(existingContent, marker); idx >= 0 {
			endIdx := indexOf(existingContent, markerEnd)
			if endIdx >= 0 {
				finalContent = existingContent[:idx] + opsmateBlock + existingContent[endIdx+len(markerEnd):]
			} else {
				finalContent = existingContent + "\n\n" + opsmateBlock
			}
		} else {
			finalContent = opsmateBlock + "\n\n" + existingContent
		}
	} else {
		finalContent = opsmateBlock
	}

	if err := os.WriteFile(claudeMDPath, []byte(finalContent), 0644); err != nil {
		return "", fmt.Errorf("write CLAUDE.md: %w", err)
	}

	return claudeMDPath, nil
}

func (l *Launcher) cleanupClaudeMD(path string) {
	// Read the file and remove opsmate markers
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	content := string(data)
	marker := "<!-- opsmate:start -->"
	markerEnd := "<!-- opsmate:end -->"

	startIdx := indexOf(content, marker)
	endIdx := indexOf(content, markerEnd)

	if startIdx >= 0 && endIdx >= 0 {
		cleaned := content[:startIdx] + content[endIdx+len(markerEnd):]
		cleaned = trimEmptyLines(cleaned)
		if cleaned == "" {
			os.Remove(path)
		} else {
			os.WriteFile(path, []byte(cleaned), 0644)
		}
	}
}

func (l *Launcher) runClaude() error {
	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude CLI not found in PATH. Install it from https://docs.anthropic.com/en/docs/claude-code")
	}

	cmd := exec.Command(claudeBin)
	cmd.Dir = l.workDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func trimEmptyLines(s string) string {
	lines := []string{}
	for _, line := range splitLines(s) {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return joinLines(lines)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}
