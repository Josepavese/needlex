package intel

import (
	"context"
	"strings"

	"github.com/josepavese/needlex/internal/config"
)

type DenseSemanticAligner struct {
	VectorSpace string
	Config      config.SemanticConfig
	Embedder    TextEmbedder
}

func (a DenseSemanticAligner) Align(ctx context.Context, objective string, candidates []SemanticCandidate) (SemanticAlignment, error) {
	scores, err := a.Score(ctx, objective, candidates)
	if err != nil {
		return SemanticAlignment{}, err
	}
	return reduceSemanticScores(a.model(), a.config(), scores), nil
}

func (a DenseSemanticAligner) Score(ctx context.Context, objective string, candidates []SemanticCandidate) ([]SemanticScore, error) {
	if strings.TrimSpace(objective) == "" || len(candidates) == 0 {
		return nil, nil
	}
	embedder := a.Embedder
	if embedder == nil {
		return nil, nil
	}
	inputs := make([]string, 0, 1+len(candidates))
	inputs = append(inputs, objective)
	candidateIndex := make([]SemanticCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		text := semanticCandidateText(candidate)
		if text == "" {
			continue
		}
		inputs = append(inputs, text)
		candidateIndex = append(candidateIndex, candidate)
	}
	if len(inputs) <= 1 {
		return nil, nil
	}
	vectors, err := embedder.Embed(ctx, inputs)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(inputs) || zeroFloat32Vector(vectors[0]) {
		return nil, nil
	}
	objectiveVector := vectors[0]
	scores := make([]SemanticScore, 0, len(candidateIndex))
	for i, candidate := range candidateIndex {
		scores = append(scores, SemanticScore{
			ID:         candidate.ID,
			Similarity: cosineSimilarityFloat32(objectiveVector, vectors[i+1]),
		})
	}
	return scores, nil
}

func (a DenseSemanticAligner) ScoreCross(ctx context.Context, left, right []SemanticCandidate) (map[string]map[string]float64, error) {
	left = nonEmptySemanticCandidates(left)
	right = nonEmptySemanticCandidates(right)
	if len(left) == 0 || len(right) == 0 {
		return nil, nil
	}
	embedder := a.Embedder
	if embedder == nil {
		return nil, nil
	}
	inputs := make([]string, 0, len(left)+len(right))
	for _, candidate := range left {
		inputs = append(inputs, semanticCandidateText(candidate))
	}
	for _, candidate := range right {
		inputs = append(inputs, semanticCandidateText(candidate))
	}
	vectors, err := embedder.Embed(ctx, inputs)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(inputs) {
		return nil, nil
	}
	out := make(map[string]map[string]float64, len(left))
	rightOffset := len(left)
	for i, leftCandidate := range left {
		if zeroFloat32Vector(vectors[i]) {
			continue
		}
		row := map[string]float64{}
		for j, rightCandidate := range right {
			rightVector := vectors[rightOffset+j]
			if zeroFloat32Vector(rightVector) {
				continue
			}
			similarity := cosineSimilarityFloat32(vectors[i], rightVector)
			if similarity < 0 {
				similarity = 0
			}
			row[rightCandidate.ID] = similarity
		}
		if len(row) > 0 {
			out[leftCandidate.ID] = row
		}
	}
	return out, nil
}

func nonEmptySemanticCandidates(candidates []SemanticCandidate) []SemanticCandidate {
	out := make([]SemanticCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.ID) == "" || semanticCandidateText(candidate) == "" {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func (a DenseSemanticAligner) model() string {
	if a.Embedder != nil {
		if model := strings.TrimSpace(a.Embedder.ModelID()); model != "" {
			return model
		}
	}
	for _, value := range []string{a.VectorSpace, a.Config.VectorSpace, DenseSemanticVectorSpace} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return DenseSemanticVectorSpace
}

func (a DenseSemanticAligner) config() config.SemanticConfig {
	cfg := a.Config
	if cfg.SimilarityThreshold <= 0 {
		cfg.SimilarityThreshold = 0.55
	}
	if cfg.DominanceDelta <= 0 {
		cfg.DominanceDelta = 0.08
	}
	return cfg
}
