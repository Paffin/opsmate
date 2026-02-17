package chatui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/gorilla/websocket"
)

//go:embed static
var staticFiles embed.FS

// Server is the HTTP/WebSocket server for the chat UI.
type Server struct {
	Port          int
	WorkDir       string
	MCPConfigPath string
	upgrader      websocket.Upgrader
}

// New creates a new chat UI server.
func New(port int, workDir, mcpConfigPath string) *Server {
	return &Server{
		Port:          port,
		WorkDir:       workDir,
		MCPConfigPath: mcpConfigPath,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Serve index.html at root
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	// Serve static files
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("create static sub-fs: %w", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))

	// WebSocket endpoint
	mux.HandleFunc("/ws", s.handleWebSocket)

	addr := fmt.Sprintf(":%d", s.Port)
	return http.ListenAndServe(addr, mux)
}

// incomingMessage is the JSON message format from the client.
type incomingMessage struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// outgoingMessage is the JSON message format sent to the client.
type outgoingMessage struct {
	Type      string `json:"type"`
	Content   string `json:"content,omitempty"`
	Tool      string `json:"tool,omitempty"`
	Input     string `json:"input,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message,omitempty"`
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	var sessionID string

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg incomingMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			s.sendError(conn, "invalid JSON message")
			continue
		}

		if msg.Type != "user_message" {
			s.sendError(conn, "unknown message type: "+msg.Type)
			continue
		}

		ctx, cancel := context.WithCancel(r.Context())
		events, err := RunQuery(ctx, msg.Content, sessionID, s.MCPConfigPath, s.WorkDir)
		if err != nil {
			cancel()
			s.sendError(conn, err.Error())
			continue
		}

		for event := range events {
			var out outgoingMessage
			switch event.Type {
			case "assistant_chunk":
				out = outgoingMessage{Type: "assistant_chunk", Content: event.Content}
			case "tool_use":
				out = outgoingMessage{Type: "tool_use", Tool: event.Tool, Input: event.Input}
			case "message_end":
				sessionID = event.SessionID
				out = outgoingMessage{Type: "message_end", SessionID: event.SessionID}
			case "error":
				out = outgoingMessage{Type: "error", Message: event.Content}
			default:
				continue
			}

			data, err := json.Marshal(out)
			if err != nil {
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				cancel()
				return
			}
		}
		cancel()
	}
}

func (s *Server) sendError(conn *websocket.Conn, msg string) {
	out := outgoingMessage{Type: "error", Message: msg}
	data, _ := json.Marshal(out)
	conn.WriteMessage(websocket.TextMessage, data)
}
