package intel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/josepavese/needlex/internal/config"
)

const persistentEmbeddingCacheSchema = "needlex.embedding-cache.v2"

var ErrNegativeCacheHit = errors.New("embedding cache negative hit")

const (
	defaultEmbeddingCacheMaxEntries = 200000
	defaultEmbeddingCacheMaxBytes   = int64(2 << 30)
	defaultEmbeddingCacheTTL        = 30 * 24 * time.Hour
	defaultEmbeddingNegativeTTL     = 2 * time.Minute
	embeddingCacheModalityText      = "text"
	embeddingCacheNormalizationUnit = "unit-l2"
)

type EmbeddingCachePolicy struct {
	Enabled        bool          `json:"enabled"`
	Dir            string        `json:"dir,omitempty"`
	MaxEntries     int           `json:"max_entries"`
	MaxBytes       int64         `json:"max_bytes"`
	TTL            time.Duration `json:"ttl"`
	NegativeTTL    time.Duration `json:"negative_ttl"`
	StaleIfError   bool          `json:"stale_if_error"`
	ExplicitEnable bool          `json:"explicit_enable,omitempty"`
}

type EmbeddingCacheStats struct {
	Enabled       bool   `json:"enabled"`
	Dir           string `json:"dir,omitempty"`
	PositiveFiles int    `json:"positive_files"`
	NegativeFiles int    `json:"negative_files"`
	Bytes         int64  `json:"bytes"`
	MaxEntries    int    `json:"max_entries"`
	MaxBytes      int64  `json:"max_bytes"`
	TTL           string `json:"ttl"`
	NegativeTTL   string `json:"negative_ttl"`
	StaleIfError  bool   `json:"stale_if_error"`
}

type EmbeddingCacheCounters struct {
	Hits         uint64 `json:"hits"`
	Misses       uint64 `json:"misses"`
	Writes       uint64 `json:"writes"`
	NegativeHits uint64 `json:"negative_hits"`
	StaleHits    uint64 `json:"stale_hits"`
	Evictions    uint64 `json:"evictions"`
	EvictedBytes uint64 `json:"evicted_bytes"`
}

type embeddingCacheCounterSet struct {
	hits         atomic.Uint64
	misses       atomic.Uint64
	writes       atomic.Uint64
	negativeHits atomic.Uint64
	staleHits    atomic.Uint64
	evictions    atomic.Uint64
	evictedBytes atomic.Uint64
}

var embeddingCacheCounters embeddingCacheCounterSet

type embeddingCacheIdentity struct {
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	VectorSpace    string `json:"vector_space"`
	EndpointDigest string `json:"endpoint_digest"`
	OptionsDigest  string `json:"options_digest"`
	Modality       string `json:"modality"`
	Normalization  string `json:"normalization"`
	SchemaVersion  string `json:"schema_version"`
	Dimensions     int    `json:"dimensions"`
}

type persistentEmbeddingCacheEntry struct {
	Schema      string                 `json:"schema"`
	KeyHash     string                 `json:"key_hash"`
	Identity    embeddingCacheIdentity `json:"identity"`
	InputDigest string                 `json:"input_digest"`
	ModelID     string                 `json:"model_id"`
	Dimensions  int                    `json:"dimensions"`
	Normalized  bool                   `json:"normalized"`
	StoredAt    time.Time              `json:"stored_at"`
	Vector      []float32              `json:"vector"`
}

type negativeEmbeddingCacheEntry struct {
	Schema   string    `json:"schema"`
	KeyHash  string    `json:"key_hash"`
	Error    string    `json:"error"`
	StoredAt time.Time `json:"stored_at"`
}

func (e DenseHTTPTextEmbedder) cacheGet(key string) ([]float32, bool) {
	policy := e.embeddingCachePolicy()
	if !policy.Enabled {
		return nil, false
	}
	if vector, ok := denseEmbeddingCache.get(key, policy.TTL); ok {
		embeddingCacheCounters.hits.Add(1)
		return vector, true
	}
	vector, ok := e.persistentCacheGet(key, false)
	if !ok {
		embeddingCacheCounters.misses.Add(1)
		return nil, false
	}
	embeddingCacheCounters.hits.Add(1)
	denseEmbeddingCache.set(key, vector)
	return vector, true
}

func (e DenseHTTPTextEmbedder) cacheSet(key string, vector []float32) {
	if !e.embeddingCachePolicy().Enabled {
		return
	}
	denseEmbeddingCache.set(key, vector)
	e.persistentCacheSet(key, vector)
	embeddingCacheCounters.writes.Add(1)
}

