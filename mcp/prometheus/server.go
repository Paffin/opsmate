package prometheus

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/paffin/opsmate/internal/config"
	"github.com/paffin/opsmate/pkg/mcputil"
)

// Run starts the Prometheus MCP server on stdio.
func Run(cfg config.PrometheusConfig) error {
	if cfg.URL == "" {
		return fmt.Errorf("prometheus URL is required")
	}

	h := &handlers{
		baseURL: cfg.URL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	if cfg.BasicAuth != nil {
		h.username = cfg.BasicAuth.Username
		h.password = cfg.BasicAuth.Password
	}
	// Prefer environment variable for password (avoids exposure in process listings)
	if envPass := os.Getenv("OPSMATE_PROM_PASSWORD"); envPass != "" {
		h.password = envPass
	}

	s := mcputil.NewServer("prometheus", "0.1.0")

	s.AddTool(toolQuery(), mcputil.SafeHandler(h.query))
	s.AddTool(toolQueryRange(), mcputil.SafeHandler(h.queryRange))
	s.AddTool(toolAlerts(), mcputil.SafeHandler(h.alerts))
	s.AddTool(toolTargets(), mcputil.SafeHandler(h.targets))
	s.AddTool(toolRules(), mcputil.SafeHandler(h.rules))
	s.AddTool(toolSeries(), mcputil.SafeHandler(h.series))
	s.AddTool(toolLabelValues(), mcputil.SafeHandler(h.labelValues))

	return mcputil.Serve(s)
}
