package core

import "strings"

// AverageLineLength counts runes on nonempty, trimmed lines.
func AverageLineLength(text string) float64 {
	lines := strings.Split(text, "\n")
	count := 0
	total := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		count++
		total += len([]rune(line))
	}
	if count == 0 {
		return 0
	}
	return float64(total) / float64(count)
}

// SentencePunctuationCount counts punctuation used by structural text policies.
func SentencePunctuationCount(text string) int {
	return strings.Count(text, ".") + strings.Count(text, "!") + strings.Count(text, "?") + strings.Count(text, ";") + strings.Count(text, ":")
}
