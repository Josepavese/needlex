package intel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestDenseHTTPTextEmbedderCachesRepeatedInputsPerVectorSpace(t *testing.T) {
	resetDenseEmbeddingCacheForTest()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode embedding request: %v", err)
		}
		out := make([][]float32, 0, len(req.Input))
		for range req.Input {
			out = append(out, []float32{1, 0, 0})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out})
	}))
	defer server.Close()

	embedder := DenseHTTPTextEmbedder{
		Endpoint:      server.URL,
		ProviderModel: "provider-dense-test",
		VectorSpace:   "cache-test-space",
		Client:        server.Client(),
	}
	if _, err := embedder.Embed(context.Background(), []string{"same", "same", "other"}); err != nil {
		t.Fatalf("first embed failed: %v", err)
	}
	if _, err := embedder.Embed(context.Background(), []string{"same", "other"}); err != nil {
		t.Fatalf("second embed failed: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected cached second call and de-duplicated first call, got %d HTTP calls", got)
	}
}

func TestDenseHTTPTextEmbedderUsesPersistentPALCache(t *testing.T) {
	resetDenseEmbeddingCacheForTest()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode embedding request: %v", err)
		}
		out := make([][]float32, 0, len(req.Input))
		for range req.Input {
			out = append(out, []float32{0, 1, 0})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out})
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	embedder := DenseHTTPTextEmbedder{
		Endpoint:      server.URL,
		ProviderModel: "provider-dense-test",
		VectorSpace:   "persistent-cache-space",
		CacheDir:      cacheDir,
		Client:        server.Client(),
	}
	if _, err := embedder.Embed(context.Background(), []string{"semantic cache item"}); err != nil {
		t.Fatalf("first embed failed: %v", err)
	}
	resetDenseEmbeddingCacheForTest()
	if _, err := embedder.Embed(context.Background(), []string{"semantic cache item"}); err != nil {
		t.Fatalf("second embed failed: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected persistent cache to avoid second HTTP call, got %d calls", got)
	}
}

func TestDenseHTTPTextEmbedderPersistsProviderIdentityDigest(t *testing.T) {
	resetDenseEmbeddingCacheForTest()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode embedding request: %v", err)
		}
		out := make([][]float32, 0, len(req.Input))
		for range req.Input {
			out = append(out, []float32{1, 1, 0})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out})
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	input := "semantic identity input"
	embedder := DenseHTTPTextEmbedder{
		Endpoint:      server.URL,
		ProviderModel: "provider-dense-test",
		VectorSpace:   "provider-identity-space-a",
		CacheDir:      cacheDir,
		Client:        server.Client(),
	}
	if _, err := embedder.Embed(context.Background(), []string{input}); err != nil {
		t.Fatalf("first embed failed: %v", err)
	}
	key := embedder.embeddingCacheKey(input)
	raw, err := os.ReadFile(embeddingCachePath(cacheDir, key, ".json"))
	if err != nil {
		t.Fatalf("read persistent cache record: %v", err)
	}
	if strings.Contains(string(raw), server.URL) || strings.Contains(string(raw), input) {
		t.Fatalf("persistent cache must not store raw endpoint or input text: %s", raw)
	}
	var entry persistentEmbeddingCacheEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatalf("decode persistent cache record: %v", err)
	}
	if !entry.validFor(key, embedder.embeddingCachePolicy(), false) {
		t.Fatalf("expected provider identity record to validate: %+v", entry.Identity)
	}
	if entry.Identity.SchemaVersion != persistentEmbeddingCacheSchema ||
		entry.Identity.EndpointDigest == "" ||
		entry.Identity.OptionsDigest == "" ||
		entry.Identity.Modality != embeddingCacheModalityText ||
		entry.Identity.Normalization != embeddingCacheNormalizationUnit ||
		entry.Identity.Dimensions != 3 ||
		entry.InputDigest == "" {
		t.Fatalf("provider identity digest was not fully persisted: %+v", entry)
	}

	resetDenseEmbeddingCacheForTest()
	otherSpace := embedder
	otherSpace.VectorSpace = "provider-identity-space-b"
	if _, err := otherSpace.Embed(context.Background(), []string{input}); err != nil {
		t.Fatalf("embed with different vector space failed: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected different vector space to bypass persistent cache, got %d provider calls", got)
	}
}

func TestDenseHTTPTextEmbedderCacheTTLForcesProviderRefresh(t *testing.T) {
	resetDenseEmbeddingCacheForTest()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float32{{float32(call), 0, 0}}})
	}))
	defer server.Close()

	embedder := DenseHTTPTextEmbedder{
		Endpoint:      server.URL,
		ProviderModel: "provider-dense-test",
		VectorSpace:   "ttl-cache-space",
		CacheDir:      t.TempDir(),
		Client:        server.Client(),
	}
	if _, err := embedder.Embed(context.Background(), []string{"ttl"}); err != nil {
		t.Fatalf("first embed failed: %v", err)
	}
	t.Setenv("NEEDLEX_EMBEDDING_CACHE_TTL", "1ns")
	time.Sleep(time.Millisecond)
	if _, err := embedder.Embed(context.Background(), []string{"ttl"}); err != nil {
		t.Fatalf("second embed failed: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected TTL to expire in-process and persistent cache, got %d provider calls", got)
	}
}

