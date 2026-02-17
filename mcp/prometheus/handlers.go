package prometheus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/paffin/opsmate/pkg/mcputil"
	"github.com/mark3labs/mcp-go/mcp"
)

type handlers struct {
	baseURL  string
	client   *http.Client
	username string
	password string
}

func (h *handlers) doGet(ctx context.Context, path string, params url.Values) ([]byte, error) {
	u := fmt.Sprintf("%s%s", h.baseURL, path)
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	if h.username != "" {
		req.SetBasicAuth(h.username, h.password)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

type promResponse struct {
	Status string          `json:"status"`
	Data   json.RawMessage `json:"data"`
	Error  string          `json:"error,omitempty"`
}

type queryData struct {
	ResultType string            `json:"resultType"`
	Result     []json.RawMessage `json:"result"`
}

func (h *handlers) query(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := mcputil.RequireString(req, "query")
	if err != nil {
		return mcputil.ToolError("query is required")
	}

	params := url.Values{"query": {query}}
	body, err := h.doGet(ctx, "/api/v1/query", params)
	if err != nil {
		return mcputil.ToolError("prometheus query failed: %v", err)
	}

	var resp promResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return mcputil.ToolError("failed to parse response: %v", err)
	}
	if resp.Status != "success" {
		return mcputil.ToolError("query error: %s", resp.Error)
	}

	return mcputil.ToolText("Query: %s\n\n%s", query, formatQueryResult(resp.Data))
}

func (h *handlers) queryRange(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := mcputil.RequireString(req, "query")
	if err != nil {
		return mcputil.ToolError("query is required")
	}

	now := time.Now()
	start := mcputil.OptionalString(req, "start", now.Add(-1*time.Hour).Format(time.RFC3339))
	end := mcputil.OptionalString(req, "end", now.Format(time.RFC3339))
	step := mcputil.OptionalString(req, "step", "60s")

	// Handle relative times
	start = resolveTime(start, now)
	end = resolveTime(end, now)

	params := url.Values{
		"query": {query},
		"start": {start},
		"end":   {end},
		"step":  {step},
	}

	body, err := h.doGet(ctx, "/api/v1/query_range", params)
	if err != nil {
		return mcputil.ToolError("prometheus range query failed: %v", err)
	}

	var resp promResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return mcputil.ToolError("failed to parse response: %v", err)
	}
	if resp.Status != "success" {
		return mcputil.ToolError("query error: %s", resp.Error)
	}

	return mcputil.ToolText("Range Query: %s\nPeriod: %s to %s (step: %s)\n\n%s",
		query, start, end, step, formatQueryResult(resp.Data))
}

