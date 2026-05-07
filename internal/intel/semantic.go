package intel

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/josepavese/needlex/internal/config"
)

type SemanticAligner interface {
	Align(ctx context.Context, objective string, candidates []SemanticCandidate) (SemanticAlignment, error)
	Score(ctx context.Context, objective string, candidates []SemanticCandidate) ([]SemanticScore, error)
}

type SemanticCandidate struct {
	ID      string
	Heading []string
	Text    string
}

type SemanticAlignment struct {
	Model            string
	TopID            string
	TopSimilarity    float64
	SecondSimilarity float64
	Suppressed       bool
	Reason           string
}

type SemanticScore struct {
	ID         string
	Similarity float64
}

func NewSemanticAligner(cfg config.Config, client *http.Client) SemanticAligner {
	embedder := NewTextEmbedder(cfg, client)
	return &resilientSemanticAligner{
		inner:    DenseSemanticAligner{VectorSpace: cfg.Semantic.VectorSpace, Config: cfg.Semantic, Embedder: embedder},
		now:      time.Now,
		cooldown: time.Duration(cfg.Semantic.FailureCooldownMS) * time.Millisecond,
	}
}

type resilientSemanticAligner struct {
	inner         SemanticAligner
	now           func() time.Time
	cooldown      time.Duration
	mu            sync.Mutex
	cooldownUntil time.Time
}

func (a *resilientSemanticAligner) Align(ctx context.Context, objective string, candidates []SemanticCandidate) (SemanticAlignment, error) {
	if a.coolingDown() {
		return SemanticAlignment{}, nil
	}
	alignment, err := a.inner.Align(ctx, objective, candidates)
	if err != nil {
		a.trip()
		return SemanticAlignment{}, fmt.Errorf("semantic alignment failed: %w", err)
	}
	a.clear()
	return alignment, nil
}

func (a *resilientSemanticAligner) Score(ctx context.Context, objective string, candidates []SemanticCandidate) ([]SemanticScore, error) {
	if a.coolingDown() {
		return nil, nil
	}
	scores, err := a.inner.Score(ctx, objective, candidates)
	if err != nil {
		a.trip()
		return nil, fmt.Errorf("semantic scoring failed: %w", err)
	}
	a.clear()
	return scores, nil
}

func (a *resilientSemanticAligner) coolingDown() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cooldownUntil.IsZero() {
		return false
	}
	now := a.now()
	return a.cooldownUntil.After(now)
}

func (a *resilientSemanticAligner) trip() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cooldown <= 0 {
		return
	}
	a.cooldownUntil = a.now().Add(a.cooldown)
}

func (a *resilientSemanticAligner) clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cooldownUntil = time.Time{}
}

func reduceSemanticScores(model string, cfg config.SemanticConfig, scores []SemanticScore) SemanticAlignment {
	if len(scores) == 0 {
		return SemanticAlignment{}
	}
	best := scores[0]
	second := 0.0
	for i := 1; i < len(scores); i++ {
		if scores[i].Similarity > best.Similarity {
			second = best.Similarity
			best = scores[i]
			continue
		}
		if scores[i].Similarity > second {
			second = scores[i].Similarity
		}
	}
	alignment := SemanticAlignment{
		Model:            model,
		TopID:            best.ID,
		TopSimilarity:    best.Similarity,
		SecondSimilarity: max(second, 0),
	}
	if alignment.TopSimilarity >= cfg.SimilarityThreshold && alignment.TopSimilarity-alignment.SecondSimilarity >= cfg.DominanceDelta {
		alignment.Suppressed = true
		alignment.Reason = "semantic_dominance"
	}
	return alignment
}

func semanticCandidateText(candidate SemanticCandidate) string {
	return strings.TrimSpace(strings.Join(candidate.Heading, " ") + " " + candidate.Text)
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	dot := 0.0
	normA := 0.0
	normB := 0.0
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
