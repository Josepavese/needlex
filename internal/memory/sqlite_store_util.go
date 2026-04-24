package memory

import (
	"encoding/json"
	"strings"
	"time"
)

func mustJSON(values []string) string {
	data, _ := json.Marshal(compactStrings(values))
	return string(data)
}

func decodeStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return compactStrings(out)
}

func compactStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func compactURLs(values []string) []string {
	return compactStrings(values)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func normalizeDomainHints(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range compactStrings(values) {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func hasDomainHint(host string, hints []string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	for _, hint := range hints {
		if hint == host {
			return true
		}
		if strings.HasSuffix(host, "."+hint) {
			return true
		}
	}
	return false
}

func parseObservedAt(raw string) (time.Time, bool) {
	value, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw))
	if err != nil || value.IsZero() {
		return time.Time{}, false
	}
	return value, true
}

func parseObservedAtOrZero(raw string) time.Time {
	value, _ := parseObservedAt(raw)
	return value
}

func recentObservationBoost(observedAt time.Time) float64 {
	if observedAt.IsZero() {
		return 0
	}
	age := time.Since(observedAt)
	switch {
	case age <= 24*time.Hour:
		return 0.12
	case age <= 7*24*time.Hour:
		return 0.08
	case age <= 30*24*time.Hour:
		return 0.04
	default:
		return 0
	}
}
