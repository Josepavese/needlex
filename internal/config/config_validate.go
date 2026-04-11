package config

import (
	"errors"
	"fmt"
	"strings"
)

func (c Config) Validate() error {
	errs := []error{}
	errs = append(errs, validateRuntime(c.Runtime)...)
	errs = append(errs, validateFetch(c.Fetch)...)
	errs = append(errs, validatePolicy(c.Policy)...)
	errs = append(errs, validateBudget(c.Budget)...)
	errs = append(errs, validateModels(c.Models)...)
	errs = append(errs, validateDiscovery(c.Discovery)...)
	errs = append(errs, validateSemantic(c.Semantic)...)
	errs = append(errs, validateMemory(c.Memory)...)

	if len(errs) == 0 {
		return nil
	}
	return errorsJoin(errs...)
}

func validateFetch(fetch FetchConfig) []error {
	errs := []error{}
	for field, value := range map[string]string{
		"fetch.profile":       fetch.Profile,
		"fetch.retry_profile": fetch.RetryProfile,
	} {
		switch strings.TrimSpace(value) {
		case "", "standard", "browser_like", "hardened":
		default:
			errs = append(errs, fmt.Errorf("%s must be one of standard, browser_like, hardened", field))
		}
	}
	for field, value := range map[string]int64{
		"fetch.blocked_retry_backoff_ms": fetch.BlockedRetryBackoffMS,
		"fetch.blocked_retry_jitter_ms":  fetch.BlockedRetryJitterMS,
		"fetch.per_host_min_gap_ms":      fetch.PerHostMinGapMS,
		"fetch.per_host_jitter_ms":       fetch.PerHostJitterMS,
		"fetch.timeout_retry_backoff_ms": fetch.TimeoutRetryBackoffMS,
		"fetch.timeout_retry_jitter_ms":  fetch.TimeoutRetryJitterMS,
	} {
		if value < 0 {
			errs = append(errs, fmt.Errorf("%s must be >= 0", field))
		}
	}
	return errs
}

func validateRuntime(runtime RuntimeConfig) []error {
	errs := []error{}
	if runtime.MaxPages <= 0 {
		errs = append(errs, fmt.Errorf("runtime.max_pages must be > 0"))
	}
	if runtime.MaxDepth <= 0 {
		errs = append(errs, fmt.Errorf("runtime.max_depth must be > 0"))
	}
	if runtime.TimeoutMS <= 0 {
		errs = append(errs, fmt.Errorf("runtime.timeout_ms must be > 0"))
	}
	if runtime.MaxBytes <= 0 {
		errs = append(errs, fmt.Errorf("runtime.max_bytes must be > 0"))
	}
	if runtime.LaneMax < 0 || runtime.LaneMax > 4 {
		errs = append(errs, fmt.Errorf("runtime.lane_max must be between 0 and 4"))
	}
	return errs
}

