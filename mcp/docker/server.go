package docker

import (
	"fmt"

	"github.com/paffin/opsmate/internal/config"
	"github.com/paffin/opsmate/pkg/mcputil"
	"github.com/docker/docker/client"
)

// Run starts the Docker MCP server on stdio.
func Run(cfg config.DockerConfig, maxLogLines int) error {
	opts := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}
	if cfg.Host != "" {
		opts = append(opts, client.WithHost(cfg.Host))
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}

	h := &handlers{
		client:   cli,
		readonly: cfg.ReadOnly,
		maxLines: maxLogLines,
	}

	s := mcputil.NewServer("docker", "0.1.0")

	// Read-only tools
	s.AddTool(toolPS(), mcputil.SafeHandler(h.ps))
	s.AddTool(toolLogs(), mcputil.SafeHandler(h.logs))
	s.AddTool(toolInspect(), mcputil.SafeHandler(h.inspect))
	s.AddTool(toolStats(), mcputil.SafeHandler(h.stats))
	s.AddTool(toolImages(), mcputil.SafeHandler(h.images))
	s.AddTool(toolComposePS(), mcputil.SafeHandler(h.composePS))
	s.AddTool(toolComposeLogs(), mcputil.SafeHandler(h.composeLogs))

	// Write tools (only if not readonly)
	if !cfg.ReadOnly {
		s.AddTool(toolExec(), mcputil.SafeHandler(h.exec))
	}

	return mcputil.Serve(s)
}
