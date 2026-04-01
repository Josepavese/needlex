package service

import "strings"

func applyIndexLikeDemotion(ranked []rankedSegment) []rankedSegment {
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
		indexWeight := indexLikeWeight(ranked[i])
		if indexWeight < 0.55 {
			continue
		}
		if ranked[i].chunk.ContextAlignment >= anchorAlignment+0.08 {
			continue
		}
		penalty := 0.16 + (0.14 * indexWeight)
		ranked[i].chunk.Score = clamp(ranked[i].chunk.Score-penalty, 0, 1)
		ranked[i].chunk.Confidence = clamp(ranked[i].chunk.Confidence-(0.05*indexWeight), 0, 0.99)
	}
	sortRankedSegments(ranked)
	return ranked
}

func strongestExplanatoryAnchor(ranked []rankedSegment) (int, float64) {
	bestIndex := -1
	bestWeight := 0.0
	bestScore := 0.0
	for i, item := range ranked {
		weight := explanatoryAnchorWeight(item)
		if weight < 0.60 {
			continue
		}
		if bestIndex < 0 || weight > bestWeight || (weight == bestWeight && item.chunk.Score > bestScore) {
			bestIndex = i
			bestWeight = weight
			bestScore = item.chunk.Score
		}
	}
	return bestIndex, bestWeight
}

func explanatoryAnchorWeight(item rankedSegment) float64 {
	text := strings.TrimSpace(item.segment.Text)
	if text == "" {
		return 0
	}
	weight := 0.0
	if len(text) >= 140 {
		weight += 0.35
	}
	if sentencePunctuationCount(text) >= 2 {
		weight += 0.30
	}
	if len(item.segment.NodePaths) <= 4 {
		weight += 0.15
	}
	if len(item.segment.HeadingPath) > 0 {
		weight += 0.10
	}
	if averageLineLength(text) >= 48 {
		weight += 0.15
	}
	if weight > 1 {
		return 1
	}
	return weight
}

func indexLikeWeight(item rankedSegment) float64 {
	text := strings.TrimSpace(item.segment.Text)
	if text == "" || len(text) > 1600 {
		return 0
	}
	weight := 0.0
	if len(item.segment.NodePaths) >= 8 {
		weight += 0.35
	} else if len(item.segment.NodePaths) >= 5 {
		weight += 0.20
	}
	if shortEntryRatio(text) >= 0.65 {
		weight += 0.35
	}
	if sentencePunctuationCount(text) == 0 {
		weight += 0.15
	}
	if averageLineLength(text) <= 28 {
		weight += 0.15
	}
	if len(item.segment.HeadingPath) <= 2 {
		weight += 0.05
	}
	if weight > 1 {
		return 1
	}
	return weight
}

func shortEntryRatio(text string) float64 {
	lines := strings.Split(text, "\n")
	entries := 0
	shortEntries := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		entries++
		if len([]rune(line)) <= 32 {
			shortEntries++
		}
	}
	if entries == 0 {
		return 0
	}
	return float64(shortEntries) / float64(entries)
}

func averageLineLength(text string) float64 {
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
