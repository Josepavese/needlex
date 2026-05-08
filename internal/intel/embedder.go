package intel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/josepavese/needlex/internal/config"
)

const (
	DenseSemanticVectorSpace = "needlex-dense-vector-space-v1"
	DenseEmbeddingSource     = "dense-http"
	denseEmbeddingCacheMax   = 4096
)

type TextEmbedder interface {
	ModelID() string
	Embed(ctx context.Context, inputs []string) ([][]float32, error)
}

func SemanticConfigured(cfg config.Config) bool {
	return strings.TrimSpace(cfg.Semantic.EmbeddingURL) != "" &&
		strings.TrimSpace(cfg.Semantic.ProviderModel) != "" &&
		strings.TrimSpace(cfg.Semantic.VectorSpace) != ""
}

func NewTextEmbedder(cfg config.Config, client *http.Client) TextEmbedder {
	return newDenseHTTPTextEmbedder(cfg, client, "")
}

func NewTextEmbedderWithCacheDir(cfg config.Config, client *http.Client, cacheDir string) TextEmbedder {
	return newDenseHTTPTextEmbedder(cfg, client, cacheDir)
}

func newDenseHTTPTextEmbedder(cfg config.Config, client *http.Client, cacheDir string) DenseHTTPTextEmbedder {
	policy := EmbeddingCachePolicyFromConfigEnv(cfg.Semantic.EmbeddingURL, cacheDir, cfg.Semantic.EmbeddingCache)
	return DenseHTTPTextEmbedder{
		Endpoint:       cfg.Semantic.EmbeddingURL,
		ProviderModel:  cfg.Semantic.ProviderModel,
		VectorSpace:    cfg.Semantic.VectorSpace,
		TimeoutMS:      cfg.Semantic.TimeoutMS,
		Client:         client,
		CacheDir:       strings.TrimSpace(cacheDir),
		CachePolicy:    policy,
		CachePolicySet: true,
	}
}

type DenseHTTPTextEmbedder struct {
	Endpoint       string
	ProviderModel  string
	VectorSpace    string
	TimeoutMS      int64
	Client         *http.Client
	CacheDir       string
	CachePolicy    EmbeddingCachePolicy
	CachePolicySet bool
}

func (e DenseHTTPTextEmbedder) ModelID() string {
	if vectorSpace := strings.TrimSpace(e.VectorSpace); vectorSpace != "" {
		return vectorSpace
	}
	return derivedDenseVectorSpace(e.Endpoint, e.ProviderModel)
}

func (e DenseHTTPTextEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	clean := compactEmbedInputs(inputs)
	if len(clean) == 0 {
		return nil, nil
	}
	endpoint := strings.TrimSpace(e.Endpoint)
	if endpoint == "" {
		return nil, nil
	}
	vectors := make([][]float32, len(clean))
	missingInputs, missingKeys, _ := e.embeddingCacheMisses(clean, vectors)
	if len(missingInputs) == 0 {
		return vectors, nil
	}
	unlock := denseEmbeddingFlights.lock(missingKeys)
	defer unlock()
	missingInputs, missingKeys, missingIndexes := e.embeddingCacheMisses(clean, vectors)
	if len(missingInputs) == 0 {
		return vectors, nil
	}
	if err := e.firstNegativeCacheHit(missingKeys); err != nil {
		return nil, err
	}
	fetched, err := e.fetchMissingEmbeddings(ctx, endpoint, missingInputs)
	if err != nil {
		if e.staleVectors(missingKeys, missingIndexes, vectors) {
			return vectors, nil
		}
		if ctx.Err() == nil {
			e.setNegativeCache(missingKeys, err)
		}
		return nil, err
	}
	e.mergeFetchedEmbeddings(vectors, fetched, missingKeys, missingIndexes)
	return vectors, nil
}

func (e DenseHTTPTextEmbedder) firstNegativeCacheHit(keys []string) error {
	for _, key := range keys {
		if err := e.negativeCacheHit(key); err != nil {
			return err
		}
	}
	return nil
}

func (e DenseHTTPTextEmbedder) fetchMissingEmbeddings(ctx context.Context, endpoint string, inputs []string) ([][]float32, error) {
	payload := map[string]any{"input": inputs}
	if providerModel := strings.TrimSpace(e.ProviderModel); providerModel != "" {
		payload["model"] = providerModel
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode embedding request: %w", err)
	}
	if e.TimeoutMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(e.TimeoutMS)*time.Millisecond)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send embedding request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding endpoint status %d", resp.StatusCode)
	}
	fetched, err := decodeEmbeddingResponse(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if len(fetched) != len(inputs) {
		return nil, fmt.Errorf("embedding endpoint returned %d vectors for %d inputs", len(fetched), len(inputs))
	}
	return fetched, nil
}

func (e DenseHTTPTextEmbedder) mergeFetchedEmbeddings(vectors [][]float32, fetched [][]float32, keys []string, indexes map[string][]int) {
	for i := range fetched {
		normalizeFloat32(fetched[i])
		e.cacheSet(keys[i], fetched[i])
		for _, idx := range indexes[keys[i]] {
			vectors[idx] = cloneFloat32Vector(fetched[i])
		}
	}
}

func (e DenseHTTPTextEmbedder) embeddingCacheMisses(clean []string, vectors [][]float32) ([]string, []string, map[string][]int) {
	missingInputs := []string{}
	missingKeys := []string{}
	missingIndexes := map[string][]int{}
	for i, input := range clean {
		key := e.embeddingCacheKey(input)
		if vector, ok := e.cacheGet(key); ok {
			vectors[i] = vector
			continue
		}
		if _, ok := missingIndexes[key]; !ok {
			missingInputs = append(missingInputs, input)
			missingKeys = append(missingKeys, key)
		}
		missingIndexes[key] = append(missingIndexes[key], i)
	}
	return missingInputs, missingKeys, missingIndexes
}

