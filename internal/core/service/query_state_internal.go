package service

import (
	"net/url"
	"slices"
	"strings"
)

func mergeDomainHints(existing []string, incoming ...string) []string {
	out := append([]string{}, existing...)
	for _, hint := range incoming {
		host := strings.TrimSpace(strings.ToLower(hint))
		if host == "" {
			continue
		}
		if parsed, err := url.Parse(host); err == nil && strings.TrimSpace(parsed.Hostname()) != "" {
			host = strings.ToLower(strings.TrimSpace(parsed.Hostname()))
		}
		if slices.Contains(out, host) {
			continue
		}
		out = append(out, host)
	}
	return out
}

func hostFromURLString(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.ToLower(parsed.Hostname()))
}
