package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/paffin/opsmate/pkg/mcputil"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/mark3labs/mcp-go/mcp"
)

type handlers struct {
	client   client.APIClient
	readonly bool
	maxLines int
}

func (h *handlers) ps(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	all := mcputil.OptionalBool(req, "all", false)
	filterStr := mcputil.OptionalString(req, "filter", "")

	opts := container.ListOptions{All: all}
	if filterStr != "" {
		parts := strings.SplitN(filterStr, "=", 2)
		if len(parts) == 2 {
			opts.Filters = filters.NewArgs(filters.Arg(parts[0], parts[1]))
		}
	}

	containers, err := h.client.ContainerList(ctx, opts)
	if err != nil {
		return mcputil.ToolError("failed to list containers: %v", err)
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "ID\tNAME\tIMAGE\tSTATUS\tPORTS\n")

	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		ports := formatPorts(c.Ports)
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			c.ID[:12], name, c.Image, c.Status, ports)
	}
	_ = w.Flush()

	return mcputil.ToolText("%d containers:\n\n%s", len(containers), buf.String())
}

func (h *handlers) logs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID, err := mcputil.RequireString(req, "container")
	if err != nil {
		return mcputil.ToolError("container is required")
	}

	tail := fmt.Sprintf("%d", mcputil.OptionalInt(req, "tail", 100))
	since := mcputil.OptionalString(req, "since", "")

	opts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tail,
	}
	if since != "" {
		opts.Since = since
	}

	reader, err := h.client.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		return mcputil.ToolError("failed to get logs: %v", err)
	}
	defer func() { _ = reader.Close() }()

	logBytes, err := io.ReadAll(reader)
	if err != nil {
		return mcputil.ToolError("failed to read logs: %v", err)
	}

	// Strip Docker log header bytes (8-byte prefix per line)
	cleaned := cleanDockerLogs(logBytes)

	return mcputil.ToolText("Logs for container %s:\n\n%s", containerID, cleaned)
}

func (h *handlers) inspect(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID, err := mcputil.RequireString(req, "container")
	if err != nil {
		return mcputil.ToolError("container is required")
	}

	info, err := h.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return mcputil.ToolError("failed to inspect container: %v", err)
	}

	var buf bytes.Buffer
	_, _ = fmt.Fprintf(&buf, "Container: %s (%s)\n", info.Name, info.ID[:12])
	_, _ = fmt.Fprintf(&buf, "Image: %s\n", info.Config.Image)
	_, _ = fmt.Fprintf(&buf, "Status: %s\n", info.State.Status)
	_, _ = fmt.Fprintf(&buf, "Started: %s\n", info.State.StartedAt)
	_, _ = fmt.Fprintf(&buf, "Platform: %s\n", info.Platform)

	if info.State.Health != nil {
		_, _ = fmt.Fprintf(&buf, "Health: %s\n", info.State.Health.Status)
	}

	_, _ = fmt.Fprintf(&buf, "\nNetwork:\n")
	for name, net := range info.NetworkSettings.Networks {
		_, _ = fmt.Fprintf(&buf, "  %s: IP=%s\n", name, net.IPAddress)
	}

	_, _ = fmt.Fprintf(&buf, "\nMounts:\n")
	for _, m := range info.Mounts {
		_, _ = fmt.Fprintf(&buf, "  %s → %s (%s)\n", m.Source, m.Destination, m.Type)
	}

	_, _ = fmt.Fprintf(&buf, "\nEnvironment:\n")
	for _, e := range info.Config.Env {
		// Redact potential secrets
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 && isSecretEnv(parts[0]) {
			_, _ = fmt.Fprintf(&buf, "  %s=***REDACTED***\n", parts[0])
		} else {
			_, _ = fmt.Fprintf(&buf, "  %s\n", e)
		}
	}

	return mcputil.ToolText("%s", buf.String())
}

