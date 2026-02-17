package files

import (
	"github.com/paffin/opsmate/internal/config"
	"github.com/paffin/opsmate/pkg/mcputil"
)

// Run starts the File Analyzer MCP server on stdio.
func Run(cfg config.FilesConfig) error {
	h := &handlers{
		rulesets: cfg.Rulesets,
	}

	s := mcputil.NewServer("files", "0.1.0")

	s.AddTool(toolAnalyze(), mcputil.SafeHandler(h.analyze))
	s.AddTool(toolLint(), mcputil.SafeHandler(h.lint))
	s.AddTool(toolValidate(), mcputil.SafeHandler(h.validate))
	s.AddTool(toolScanDir(), mcputil.SafeHandler(h.scanDir))

	return mcputil.Serve(s)
}
