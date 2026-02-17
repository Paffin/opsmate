# opsmate Development Guide

## Project Overview
Go CLI that launches MCP servers for Kubernetes, Docker, Prometheus, and file analysis,
then starts Claude Code with DevOps context.

## Build & Test
```bash
go build ./...                    # Build all packages
go build -o opsmate ./cmd/opsmate # Build binary
go test ./...                     # Run all tests
go test -v ./internal/config/     # Test specific package
```

## Architecture
- `cmd/opsmate/` — CLI entry point (Cobra commands)
- `internal/config/` — Viper-based config management
- `internal/launcher/` — Claude Code process lifecycle
- `internal/mcphost/` — MCP server config generation
- `internal/context/` — Infrastructure context collector
- `mcp/kubernetes/` — K8s MCP server (client-go)
- `mcp/docker/` — Docker MCP server (docker client)
- `mcp/prometheus/` — Prometheus MCP server (HTTP API)
- `mcp/files/` — File analyzer MCP server (lint rules)
- `pkg/mcputil/` — Shared MCP server helpers

## MCP Server Pattern
Each MCP server has: `server.go` (entry), `tools.go` (tool definitions), `handlers.go` (implementations).
Uses github.com/mark3labs/mcp-go for MCP protocol over stdio.

## Adding a New MCP Server
1. Create `mcp/newserver/` with server.go, tools.go, handlers.go
2. Add config struct to `internal/config/config.go`
3. Add subcommand in `cmd/opsmate/main.go`
4. Add to `internal/mcphost/manager.go` for .mcp.json generation
