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

		ch, err := RunQuery(ctx, query, sessionID, mcpConfigPath, workDir)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stdout, errorStyle.Render("Error: "+err.Error()))
			_, _ = fmt.Fprintln(os.Stdout)
			signal.Stop(sigCh)
			cancel()
			continue
		}

		cancelled := false
		for event := range ch {
			switch event.Type {
			case "assistant_chunk":
				_, _ = fmt.Fprint(os.Stdout, event.Content)

			case "tool_use":
				_, _ = fmt.Fprintln(os.Stdout)
				_, _ = fmt.Fprintln(os.Stdout, toolStyle.Render("  ["+event.Tool+"]"))

			case "message_end":
				if event.SessionID != "" {
					sessionID = event.SessionID
				}
				_, _ = fmt.Fprintln(os.Stdout)
				_, _ = fmt.Fprintln(os.Stdout)

			case "error":
				_, _ = fmt.Fprintln(os.Stdout)
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