func (h *handlers) stats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := mcputil.OptionalString(req, "container", "")

	if containerID != "" {
		return h.singleContainerStats(ctx, containerID)
	}

	// List all running containers and get stats
	containers, err := h.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return mcputil.ToolError("failed to list containers: %v", err)
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "NAME\tCPU %%\tMEM USAGE\tMEM LIMIT\tMEM %%\n")

	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		statsResp, err := h.client.ContainerStatsOneShot(ctx, c.ID)
		if err != nil {
			continue
		}
		var stats container.StatsResponse
		if err := json.NewDecoder(statsResp.Body).Decode(&stats); err != nil {
			_ = statsResp.Body.Close()
			continue
		}
		_ = statsResp.Body.Close()

		cpuPercent := calculateCPUPercent(&stats)
		memUsage := float64(stats.MemoryStats.Usage) / 1024 / 1024
		memLimit := float64(stats.MemoryStats.Limit) / 1024 / 1024
		memPercent := 0.0
		if memLimit > 0 {
			memPercent = (memUsage / memLimit) * 100
		}

		_, _ = fmt.Fprintf(w, "%s\t%.2f%%\t%.1fMiB\t%.1fMiB\t%.1f%%\n",
			name, cpuPercent, memUsage, memLimit, memPercent)
	}
	_ = w.Flush()

	return mcputil.ToolText("Container stats:\n\n%s", buf.String())
}

func (h *handlers) singleContainerStats(ctx context.Context, containerID string) (*mcp.CallToolResult, error) {
	statsResp, err := h.client.ContainerStatsOneShot(ctx, containerID)
	if err != nil {
		return mcputil.ToolError("failed to get stats: %v", err)
	}
	defer func() { _ = statsResp.Body.Close() }()

	var stats container.StatsResponse
	if err := json.NewDecoder(statsResp.Body).Decode(&stats); err != nil {
		return mcputil.ToolError("failed to decode stats: %v", err)
	}

	cpuPercent := calculateCPUPercent(&stats)
	memUsage := float64(stats.MemoryStats.Usage) / 1024 / 1024
	memLimit := float64(stats.MemoryStats.Limit) / 1024 / 1024

	return mcputil.ToolText("Stats for %s:\nCPU: %.2f%%\nMemory: %.1f MiB / %.1f MiB (%.1f%%)\nNetwork I/O: %s\nBlock I/O: %s",
		containerID, cpuPercent, memUsage, memLimit,
		(memUsage/memLimit)*100,
		formatNetworkIO(&stats), formatBlockIO(&stats))
}

func (h *handlers) images(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filterStr := mcputil.OptionalString(req, "filter", "")

	opts := image.ListOptions{}
	if filterStr != "" {
		parts := strings.SplitN(filterStr, "=", 2)
		if len(parts) == 2 {
			opts.Filters = filters.NewArgs(filters.Arg(parts[0], parts[1]))
		}
	}

	images, err := h.client.ImageList(ctx, opts)
	if err != nil {
		return mcputil.ToolError("failed to list images: %v", err)
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "REPOSITORY\tTAG\tSIZE\n")

	for _, img := range images {
		for _, tag := range img.RepoTags {
			parts := strings.SplitN(tag, ":", 2)
			repo := parts[0]
			tagName := "latest"
			if len(parts) > 1 {
				tagName = parts[1]
			}
			size := float64(img.Size) / 1024 / 1024
			_, _ = fmt.Fprintf(w, "%s\t%s\t%.1fMB\n", repo, tagName, size)
		}
	}
	_ = w.Flush()

	return mcputil.ToolText("%d images:\n\n%s", len(images), buf.String())
}

func (h *handlers) composePS(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projectDir := mcputil.OptionalString(req, "project_dir", ".")

	// Use label filter to find compose containers
	opts := container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project"),
		),
	}

	containers, err := h.client.ContainerList(ctx, opts)
	if err != nil {
		return mcputil.ToolError("failed to list compose containers: %v", err)
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "SERVICE\tSTATUS\tPORTS\n")

	for _, c := range containers {
		service := c.Labels["com.docker.compose.service"]
		ports := formatPorts(c.Ports)
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", service, c.Status, ports)
	}
	_ = w.Flush()

	return mcputil.ToolText("Compose services (dir: %s):\n\n%s", projectDir, buf.String())
}