func (e DenseHTTPTextEmbedder) persistentCacheGet(key string, allowExpired bool) ([]float32, bool) {
	policy := e.embeddingCachePolicy()
	path := embeddingCachePath(policy.Dir, key, ".json")
	if path == "" || !policy.Enabled {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var entry persistentEmbeddingCacheEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, false
	}
	if !entry.validFor(key, policy, allowExpired) {
		return nil, false
	}
	vector := cloneFloat32Vector(entry.Vector)
	normalizeFloat32(vector)
	_ = os.Chtimes(path, time.Now(), time.Now())
	return vector, true
}

func (e DenseHTTPTextEmbedder) persistentCacheSet(key string, vector []float32) {
	policy := e.embeddingCachePolicy()
	path := embeddingCachePath(policy.Dir, key, ".json")
	if path == "" || len(vector) == 0 || !policy.Enabled {
		return
	}
	identity := e.embeddingCacheIdentity(len(vector))
	entry := persistentEmbeddingCacheEntry{
		Schema:      persistentEmbeddingCacheSchema,
		KeyHash:     embeddingCacheKeyHash(key),
		Identity:    identity,
		InputDigest: embeddingCacheInputDigestFromKey(key),
		ModelID:     identity.VectorSpace,
		Dimensions:  len(vector),
		Normalized:  true,
		StoredAt:    time.Now().UTC(),
		Vector:      cloneFloat32Vector(vector),
	}
	normalizeFloat32(entry.Vector)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.%d.%d.tmp", path, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return
	}
	_ = os.Remove(embeddingCachePath(policy.Dir, key, ".neg.json"))
	now := time.Now().UTC()
	if embeddingCachePruneGate.shouldPrune(policy.Dir, now) {
		_, _ = PruneEmbeddingCache(policy.Dir, false, now, policy)
	}
}

func (entry persistentEmbeddingCacheEntry) validFor(key string, policy EmbeddingCachePolicy, allowExpired bool) bool {
	if entry.Schema != persistentEmbeddingCacheSchema {
		return false
	}
	if entry.KeyHash != embeddingCacheKeyHash(key) {
		return false
	}
	if !entry.validIdentity(key) {
		return false
	}
	if entry.Dimensions <= 0 || len(entry.Vector) != entry.Dimensions {
		return false
	}
	for _, value := range entry.Vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
	}
	if !allowExpired && policy.TTL > 0 && time.Since(entry.StoredAt) > policy.TTL {
		return false
	}
	return true
}

func (entry persistentEmbeddingCacheEntry) validIdentity(key string) bool {
	if entry.InputDigest == "" {
		return false
	}
	identity := entry.Identity
	if identity.Provider == "" ||
		identity.VectorSpace == "" ||
		identity.EndpointDigest == "" ||
		identity.OptionsDigest == "" ||
		identity.Modality == "" ||
		identity.Normalization == "" ||
		identity.SchemaVersion != persistentEmbeddingCacheSchema {
		return false
	}
	if identity.Dimensions <= 0 || identity.Dimensions != entry.Dimensions {
		return false
	}
	if !entry.Normalized {
		return false
	}
	if entry.ModelID != "" && entry.ModelID != identity.VectorSpace {
		return false
	}
	keyIdentity := identity
	keyIdentity.Dimensions = 0
	if embeddingCacheIdentityKey(keyIdentity, entry.InputDigest) != strings.TrimSpace(key) {
		return false
	}
	return entry.KeyHash == embeddingCacheKeyHash(embeddingCacheIdentityKey(keyIdentity, entry.InputDigest))
}

func (e DenseHTTPTextEmbedder) embeddingCachePolicy() EmbeddingCachePolicy {
	if e.CachePolicySet {
		return e.CachePolicy
	}
	return EmbeddingCachePolicyFromEnv(e.Endpoint, e.CacheDir)
}

func EmbeddingCachePolicyFromEnv(endpoint, cacheDir string) EmbeddingCachePolicy {
	return EmbeddingCachePolicyFromConfigEnv(endpoint, cacheDir, config.SemanticEmbeddingCacheConfig{})
}

