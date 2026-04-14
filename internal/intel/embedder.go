package intel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/josepavese/needlex/internal/config"
)

type TextEmbedder interface {
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
}

type NoopTextEmbedder struct{}

func (NoopTextEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return nil, nil
}

type OllamaTextEmbedder struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

type OpenAITextEmbedder struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewTextEmbedder(cfg config.Config, client *http.Client) TextEmbedder {
	backend := strings.TrimSpace(cfg.Memory.EmbeddingBackend)
	model := strings.TrimSpace(cfg.Memory.EmbeddingModel)
	if backend == "" || model == "" {
		return NoopTextEmbedder{}
	}
	switch backend {
	case "ollama-embed":
		return OllamaTextEmbedder{BaseURL: strings.TrimRight(cfg.Semantic.BaseURL, "/"), Model: model, Client: client}
	case "openai-embeddings":
		return OpenAITextEmbedder{BaseURL: strings.TrimRight(cfg.Semantic.BaseURL, "/"), Model: model, Client: client}
	default:
		return NoopTextEmbedder{}
	}
}

func (e OllamaTextEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	clean := compactEmbedInputs(inputs)
	if len(clean) == 0 {
		return nil, nil
	}
	body, _ := json.Marshal(map[string]any{"model": e.Model, "input": clean})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
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
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
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
	body, _ := json.Marshal(map[string]any{"model": e.Model, "input": clean})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.BaseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
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
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
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
