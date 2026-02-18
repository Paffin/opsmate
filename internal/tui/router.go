package tui

import (
	"strings"
	"unicode/utf8"
)

// Model tiers for routing.
const (
	ModelFast    = "claude-haiku-4-5-20251001"
	ModelDefault = "claude-sonnet-4-20250514"
	ModelDeep    = "claude-opus-4-20250514"
)

// RouteModel selects an appropriate Claude model based on query complexity.
// Returns empty string to use whatever default is configured.
func RouteModel(query string) string {
	lower := strings.ToLower(query)
	charCount := utf8.RuneCountInString(query)

	// Deep: complex analytical/architectural queries
	if isDeepQuery(lower) {
		return ModelDeep
	}

	// Fast: short, simple status/list queries
	if charCount < 80 && isSimpleQuery(lower) {
		return ModelFast
	}

	// Standard for everything else
	return ModelDefault
}

func isDeepQuery(q string) bool {
	deepKeywords := []string{
		"analyze", "debug", "architecture", "refactor",
		"design", "optimize", "security", "audit",
		"explain in detail", "root cause", "investigate",
		"performance", "migration", "compare",
		"анализ", "архитектур", "рефактор", "отлад",
		"оптимиз", "безопасност", "миграц",
	}
	for _, kw := range deepKeywords {
		if strings.Contains(q, kw) {
			return true
		}
	}
	// Long queries with multiple sentences tend to be complex
	if utf8.RuneCountInString(q) > 300 {
		return true
	}
	return false
}

func isSimpleQuery(q string) bool {
	simplePatterns := []string{
		"list", "show", "what is", "status",
		"check", "get", "describe", "how many",
		"version", "help", "ping", "uptime",
		"покажи", "список", "статус", "что запущено",
		"сколько", "версия", "проверь",
	}
	for _, p := range simplePatterns {
		if strings.Contains(q, p) {
			return true
		}
	}
	return false
}