func EmbeddingCachePolicyFromConfigEnv(endpoint, cacheDir string, cfg config.SemanticEmbeddingCacheConfig) EmbeddingCachePolicy {
	policy := EmbeddingCachePolicy{
		Dir:          strings.TrimSpace(firstNonEmpty(cfg.Dir, cacheDir)),
		MaxEntries:   positiveInt(cfg.MaxEntries, defaultEmbeddingCacheMaxEntries),
		MaxBytes:     positiveInt64(cfg.MaxBytes, defaultEmbeddingCacheMaxBytes),
		TTL:          configDuration(cfg.TTL, defaultEmbeddingCacheTTL),
		NegativeTTL:  configDuration(cfg.NegativeTTL, defaultEmbeddingNegativeTTL),
		StaleIfError: boolPtrValue(cfg.StaleIfError, true),
	}
	if cfg.Enabled != nil {
		policy.Enabled = *cfg.Enabled
		policy.ExplicitEnable = true
	} else {
		policy.Enabled = localEmbeddingEndpoint(endpoint)
	}
	if value := strings.TrimSpace(os.Getenv("NEEDLEX_EMBEDDING_CACHE_DIR")); value != "" {
		policy.Dir = value
	}
	policy.MaxEntries = envInt("NEEDLEX_EMBEDDING_CACHE_MAX_ENTRIES", policy.MaxEntries)
	policy.MaxBytes = envInt64("NEEDLEX_EMBEDDING_CACHE_MAX_BYTES", policy.MaxBytes)
	policy.TTL = envDuration("NEEDLEX_EMBEDDING_CACHE_TTL", policy.TTL)
	policy.NegativeTTL = envDuration("NEEDLEX_EMBEDDING_CACHE_NEGATIVE_TTL", policy.NegativeTTL)
	policy.StaleIfError = envBool("NEEDLEX_EMBEDDING_CACHE_STALE_IF_ERROR", policy.StaleIfError)
	switch strings.ToLower(strings.TrimSpace(os.Getenv("NEEDLEX_EMBEDDING_CACHE"))) {
	case "0", "false", "no", "off", "disabled":
		policy.Enabled = false
		policy.ExplicitEnable = true
	case "1", "true", "yes", "on", "enabled":
		policy.Enabled = true
		policy.ExplicitEnable = true
	}
	if policy.MaxEntries < 0 {
		policy.MaxEntries = 0
	}
	if policy.MaxBytes < 0 {
		policy.MaxBytes = 0
	}
	return policy
}

func EmbeddingCacheStatsForDir(cacheDir string, policy EmbeddingCachePolicy) EmbeddingCacheStats {
	stats := EmbeddingCacheStats{
		Enabled:      policy.Enabled,
		Dir:          strings.TrimSpace(cacheDir),
		MaxEntries:   policy.MaxEntries,
		MaxBytes:     policy.MaxBytes,
		TTL:          policy.TTL.String(),
		NegativeTTL:  policy.NegativeTTL.String(),
		StaleIfError: policy.StaleIfError,
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return stats
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		stats.Bytes += info.Size()
		if strings.HasSuffix(entry.Name(), ".neg.json") {
			stats.NegativeFiles++
		} else if strings.HasSuffix(entry.Name(), ".json") {
			stats.PositiveFiles++
		}
	}
	return stats
}

func embeddingCachePath(dir, key, suffix string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" || strings.TrimSpace(key) == "" {
		return ""
	}
	return filepath.Join(dir, embeddingCacheKeyHash(key)+suffix)
}

func embeddingCacheKeyHash(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])
}

func (e DenseHTTPTextEmbedder) embeddingCacheIdentity(dimensions int) embeddingCacheIdentity {
	endpointDigest := embeddingCacheDigest("endpoint", canonicalEmbeddingEndpoint(e.Endpoint))
	identity := embeddingCacheIdentity{
		Provider:       DenseEmbeddingSource,
		Model:          strings.TrimSpace(e.ProviderModel),
		VectorSpace:    strings.TrimSpace(e.ModelID()),
		EndpointDigest: endpointDigest,
		Modality:       embeddingCacheModalityText,
		Normalization:  embeddingCacheNormalizationUnit,
		SchemaVersion:  persistentEmbeddingCacheSchema,
		Dimensions:     dimensions,
	}
	identity.OptionsDigest = embeddingCacheDigest(
		"embedding-options",
		identity.Provider,
		identity.Model,
		identity.VectorSpace,
		identity.EndpointDigest,
		identity.Modality,
		identity.Normalization,
		identity.SchemaVersion,
	)
	return identity
}

func embeddingCacheIdentityKey(identity embeddingCacheIdentity, inputDigest string) string {
	parts := []string{
		identity.SchemaVersion,
		identity.Provider,
		identity.Model,
		identity.VectorSpace,
		identity.EndpointDigest,
		identity.OptionsDigest,
		identity.Modality,
		identity.Normalization,
		strings.TrimSpace(inputDigest),
	}
	return strings.Join(parts, "\x00")
}

