package service

import "strings"

var tinyStopwords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {}, "before": {},
	"by": {}, "for": {}, "from": {}, "in": {}, "into": {}, "is": {}, "it": {}, "of": {},
	"on": {}, "or": {}, "that": {}, "the": {}, "their": {}, "to": {}, "when": {}, "with": {},
	"without": {},
}

func compactTinyText(text, objective string) (string, bool) {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if normalized == "" {
		return "", false
	}

	candidate := bestCompactSentence(normalized, objective)
	words := strings.Fields(candidate)
	if len(words) == 0 {
		return normalized, false
	}

	compactedWords := make([]string, 0, len(words))
	for _, word := range words {
		clean := trimWord(word)
		if clean == "" {
			continue
		}
		if _, ok := tinyStopwords[strings.ToLower(clean)]; ok && len(words) > 8 {
			continue
		}
		compactedWords = append(compactedWords, clean)
		if len(compactedWords) == 14 {
			break
		}
	}

	if len(compactedWords) < 6 {
		compactedWords = fallbackWords(words, 10)
	}

	compacted := strings.Join(compactedWords, " ")
	if compacted == "" {
		compacted = normalized
	}
	if !strings.HasSuffix(compacted, ".") {
		compacted += "."
	}
	return compacted, compacted != normalized
}

func bestCompactSentence(text, objective string) string {
	sentences := splitTinySentences(text)
	if len(sentences) == 0 {
		return text
	}
	if strings.TrimSpace(objective) == "" {
		return sentences[0]
	}

	tokens := uniqueTokens(objective)
	best := sentences[0]
	bestScore := -1
	for _, sentence := range sentences {
		score := 0
		haystack := strings.ToLower(sentence)
		for _, token := range tokens {
			if strings.Contains(haystack, token) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			best = sentence
		}
	}
	return best
}

func splitTinySentences(text string) []string {
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func trimWord(word string) string {
	return strings.Trim(word, " \t\r\n,;:!?()[]{}\"'")
}

func fallbackWords(words []string, limit int) []string {
	out := make([]string, 0, min(len(words), limit))
	for _, word := range words {
		clean := trimWord(word)
		if clean == "" {
			continue
		}
		out = append(out, clean)
		if len(out) == limit {
			break
		}
	}
	return out
}
