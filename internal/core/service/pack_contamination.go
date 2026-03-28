package service

import (
	"slices"
	"strings"
)

var contaminationTerms = []string{
	"casino",
	"casinò",
	"aams",
	"22bet",
	"gambling",
	"scommesse",
	"bookmaker",
	"payout",
}

func applyContaminationPenalty(ranked []rankedSegment, objective string) []rankedSegment {
	if contaminationContextAllowed(objective) {
		return ranked
	}
	for i := range ranked {
		weight := contaminationWeight(ranked[i].chunk.Text)
		if weight == 0 {
			continue
		}
		ranked[i].chunk.Score = clamp(ranked[i].chunk.Score-(0.45*weight), 0, 1)
		ranked[i].chunk.Confidence = clamp(ranked[i].chunk.Confidence-(0.20*weight), 0, 0.99)
	}

	slices.SortStableFunc(ranked, func(a, b rankedSegment) int {
		switch {
		case a.chunk.Score > b.chunk.Score:
			return -1
		case a.chunk.Score < b.chunk.Score:
			return 1
		case a.index < b.index:
			return -1
		case a.index > b.index:
			return 1
		default:
			return 0
		}
	})
	return ranked
}

func contaminationRiskFlags(text, objective string) []string {
	if contaminationContextAllowed(objective) {
		return nil
	}
	if contaminationWeight(text) == 0 {
		return nil
	}
	return []string{"contamination_risk"}
}

func contaminationContextAllowed(objective string) bool {
	obj := strings.ToLower(objective)
	return strings.Contains(obj, "casino") ||
		strings.Contains(obj, "gambling") ||
		strings.Contains(obj, "bet") ||
		strings.Contains(obj, "scommesse")
}

func contaminationWeight(text string) float64 {
	lower := strings.ToLower(text)
	hits := 0
	for _, term := range contaminationTerms {
		if strings.Contains(lower, term) {
			hits++
		}
	}
	if hits == 0 {
		return 0
	}
	if hits > 3 {
		hits = 3
	}
	return float64(hits) / 3.0
}
