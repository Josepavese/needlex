package service

import "strings"

func applyBoilerplatePenalty(ranked []rankedSegment) []rankedSegment {
	for i := range ranked {
		weight := boilerplateWeight(ranked[i])
		if weight == 0 {
			continue
		}
		ranked[i].chunk.Score = clamp(ranked[i].chunk.Score-(0.28*weight), 0, 1)
		ranked[i].chunk.Confidence = clamp(ranked[i].chunk.Confidence-(0.10*weight), 0, 0.99)
	}
	sortRankedSegments(ranked)
	return ranked
}

func boilerplateWeight(item rankedSegment) float64 {
	text := strings.TrimSpace(item.segment.Text)
	if text == "" || len(text) > 220 {
		return 0
	}
	tokens := strings.Fields(text)
	if len(tokens) < 5 {
		return 0
	}
	if sentencePunctuationCount(text) > 0 {
		return 0
	}
	weight := 0.0
	if len(item.segment.HeadingPath) >= 2 {
		return 0
	}
	if len(item.segment.NodePaths) >= 4 {
		weight += 0.55
	}
	if averageTokenLength(tokens) <= 9 {
		weight += 0.25
	}
	if titleLikeTokenRatio(tokens) >= 0.7 {
		weight += 0.20
	}
	if weight > 1 {
		return 1
	}
	return weight
}

func sentencePunctuationCount(text string) int {
	return strings.Count(text, ".") + strings.Count(text, "!") + strings.Count(text, "?") + strings.Count(text, ";") + strings.Count(text, ":")
}

func averageTokenLength(tokens []string) float64 {
	if len(tokens) == 0 {
		return 0
	}
	total := 0
	for _, token := range tokens {
		total += len(strings.TrimSpace(token))
	}
	return float64(total) / float64(len(tokens))
}

func titleLikeTokenRatio(tokens []string) float64 {
	if len(tokens) == 0 {
		return 0
	}
	titleLike := 0
	for _, token := range tokens {
		token = strings.Trim(token, "[](){}<>\"'`,")
		if token == "" {
			continue
		}
		runes := []rune(token)
		if len(runes) == 1 {
			continue
		}
		first := runes[0]
		if first >= 'A' && first <= 'Z' {
			titleLike++
			continue
		}
		if isASCIIUpper(token) {
			titleLike++
		}
	}
	return float64(titleLike) / float64(len(tokens))
}

func isASCIIUpper(text string) bool {
	hasLetter := false
	for _, r := range text {
		switch {
		case r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= 'a' && r <= 'z':
			return false
		}
	}
	return hasLetter
}
