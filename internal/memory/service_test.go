package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/proof"
)

type stubEmbedder struct {
	model   string
	vectors map[string][]float32
}

func (s stubEmbedder) ModelID() string {
	if s.model != "" {
		return s.model
	}
	return "dense-test"
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

type countingEmbedder struct {
	calls   int32
	model   string
	vectors map[string][]float32
}

func (s *countingEmbedder) ModelID() string {
	if s.model != "" {
		return s.model
	}
	return "dense-test"
}

func (s *countingEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	atomic.AddInt32(&s.calls, 1)
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
	svc := NewService(config.MemoryConfig{}, store, stubEmbedder{model: "embed-x", vectors: map[string][]float32{
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

func TestServiceObserveReusesUnchangedEmbeddingVector(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStore(root, "discovery/discovery.db")
	embedder := &countingEmbedder{model: "embed-x", vectors: map[string][]float32{
		"Reusable\nStable semantic page.": {1, 0, 0},
	}}
	svc := NewService(config.MemoryConfig{}, store, embedder)
	now := time.Now().UTC()
	obs := Observation{
		Document:   core.Document{URL: "https://example.com/doc", FinalURL: "https://example.com/doc", Title: "Reusable", FetchedAt: now, FetchMode: core.FetchModeHTTP},
		ResultPack: core.ResultPack{Profile: core.ProfileStandard, Chunks: []core.Chunk{{ID: "chunk", DocID: "doc", Text: "Stable semantic page.", Confidence: 0.9}}},
		TraceID:    "trace-reuse",
		SourceKind: "read",
		ObservedAt: now,
	}
	if err := svc.Observe(context.Background(), obs); err != nil {
		t.Fatalf("first observe: %v", err)
	}
	if err := svc.Observe(context.Background(), obs); err != nil {
		t.Fatalf("second observe: %v", err)
	}
	if got := atomic.LoadInt32(&embedder.calls); got != 1 {
		t.Fatalf("expected unchanged observation to reuse embedding vector, got %d provider calls", got)
	}
	stats, err := store.GetStats(context.Background())
	if err != nil {
		t.Fatalf("memory stats: %v", err)
	}
	if stats.EmbeddingCount != 1 {
		t.Fatalf("expected one embedding row after refresh, got %+v", stats)
	}
}

func TestServiceObserveCanForceEmbeddingRefresh(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStore(root, "discovery/discovery.db")
	embedder := &countingEmbedder{model: "embed-x", vectors: map[string][]float32{
		"Reusable\nStable semantic page.": {1, 0, 0},
	}}
	svc := NewService(config.MemoryConfig{}, store, embedder)
	now := time.Now().UTC()
	obs := Observation{
		Document:   core.Document{URL: "https://example.com/doc", FinalURL: "https://example.com/doc", Title: "Reusable", FetchedAt: now, FetchMode: core.FetchModeHTTP},
		ResultPack: core.ResultPack{Profile: core.ProfileStandard, Chunks: []core.Chunk{{ID: "chunk", DocID: "doc", Text: "Stable semantic page.", Confidence: 0.9}}},
		TraceID:    "trace-refresh",
		SourceKind: "read",
		ObservedAt: now,
	}
	if err := svc.Observe(context.Background(), obs); err != nil {
		t.Fatalf("first observe: %v", err)
	}
	obs.ForceEmbeddingRefresh = true
	if err := svc.Observe(context.Background(), obs); err != nil {
		t.Fatalf("forced observe: %v", err)
	}
	if got := atomic.LoadInt32(&embedder.calls); got != 2 {
		t.Fatalf("expected force refresh to bypass reusable vector, got %d provider calls", got)
	}
}

func TestServiceRefreshEmbeddingsForceRecomputesDocuments(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStore(root, "discovery/discovery.db")
	embedder := &countingEmbedder{model: "embed-x", vectors: map[string][]float32{
		"Reusable\nStable semantic page.": {1, 0, 0},
	}}
	svc := NewService(config.MemoryConfig{}, store, embedder)
	now := time.Now().UTC()
	if err := svc.Observe(context.Background(), Observation{
		Document:   core.Document{URL: "https://example.com/doc", FinalURL: "https://example.com/doc", Title: "Reusable", FetchedAt: now, FetchMode: core.FetchModeHTTP},
		ResultPack: core.ResultPack{Profile: core.ProfileStandard, Chunks: []core.Chunk{{ID: "chunk", DocID: "doc", Text: "Stable semantic page.", Confidence: 0.9}}},
		TraceID:    "trace-refresh",
		SourceKind: "read",
		ObservedAt: now,
	}); err != nil {
		t.Fatalf("observe: %v", err)
	}
	stats, err := svc.RefreshEmbeddings(context.Background(), false)
	if err != nil {
		t.Fatalf("refresh embeddings: %v", err)
	}
	if stats.ReusedCount != 1 || stats.EmbeddedCount != 0 {
		t.Fatalf("expected non-forced refresh to reuse vector, got %+v", stats)
	}
	stats, err = svc.RefreshEmbeddings(context.Background(), true)
	if err != nil {
		t.Fatalf("force refresh embeddings: %v", err)
	}
	if stats.DocumentCount != 1 || stats.EmbeddedCount != 1 || stats.ReusedCount != 0 {
		t.Fatalf("expected forced refresh to recompute vector, got %+v", stats)
	}
}

func TestServiceSearchInfersFamilyRootFromObservedDescendants(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStore(root, "discovery/discovery.db")
	svc := NewService(config.MemoryConfig{}, store, stubEmbedder{model: "embed-x", vectors: map[string][]float32{
		"JavaScript | MDN\nJavaScript overview language guide on MDN.":                               {1, 0, 0},
		"AsyncGenerator | MDN\nAsyncGenerator reference details on MDN JavaScript reference page.":   {1, 0, 0},
		"Enumerability | MDN\nEnumerability and ownership details for JavaScript properties on MDN.": {1, 0, 0},
		"MDN JavaScript overview": {1, 0, 0},
	}})
	now := time.Now().UTC()
	for _, item := range []struct {
		url   string
		title string
		text  string
		trace string
	}{
		{"https://developer.mozilla.org/en-US/docs/Web/JavaScript", "JavaScript | MDN", "JavaScript overview language guide on MDN.", "trace_root"},
		{"https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/AsyncGenerator", "AsyncGenerator | MDN", "AsyncGenerator reference details on MDN JavaScript reference page.", "trace_ref"},
		{"https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide/Enumerability_and_ownership_of_properties", "Enumerability | MDN", "Enumerability and ownership details for JavaScript properties on MDN.", "trace_guide"},
	} {
		if err := svc.Observe(context.Background(), Observation{
			Document:    core.Document{URL: item.url, FinalURL: item.url, Title: item.title, FetchedAt: now, FetchMode: core.FetchModeHTTP},
			ResultPack:  core.ResultPack{Profile: core.ProfileStandard, Chunks: []core.Chunk{{ID: item.trace, DocID: item.trace, Text: item.text, Confidence: 0.95}}},
			TraceID:     item.trace,
			ObservedAt:  now,
			SourceKind:  "read",
			StableRatio: 0.8,
		}); err != nil {
			t.Fatalf("observe %s: %v", item.url, err)
		}
	}
	matches, err := svc.Search(context.Background(), "MDN JavaScript overview", SearchOptions{Limit: 5, ExpandLimit: 3})
	if err != nil {
		t.Fatalf("search memory: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected matches")
	}
	if matches[0].URL != "https://developer.mozilla.org/en-US/docs/Web/JavaScript" {
		t.Fatalf("expected root overview first, got %+v", matches[0])
	}
	if !containsReason(matches[0].Reasons, "family_root_inference") && !containsReason(matches[0].Reasons, "local_memory_hit") {
		t.Fatalf("expected root inference signal, got %+v", matches[0])
	}
}

func TestServiceSearchUsesSemanticFamilyGraph(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStore(root, "discovery/discovery.db")
	svc := NewService(config.MemoryConfig{}, store, stubEmbedder{model: "embed-x", vectors: map[string][]float32{
		"Global Reference\nAuthoritative maintained reference for a public standard.": {1, 0, 0},
		"Regional Mirror\nTranslated secondary copy of the same public standard.":     {0.96, 0.02, 0},
		"authoritative public standard":                                               {1, 0, 0},
	}})
	now := time.Now().UTC()
	for _, item := range []struct {
		url   string
		title string
		text  string
	}{
		{"https://origin.example/ref", "Global Reference", "Authoritative maintained reference for a public standard."},
		{"https://mirror.example/translated", "Regional Mirror", "Translated secondary copy of the same public standard."},
	} {
		if err := svc.Observe(context.Background(), Observation{
			Document:    core.Document{URL: item.url, FinalURL: item.url, Title: item.title, FetchedAt: now, FetchMode: core.FetchModeHTTP},
			ResultPack:  core.ResultPack{Profile: core.ProfileStandard, Chunks: []core.Chunk{{ID: item.title, DocID: item.title, Text: item.text, Confidence: 0.95}}},
			TraceID:     item.title,
			ObservedAt:  now,
			SourceKind:  "read",
			StableRatio: 0.8,
		}); err != nil {
			t.Fatalf("observe %s: %v", item.url, err)
		}
	}
	stats, err := store.GetStats(context.Background())
	if err != nil {
		t.Fatalf("memory stats: %v", err)
	}
	if stats.SemanticFamilyCount != 1 {
		t.Fatalf("expected semantically close pages to join one family, got %+v", stats)
	}
	matches, err := svc.Search(context.Background(), "authoritative public standard", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("search memory: %v", err)
	}
	foundFamily := false
	for _, match := range matches {
		if containsReason(match.Reasons, "entity_family_graph_recall") {
			foundFamily = true
			break
		}
	}
	if !foundFamily {
		t.Fatalf("expected semantic family graph recall, got %+v", matches)
	}
}

func TestServiceSearchSkipsEmbeddingRowsWithDifferentDimensions(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStore(root, "discovery/discovery.db")
	svc := NewService(config.MemoryConfig{}, store, stubEmbedder{model: "embed-x", vectors: map[string][]float32{
		"Current Model\nCurrent dimension document.": {1, 0},
		"Old Model\nOld dimension document.":         {1, 0, 0},
		"current dimension":                          {1, 0},
	}})
	now := time.Now().UTC()
	for _, item := range []struct {
		url   string
		title string
		text  string
	}{
		{"https://current.example/doc", "Current Model", "Current dimension document."},
		{"https://old.example/doc", "Old Model", "Old dimension document."},
	} {
		if err := svc.Observe(context.Background(), Observation{
			Document:   core.Document{URL: item.url, FinalURL: item.url, Title: item.title, FetchedAt: now, FetchMode: core.FetchModeHTTP},
			ResultPack: core.ResultPack{Profile: core.ProfileStandard, Chunks: []core.Chunk{{ID: item.title, DocID: item.title, Text: item.text, Confidence: 0.95}}},
			TraceID:    item.title,
			ObservedAt: now,
			SourceKind: "read",
		}); err != nil {
			t.Fatalf("observe %s: %v", item.url, err)
		}
	}
	matches, err := svc.Search(context.Background(), "current dimension", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("search should tolerate mixed embedding dimensions: %v", err)
	}
	if len(matches) == 0 || matches[0].URL != "https://current.example/doc" {
		t.Fatalf("expected current-dimension match first, got %+v", matches)
	}
}

func TestServiceSearchFiltersVectorSpace(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStore(root, "discovery/discovery.db")
	svc := NewService(config.MemoryConfig{}, store, stubEmbedder{model: "current-space", vectors: map[string][]float32{
		"current objective": {1, 0, 0},
	}})
	now := time.Now().UTC()
	ctx := context.Background()
	for _, item := range []struct {
		url         string
		vectorSpace string
		vector      []float32
	}{
		{"https://old.example/doc", "old-space", []float32{1, 0, 0}},
		{"https://current.example/doc", "current-space", []float32{1, 0, 0}},
	} {
		doc := Document{URL: item.url, FinalURL: item.url, Host: strings.TrimPrefix(strings.TrimSuffix(item.url, "/doc"), "https://"), Path: "/doc", Title: item.vectorSpace, SemanticSummary: item.vectorSpace, LastTraceID: item.vectorSpace, SourceKind: "read", ObservedAt: now, UpdatedAt: now}
		if err := store.UpsertDocument(ctx, doc); err != nil {
			t.Fatalf("upsert doc %s: %v", item.url, err)
		}
		emb := Embedding{EmbeddingRef: embeddingRef(item.url, item.vectorSpace, "dense-http"), DocumentURL: item.url, Model: item.vectorSpace, Backend: "dense-http", InputText: item.vectorSpace, Dimension: 3, CreatedAt: now, UpdatedAt: now}
		if err := store.UpsertEmbedding(ctx, emb, item.vector); err != nil {
			t.Fatalf("upsert embedding %s: %v", item.url, err)
		}
	}
	matches, err := svc.Search(ctx, "current objective", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("search memory: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected current vector-space match")
	}
	for _, match := range matches {
		if match.URL == "https://old.example/doc" {
			t.Fatalf("old vector-space row leaked into results: %+v", matches)
		}
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
	if err := store.UpsertSemanticFamilyEvidence(ctx, doc, []float32{1, 0, 0}, "m"); err != nil {
		t.Fatalf("upsert semantic family: %v", err)
	}

	exportDir := filepath.Join(root, "export")
	exportStats, err := store.ExportJSONL(ctx, exportDir)
	if err != nil {
		t.Fatalf("export jsonl: %v", err)
	}
	if exportStats.DocumentCount != 1 || exportStats.EdgeCount != 1 || exportStats.EmbeddingCount != 1 || exportStats.FamilyCount != 1 || exportStats.MemberCount != 1 {
		t.Fatalf("unexpected export stats: %+v", exportStats)
	}
	for _, path := range []string{exportStats.DocumentsPath, exportStats.EdgesPath, exportStats.EmbeddingsPath, exportStats.FamiliesPath, exportStats.FamilyMembersPath} {
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
	if importStats.DocumentCount != 1 || importStats.EdgeCount != 1 || importStats.EmbeddingCount != 1 || importStats.FamilyCount != 1 || importStats.MemberCount != 1 {
		t.Fatalf("unexpected import stats: %+v", importStats)
	}
	if err := importStore.RebuildIndex(ctx); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	stats, err := importStore.GetStats(ctx)
	if err != nil {
		t.Fatalf("get stats after import: %v", err)
	}
	if stats.DocumentCount != 1 ||
		stats.EdgeCount != 1 ||
		stats.EmbeddingCount != 1 ||
		stats.SemanticFamilyCount != 1 ||
		stats.SemanticMemberCount != 1 ||
		stats.VectorEngine != "exact" ||
		len(stats.VectorDimensions) != 1 ||
		stats.VectorDimensions[0] != 3 ||
		stats.LastRebuildAt.IsZero() {
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
