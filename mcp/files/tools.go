package files

import "github.com/mark3labs/mcp-go/mcp"

func toolAnalyze() mcp.Tool {
	return mcp.NewTool("file_analyze",
		mcp.WithDescription("Analyze an infrastructure file (Dockerfile, K8s YAML, docker-compose, Terraform)"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the file to analyze"),
		),
	)
}

func toolLint() mcp.Tool {
	return mcp.NewTool("file_lint",
		mcp.WithDescription("Lint an infrastructure file with best practices"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the file to lint"),
		),
		mcp.WithString("ruleset",
			mcp.Description("Specific ruleset to apply (dockerfile, kubernetes, compose, terraform). Auto-detected if not specified."),
		),
	)
}

func toolValidate() mcp.Tool {
	return mcp.NewTool("file_validate",
		mcp.WithDescription("Validate syntax of an infrastructure file"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the file to validate"),
		),
	)
}

func toolScanDir() mcp.Tool {
	return mcp.NewTool("file_scan_dir",
		mcp.WithDescription("Scan a directory for infrastructure files"),
		mcp.WithString("dir",
			mcp.Description("Directory to scan (default: current directory)"),
		),
		mcp.WithBoolean("recursive",
			mcp.Description("Scan subdirectories recursively (default: true)"),
		),
	)
}
