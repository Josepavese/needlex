package service

import (
	"strings"

	"github.com/josepavese/needlex/internal/pipeline"
)

func segmentScore(segment pipeline.Segment, objective string, index, total int, irEvidence segmentIREvidence) float64 {
	kindWeight := map[string]float64{
		"paragraph":  0.80,
		"list_item":  0.74,
		"table_cell": 0.70,
		"code":       0.68,
	}
	base := kindWeight[segment.Kind]
	if base == 0 {
		base = 0.63
	}
	if len(segment.HeadingPath) > 0 {
		base += 0.05
	}
	if total > 1 {
		base += float64(total-index-1) / float64(total-1) * 0.08
	}
	base += objectiveCoverage(segment, objective) * 0.18
	if len(segment.Text) > 60 {
		base += 0.03
	}
	base += irEvidence.scoreBoost()
	return clamp(base, 0, 1)
}

func (e segmentIREvidence) scoreBoost() float64 {
	boost := 0.0
	if e.kindMatch {
		boost += 0.05
	}
	if e.headingBacked {
		boost += 0.03
	}
	if e.embedded {
		boost += 0.04
	}
	if e.averageNodeDepth > 0 && e.averageNodeDepth <= 4 {
		boost += 0.02
	}
	return boost
}

func segmentConfidence(segment pipeline.Segment, objective string) float64 {
	confidence := 0.75
	if len(segment.NodePaths) > 0 {
		confidence += 0.10
	}
	if len(segment.HeadingPath) > 0 {
		confidence += 0.04
	}
	if objectiveCoverage(segment, objective) > 0 {
		confidence += 0.06
	}
	if len(segment.Text) > 40 {
		confidence += 0.04
	}
	return clamp(confidence, 0, 0.99)
}

func objectiveCoverage(segment pipeline.Segment, objective string) float64 {
	tokens := objectiveTokens(objective)
	if len(tokens) == 0 {
		return 0
	}

	haystack := strings.ToLower(strings.Join(segment.HeadingPath, " ") + " " + segment.Text)
	matches := 0
	for _, token := range tokens {
		if strings.Contains(haystack, token) {
			matches++
		}
	}
	return float64(matches) / float64(len(tokens))
}

func objectiveTokens(objective string) []string {
	fields := strings.Fields(strings.ToLower(objective))
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		token := strings.Trim(field, " \t\r\n,.;:!?()[]{}<>\"'")
		if len(token) < 3 {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	return out
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
