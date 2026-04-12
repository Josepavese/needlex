package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/proof"
)

type stubEmbedder struct {
	vectors map[string][]float32
}

func (s stubEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	out := make([][]float32, 0, len(inputs))
	for _, input := range inputs {
		if vector, ok := s.vectors[input]; ok {
			out = append(out, vector)
			continue
		}
		out = append(out, []float32{0, 0, 0})
	}
	return out, nil
}

func TestServiceObserveAndSearch(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStore(root, "discovery/discovery.db")
	svc := NewService(config.MemoryConfig{EmbeddingBackend: "openai-embeddings", EmbeddingModel: "embed-x"}, store, stubEmbedder{vectors: map[string][]float32{
		"Playwright\nPlaywright is an end-to-end testing framework for modern apps.": {1, 0, 0},
		"Installation\nInstall Playwright with npm and run install browsers.":        {0.9, 0.1, 0},
		"playwright install": {1, 0, 0},
	}})
	obsAt := time.Now().UTC()
	if err := svc.Observe(context.Background(), Observation{
		Document:     core.Document{URL: "https://playwright.dev/", FinalURL: "https://playwright.dev/", Title: "Playwright", FetchedAt: obsAt, FetchMode: core.FetchModeHTTP, RawHash: "hash-a", ID: "doc-a"},
		ResultPack:   core.ResultPack{Query: "playwright", Profile: core.ProfileStandard, Chunks: []core.Chunk{{ID: "c1", DocID: "doc-a", Text: "Playwright is an end-to-end testing framework for modern apps.", Fingerprint: "fp1", Confidence: 0.9}}, Sources: []core.SourceRef{{DocumentID: "doc-a", URL: "https://playwright.dev/"}}, Links: []string{"https://playwright.dev/docs/intro"}, CostReport: core.CostReport{LanePath: []int{0}}},
		ProofRecords: []proof.ProofRecord{{ID: "proof_a", Proof: core.Proof{ChunkID: "c1", SourceSpan: core.SourceSpan{Selector: "main", CharStart: 0, CharEnd: 20}, TransformChain: []string{"reduce"}, Lane: 0}}},
		TraceID:      "trace_a",
		SourceKind:   "read",
		ObservedAt:   obsAt,
	}); err != nil {
		t.Fatalf("observe root doc: %v", err)
	}
	if err := svc.Observe(context.Background(), Observation{
		Document:     core.Document{URL: "https://playwright.dev/docs/intro", FinalURL: "https://playwright.dev/docs/intro", Title: "Installation", FetchedAt: obsAt, FetchMode: core.FetchModeHTTP, RawHash: "hash-b", ID: "doc-b"},
		ResultPack:   core.ResultPack{Query: "playwright install", Profile: core.ProfileStandard, Chunks: []core.Chunk{{ID: "c2", DocID: "doc-b", Text: "Install Playwright with npm and run install browsers.", Fingerprint: "fp2", Confidence: 0.91}}, Sources: []core.SourceRef{{DocumentID: "doc-b", URL: "https://playwright.dev/docs/intro"}}, CostReport: core.CostReport{LanePath: []int{0}}},
		ProofRecords: []proof.ProofRecord{{ID: "proof_b", Proof: core.Proof{ChunkID: "c2", SourceSpan: core.SourceSpan{Selector: "main", CharStart: 0, CharEnd: 20}, TransformChain: []string{"reduce"}, Lane: 0}}},
		TraceID:      "trace_b",
		SourceKind:   "query",
		ObservedAt:   obsAt,
	}); err != nil {
		t.Fatalf("observe child doc: %v", err)
	}
	matches, err := svc.Search(context.Background(), "playwright install", SearchOptions{Limit: 5, ExpandLimit: 2})
	if err != nil {
		t.Fatalf("search memory: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least one discovery memory match")
	}
	if matches[0].URL != "https://playwright.dev/" && matches[0].URL != "https://playwright.dev/docs/intro" {
		t.Fatalf("unexpected top memory match: %+v", matches[0])
	}
	foundHostRecall := false
	for _, match := range matches {
		if match.URL == "https://playwright.dev/docs/intro" && containsReason(match.Reasons, "host_memory_recall") {
			foundHostRecall = true
			break
		}
	}
	if !foundHostRecall {
		t.Fatalf("expected same-host memory recall in matches, got %+v", matches)
	}
	stats, err := store.GetStats(context.Background())
	if err != nil {
		t.Fatalf("memory stats: %v", err)
	}
	if stats.DocumentCount != 2 || stats.EmbeddingCount != 2 || stats.EdgeCount != 1 {
		t.Fatalf("unexpected memory stats: %+v", stats)
	}
}

