package intel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
)

type TextEmbedder interface {
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
}

type NativeTextEmbedder struct {
	Dimensions int
}

type OllamaTextEmbedder struct {
	BaseURL   string
	Model     string
	Client    *http.Client
	TimeoutMS int64
}

type OpenAITextEmbedder struct {
	BaseURL   string
	Model     string
	Client    *http.Client
	TimeoutMS int64
}

type fallbackTextEmbedder struct {
	primary  TextEmbedder
	fallback TextEmbedder
}

func NewTextEmbedder(cfg config.Config, client *http.Client) TextEmbedder {
	backend := strings.TrimSpace(cfg.Memory.EmbeddingBackend)
	model := strings.TrimSpace(cfg.Memory.EmbeddingModel)
	fallback := NativeTextEmbedder{Dimensions: 384}
	if backend == "" || model == "" {
		return fallback
	}
	var primary TextEmbedder
	switch backend {
	case "ollama-embed":
		primary = OllamaTextEmbedder{BaseURL: strings.TrimRight(cfg.Semantic.BaseURL, "/"), Model: model, Client: client, TimeoutMS: cfg.Semantic.TimeoutMS}
	case "openai-embeddings":
		primary = OpenAITextEmbedder{BaseURL: strings.TrimRight(cfg.Semantic.BaseURL, "/"), Model: model, Client: client, TimeoutMS: cfg.Semantic.TimeoutMS}
	default:
		return fallback
	}
	return fallbackTextEmbedder{primary: primary, fallback: fallback}
}

func (e fallbackTextEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	vectors, err := e.primary.Embed(ctx, inputs)
	if err == nil && len(vectors) > 0 {
		return vectors, nil
	}
	return e.fallback.Embed(ctx, inputs)
}

func (e NativeTextEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	clean := compactEmbedInputs(inputs)
	if len(clean) == 0 {
		return nil, nil
	}
	dimensions := e.Dimensions
	if dimensions <= 0 {
		dimensions = 384
	}
	out := make([][]float32, 0, len(clean))
	for _, input := range clean {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		out = append(out, nativeTextEmbedding(input, dimensions))
	}
	return out, nil
}

func (e OllamaTextEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	clean := compactEmbedInputs(inputs)
	if len(clean) == 0 {
		return nil, nil
	}
	ctx, cancel := contextWithTimeoutMS(ctx, e.TimeoutMS)
	defer cancel()
	body, _ := json.Marshal(map[string]any{"model": e.Model, "input": clean})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := httpClientOrDefault(e.Client, time.Duration(e.TimeoutMS)*time.Millisecond)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("memory embed upstream returned %d", resp.StatusCode)
	}
	var payload struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := decodeJSONLimited(resp.Body, maxEmbeddingResponseBytes, "memory embed", &payload); err != nil {
		return nil, err
	}
	if len(payload.Embeddings) != len(clean) {
		return nil, fmt.Errorf("memory embed returned %d vectors for %d inputs", len(payload.Embeddings), len(clean))
	}
	return convertEmbeddings(payload.Embeddings), nil
}

func (e OpenAITextEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	clean := compactEmbedInputs(inputs)
	if len(clean) == 0 {
		return nil, nil
	}
	ctx, cancel := contextWithTimeoutMS(ctx, e.TimeoutMS)
	defer cancel()
	body, _ := json.Marshal(map[string]any{"model": e.Model, "input": clean})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := httpClientOrDefault(e.Client, time.Duration(e.TimeoutMS)*time.Millisecond)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("memory embeddings upstream returned %d", resp.StatusCode)
	}
	var payload struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := decodeJSONLimited(resp.Body, maxEmbeddingResponseBytes, "memory embeddings", &payload); err != nil {
		return nil, err
	}
	if len(payload.Data) != len(clean) {
		return nil, fmt.Errorf("memory embeddings returned %d vectors for %d inputs", len(payload.Data), len(clean))
	}
	ordered := make([][]float64, len(payload.Data))
	for _, row := range payload.Data {
		if row.Index < 0 || row.Index >= len(payload.Data) {
			return nil, fmt.Errorf("memory embeddings index %d out of range", row.Index)
		}
		ordered[row.Index] = row.Embedding
	}
	return convertEmbeddings(ordered), nil
}

func compactEmbedInputs(inputs []string) []string {
	out := make([]string, 0, len(inputs))
	for _, input := range inputs {
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		out = append(out, input)
	}
	return out
}

func convertEmbeddings(vectors [][]float64) [][]float32 {
	out := make([][]float32, 0, len(vectors))
	for _, vector := range vectors {
		converted := make([]float32, 0, len(vector))
		for _, value := range vector {
			converted = append(converted, float32(value))
		}
		out = append(out, converted)
	}
	return out
}

func nativeTextEmbedding(input string, dimensions int) []float32 {
	vector := make([]float32, dimensions)
	features := nativeEmbeddingFeatures(input)
	for key, weight := range features {
		idx := nativeFeatureIndex(key, dimensions)
		vector[idx] += float32(weight)
	}
	normalizeFloat32(vector)
	return vector
}

func nativeFeatureIndex(key string, dimensions int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(dimensions))
}

func normalizeFloat32(vector []float32) {
	norm := 0.0
	for _, value := range vector {
		norm += float64(value * value)
	}
	if norm == 0 {
		return
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vector {
		vector[i] *= scale
	}
}