func validatePolicy(policy PolicyConfig) []error {
	errs := []error{}
	for field, value := range map[string]float64{
		"policy.threshold_conflict":   policy.ThresholdConflict,
		"policy.threshold_ambiguity":  policy.ThresholdAmbiguity,
		"policy.threshold_coverage":   policy.ThresholdCoverage,
		"policy.threshold_confidence": policy.ThresholdConfidence,
	} {
		if err := validateRatio(field, value); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func validateBudget(budget BudgetConfig) []error {
	errs := []error{}
	if budget.MaxTokens <= 0 {
		errs = append(errs, fmt.Errorf("budget.max_tokens must be > 0"))
	}
	if budget.MaxLatencyMS <= 0 {
		errs = append(errs, fmt.Errorf("budget.max_latency_ms must be > 0"))
	}
	return errs
}

func validateSemantic(semantic SemanticConfig) []error {
	errs := []error{}
	if semantic.TimeoutMS <= 0 {
		errs = append(errs, fmt.Errorf("semantic.timeout_ms must be > 0"))
	}
	if semantic.MaxCandidates <= 0 {
		errs = append(errs, fmt.Errorf("semantic.max_candidates must be > 0"))
	}
	for field, value := range map[string]float64{
		"semantic.similarity_threshold": semantic.SimilarityThreshold,
		"semantic.dominance_delta":      semantic.DominanceDelta,
	} {
		if err := validateRatio(field, value); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func validateModels(models ModelsConfig) []error {
	errs := []error{}
	for field, value := range map[string]string{
		"models.router":    models.Router,
		"models.judge":     models.Judge,
		"models.extractor": models.Extractor,
		"models.formatter": models.Formatter,
	} {
		if strings.TrimSpace(value) == "" {
			errs = append(errs, fmt.Errorf("%s must not be empty", field))
		}
	}
	switch strings.TrimSpace(models.Backend) {
	case "", "noop", "openai-compatible", "ollama":
	default:
		errs = append(errs, fmt.Errorf("models.backend must be one of noop, openai-compatible, ollama"))
	}
	switch strings.TrimSpace(models.Backend) {
	case "openai-compatible", "ollama":
		if strings.TrimSpace(models.BaseURL) == "" {
			errs = append(errs, fmt.Errorf("models.base_url must not be empty when models.backend is %s", strings.TrimSpace(models.Backend)))
		}
	}
	for field, value := range map[string]int64{
		"models.micro_timeout_ms":      models.MicroTimeoutMS,
		"models.structured_timeout_ms": models.StructuredTimeoutMS,
		"models.specialist_timeout_ms": models.SpecialistTimeoutMS,
	} {
		if value <= 0 {
			errs = append(errs, fmt.Errorf("%s must be > 0", field))
		}
	}
	return errs
}

func validateDiscovery(discovery DiscoveryConfig) []error {
	if strings.TrimSpace(discovery.ProviderChain) == "" {
		return []error{fmt.Errorf("discovery.provider_chain must not be empty")}
	}
	return nil
}

func validateMemory(memory MemoryConfig) []error {
	errs := []error{}
	switch strings.TrimSpace(memory.Backend) {
	case "", "sqlite":
	default:
		errs = append(errs, fmt.Errorf("memory.backend must be sqlite"))
	}
	if strings.TrimSpace(memory.Path) == "" {
		errs = append(errs, fmt.Errorf("memory.path must not be empty"))
	}
	for field, value := range map[string]int{
		"memory.max_documents":  memory.MaxDocuments,
		"memory.max_edges":      memory.MaxEdges,
		"memory.max_embeddings": memory.MaxEmbeddings,
	} {
		if value <= 0 {
			errs = append(errs, fmt.Errorf("%s must be > 0", field))
		}
	}
	if strings.TrimSpace(memory.EmbeddingBackend) == "" {
		errs = append(errs, fmt.Errorf("memory.embedding_backend must not be empty"))
	}
	if strings.TrimSpace(memory.EmbeddingModel) == "" {
		errs = append(errs, fmt.Errorf("memory.embedding_model must not be empty"))
	}
	switch strings.TrimSpace(memory.VectorMode) {
	case "", "fallback-linear", "embedded":
	default:
		errs = append(errs, fmt.Errorf("memory.vector_mode must be one of fallback-linear, embedded"))
	}
	switch strings.TrimSpace(memory.VectorEngine) {
	case "", "sqlite-vec", "vec1":
	default:
		errs = append(errs, fmt.Errorf("memory.vector_engine must be one of sqlite-vec, vec1"))
	}
	if strings.TrimSpace(memory.PrunePolicy) == "" {
		errs = append(errs, fmt.Errorf("memory.prune_policy must not be empty"))
	}
	return errs
}

func validateRatio(field string, value float64) error {
	if value < 0 || value > 1 {
		return fmt.Errorf("%s must be between 0 and 1", field)
	}
	return nil
}

func errorsJoin(errs ...error) error {
	filtered := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return errors.Join(filtered...)
}