func TestSQLiteStoreExportImportAndRebuild(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStore(root, "discovery/discovery.db")
	now := time.Now().UTC()
	ctx := context.Background()
	doc := Document{
		URL:             "https://example.com/about",
		FinalURL:        "https://example.com/about",
		Host:            "example.com",
		Path:            "/about",
		Title:           "About Example",
		SemanticSummary: "Example is a studio.",
		Language:        "en",
		LocalityHints:   []string{"Turin"},
		EntityHints:     []string{"Example Studio"},
		CategoryHints:   []string{"design"},
		ProofRefs:       []string{"proof_1"},
		LastTraceID:     "trace_1",
		SourceKind:      "read",
		StableRatio:     0.8,
		NoveltyRatio:    0.1,
		ObservedAt:      now,
		UpdatedAt:       now,
	}
	if err := store.UpsertDocument(ctx, doc); err != nil {
		t.Fatalf("upsert doc: %v", err)
	}
	if err := store.UpsertEdges(ctx, []Edge{{SourceURL: doc.URL, TargetURL: "https://example.com/services", AnchorText: "Services", SameHost: true, TraceRef: "trace_1", ObservedAt: now}}); err != nil {
		t.Fatalf("upsert edge: %v", err)
	}
	if err := store.UpsertEmbedding(ctx, Embedding{EmbeddingRef: "emb_1", DocumentURL: doc.URL, Model: "m", Backend: "b", InputText: "About Example\nExample is a studio.", Dimension: 3, CreatedAt: now, UpdatedAt: now}, []float32{1, 0, 0}); err != nil {
		t.Fatalf("upsert embedding: %v", err)
	}

	exportDir := filepath.Join(root, "export")
	exportStats, err := store.ExportJSONL(ctx, exportDir)
	if err != nil {
		t.Fatalf("export jsonl: %v", err)
	}
	if exportStats.DocumentCount != 1 || exportStats.EdgeCount != 1 || exportStats.EmbeddingCount != 1 {
		t.Fatalf("unexpected export stats: %+v", exportStats)
	}
	for _, path := range []string{exportStats.DocumentsPath, exportStats.EdgesPath, exportStats.EmbeddingsPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected exported file %s: %v", path, err)
		}
	}

	importRoot := t.TempDir()
	importStore := NewSQLiteStore(importRoot, "discovery/discovery.db")
	importStats, err := importStore.ImportJSONL(ctx, exportDir)
	if err != nil {
		t.Fatalf("import jsonl: %v", err)
	}
	if importStats.DocumentCount != 1 || importStats.EdgeCount != 1 || importStats.EmbeddingCount != 1 {
		t.Fatalf("unexpected import stats: %+v", importStats)
	}
	if err := importStore.RebuildIndex(ctx); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	stats, err := importStore.GetStats(ctx)
	if err != nil {
		t.Fatalf("get stats after import: %v", err)
	}
	if stats.DocumentCount != 1 || stats.EdgeCount != 1 || stats.EmbeddingCount != 1 || stats.LastRebuildAt.IsZero() {
		t.Fatalf("unexpected imported stats: %+v", stats)
	}
}

func TestSQLiteStorePrune(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStore(root, "discovery/discovery.db")
	now := time.Now().UTC()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		url := "https://example.com/page/" + string(rune('a'+i))
		doc := Document{URL: url, FinalURL: url, Host: "example.com", Path: "/page", Title: url, SemanticSummary: url, LastTraceID: "trace", SourceKind: "read", ObservedAt: now.Add(time.Duration(i) * time.Minute), UpdatedAt: now.Add(time.Duration(i) * time.Minute)}
		if err := store.UpsertDocument(ctx, doc); err != nil {
			t.Fatalf("upsert doc %d: %v", i, err)
		}
		emb := Embedding{EmbeddingRef: embeddingRef(url, "m", "b"), DocumentURL: url, Model: "m", Backend: "b", InputText: url, Dimension: 3, CreatedAt: now, UpdatedAt: now}
		if err := store.UpsertEmbedding(ctx, emb, []float32{1, 0, 0}); err != nil {
			t.Fatalf("upsert embedding %d: %v", i, err)
		}
	}
	if err := store.Prune(ctx, PrunePolicy{MaxDocuments: 2, MaxEdges: 10, MaxEmbeddings: 2}); err != nil {
		t.Fatalf("prune memory: %v", err)
	}
	stats, err := store.GetStats(ctx)
	if err != nil {
		t.Fatalf("stats after prune: %v", err)
	}
	if stats.DocumentCount != 2 || stats.EmbeddingCount != 2 {
		t.Fatalf("unexpected stats after prune: %+v", stats)
	}
}

func containsReason(reasons []string, needle string) bool {
	for _, reason := range reasons {
		if reason == needle {
			return true
		}
	}
	return false
}
