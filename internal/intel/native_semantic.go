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

	dimensions := 384
	objectiveVec := nativeTextEmbedding(objective, dimensions)
	if zeroFloat32Vector(objectiveVec) {
		return nil, nil
	}
	scores := make([]SemanticScore, 0, len(candidates))
	for _, candidate := range candidates {
		candidateVec := nativeTextEmbedding(semanticCandidateText(candidate), dimensions)
		scores = append(scores, SemanticScore{
			ID:         candidate.ID,
			Similarity: cosineSimilarityFloat32(objectiveVec, candidateVec),
		})
	}
	return scores, nil
}

func nativeEmbeddingFeatures(text string) map[string]float64 {
	normalized := normalizeNativeEmbeddingText(text)
	if normalized == "" {
		return nil
	}
	vec := map[string]float64{}
	addNativeCharNGrams(vec, normalized, 3, 1.0)
	addNativeCharNGrams(vec, normalized, 4, 1.15)
	return vec
}

func normalizeNativeEmbeddingText(text string) string {
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

func zeroFloat32Vector(vector []float32) bool {
	for _, value := range vector {
		if value != 0 {
			return false
		}
	}
	return true
}

func cosineSimilarityFloat32(left, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	dot := 0.0
	normLeft := 0.0
	normRight := 0.0
	for i := range left {
		leftValue := float64(left[i])
		rightValue := float64(right[i])
		dot += leftValue * rightValue
		normLeft += leftValue * leftValue
		normRight += rightValue * rightValue
	}
	if normLeft == 0 || normRight == 0 {
		return 0
	}
	return dot / (math.Sqrt(normLeft) * math.Sqrt(normRight))
}
