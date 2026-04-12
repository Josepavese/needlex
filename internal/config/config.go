package config

type Config struct {
	Runtime   RuntimeConfig   `json:"runtime"`
	Fetch     FetchConfig     `json:"fetch,omitempty"`
	Policy    PolicyConfig    `json:"policy"`
	Budget    BudgetConfig    `json:"budget"`
	Models    ModelsConfig    `json:"models"`
	Discovery DiscoveryConfig `json:"discovery,omitempty"`
	Semantic  SemanticConfig  `json:"semantic,omitempty"`
	Memory    MemoryConfig    `json:"memory,omitempty"`
}

type RuntimeConfig struct {
	MaxPages  int   `json:"max_pages"`
	MaxDepth  int   `json:"max_depth"`
	TimeoutMS int64 `json:"timeout_ms"`
	MaxBytes  int64 `json:"max_bytes"`
	LaneMax   int   `json:"lane_max"`
}

type FetchConfig struct {
	Profile               string `json:"profile,omitempty"`
	RetryProfile          string `json:"retry_profile,omitempty"`
	BlockedRetryBackoffMS int64  `json:"blocked_retry_backoff_ms,omitempty"`
	BlockedRetryJitterMS  int64  `json:"blocked_retry_jitter_ms,omitempty"`
	TimeoutRetryBackoffMS int64  `json:"timeout_retry_backoff_ms,omitempty"`
	TimeoutRetryJitterMS  int64  `json:"timeout_retry_jitter_ms,omitempty"`
	PerHostMinGapMS       int64  `json:"per_host_min_gap_ms,omitempty"`
	PerHostJitterMS       int64  `json:"per_host_jitter_ms,omitempty"`
}

type PolicyConfig struct {
	ThresholdConflict   float64 `json:"threshold_conflict"`
	ThresholdAmbiguity  float64 `json:"threshold_ambiguity"`
	ThresholdCoverage   float64 `json:"threshold_coverage"`
	ThresholdConfidence float64 `json:"threshold_confidence"`
}

type BudgetConfig struct {
	MaxTokens    int   `json:"max_tokens"`
	MaxLatencyMS int64 `json:"max_latency_ms"`
}

type ModelsConfig struct {
	Backend             string `json:"backend,omitempty"`
	BaseURL             string `json:"base_url,omitempty"`
	APIKey              string `json:"api_key,omitempty"`
	Router              string `json:"router"`
	Judge               string `json:"judge"`
	Extractor           string `json:"extractor"`
	Formatter           string `json:"formatter"`
	MicroTimeoutMS      int64  `json:"micro_timeout_ms,omitempty"`
	StructuredTimeoutMS int64  `json:"structured_timeout_ms,omitempty"`
	SpecialistTimeoutMS int64  `json:"specialist_timeout_ms,omitempty"`
}

type DiscoveryConfig struct {
	ProviderChain                 string `json:"provider_chain,omitempty"`
	BraveAPIKey                   string `json:"brave_api_key,omitempty"`
	ProviderFailureCooldownMS     int64  `json:"provider_failure_cooldown_ms,omitempty"`
	ProviderBlockedCooldownMS     int64  `json:"provider_blocked_cooldown_ms,omitempty"`
	ProviderTimeoutCooldownMS     int64  `json:"provider_timeout_cooldown_ms,omitempty"`
	ProviderUnavailableCooldownMS int64  `json:"provider_unavailable_cooldown_ms,omitempty"`
}

type SemanticConfig struct {
	Enabled             bool    `json:"enabled,omitempty"`
	Backend             string  `json:"backend,omitempty"`
	BaseURL             string  `json:"base_url,omitempty"`
	Model               string  `json:"model,omitempty"`
	TimeoutMS           int64   `json:"timeout_ms,omitempty"`
	FailureCooldownMS   int64   `json:"failure_cooldown_ms,omitempty"`
	SimilarityThreshold float64 `json:"similarity_threshold,omitempty"`
	DominanceDelta      float64 `json:"dominance_delta,omitempty"`
	MaxCandidates       int     `json:"max_candidates,omitempty"`
}

type MemoryConfig struct {
	Enabled          bool   `json:"enabled,omitempty"`
	Backend          string `json:"backend,omitempty"`
	Path             string `json:"path,omitempty"`
	MaxDocuments     int    `json:"max_documents,omitempty"`
	MaxEdges         int    `json:"max_edges,omitempty"`
	MaxEmbeddings    int    `json:"max_embeddings,omitempty"`
	EmbeddingBackend string `json:"embedding_backend,omitempty"`
	EmbeddingModel   string `json:"embedding_model,omitempty"`
	VectorMode       string `json:"vector_mode,omitempty"`
	VectorEngine     string `json:"vector_engine,omitempty"`
	PrunePolicy      string `json:"prune_policy,omitempty"`
}
