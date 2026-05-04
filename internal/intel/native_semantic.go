package intel

import (
	"context"
	"math"
	"slices"
	"strings"
	"unicode"
)

type NativeSemanticAligner struct{}

func (a NativeSemanticAligner) Score(ctx context.Context, objective string, candidates []SemanticCandidate) ([]SemanticScore, error) {
	if strings.TrimSpace(objective) == "" || len(candidates) == 0 {
		return nil, nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	objectiveVec := nativeSemanticVector(objective)
	if len(objectiveVec) == 0 {
		return nil, nil
	}
	scores := make([]SemanticScore, 0, len(candidates))
	for _, candidate := range candidates {
		candidateVec := nativeSemanticVector(semanticCandidateText(candidate))
		scores = append(scores, SemanticScore{
			ID:         candidate.ID,
			Similarity: sparseCosine(objectiveVec, candidateVec),
		})
	}
	return scores, nil
}

func nativeSemanticVector(text string) map[string]float64 {
	normalized := normalizeNativeSemanticText(text)
	if normalized == "" {
		return nil
	}
	vec := map[string]float64{}
	addNativeCharNGrams(vec, normalized, 3, 1.0)
	addNativeCharNGrams(vec, normalized, 4, 1.15)
	for _, token := range strings.Fields(normalized) {
		if len([]rune(token)) < 2 {
			continue
		}
		vec["tok:"+token] += 0.70
	}
	return vec
}

func normalizeNativeSemanticText(text string) string {
	fields := make([]string, 0)
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		fields = append(fields, string(current))
		current = current[:0]
	}
	for _, r := range strings.ToLower(strings.TrimSpace(text)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			current = append(current, r)
		default:
			flush()
		}
	}
	flush()
	slices.Sort(fields)
	return strings.Join(fields, " ")
}

func addNativeCharNGrams(vec map[string]float64, text string, size int, weight float64) {
	runes := []rune(" " + text + " ")
	if len(runes) < size {
		return
	}
	for i := 0; i <= len(runes)-size; i++ {
		gram := string(runes[i : i+size])
		if strings.TrimSpace(gram) == "" {
			continue
		}
		vec["chr:"+gram] += weight
	}
}

func sparseCosine(left, right map[string]float64) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	dot := 0.0
	normLeft := 0.0
	normRight := 0.0
	for key, leftValue := range left {
		dot += leftValue * right[key]
		normLeft += leftValue * leftValue
	}
	for _, rightValue := range right {
		normRight += rightValue * rightValue
	}
	if normLeft == 0 || normRight == 0 {
		return 0
	}
	return dot / (math.Sqrt(normLeft) * math.Sqrt(normRight))
}
