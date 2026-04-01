package intel

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/josepavese/needlex/internal/config"
)

func TestCosineSimilarity(t *testing.T) {
	if got := cosineSimilarity([]float64{1, 0}, []float64{1, 0}); got < 0.99 {
		t.Fatalf("expected near-1 cosine, got %.4f", got)
	}
	if got := cosineSimilarity([]float64{1, 0}, []float64{0, 1}); got > 0.01 {
		t.Fatalf("expected near-0 cosine, got %.4f", got)
	}
}

func TestOllamaSemanticAlignerSuppressesOnSemanticDominance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"embeddings":[[1,0],[0.9,0.1],[0.2,0.8]]}`)
	}))
	defer server.Close()
	aligner := OllamaSemanticAligner{BaseURL: server.URL, Model: "embed-x", Config: config.SemanticConfig{SimilarityThreshold: 0.55, DominanceDelta: 0.08}}
	out, err := aligner.Align(context.Background(), "service offering", []SemanticCandidate{{ID: "chk_1", Text: "sviluppo siti web"}, {ID: "chk_2", Text: "contattaci ora"}})
	if err != nil {
		t.Fatalf("align: %v", err)
	}
	if !out.Suppressed || out.TopID != "chk_1" {
		t.Fatalf("expected semantic suppression on chk_1, got %#v", out)
	}
	if out.Reason == "" {
		t.Fatalf("expected semantic suppression reason, got %#v", out)
	}
}

func TestOpenAISemanticAlignerSuppressesOnSemanticDominance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[1,0]},{"object":"embedding","index":1,"embedding":[0.92,0.08]},{"object":"embedding","index":2,"embedding":[0.15,0.85]}],"model":"embed-x"}`)
	}))
	defer server.Close()
	aligner := OpenAISemanticAligner{BaseURL: server.URL, Model: "embed-x", Config: config.SemanticConfig{SimilarityThreshold: 0.55, DominanceDelta: 0.08}}
	out, err := aligner.Align(context.Background(), "service offering", []SemanticCandidate{{ID: "chk_1", Text: "sviluppo siti web e marketing digitale"}, {ID: "chk_2", Text: "contattaci ora"}})
	if err != nil {
		t.Fatalf("align: %v", err)
	}
	if !out.Suppressed || out.TopID != "chk_1" {
		t.Fatalf("expected openai semantic suppression on chk_1, got %#v", out)
	}
	if out.Reason != "semantic_dominance" {
		t.Fatalf("expected semantic dominance reason, got %#v", out)
	}
}
