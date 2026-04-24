package intel

import (
	"context"
	"testing"

	"github.com/josepavese/needlex/internal/config"
)

func TestNativeSemanticAlignerRanksContextualCandidate(t *testing.T) {
	aligner := NativeSemanticAligner{Config: config.Defaults().Semantic}
	scores, err := aligner.Score(context.Background(), "virtual environments", []SemanticCandidate{
		{ID: "root", Text: "Python documentation docs.python.org/3/index.html"},
		{ID: "venv", Text: "Virtual environments docs.python.org/3/tutorial/venv.html"},
	})
	if err != nil {
		t.Fatalf("native score failed: %v", err)
	}
	if len(scores) != 2 {
		t.Fatalf("expected 2 scores, got %d", len(scores))
	}
	if scores[1].Similarity <= scores[0].Similarity {
		t.Fatalf("expected contextual candidate to win, got %#v", scores)
	}
}

func TestNativeTextEmbedderProducesSearchableVectors(t *testing.T) {
	embedder := NativeTextEmbedder{Dimensions: 64}
	vectors, err := embedder.Embed(context.Background(), []string{
		"Install Playwright and browser binaries",
		"Release chronology and prior versions",
	})
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vectors))
	}
	if len(vectors[0]) != 64 {
		t.Fatalf("expected 64 dimensions, got %d", len(vectors[0]))
	}
	if sparseCosine(nativeSemanticVector("playwright installation"), nativeSemanticVector("Install Playwright and browser binaries")) <= 0 {
		t.Fatal("expected native semantic overlap")
	}
}

func TestTextEmbedderFallsBackToNativeForUnsupportedBackend(t *testing.T) {
	cfg := config.Defaults()
	cfg.Memory.EmbeddingBackend = "unsupported"
	embedder := NewTextEmbedder(cfg, nil)
	vectors, err := embedder.Embed(context.Background(), []string{"semantic memory fallback"})
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) == 0 {
		t.Fatalf("expected native fallback vector, got %#v", vectors)
	}
}
