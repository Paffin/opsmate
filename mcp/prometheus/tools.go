package prometheus

import "github.com/mark3labs/mcp-go/mcp"

func toolQuery() mcp.Tool {
	return mcp.NewTool("prom_query",
		mcp.WithDescription("Execute an instant PromQL query"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("PromQL query expression"),
		),
	)
}

func toolQueryRange() mcp.Tool {
	return mcp.NewTool("prom_query_range",
		mcp.WithDescription("Execute a range PromQL query over a time period"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("PromQL query expression"),
		),
		mcp.WithString("start",
			mcp.Description("Start time (RFC3339 or relative like -1h). Default: -1h"),
		),
		mcp.WithString("end",
			mcp.Description("End time (RFC3339 or relative). Default: now"),
		),
		mcp.WithString("step",
			mcp.Description("Query resolution step (e.g. 15s, 1m). Default: 60s"),
		),
	)
}

func toolAlerts() mcp.Tool {
	return mcp.NewTool("prom_alerts",
		mcp.WithDescription("Get currently firing alerts"),
		mcp.WithString("filter",
			mcp.Description("Filter alerts by name or label (optional)"),
		),
	)
}

func toolTargets() mcp.Tool {
	return mcp.NewTool("prom_targets",
		mcp.WithDescription("Get scrape targets and their status"),
		mcp.WithString("state",
			mcp.Description("Filter by state: active, dropped, any (default: active)"),
		),
	)
}

func toolRules() mcp.Tool {
	return mcp.NewTool("prom_rules",
		mcp.WithDescription("Get alerting and recording rules"),
		mcp.WithString("type",
			mcp.Description("Rule type: alert, record, or all (default: all)"),
		),
	)
}

func toolSeries() mcp.Tool {
	return mcp.NewTool("prom_series",
		mcp.WithDescription("Find time series matching label selectors"),
		mcp.WithString("match",
			mcp.Required(),
			mcp.Description("Series selector (e.g. up, {job=\"prometheus\"})"),
		),
	)
}

func toolLabelValues() mcp.Tool {
	return mcp.NewTool("prom_label_values",
		mcp.WithDescription("Get values for a specific label"),
		mcp.WithString("label_name",
			mcp.Required(),
			mcp.Description("Label name to get values for"),
		),
		mcp.WithString("match",
			mcp.Description("Optional series selector to scope the label values"),
		),
	)
}