func (e DenseHTTPTextEmbedder) embeddingCacheKey(input string) string {
	return embeddingCacheIdentityKey(e.embeddingCacheIdentity(0), embeddingInputDigest(input))
}

func derivedDenseVectorSpace(endpoint, providerModel string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(DenseEmbeddingSource + "|" + endpoint + "|" + strings.TrimSpace(providerModel)))
	return DenseEmbeddingSource + ":" + hex.EncodeToString(sum[:8])
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

type embeddingCacheEntry struct {
	Vector   []float32
	Tick     uint64
	StoredAt time.Time
}

type embeddingCache struct {
	mu    sync.Mutex
	tick  uint64
	items map[string]embeddingCacheEntry
}

var denseEmbeddingCache = &embeddingCache{items: map[string]embeddingCacheEntry{}}

func (c *embeddingCache) get(key string, ttl time.Duration) ([]float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[key]
	if !ok {
		return nil, false
	}
	if ttl > 0 && time.Since(entry.StoredAt) > ttl {
		delete(c.items, key)
		return nil, false
	}
	c.tick++
	entry.Tick = c.tick
	c.items[key] = entry
	return cloneFloat32Vector(entry.Vector), true
}

func (c *embeddingCache) set(key string, vector []float32) {
	if strings.TrimSpace(key) == "" || len(vector) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tick++
	c.items[key] = embeddingCacheEntry{Vector: cloneFloat32Vector(vector), Tick: c.tick, StoredAt: time.Now().UTC()}
	if len(c.items) > denseEmbeddingCacheMax {
		c.evictOldest()
	}
}

func (c *embeddingCache) evictOldest() {
	var oldestKey string
	var oldestTick uint64
	first := true
	for key, entry := range c.items {
		if first || entry.Tick < oldestTick {
			first = false
			oldestKey = key
			oldestTick = entry.Tick
		}
	}
	if oldestKey != "" {
		delete(c.items, oldestKey)
	}
}

func cloneFloat32Vector(vector []float32) []float32 {
	if len(vector) == 0 {
		return nil
	}
	out := make([]float32, len(vector))
	copy(out, vector)
	return out
}

type embeddingWireResponse struct {
	Embedding  json.RawMessage          `json:"embedding"`
	Embeddings []json.RawMessage        `json:"embeddings"`
	Vectors    []json.RawMessage        `json:"vectors"`
	Data       []embeddingWireDataEntry `json:"data"`
}

type embeddingWireDataEntry struct {
	Index     int             `json:"index"`
	Embedding json.RawMessage `json:"embedding"`
}

func decodeEmbeddingResponse(reader io.Reader) ([][]float32, error) {
	var wire embeddingWireResponse
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	if len(wire.Embeddings) > 0 {
		return decodeRawVectors(wire.Embeddings)
	}
	if len(wire.Vectors) > 0 {
		return decodeRawVectors(wire.Vectors)
	}
	if len(wire.Data) > 0 {
		sort.SliceStable(wire.Data, func(i, j int) bool { return wire.Data[i].Index < wire.Data[j].Index })
		raw := make([]json.RawMessage, 0, len(wire.Data))
		seen := map[int]struct{}{}
		for idx, entry := range wire.Data {
			if entry.Index < 0 || entry.Index >= len(wire.Data) {
				return nil, fmt.Errorf("embedding response data index %d out of range for %d vectors", entry.Index, len(wire.Data))
			}
			if _, ok := seen[entry.Index]; ok {
				return nil, fmt.Errorf("embedding response data index %d is duplicated", entry.Index)
			}
			seen[entry.Index] = struct{}{}
			if len(entry.Embedding) == 0 {
				return nil, fmt.Errorf("embedding response data[%d] contains no vector", idx)
			}
			raw = append(raw, entry.Embedding)
		}
		return decodeRawVectors(raw)
	}
	if len(wire.Embedding) > 0 {
		vector, err := decodeRawVector(wire.Embedding)
		if err != nil {
			return nil, err
		}
		return [][]float32{vector}, nil
	}
	return nil, fmt.Errorf("embedding response contains no vectors")
}

func decodeRawVectors(raw []json.RawMessage) ([][]float32, error) {
	vectors := make([][]float32, 0, len(raw))
	for _, item := range raw {
		vector, err := decodeRawVector(item)
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func decodeRawVector(raw json.RawMessage) ([]float32, error) {
	var values []float64
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode embedding vector: %w", err)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("embedding vector must not be empty")
	}
	vector := make([]float32, len(values))
	for i, value := range values {
		vector[i] = float32(value)
	}
	return vector, nil
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

func zeroFloat32Vector(vector []float32) bool {
	for _, value := range vector {
		if value != 0 {
			return false
		}
	}
	return true
}

func cosineSimilarityFloat32(left, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	dot := 0.0
	normLeft := 0.0
	normRight := 0.0
	for i := range left {
		leftValue := float64(left[i])
		rightValue := float64(right[i])
		dot += leftValue * rightValue
		normLeft += leftValue * leftValue
		normRight += rightValue * rightValue
	}
	if normLeft == 0 || normRight == 0 {
		return 0
	}
	return dot / (math.Sqrt(normLeft) * math.Sqrt(normRight))
}