func TestDenseHTTPTextEmbedderCacheCanBeDisabled(t *testing.T) {
	resetDenseEmbeddingCacheForTest()
	t.Setenv("NEEDLEX_EMBEDDING_CACHE", "0")
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float32{{1, 0, 0}}})
	}))
	defer server.Close()

	embedder := DenseHTTPTextEmbedder{
		Endpoint:      server.URL,
		ProviderModel: "provider-dense-test",
		VectorSpace:   "disabled-cache-space",
		CacheDir:      t.TempDir(),
		Client:        server.Client(),
	}
	for i := 0; i < 2; i++ {
		if _, err := embedder.Embed(context.Background(), []string{"same"}); err != nil {
			t.Fatalf("embed %d failed: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected disabled cache to call provider twice, got %d", got)
	}
}

func TestDenseHTTPTextEmbedderSingleflightsConcurrentMisses(t *testing.T) {
	resetDenseEmbeddingCacheForTest()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(25 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float32{{1, 0, 0}}})
	}))
	defer server.Close()

	embedder := DenseHTTPTextEmbedder{
		Endpoint:      server.URL,
		ProviderModel: "provider-dense-test",
		VectorSpace:   "singleflight-cache-space",
		CacheDir:      t.TempDir(),
		Client:        server.Client(),
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := embedder.Embed(context.Background(), []string{"same"}); err != nil {
				t.Errorf("embed failed: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected one provider call for concurrent identical miss, got %d", got)
	}
}

func TestDenseHTTPTextEmbedderNegativeCacheAvoidsRetryStorm(t *testing.T) {
	resetDenseEmbeddingCacheForTest()
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	t.Setenv("NEEDLEX_EMBEDDING_CACHE_NEGATIVE_TTL", "1h")

	embedder := DenseHTTPTextEmbedder{
		Endpoint:      server.URL,
		ProviderModel: "provider-dense-test",
		VectorSpace:   "negative-cache-space",
		CacheDir:      t.TempDir(),
		Client:        server.Client(),
	}
	if _, err := embedder.Embed(context.Background(), []string{"miss"}); err == nil {
		t.Fatal("expected first provider failure")
	}
	if _, err := embedder.Embed(context.Background(), []string{"miss"}); !errors.Is(err, ErrNegativeCacheHit) {
		t.Fatalf("expected negative cache hit, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected negative cache to block retry, got %d provider calls", got)
	}
}

func TestDenseHTTPTextEmbedderDoesNotNegativeCacheCanceledContext(t *testing.T) {
	resetDenseEmbeddingCacheForTest()
	t.Setenv("NEEDLEX_EMBEDDING_CACHE_NEGATIVE_TTL", "1h")
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float32{{1, 0, 0}}})
	}))
	defer server.Close()

	embedder := DenseHTTPTextEmbedder{
		Endpoint:      server.URL,
		ProviderModel: "provider-dense-test",
		VectorSpace:   "context-cache-space",
		CacheDir:      t.TempDir(),
		Client:        server.Client(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := embedder.Embed(ctx, []string{"cancelled"}); err == nil {
		t.Fatal("expected canceled context to fail")
	}
	if _, err := embedder.Embed(context.Background(), []string{"cancelled"}); err != nil {
		t.Fatalf("expected retry after canceled context to reach provider, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected only live retry to reach provider, got %d calls", got)
	}
}

func TestDenseHTTPTextEmbedderReturnsStaleVectorOnProviderError(t *testing.T) {
	resetDenseEmbeddingCacheForTest()
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": [][]float32{{0, 1, 0}}})
	}))
	defer server.Close()
	t.Setenv("NEEDLEX_EMBEDDING_CACHE_STALE_IF_ERROR", "1")

	embedder := DenseHTTPTextEmbedder{
		Endpoint:      server.URL,
		ProviderModel: "provider-dense-test",
		VectorSpace:   "stale-cache-space",
		CacheDir:      t.TempDir(),
		Client:        server.Client(),
	}
	if _, err := embedder.Embed(context.Background(), []string{"stale"}); err != nil {
		t.Fatalf("initial embed failed: %v", err)
	}
	t.Setenv("NEEDLEX_EMBEDDING_CACHE_TTL", "1ns")
	resetDenseEmbeddingCacheForTest()
	fail.Store(true)
	time.Sleep(time.Millisecond)
	vectors, err := embedder.Embed(context.Background(), []string{"stale"})
	if err != nil {
		t.Fatalf("expected stale vector, got %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 3 {
		t.Fatalf("expected stale vector, got %#v", vectors)
	}
}

