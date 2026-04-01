package service

import "strings"

func applyCodeLikeDemotion(ranked []rankedSegment) []rankedSegment {
	if len(ranked) < 2 {
		return ranked
	}
	anchorIndex, anchorWeight := strongestExplanatoryAnchor(ranked)
	if anchorIndex < 0 || anchorWeight < 0.70 || ranked[anchorIndex].chunk.Score < 0.82 {
		return ranked
	}
	anchorAlignment := ranked[anchorIndex].chunk.ContextAlignment
	for i := range ranked {
		if i == anchorIndex {
			continue
		}
		codeWeight := codeLikeWeight(ranked[i])
		if codeWeight < 0.55 {
			continue
		}
		if ranked[i].chunk.ContextAlignment >= anchorAlignment+0.08 {
			continue
		}
		penalty := 0.18 + (0.16 * codeWeight)
		ranked[i].chunk.Score = clamp(ranked[i].chunk.Score-penalty, 0, 1)
		ranked[i].chunk.Confidence = clamp(ranked[i].chunk.Confidence-(0.05*codeWeight), 0, 0.99)
	}
	sortRankedSegments(ranked)
	return ranked
}

func codeLikeWeight(item rankedSegment) float64 {
	text := strings.TrimSpace(item.segment.Text)
	if text == "" || len(text) > 420 {
		return 0
	}
	weight := 0.0
	if identifierDenseRatio(text) >= 0.45 {
		weight += 0.40
	}
	if symbolicLineRatio(text) >= 0.50 {
		weight += 0.25
	}
	if sentencePunctuationCount(text) == 0 {
		weight += 0.10
	}
	if averageLineLength(text) <= 36 {
		weight += 0.10
	}
	if len(item.segment.NodePaths) <= 4 {
		weight += 0.05
	}
	if weight > 1 {
		return 1
	}
	return weight
}

func identifierDenseRatio(text string) float64 {
	lines := strings.Split(text, "\n")
	entries := 0
	identifierLike := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		entries++
		if looksLikeIdentifierLine(line) {
			identifierLike++
		}
	}
	if entries == 0 {
		return 0
	}
	return float64(identifierLike) / float64(entries)
}

func symbolicLineRatio(text string) float64 {
	lines := strings.Split(text, "\n")
	entries := 0
	symbolic := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		entries++
		if strings.ContainsAny(line, "_=:`{}[]();\\") {
			symbolic++
		}
	}
	if entries == 0 {
		return 0
	}
	return float64(symbolic) / float64(entries)
}

func looksLikeIdentifierLine(line string) bool {
	if line == "" {
		return false
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false
	}
	identifierish := 0
	for _, field := range fields {
		field = strings.Trim(field, "\"'`,")
		if field == "" {
			continue
		}
		if strings.Contains(field, "_") || isASCIIUpper(field) || strings.Count(field, "-") >= 1 || hasDigit(field) {
			identifierish++
		}
	}
	return float64(identifierish)/float64(len(fields)) >= 0.5
}

func hasDigit(text string) bool {
	for _, r := range text {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}