func (h *handlers) composeLogs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	service := mcputil.OptionalString(req, "service", "")

	listOpts := container.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project"),
		),
	}
	if service != "" {
		listOpts.Filters.Add("label", fmt.Sprintf("com.docker.compose.service=%s", service))
	}

	containers, err := h.client.ContainerList(ctx, listOpts)
	if err != nil {
		return mcputil.ToolError("failed to list compose containers: %v", err)
	}

	var buf bytes.Buffer
	for _, c := range containers {
		svc := c.Labels["com.docker.compose.service"]
		reader, err := h.client.ContainerLogs(ctx, c.ID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Tail:       "50",
		})
		if err != nil {
			continue
		}
		logBytes, _ := io.ReadAll(reader)
		_ = reader.Close()
		_, _ = fmt.Fprintf(&buf, "=== %s ===\n%s\n\n", svc, cleanDockerLogs(logBytes))
	}

	return mcputil.ToolText("Compose logs:\n\n%s", buf.String())
}

func (h *handlers) exec(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if h.readonly {
		return mcputil.ToolError("docker is in read-only mode, exec is disabled")
	}

	containerID, err := mcputil.RequireString(req, "container")
	if err != nil {
		return mcputil.ToolError("container is required")
	}
	command, err := mcputil.RequireString(req, "command")
	if err != nil {
		return mcputil.ToolError("command is required")
	}

	execConfig := container.ExecOptions{
		Cmd:          strings.Fields(command),
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := h.client.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return mcputil.ToolError("failed to create exec: %v", err)
	}

	resp, err := h.client.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return mcputil.ToolError("failed to attach exec: %v", err)
	}
	defer resp.Close()

	output, err := io.ReadAll(resp.Reader)
	if err != nil {
		return mcputil.ToolError("failed to read exec output: %v", err)
	}

	return mcputil.ToolText("Exec output:\n\n%s", cleanDockerLogs(output))
}

func formatPorts(ports []container.Port) string {
	parts := []string{}
	for _, p := range ports {
		if p.PublicPort > 0 {
			parts = append(parts, fmt.Sprintf("%d→%d/%s", p.PublicPort, p.PrivatePort, p.Type))
		} else {
			parts = append(parts, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
		}
	}
	return strings.Join(parts, ", ")
}

func calculateCPUPercent(stats *container.StatsResponse) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)
	if systemDelta > 0 && cpuDelta > 0 {
		return (cpuDelta / systemDelta) * float64(stats.CPUStats.OnlineCPUs) * 100.0
	}
	return 0.0
}

func formatNetworkIO(stats *container.StatsResponse) string {
	var rxBytes, txBytes uint64
	for _, net := range stats.Networks {
		rxBytes += net.RxBytes
		txBytes += net.TxBytes
	}
	return fmt.Sprintf("RX: %.1fMB / TX: %.1fMB",
		float64(rxBytes)/1024/1024, float64(txBytes)/1024/1024)
}

func formatBlockIO(stats *container.StatsResponse) string {
	var read, write uint64
	for _, bio := range stats.BlkioStats.IoServiceBytesRecursive {
		switch bio.Op {
		case "read":
			read += bio.Value
		case "write":
			write += bio.Value
		}
	}
	return fmt.Sprintf("Read: %.1fMB / Write: %.1fMB",
		float64(read)/1024/1024, float64(write)/1024/1024)
}

func cleanDockerLogs(b []byte) string {
	// Docker multiplexed stream has 8-byte header per frame
	var buf bytes.Buffer
	for len(b) > 0 {
		if len(b) < 8 {
			buf.Write(b)
			break
		}
		// Header: [stream_type, 0, 0, 0, size_bytes...]
		streamType := b[0]
		if streamType > 2 {
			// Not a Docker stream header, write as-is
			buf.Write(b)
			break
		}
		size := int(b[4])<<24 | int(b[5])<<16 | int(b[6])<<8 | int(b[7])
		b = b[8:]
		if size > len(b) {
			size = len(b)
		}
		buf.Write(b[:size])
		b = b[size:]
	}
	return buf.String()
}

func isSecretEnv(key string) bool {
	lower := strings.ToLower(key)
	secrets := []string{"password", "secret", "token", "key", "credential", "auth"}
	for _, s := range secrets {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
