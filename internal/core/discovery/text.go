package discovery

import "strings"

// CompactSemanticText normalizes whitespace and bounds semantic evidence by rune count.
func CompactSemanticText(value string, maxRunes int) string {
	clean := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if maxRunes <= 0 || len([]rune(clean)) <= maxRunes {
		return clean
	}
	return strings.TrimSpace(string([]rune(clean)[:maxRunes]))
}