func TestEmbeddingCachePolicyFromEnv(t *testing.T) {
	t.Setenv("NEEDLEX_EMBEDDING_CACHE", "")
	if !EmbeddingCachePolicyFromEnv("http://localhost:11434/api/embed", "/tmp/cache").Enabled {
		t.Fatal("expected local embedding endpoint to enable cache by default")
	}
	if EmbeddingCachePolicyFromEnv("https://api.example.com/embed", "/tmp/cache").Enabled {
		t.Fatal("expected remote embedding endpoint to keep cache disabled by default")
	}
	t.Setenv("NEEDLEX_EMBEDDING_CACHE", "1")
	if !EmbeddingCachePolicyFromEnv("https://api.example.com/embed", "/tmp/cache").Enabled {
		t.Fatal("expected explicit env enable to override remote default")
	}
	t.Setenv("NEEDLEX_EMBEDDING_CACHE", "0")
	if EmbeddingCachePolicyFromEnv("http://localhost:11434/api/embed", "/tmp/cache").Enabled {
		t.Fatal("expected explicit env disable to override local default")
	}
}

func TestEmbeddingCachePolicyFromConfigEnv(t *testing.T) {
	enabled := true
	stale := false
	policy := EmbeddingCachePolicyFromConfigEnv("https://api.example.com/embed", "/pal/cache", config.SemanticEmbeddingCacheConfig{
		Enabled:      &enabled,
		Dir:          "/configured/cache",
		MaxEntries:   9,
		MaxBytes:     1024,
		TTL:          "24h",
		NegativeTTL:  "30s",
		StaleIfError: &stale,
	})
	if !policy.Enabled || policy.Dir != "/configured/cache" || policy.MaxEntries != 9 || policy.MaxBytes != 1024 || policy.TTL != 24*time.Hour || policy.NegativeTTL != 30*time.Second || policy.StaleIfError {
		t.Fatalf("unexpected config-driven cache policy: %+v", policy)
	}
	t.Setenv("NEEDLEX_EMBEDDING_CACHE", "0")
	if EmbeddingCachePolicyFromConfigEnv("https://api.example.com/embed", "/pal/cache", config.SemanticEmbeddingCacheConfig{Enabled: &enabled}).Enabled {
		t.Fatal("expected env disable to override config enable")
	}
}

func TestPruneEmbeddingCacheHonorsTTLAndBounds(t *testing.T) {
	cacheDir := t.TempDir()
	now := time.Now().UTC()
	writeCacheFile := func(name string, size int, modTime time.Time) {
		t.Helper()
		path := filepath.Join(cacheDir, name)
		if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o600); err != nil {
			t.Fatalf("write cache file %s: %v", name, err)
		}
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("chtimes cache file %s: %v", name, err)
		}
	}
	writeCacheFile("expired.json", 10, now.Add(-2*time.Hour))
	writeCacheFile("expired.neg.json", 10, now.Add(-10*time.Minute))
	writeCacheFile("oldest.json", 10, now.Add(-3*time.Minute))
	writeCacheFile("middle.json", 10, now.Add(-2*time.Minute))
	writeCacheFile("newest.json", 10, now.Add(-time.Minute))

	policy := EmbeddingCachePolicy{MaxEntries: 2, MaxBytes: 25, TTL: time.Hour, NegativeTTL: time.Minute}
	report, err := PruneEmbeddingCache(cacheDir, true, now, policy)
	if err != nil {
		t.Fatalf("dry-run prune embedding cache: %v", err)
	}
	if report.MatchedFiles != 5 || report.RemovedFiles != 3 || report.RemovedBytes != 30 {
		t.Fatalf("unexpected dry-run prune report: %+v", report)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "expired.json")); err != nil {
		t.Fatalf("dry-run must not remove files: %v", err)
	}

	report, err = PruneEmbeddingCache(cacheDir, false, now, policy)
	if err != nil {
		t.Fatalf("prune embedding cache: %v", err)
	}
	if report.RemovedFiles != 3 || report.RemovedBytes != 30 {
		t.Fatalf("unexpected prune report: %+v", report)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "newest.json")); err != nil {
		t.Fatalf("expected newest cache file to remain: %v", err)
	}
}

func resetDenseEmbeddingCacheForTest() {
	denseEmbeddingCache.mu.Lock()
	defer denseEmbeddingCache.mu.Unlock()
	denseEmbeddingCache.tick = 0
	denseEmbeddingCache.items = map[string]embeddingCacheEntry{}
	denseEmbeddingFlights = &embeddingFlightGroup{waiters: map[string]chan struct{}{}}
	embeddingCacheCounters.hits.Store(0)
	embeddingCacheCounters.misses.Store(0)
	embeddingCacheCounters.writes.Store(0)
	embeddingCacheCounters.negativeHits.Store(0)
	embeddingCacheCounters.staleHits.Store(0)
	embeddingCacheCounters.evictions.Store(0)
	embeddingCacheCounters.evictedBytes.Store(0)
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
