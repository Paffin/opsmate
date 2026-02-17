# Contributing to opsmate

Thanks for your interest in contributing! Here's how to get started.

## Development Setup

```bash
git clone https://github.com/paffin/opsmate
cd opsmate
go build ./...
go test ./...
```

Requirements:
- Go 1.22+
- (Optional) kubectl, docker, prometheus for integration testing

## Project Structure

```
opsmate/
├── cmd/opsmate/         # CLI entry point
├── internal/
│   ├── config/          # Configuration management
│   ├── launcher/        # Claude Code process lifecycle
│   ├── mcphost/         # MCP config generation
│   └── context/         # Infrastructure context collection
├── mcp/
│   ├── kubernetes/      # K8s MCP server
│   ├── docker/          # Docker MCP server
│   ├── prometheus/      # Prometheus MCP server
│   └── files/           # File analyzer MCP server
├── pkg/mcputil/         # Shared MCP helpers
├── prompts/             # System prompts
└── configs/             # Default config templates
```

## Adding a New MCP Server

This is the most impactful contribution you can make. Each MCP server lives in `mcp/<name>/` with three files:

### 1. `tools.go` — Define your tools

```go
package myserver

import "github.com/mark3labs/mcp-go/mcp"

func toolDoSomething() mcp.Tool {
    return mcp.NewTool("myserver_do_something",
        mcp.WithDescription("What this tool does"),
        mcp.WithString("param",
            mcp.Required(),
            mcp.Description("Parameter description"),
        ),
    )
}
```

### 2. `handlers.go` — Implement the logic

```go
package myserver

import (
    "context"
    "github.com/paffin/opsmate/pkg/mcputil"
    "github.com/mark3labs/mcp-go/mcp"
)

type handlers struct {
    // your client here
}

func (h *handlers) doSomething(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    param, err := mcputil.RequireString(req, "param")
    if err != nil {
        return mcputil.ToolError("param is required")
    }
    // ... your logic ...
    return mcputil.ToolText("Result: %s", param)
}
```

### 3. `server.go` — Wire it up

```go
package myserver

import (
    "github.com/paffin/opsmate/internal/config"
    "github.com/paffin/opsmate/pkg/mcputil"
)

func Run(cfg config.MyServerConfig) error {
    h := &handlers{/* init client */}
    s := mcputil.NewServer("myserver", "0.1.0")
    s.AddTool(toolDoSomething(), mcputil.SafeHandler(h.doSomething))
    return mcputil.Serve(s)
}
```

### 4. Register it

- Add config struct to `internal/config/config.go`
- Add subcommand in `cmd/opsmate/main.go`
- Add to `internal/mcphost/manager.go`

## Adding Lint Rules

Add new rules in `mcp/files/handlers.go`. Each lint function returns `[]lintIssue`:

```go
func lintMyFormat(content []byte) []lintIssue {
    var issues []lintIssue
    // ... check content ...
    issues = append(issues, lintIssue{
        Severity: severityWarning,
        Message:  "Description of the issue",
        Line:     42,
        Fix:      "How to fix it",
    })
    return issues
}
```

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Use `mcputil.ToolError()` and `mcputil.ToolText()` for tool responses
- Use `mcputil.SafeHandler()` to wrap all tool handlers
- Add tests for new lint rules and file type detection
- Keep tool descriptions clear and concise (Claude reads them)

## Pull Requests

1. Fork the repo
2. Create a feature branch (`git checkout -b feature/terraform-mcp`)
3. Write tests for new functionality
4. Make sure `go test ./...` passes
5. Submit a PR with a clear description

## Ideas for Contributions

- **New MCP servers**: Terraform, Ansible, Grafana, ArgoCD, Vault
- **New lint rules**: Helm charts, GitHub Actions, Nginx configs
- **CLI improvements**: `opsmate doctor`, `opsmate init`, TUI dashboard
- **Documentation**: Tutorials, video demos, blog posts

## Questions?

Open an issue or start a discussion. We're happy to help!
