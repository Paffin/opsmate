package tui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RunREPL starts an interactive REPL that streams Claude responses inline.
// This matches the terminal-native scrolling style (no alt-screen).
func RunREPL(mcpConfigPath, workDir string, servers []string) error {
	reader := bufio.NewReader(os.Stdin)
	sessionID := ""

	for {
		// Show prompt
		_, _ = fmt.Fprint(os.Stdout, promptStyle.Render("> "))

		line, err := reader.ReadString('\n')
		if err != nil {
			// EOF or read error — exit gracefully
			_, _ = fmt.Fprintln(os.Stdout)
			return nil
		}

		query := strings.TrimSpace(line)
		if query == "" {
			continue
		}
		if query == "exit" || query == "quit" || query == "/quit" || query == "/exit" {
			return nil
		}

		_, _ = fmt.Fprintln(os.Stdout)

		// Route model: override takes precedence, otherwise auto-detect
		model := ModelOverride
		if model == "" {
			model = RouteModel(query)
		}
		modelLabel := modelTag(model)
		if modelLabel != "" {
			_, _ = fmt.Fprintln(os.Stdout, modelLabel)
		}

		// Create cancellable context for streaming
		ctx, cancel := context.WithCancel(context.Background())

		// Set up Ctrl+C handler to cancel streaming (not kill the process)
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		go func() {
			select {
			case <-sigCh:
				cancel()
			case <-ctx.Done():
			}
		}()

		ch, err := RunQuery(ctx, query, sessionID, mcpConfigPath, workDir, model)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stdout, errorStyle.Render("Error: "+err.Error()))
			_, _ = fmt.Fprintln(os.Stdout)
			signal.Stop(sigCh)
			cancel()
			continue
		}

		// Accumulate response for markdown rendering
		var responseBuf strings.Builder
		var toolCalls []string
		cancelled := false

		for event := range ch {
			switch event.Type {
			case "assistant_chunk":
				responseBuf.WriteString(event.Content)

			case "tool_use":
				toolCalls = append(toolCalls, event.Tool)
				_, _ = fmt.Fprint(os.Stdout, toolStyle.Render("  ▸ "+formatToolName(event.Tool))+"\n")

			case "message_end":
				if event.SessionID != "" {
					sessionID = event.SessionID
				}
				// Render accumulated markdown
				raw := responseBuf.String()
				if strings.TrimSpace(raw) != "" {
					rendered := renderMarkdown(raw)
					_, _ = fmt.Fprint(os.Stdout, rendered)
				}
				_, _ = fmt.Fprintln(os.Stdout)

			case "error":
				// Flush any partial content first
				if partial := responseBuf.String(); strings.TrimSpace(partial) != "" {
					_, _ = fmt.Fprint(os.Stdout, partial)
					_, _ = fmt.Fprintln(os.Stdout)
				}
				_, _ = fmt.Fprintln(os.Stdout, errorStyle.Render("Error: "+event.Content))
				_, _ = fmt.Fprintln(os.Stdout)
			}
		}

		signal.Stop(sigCh)
		cancel()

		if cancelled {
			_, _ = fmt.Fprintln(os.Stdout, dimStyle.Render("(cancelled)"))
			_, _ = fmt.Fprintln(os.Stdout)
		}
	}
}

// PrintBanner displays the startup banner with server information.
// This is called before entering the REPL loop.
func PrintBanner(servers []string) {
	green := lipgloss.Color("#9ece6a")
	greenStyle := lipgloss.NewStyle().Foreground(green)

	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, "  "+headerStyle.Render("opsmate")+" "+dimStyle.Render("— DevOps AI Assistant"))
	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, "  MCP Servers:")
	for _, s := range servers {
		_, _ = fmt.Fprintln(os.Stdout, "  "+greenStyle.Render("✔")+" "+s)
	}
	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, "  "+dimStyle.Render("Launching Claude Code with DevOps superpowers..."))
	_, _ = fmt.Fprintln(os.Stdout)
}

// formatToolName makes MCP tool names human-readable.
// "mcp_docker__docker_ps" -> "docker: ps"
// "Bash" -> "bash"
func formatToolName(name string) string {
	// MCP tool pattern: mcp_{server}__{tool}
	if strings.HasPrefix(name, "mcp_") || strings.HasPrefix(name, "mcp__") {
		parts := strings.SplitN(name, "__", 2)
		if len(parts) == 2 {
			server := strings.TrimPrefix(parts[0], "mcp_")
			tool := parts[1]
			return server + ": " + tool
		}
	}
	return strings.ToLower(name)
}

// modelTag returns a styled label showing which model tier is being used.
func modelTag(model string) string {
	switch model {
	case ModelFast:
		return dimStyle.Render("  ⚡ haiku (fast)")
	case ModelDeep:
		return dimStyle.Render("  🧠 opus (deep)")
	default:
		return "" // don't clutter output for default model
	}
}
