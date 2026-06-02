package core

import "strings"

func WebIRUtilityReasons(webIR WebIR) []string {
	reasons := []string{}
	if webIR.NodeCount <= 1 {
		reasons = appendUniqueReason(reasons, "low_node_count")
	}
	reducedChars := 0
	navLike := 0
	for _, node := range webIR.Nodes {
		text := strings.TrimSpace(node.Text)
		reducedChars += len([]rune(text))
		if isNavigationLikeSurface(text) {
			navLike++
		}
	}
	if reducedChars > 0 && reducedChars < 180 {
		reasons = appendUniqueReason(reasons, "low_reduced_chars")
	}
	if len(webIR.Nodes) > 0 && navLike == len(webIR.Nodes) {
		reasons = appendUniqueReason(reasons, "navigation_like_surface")
	}
	if webIR.Signals.SubstrateClass == "client_rendered_app" {
		reasons = appendUniqueReason(reasons, "client_rendered_app_surface")
	}
	return reasons
}

func appendUniqueReason(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func isNavigationLikeSurface(text string) bool {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(fields) == 0 {
		return false
	}
	if len(fields) > 18 {
		return false
	}
	navTokens := 0
	for _, field := range fields {
		field = strings.Trim(field, ".,:;!?()[]{}|/")
		switch field {
		case "search", "ask", "ai", "help", "center", "status", "sign", "in", "login", "guides", "api", "reference", "changelog", "docs", "home", "pricing", "contact", "blog", "menu":
			navTokens++
		}
	}
	return float64(navTokens)/float64(len(fields)) >= 0.55
}
