package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// renderer wraps glamour for terminal markdown rendering.
var renderer *glamour.TermRenderer

func init() {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err == nil {
		renderer = r
	}
}

// renderMarkdown converts markdown text to styled terminal output.
// Falls back to plain text if glamour is unavailable.
func renderMarkdown(md string) string {
	if renderer == nil || strings.TrimSpace(md) == "" {
		return md
	}
	out, err := renderer.Render(md)
	if err != nil {
		return md
	}
	return out
}