func (h *handlers) alerts(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filter := mcputil.OptionalString(req, "filter", "")

	body, err := h.doGet(ctx, "/api/v1/alerts", nil)
	if err != nil {
		return mcputil.ToolError("failed to get alerts: %v", err)
	}

	var resp promResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return mcputil.ToolError("failed to parse response: %v", err)
	}

	var alertsData struct {
		Alerts []struct {
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
			State       string            `json:"state"`
			ActiveAt    string            `json:"activeAt"`
			Value       string            `json:"value"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal(resp.Data, &alertsData); err != nil {
		return mcputil.ToolError("failed to parse alerts: %v", err)
	}

	var sb strings.Builder
	count := 0
	for _, alert := range alertsData.Alerts {
		name := alert.Labels["alertname"]
		if filter != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(filter)) {
			continue
		}
		count++
		icon := "🔴"
		if alert.State == "pending" {
			icon = "🟡"
		}
		_, _ = fmt.Fprintf(&sb, "%s %s [%s]\n", icon, name, alert.State)
		_, _ = fmt.Fprintf(&sb, "  Active since: %s\n", alert.ActiveAt)
		if summary, ok := alert.Annotations["summary"]; ok {
			_, _ = fmt.Fprintf(&sb, "  Summary: %s\n", summary)
		}
		sb.WriteString("\n")
	}

	if count == 0 {
		return mcputil.ToolText("No active alerts found.")
	}

	return mcputil.ToolText("%d active alerts:\n\n%s", count, sb.String())
}

func (h *handlers) targets(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state := mcputil.OptionalString(req, "state", "active")

	params := url.Values{}
	if state != "any" {
		params.Set("state", state)
	}

	body, err := h.doGet(ctx, "/api/v1/targets", params)
	if err != nil {
		return mcputil.ToolError("failed to get targets: %v", err)
	}

	var resp promResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return mcputil.ToolError("failed to parse response: %v", err)
	}

	var targetsData struct {
		ActiveTargets []struct {
			Labels       map[string]string `json:"labels"`
			ScrapeURL    string            `json:"scrapeUrl"`
			Health       string            `json:"health"`
			LastScrape   string            `json:"lastScrape"`
			LastError    string            `json:"lastError"`
		} `json:"activeTargets"`
	}
	if err := json.Unmarshal(resp.Data, &targetsData); err != nil {
		return mcputil.ToolError("failed to parse targets: %v", err)
	}

	var sb strings.Builder
	for _, t := range targetsData.ActiveTargets {
		icon := "✓"
		if t.Health != "up" {
			icon = "✗"
		}
		job := t.Labels["job"]
		_, _ = fmt.Fprintf(&sb, "%s %s (%s) — %s\n", icon, job, t.ScrapeURL, t.Health)
		if t.LastError != "" {
			_, _ = fmt.Fprintf(&sb, "  Error: %s\n", t.LastError)
		}
	}

	return mcputil.ToolText("%d targets:\n\n%s", len(targetsData.ActiveTargets), sb.String())
}

func (h *handlers) rules(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ruleType := mcputil.OptionalString(req, "type", "all")

	params := url.Values{}
	if ruleType != "all" {
		params.Set("type", ruleType)
	}

	body, err := h.doGet(ctx, "/api/v1/rules", params)
	if err != nil {
		return mcputil.ToolError("failed to get rules: %v", err)
	}

	var resp promResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return mcputil.ToolError("failed to parse response: %v", err)
	}

	return mcputil.ToolText("Rules (type: %s):\n\n%s", ruleType, formatJSON(resp.Data))
}

func (h *handlers) series(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	match, err := mcputil.RequireString(req, "match")
	if err != nil {
		return mcputil.ToolError("match selector is required")
	}

	params := url.Values{"match[]": {match}}
	body, err := h.doGet(ctx, "/api/v1/series", params)
	if err != nil {
		return mcputil.ToolError("failed to get series: %v", err)
	}

	var resp promResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return mcputil.ToolError("failed to parse response: %v", err)
	}

	var seriesList []map[string]string
	if err := json.Unmarshal(resp.Data, &seriesList); err != nil {
		return mcputil.ToolError("failed to parse series: %v", err)
	}

	var sb strings.Builder
	for _, s := range seriesList {
		parts := []string{}
		for k, v := range s {
			if k != "__name__" {
				parts = append(parts, fmt.Sprintf("%s=%q", k, v))
			}
		}
		name := s["__name__"]
		_, _ = fmt.Fprintf(&sb, "%s{%s}\n", name, strings.Join(parts, ", "))
	}

	return mcputil.ToolText("%d series matching %q:\n\n%s", len(seriesList), match, sb.String())
}

func (h *handlers) labelValues(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	labelName, err := mcputil.RequireString(req, "label_name")
	if err != nil {
		return mcputil.ToolError("label_name is required")
	}

	path := fmt.Sprintf("/api/v1/label/%s/values", labelName)
	params := url.Values{}
	if match := mcputil.OptionalString(req, "match", ""); match != "" {
		params.Set("match[]", match)
	}

	body, err := h.doGet(ctx, path, params)
	if err != nil {
		return mcputil.ToolError("failed to get label values: %v", err)
	}

	var resp promResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return mcputil.ToolError("failed to parse response: %v", err)
	}

	var values []string
	if err := json.Unmarshal(resp.Data, &values); err != nil {
		return mcputil.ToolError("failed to parse values: %v", err)
	}

	return mcputil.ToolText("Label %q has %d values:\n\n%s", labelName, len(values), strings.Join(values, "\n"))
}

func formatQueryResult(data json.RawMessage) string {
	var qd queryData
	if err := json.Unmarshal(data, &qd); err != nil {
		return formatJSON(data)
	}

	var sb strings.Builder
	_, _ = fmt.Fprintf(&sb, "Result type: %s\n\n", qd.ResultType)

	for _, r := range qd.Result {
		var metric struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
			Values [][]interface{}   `json:"values"`
		}
		if err := json.Unmarshal(r, &metric); err != nil {
			sb.WriteString(string(r))
			sb.WriteString("\n")
			continue
		}

		// Format metric name
		labels := []string{}
		name := metric.Metric["__name__"]
		for k, v := range metric.Metric {
			if k != "__name__" {
				labels = append(labels, fmt.Sprintf("%s=%q", k, v))
			}
		}
		_, _ = fmt.Fprintf(&sb, "%s{%s}", name, strings.Join(labels, ", "))

		if metric.Value != nil && len(metric.Value) == 2 {
			_, _ = fmt.Fprintf(&sb, " => %v\n", metric.Value[1])
		} else if metric.Values != nil {
			sb.WriteString(":\n")
			for _, v := range metric.Values {
				if len(v) == 2 {
					ts, _ := v[0].(float64)
					t := time.Unix(int64(ts), 0).Format("15:04:05")
					_, _ = fmt.Fprintf(&sb, "  %s: %v\n", t, v[1])
				}
			}
		}
	}

	return sb.String()
}

func formatJSON(data json.RawMessage) string {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err != nil {
		return string(data)
	}
	return pretty.String()
}

func resolveTime(t string, now time.Time) string {
	if strings.HasPrefix(t, "-") {
		d, err := time.ParseDuration(t[1:])
		if err == nil {
			return now.Add(-d).Format(time.RFC3339)
		}
	}
	return t
}
