package intel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/config"
)

func TestDenseSemanticAlignerRanksByEmbeddingSimilarity(t *testing.T) {
	server := newDenseEmbeddingTestServer(t, map[string][]float32{
		"virtual environments":                                      {1, 0, 0},
		"Python documentation docs.python.org/3/index.html":         {0, 1, 0},
		"Virtual environments docs.python.org/3/tutorial/venv.html": {0.95, 0.05, 0},
	})
	cfg := config.Defaults()
	cfg.Semantic.EmbeddingURL = server.URL
	cfg.Semantic.ProviderModel = "provider-dense-test"
	cfg.Semantic.VectorSpace = "dense-test-space"
	aligner := NewSemanticAligner(cfg, server.Client())

	scores, err := aligner.Score(context.Background(), "virtual environments", []SemanticCandidate{
		{ID: "root", Text: "Python documentation docs.python.org/3/index.html"},
		{ID: "venv", Text: "Virtual environments docs.python.org/3/tutorial/venv.html"},
	})
	if err != nil {
		t.Fatalf("dense score failed: %v", err)
	}
	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}
	if scores[1].Similarity <= scores[0].Similarity {
		t.Fatalf("expected embedding-nearest candidate to win, got %#v", scores)
	}
}

func TestSemanticConfigRequiresEmbeddingEndpoint(t *testing.T) {
	cfg := config.Defaults()
	cfg.Semantic.EmbeddingURL = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing embedding endpoint to fail validation")
	}
}

func TestTextEmbedderUsesDenseHTTPVectors(t *testing.T) {
	server := newDenseEmbeddingTestServer(t, map[string][]float32{
		"semantic memory vector": {0.2, 0.8, 0},
	})
	cfg := config.Defaults()
	cfg.Semantic.EmbeddingURL = server.URL
	cfg.Semantic.ProviderModel = "provider-dense-test"
	cfg.Semantic.VectorSpace = "dense-test-space"
	embedder := NewTextEmbedder(cfg, server.Client())
	vectors, err := embedder.Embed(context.Background(), []string{"semantic memory vector"})
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 3 {
		t.Fatalf("expected one dense vector, got %#v", vectors)
	}
	if embedder.ModelID() != "dense-test-space" {
		t.Fatalf("expected configured model id, got %q", embedder.ModelID())
	}
}

func TestDenseHTTPTextEmbedderOmitsProviderModelWhenUnset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode embedding request: %v", err)
		}
		if _, ok := req["model"]; ok {
			t.Fatalf("provider model must be omitted when unset, got %#v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float32{{1, 0, 0}}})
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.Semantic.EmbeddingURL = server.URL
	cfg.Semantic.ProviderModel = ""
	cfg.Semantic.VectorSpace = ""
	embedder := NewTextEmbedder(cfg, server.Client())
	if _, err := embedder.Embed(context.Background(), []string{"goal"}); err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if strings.TrimSpace(embedder.ModelID()) == "" {
		t.Fatal("expected derived vector-space id")
	}
}

func TestDecodeEmbeddingDataRejectsInvalidIndexes(t *testing.T) {
	_, err := decodeEmbeddingResponse(strings.NewReader(`{"data":[{"index":0,"embedding":[1,0]},{"index":0,"embedding":[0,1]}]}`))
	if err == nil {
		t.Fatal("expected duplicate data index to fail")
	}
}

func newDenseEmbeddingTestServer(t *testing.T, vectors map[string][]float32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		var req struct {
			Input []string `json:"input"`
			Model string   `json:"model,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode embedding request: %v", err)
		}
		if req.Model != "" && req.Model != "provider-dense-test" {
			t.Fatalf("unexpected provider model %q", req.Model)
		}
		out := make([][]float32, 0, len(req.Input))
		for _, input := range req.Input {
			vector, ok := vectors[input]
			if !ok {
				vector = []float32{0, 0, 0}
			}
			out = append(out, vector)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out})
	}))
	t.Cleanup(server.Close)
	return server
}
