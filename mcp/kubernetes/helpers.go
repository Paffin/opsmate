package kubernetes

import (
	"fmt"
	"strings"
	"time"
)

func formatAge(t time.Time) string {
	if t.IsZero() {
		return "<unknown>"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

// derefInt32 safely dereferences an *int32, returning 0 if nil.
func derefInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

// pluralizeKind converts a Kubernetes Kind name to its plural resource name.
// Handles standard English pluralization rules that match K8s API conventions.
func pluralizeKind(kind string) string {
	lower := strings.ToLower(kind)
	switch {
	case strings.HasSuffix(lower, "ss"), // Ingress -> ingresses
		strings.HasSuffix(lower, "sh"),
		strings.HasSuffix(lower, "ch"),
		strings.HasSuffix(lower, "x"),
		strings.HasSuffix(lower, "z"):
		return lower + "es"
	case strings.HasSuffix(lower, "y") && len(lower) > 1 &&
		!strings.ContainsAny(string(lower[len(lower)-2]), "aeiou"):
		// NetworkPolicy -> networkpolicies
		return lower[:len(lower)-1] + "ies"
	default:
		return lower + "s"
	}
}