func embeddingCacheInputDigestFromKey(key string) string {
	parts := strings.Split(strings.TrimSpace(key), "\x00")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func embeddingInputDigest(input string) string {
	return embeddingCacheDigest("input", strings.TrimSpace(input))
}

func embeddingCacheDigest(parts ...string) string {
	h := sha256.New()
	for i, part := range parts {
		if i > 0 {
			_, _ = h.Write([]byte{0})
		}
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func canonicalEmbeddingEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return endpoint
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	return parsed.String()
}

func (e DenseHTTPTextEmbedder) negativeCacheHit(key string) error {
	policy := e.embeddingCachePolicy()
	path := embeddingCachePath(policy.Dir, key, ".neg.json")
	if path == "" || !policy.Enabled || policy.NegativeTTL <= 0 {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var entry negativeEmbeddingCacheEntry
	if json.Unmarshal(raw, &entry) != nil || entry.Schema != persistentEmbeddingCacheSchema || entry.KeyHash != embeddingCacheKeyHash(key) {
		return nil
	}
	if time.Since(entry.StoredAt) > policy.NegativeTTL {
		_ = os.Remove(path)
		return nil
	}
	embeddingCacheCounters.negativeHits.Add(1)
	return fmt.Errorf("%w: %s", ErrNegativeCacheHit, entry.Error)
}

func (e DenseHTTPTextEmbedder) setNegativeCache(keys []string, cause error) {
	policy := e.embeddingCachePolicy()
	if !policy.Enabled || policy.NegativeTTL <= 0 || cause == nil {
		return
	}
	if os.MkdirAll(policy.Dir, 0o755) != nil {
		return
	}
	for _, key := range keys {
		path := embeddingCachePath(policy.Dir, key, ".neg.json")
		if path == "" {
			continue
		}
		entry := negativeEmbeddingCacheEntry{Schema: persistentEmbeddingCacheSchema, KeyHash: embeddingCacheKeyHash(key), Error: cause.Error(), StoredAt: time.Now().UTC()}
		raw, err := json.Marshal(entry)
		if err == nil {
			_ = os.WriteFile(path, raw, 0o600)
		}
	}
}

func (e DenseHTTPTextEmbedder) staleVectors(keys []string, indexes map[string][]int, vectors [][]float32) bool {
	policy := e.embeddingCachePolicy()
	if !policy.Enabled || !policy.StaleIfError {
		return false
	}
	for _, key := range keys {
		vector, ok := e.persistentCacheGet(key, true)
		if !ok {
			return false
		}
		for _, idx := range indexes[key] {
			vectors[idx] = cloneFloat32Vector(vector)
		}
	}
	embeddingCacheCounters.staleHits.Add(uint64(len(keys)))
	return true
}

type embeddingCacheFile struct {
	path     string
	size     int64
	modTime  time.Time
	positive bool
}

type EmbeddingCachePruneReport struct {
	RemovedFiles int      `json:"removed_files"`
	RemovedBytes int64    `json:"removed_bytes"`
	MatchedFiles int      `json:"matched_files"`
	MatchedBytes int64    `json:"matched_bytes"`
	DryRun       bool     `json:"dry_run,omitempty"`
	RemovedPaths []string `json:"removed_paths,omitempty"`
}

func PruneEmbeddingCache(cacheDir string, dryRun bool, now time.Time, policy EmbeddingCachePolicy) (EmbeddingCachePruneReport, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	files, err := embeddingCacheFiles(cacheDir)
	if err != nil {
		return EmbeddingCachePruneReport{}, err
	}
	report := EmbeddingCachePruneReport{DryRun: dryRun, MatchedFiles: len(files)}
	for _, file := range files {
		report.MatchedBytes += file.size
	}
	remove := embeddingCacheRemovalSet(files, now, policy)
	for _, file := range remove {
		report.RemovedFiles++
		report.RemovedBytes += file.size
		report.RemovedPaths = append(report.RemovedPaths, file.path)
		if !dryRun {
			_ = os.Remove(file.path)
		}
	}
	if !dryRun && report.RemovedFiles > 0 {
		embeddingCacheCounters.evictions.Add(uint64(report.RemovedFiles))
		embeddingCacheCounters.evictedBytes.Add(uint64(report.RemovedBytes))
	}
	return report, nil
}

func embeddingCacheFiles(cacheDir string) ([]embeddingCacheFile, error) {
	if strings.TrimSpace(cacheDir) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := []embeddingCacheFile{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, embeddingCacheFile{
			path:     filepath.Join(cacheDir, entry.Name()),
			size:     info.Size(),
			modTime:  info.ModTime(),
			positive: !strings.HasSuffix(entry.Name(), ".neg.json"),
		})
	}
	return out, nil
}

func embeddingCacheRemovalSet(files []embeddingCacheFile, now time.Time, policy EmbeddingCachePolicy) []embeddingCacheFile {
	remove := []embeddingCacheFile{}
	kept := []embeddingCacheFile{}
	for _, file := range files {
		ttl := policy.TTL
		if !file.positive {
			ttl = policy.NegativeTTL
		}
		if ttl > 0 && now.Sub(file.modTime) > ttl {
			remove = append(remove, file)
			continue
		}
		kept = append(kept, file)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].modTime.Before(kept[j].modTime) })
	total := int64(0)
	positive := 0
	for _, file := range kept {
		total += file.size
		if file.positive {
			positive++
		}
	}
	for _, file := range kept {
		if (policy.MaxBytes <= 0 || total <= policy.MaxBytes) && (policy.MaxEntries <= 0 || positive <= policy.MaxEntries) {
			break
		}
		remove = append(remove, file)
		total -= file.size
		if file.positive {
			positive--
		}
	}
	return remove
}

