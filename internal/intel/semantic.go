package intel

import (
	"bytes"
	"context"
	"encoding/json"
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

type NoopSemanticAligner struct{}

func (NoopSemanticAligner) Align(context.Context, string, []SemanticCandidate) (SemanticAlignment, error) {
	return SemanticAlignment{}, nil
}

func (NoopSemanticAligner) Score(context.Context, string, []SemanticCandidate) ([]SemanticScore, error) {
	return nil, nil
}

type OllamaSemanticAligner struct {
	BaseURL string
	Model   string
	Client  *http.Client
	Config  config.SemanticConfig
}

type OpenAISemanticAligner struct {
	BaseURL string
	Model   string
	Client  *http.Client
	Config  config.SemanticConfig
}

func NewSemanticAligner(cfg config.Config, client *http.Client) SemanticAligner {
	if !cfg.Semantic.Enabled || strings.TrimSpace(cfg.Semantic.Model) == "" {
		return NoopSemanticAligner{}
	}
	var aligner SemanticAligner
	switch strings.TrimSpace(cfg.Semantic.Backend) {
	case "ollama-embed":
		aligner = OllamaSemanticAligner{BaseURL: strings.TrimRight(cfg.Semantic.BaseURL, "/"), Model: cfg.Semantic.Model, Client: client, Config: cfg.Semantic}
	case "openai-embeddings":
		aligner = OpenAISemanticAligner{BaseURL: strings.TrimRight(cfg.Semantic.BaseURL, "/"), Model: cfg.Semantic.Model, Client: client, Config: cfg.Semantic}
	default:
		return NoopSemanticAligner{}
	}
	return &resilientSemanticAligner{
		inner:    aligner,
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
		return SemanticAlignment{}, nil
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
		return nil, nil
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

func (a OllamaSemanticAligner) Align(ctx context.Context, objective string, candidates []SemanticCandidate) (SemanticAlignment, error) {
	scores, err := a.Score(ctx, objective, candidates)
	if err != nil {
		return SemanticAlignment{}, err
	}
	return reduceSemanticScores(a.Model, a.Config, scores), nil
}

func (a OllamaSemanticAligner) Score(ctx context.Context, objective string, candidates []SemanticCandidate) ([]SemanticScore, error) {
	if strings.TrimSpace(objective) == "" || len(candidates) == 0 {
		return nil, nil
	}
	input := []string{strings.TrimSpace(objective)}
	for _, candidate := range candidates {
		input = append(input, semanticCandidateText(candidate))
	}
	body, _ := json.Marshal(map[string]any{"model": a.Model, "input": input})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("semantic embed upstream returned %d", resp.StatusCode)
	}
	var payload struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Embeddings) != len(input) {
		return nil, fmt.Errorf("semantic embed returned %d vectors for %d inputs", len(payload.Embeddings), len(input))
	}
	return scoreSemanticCandidates(candidates, payload.Embeddings)
}

func (a OpenAISemanticAligner) Align(ctx context.Context, objective string, candidates []SemanticCandidate) (SemanticAlignment, error) {
	scores, err := a.Score(ctx, objective, candidates)
	if err != nil {
		return SemanticAlignment{}, err
	}
	return reduceSemanticScores(a.Model, a.Config, scores), nil
}

func (a OpenAISemanticAligner) Score(ctx context.Context, objective string, candidates []SemanticCandidate) ([]SemanticScore, error) {
	if strings.TrimSpace(objective) == "" || len(candidates) == 0 {
		return nil, nil
	}
	input := []string{strings.TrimSpace(objective)}
	for _, candidate := range candidates {
		input = append(input, semanticCandidateText(candidate))
	}
	body, _ := json.Marshal(map[string]any{"model": a.Model, "input": input})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("semantic embeddings upstream returned %d", resp.StatusCode)
	}
	var payload struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Data) != len(input) {
		return nil, fmt.Errorf("semantic embeddings returned %d vectors for %d inputs", len(payload.Data), len(input))
	}
	vectors := make([][]float64, len(payload.Data))
	for _, row := range payload.Data {
		if row.Index < 0 || row.Index >= len(payload.Data) {
			return nil, fmt.Errorf("semantic embeddings index %d out of range", row.Index)
		}
		vectors[row.Index] = row.Embedding
	}
	return scoreSemanticCandidates(candidates, vectors)
}

func scoreSemanticCandidates(candidates []SemanticCandidate, embeddings [][]float64) ([]SemanticScore, error) {
	if len(embeddings) != len(candidates)+1 {
		return nil, fmt.Errorf("semantic embeddings mismatch: got %d vectors for %d candidates", len(embeddings), len(candidates))
	}
	objectiveVec := embeddings[0]
	scores := make([]SemanticScore, 0, len(candidates))
	for i := 1; i < len(embeddings); i++ {
		scores = append(scores, SemanticScore{
			ID:         candidates[i-1].ID,
			Similarity: cosineSimilarity(objectiveVec, embeddings[i]),
		})
	}
	return scores, nil
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
		SecondSimilarity: maxFloat(second, 0),
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

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}
