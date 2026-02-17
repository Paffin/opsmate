package mcputil

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// NewServer creates a new MCP server with standard opsmate configuration.
func NewServer(name, version string) *server.MCPServer {
	return server.NewMCPServer(
		fmt.Sprintf("opsmate-%s", name),
		version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)
}

// Serve starts the MCP server on stdio transport.
func Serve(s *server.MCPServer) error {
	return server.ServeStdio(s)
}

// ToolError returns a standardized tool error result.
func ToolError(format string, args ...any) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(fmt.Sprintf(format, args...)), nil
}

// ToolText returns a text tool result.
func ToolText(format string, args ...any) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(fmt.Sprintf(format, args...)), nil
}

// RequireString extracts a required string parameter from a tool request.
func RequireString(req mcp.CallToolRequest, name string) (string, error) {
	return req.RequireString(name)
}

// OptionalString extracts an optional string parameter with a default.
func OptionalString(req mcp.CallToolRequest, name, defaultVal string) string {
	val, err := req.RequireString(name)
	if err != nil || val == "" {
		return defaultVal
	}
	return val
}

// OptionalInt extracts an optional integer parameter with a default.
func OptionalInt(req mcp.CallToolRequest, name string, defaultVal int) int {
	val, err := req.RequireFloat(name)
	if err != nil {
		return defaultVal
	}
	return int(val)
}

// OptionalBool extracts an optional boolean parameter with a default.
func OptionalBool(req mcp.CallToolRequest, name string, defaultVal bool) bool {
	args := req.GetArguments()
	if args == nil {
		return defaultVal
	}
	val, ok := args[name]
	if !ok {
		return defaultVal
	}
	b, ok := val.(bool)
	if !ok {
		return defaultVal
	}
	return b
}

// SafeHandler wraps a tool handler with panic recovery and context cancellation check.
func SafeHandler(fn func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if ctx.Err() != nil {
			return ToolError("request cancelled")
		}
		return fn(ctx, req)
	}
}
