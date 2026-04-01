package service

import "strings"

func applySubordinateFragmentDemotion(ranked []rankedSegment) []rankedSegment {
	if len(ranked) < 2 {
		return ranked
	}
	anchorIndex, anchorWeight := strongestExplanatoryAnchor(ranked)
	if anchorIndex < 0 || anchorWeight < 0.70 || ranked[anchorIndex].chunk.Score < 0.82 {
		return ranked
	}
	for i := range ranked {
		if i == anchorIndex {
			continue
		}
		fragmentWeight := subordinateFragmentWeight(ranked[i])
		if fragmentWeight < 0.60 {
			continue
		}
		penalty := 0.18 + (0.12 * fragmentWeight)
		ranked[i].chunk.Score = clamp(ranked[i].chunk.Score-penalty, 0, 1)
		ranked[i].chunk.Confidence = clamp(ranked[i].chunk.Confidence-(0.05*fragmentWeight), 0, 0.99)
	}
	sortRankedSegments(ranked)
	return ranked
}

func subordinateFragmentWeight(item rankedSegment) float64 {
	text := strings.TrimSpace(item.segment.Text)
	if text == "" {
		return 0
	}
	runeLen := len([]rune(text))
	if runeLen < 60 || runeLen > 180 {
		return 0
	}
	weight := 0.0
	if len(item.segment.HeadingPath) >= 3 {
		weight += 0.40
	} else if len(item.segment.HeadingPath) == 2 {
		weight += 0.18
	}
	if len(item.segment.NodePaths) <= 2 {
		weight += 0.25
	} else if len(item.segment.NodePaths) <= 4 {
		weight += 0.12
	}
	if averageLineLength(text) >= 70 {
		weight += 0.15
	}
	if sentencePunctuationCount(text) <= 2 {
		weight += 0.10
	}
	if !item.ir.headingBacked && len(item.segment.HeadingPath) >= 3 {
		weight += 0.10
	}
	if weight > 1 {
		return 1
	}
	return weight
}
