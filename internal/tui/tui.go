package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Tokyo Night palette
var (
	blue   = lipgloss.Color("#7aa2f7")
	orange = lipgloss.Color("#e0af68")
	red    = lipgloss.Color("#f7768e")
	dim    = lipgloss.Color("#565f89")
	fg     = lipgloss.Color("#c0caf5")

	promptStyle = lipgloss.NewStyle().Foreground(blue).Bold(true)
	toolStyle   = lipgloss.NewStyle().Foreground(orange)
	errorStyle  = lipgloss.NewStyle().Foreground(red)
	dimStyle    = lipgloss.NewStyle().Foreground(dim)
	headerStyle = lipgloss.NewStyle().Foreground(blue).Bold(true)
)

// streamEventMsg wraps a StreamEvent as a bubbletea message.
type streamEventMsg StreamEvent

type model struct {
	viewport  viewport.Model
	input     textinput.Model
	content   strings.Builder
	streaming bool
	sessionID string
	mcpConfig string
	workDir   string
	eventCh   <-chan StreamEvent
	cancel    context.CancelFunc
	width     int
	height    int
	ready     bool
	servers   []string
}

func newModel(mcpConfigPath, workDir string, servers []string) model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.PromptStyle = promptStyle
	ti.TextStyle = lipgloss.NewStyle().Foreground(fg)
	ti.Focus()
	ti.CharLimit = 4096

	return model{
		input:     ti,
		mcpConfig: mcpConfigPath,
		workDir:   workDir,
		servers:   servers,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit

		case tea.KeyEsc:
			if m.streaming && m.cancel != nil {
				m.cancel()
				m.streaming = false
				m.content.WriteString("\n" + dimStyle.Render("(cancelled)") + "\n\n")
				m.viewport.SetContent(m.content.String())
				m.viewport.GotoBottom()
				return m, nil
			}

		case tea.KeyEnter:
			if m.streaming {
				return m, nil
			}
			query := strings.TrimSpace(m.input.Value())
			if query == "" {
				return m, nil
			}

			if query == "exit" || query == "quit" {
				if m.cancel != nil {
					m.cancel()
				}
				return m, tea.Quit
			}

			m.input.SetValue("")

			// Add user message to content
			m.content.WriteString(promptStyle.Render("> ") + lipgloss.NewStyle().Foreground(fg).Render(query) + "\n\n")
			m.viewport.SetContent(m.content.String())
			m.viewport.GotoBottom()

			// Start streaming
			m.streaming = true
			ctx, cancel := context.WithCancel(context.Background())
			m.cancel = cancel

			ch, err := RunQuery(ctx, query, m.sessionID, m.mcpConfig, m.workDir)
			if err != nil {
				m.content.WriteString(errorStyle.Render("Error: "+err.Error()) + "\n\n")
				m.viewport.SetContent(m.content.String())
				m.viewport.GotoBottom()
				m.streaming = false
				cancel()
				return m, nil
			}
			m.eventCh = ch
			return m, waitForEvent(ch)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 2
		inputHeight := 1

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-headerHeight-inputHeight)
			m.viewport.SetContent(m.welcomeMessage())
			m.content.WriteString(m.welcomeMessage())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - headerHeight - inputHeight
		}
		m.input.Width = msg.Width - 4
		return m, nil

	case streamEventMsg:
		event := StreamEvent(msg)
		switch event.Type {
		case "assistant_chunk":
			m.content.WriteString(event.Content)
			m.viewport.SetContent(m.content.String())
			m.viewport.GotoBottom()
		case "tool_use":
			m.content.WriteString("\n" + toolStyle.Render("  ["+event.Tool+"]") + "\n")
			m.viewport.SetContent(m.content.String())
			m.viewport.GotoBottom()
		case "message_end":
			m.content.WriteString("\n\n")
			m.viewport.SetContent(m.content.String())
			m.viewport.GotoBottom()
			m.streaming = false
			if event.SessionID != "" {
				m.sessionID = event.SessionID
			}
			return m, nil
		case "error":
			m.content.WriteString("\n" + errorStyle.Render("Error: "+event.Content) + "\n\n")
			m.viewport.SetContent(m.content.String())
			m.viewport.GotoBottom()
			m.streaming = false
			return m, nil
		}
		return m, waitForEvent(m.eventCh)
	}

	// Update sub-components
	var cmd tea.Cmd
	if !m.streaming {
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
	}
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return ""
	}

	header := headerStyle.Render("  opsmate")
	if len(m.servers) > 0 {
		header += dimStyle.Render("  " + strings.Join(m.servers, " · "))
	}
	header += "\n"

	var status string
	if m.streaming {
		status = dimStyle.Render("  streaming...")
	}

	headerLine := header + status
	if status == "" {
		headerLine = header
	}

	return headerLine + m.viewport.View() + "\n" + m.input.View()
}

func (m model) welcomeMessage() string {
	var b strings.Builder
	b.WriteString(dimStyle.Render("  Type your question and press Enter. Ctrl+C to exit.") + "\n\n")
	return b.String()
}

func waitForEvent(ch <-chan StreamEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return streamEventMsg(StreamEvent{Type: "message_end"})
		}
		return streamEventMsg(event)
	}
}

// Run starts the TUI application in REPL mode (terminal-native scrolling).
func Run(mcpConfigPath, workDir string, servers []string) error {
	PrintBanner(servers)
	return RunREPL(mcpConfigPath, workDir, servers)
}

// RunAltScreen starts the TUI in full-screen alt-screen mode (legacy).
func RunAltScreen(mcpConfigPath, workDir string, servers []string) error {
	m := newModel(mcpConfigPath, workDir, servers)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