func localEmbeddingEndpoint(endpoint string) bool {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.Trim(parsed.Hostname(), "[]"))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func positiveInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveInt64(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func configDuration(raw string, fallback time.Duration) time.Duration {
	if value, err := time.ParseDuration(strings.TrimSpace(raw)); err == nil && value >= 0 {
		return value
	}
	return fallback
}

func boolPtrValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func SnapshotEmbeddingCacheCounters() EmbeddingCacheCounters {
	return EmbeddingCacheCounters{
		Hits:         embeddingCacheCounters.hits.Load(),
		Misses:       embeddingCacheCounters.misses.Load(),
		Writes:       embeddingCacheCounters.writes.Load(),
		NegativeHits: embeddingCacheCounters.negativeHits.Load(),
		StaleHits:    embeddingCacheCounters.staleHits.Load(),
		Evictions:    embeddingCacheCounters.evictions.Load(),
		EvictedBytes: embeddingCacheCounters.evictedBytes.Load(),
	}
}

func DiffEmbeddingCacheCounters(before, after EmbeddingCacheCounters) EmbeddingCacheCounters {
	return EmbeddingCacheCounters{
		Hits:         subtractCounter(after.Hits, before.Hits),
		Misses:       subtractCounter(after.Misses, before.Misses),
		Writes:       subtractCounter(after.Writes, before.Writes),
		NegativeHits: subtractCounter(after.NegativeHits, before.NegativeHits),
		StaleHits:    subtractCounter(after.StaleHits, before.StaleHits),
		Evictions:    subtractCounter(after.Evictions, before.Evictions),
		EvictedBytes: subtractCounter(after.EvictedBytes, before.EvictedBytes),
	}
}

func (c EmbeddingCacheCounters) Empty() bool {
	return c.Hits == 0 &&
		c.Misses == 0 &&
		c.Writes == 0 &&
		c.NegativeHits == 0 &&
		c.StaleHits == 0 &&
		c.Evictions == 0 &&
		c.EvictedBytes == 0
}

func subtractCounter(after, before uint64) uint64 {
	if after < before {
		return 0
	}
	return after - before
}

func envBool(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on", "enabled":
		return true
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil {
		return fallback
	}
	return value
}

func envInt64(key string, fallback int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(key)), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	if value, err := time.ParseDuration(raw); err == nil {
		return value
	}
	if hours, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.Duration(hours) * time.Hour
	}
	return fallback
}

type embeddingFlightGroup struct {
	mu      sync.Mutex
	waiters map[string]chan struct{}
}

var denseEmbeddingFlights = &embeddingFlightGroup{waiters: map[string]chan struct{}{}}

type embeddingPruneGate struct {
	mu   sync.Mutex
	last map[string]time.Time
}

var embeddingCachePruneGate = &embeddingPruneGate{last: map[string]time.Time{}}

func (g *embeddingPruneGate) shouldPrune(dir string, now time.Time) bool {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if previous, ok := g.last[dir]; ok && now.Sub(previous) < time.Minute {
		return false
	}
	g.last[dir] = now
	return true
}

func (g *embeddingFlightGroup) lock(keys []string) func() {
	owned := []string{}
	for _, key := range sortedUnique(keys) {
		for {
			g.mu.Lock()
			waiter := g.waiters[key]
			if waiter == nil {
				g.waiters[key] = make(chan struct{})
				owned = append(owned, key)
				g.mu.Unlock()
				break
			}
			g.mu.Unlock()
			<-waiter
		}
	}
	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		for _, key := range owned {
			close(g.waiters[key])
			delete(g.waiters, key)
		}
	}
}

func sortedUnique(items []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
