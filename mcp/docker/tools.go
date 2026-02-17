package docker

import "github.com/mark3labs/mcp-go/mcp"

func toolPS() mcp.Tool {
	return mcp.NewTool("docker_ps",
		mcp.WithDescription("List Docker containers"),
		mcp.WithBoolean("all",
			mcp.Description("Show all containers (default shows just running)"),
		),
		mcp.WithString("filter",
			mcp.Description("Filter containers (e.g. name=nginx, status=running)"),
		),
	)
}

func toolLogs() mcp.Tool {
	return mcp.NewTool("docker_logs",
		mcp.WithDescription("Get container logs"),
		mcp.WithString("container",
			mcp.Required(),
			mcp.Description("Container name or ID"),
		),
		mcp.WithNumber("tail",
			mcp.Description("Number of lines from the end (default: 100)"),
		),
		mcp.WithString("since",
			mcp.Description("Show logs since timestamp or relative (e.g. 1h, 30m)"),
		),
	)
}

func toolInspect() mcp.Tool {
	return mcp.NewTool("docker_inspect",
		mcp.WithDescription("Get detailed container information"),
		mcp.WithString("container",
			mcp.Required(),
			mcp.Description("Container name or ID"),
		),
	)
}

func toolStats() mcp.Tool {
	return mcp.NewTool("docker_stats",
		mcp.WithDescription("Get container resource usage statistics"),
		mcp.WithString("container",
			mcp.Description("Container name or ID (default: all running)"),
		),
	)
}

func toolImages() mcp.Tool {
	return mcp.NewTool("docker_images",
		mcp.WithDescription("List Docker images"),
		mcp.WithString("filter",
			mcp.Description("Filter images (e.g. reference=nginx)"),
		),
	)
}

func toolComposePS() mcp.Tool {
	return mcp.NewTool("docker_compose_ps",
		mcp.WithDescription("Show status of Docker Compose services"),
		mcp.WithString("project_dir",
			mcp.Description("Path to docker-compose project directory (default: current)"),
		),
	)
}

func toolComposeLogs() mcp.Tool {
	return mcp.NewTool("docker_compose_logs",
		mcp.WithDescription("Get Docker Compose service logs"),
		mcp.WithString("project_dir",
			mcp.Description("Path to docker-compose project directory"),
		),
		mcp.WithString("service",
			mcp.Description("Service name (default: all services)"),
		),
	)
}

func toolExec() mcp.Tool {
	return mcp.NewTool("docker_exec",
		mcp.WithDescription("Execute a command in a running container (read-only by default)"),
		mcp.WithString("container",
			mcp.Required(),
			mcp.Description("Container name or ID"),
		),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("Command to execute"),
		),
	)
}
