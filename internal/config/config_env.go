package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func (c *Config) ApplyEnv(env map[string]string) error {
	if err := c.applyIntEnv(env); err != nil {
		return err
	}
	if err := c.applyInt64Env(env); err != nil {
		return err
	}
	if err := c.applyFloat64Env(env); err != nil {
		return err
	}
	c.applyStringEnv(env)
	if err := applyBool(env, "NEEDLEX_SEMANTIC_ENABLED", &c.Semantic.Enabled); err != nil {
		return err
	}
	if err := applyBool(env, "NEEDLEX_MEMORY_ENABLED", &c.Memory.Enabled); err != nil {
		return err
	}
	return nil
}

func (c *Config) applyIntEnv(env map[string]string) error {
	return applyIntEnv(env, []struct {
		key    string
		target *int
	}{
		{"NEEDLEX_RUNTIME_MAX_PAGES", &c.Runtime.MaxPages},
		{"NEEDLEX_RUNTIME_MAX_DEPTH", &c.Runtime.MaxDepth},
		{"NEEDLEX_RUNTIME_LANE_MAX", &c.Runtime.LaneMax},
		{"NEEDLEX_BUDGET_MAX_TOKENS", &c.Budget.MaxTokens},
		{"NEEDLEX_SEMANTIC_MAX_CANDIDATES", &c.Semantic.MaxCandidates},
		{"NEEDLEX_MEMORY_MAX_DOCUMENTS", &c.Memory.MaxDocuments},
		{"NEEDLEX_MEMORY_MAX_EDGES", &c.Memory.MaxEdges},
		{"NEEDLEX_MEMORY_MAX_EMBEDDINGS", &c.Memory.MaxEmbeddings},
	})
}

func (c *Config) applyInt64Env(env map[string]string) error {
	return applyInt64Env(env, []struct {
		key    string
		target *int64
	}{
		{"NEEDLEX_RUNTIME_TIMEOUT_MS", &c.Runtime.TimeoutMS},
		{"NEEDLEX_RUNTIME_MAX_BYTES", &c.Runtime.MaxBytes},
		{"NEEDLEX_BUDGET_MAX_LATENCY_MS", &c.Budget.MaxLatencyMS},
		{"NEEDLEX_FETCH_BLOCKED_RETRY_BACKOFF_MS", &c.Fetch.BlockedRetryBackoffMS},
		{"NEEDLEX_FETCH_BLOCKED_RETRY_JITTER_MS", &c.Fetch.BlockedRetryJitterMS},
		{"NEEDLEX_FETCH_PER_HOST_MIN_GAP_MS", &c.Fetch.PerHostMinGapMS},
		{"NEEDLEX_FETCH_PER_HOST_JITTER_MS", &c.Fetch.PerHostJitterMS},
		{"NEEDLEX_FETCH_TIMEOUT_RETRY_BACKOFF_MS", &c.Fetch.TimeoutRetryBackoffMS},
		{"NEEDLEX_FETCH_TIMEOUT_RETRY_JITTER_MS", &c.Fetch.TimeoutRetryJitterMS},
		{"NEEDLEX_MODELS_MICRO_TIMEOUT_MS", &c.Models.MicroTimeoutMS},
		{"NEEDLEX_MODELS_STRUCTURED_TIMEOUT_MS", &c.Models.StructuredTimeoutMS},
		{"NEEDLEX_MODELS_SPECIALIST_TIMEOUT_MS", &c.Models.SpecialistTimeoutMS},
		{"NEEDLEX_SEMANTIC_TIMEOUT_MS", &c.Semantic.TimeoutMS},
		{"NEEDLEX_DISCOVERY_PROVIDER_FAILURE_COOLDOWN_MS", &c.Discovery.ProviderFailureCooldownMS},
		{"NEEDLEX_DISCOVERY_PROVIDER_BLOCKED_COOLDOWN_MS", &c.Discovery.ProviderBlockedCooldownMS},
		{"NEEDLEX_DISCOVERY_PROVIDER_TIMEOUT_COOLDOWN_MS", &c.Discovery.ProviderTimeoutCooldownMS},
		{"NEEDLEX_DISCOVERY_PROVIDER_UNAVAILABLE_COOLDOWN_MS", &c.Discovery.ProviderUnavailableCooldownMS},
	})
}

func (c *Config) applyFloat64Env(env map[string]string) error {
	return applyFloat64Env(env, []struct {
		key    string
		target *float64
	}{
		{"NEEDLEX_POLICY_THRESHOLD_CONFLICT", &c.Policy.ThresholdConflict},
		{"NEEDLEX_POLICY_THRESHOLD_AMBIGUITY", &c.Policy.ThresholdAmbiguity},
		{"NEEDLEX_POLICY_THRESHOLD_COVERAGE", &c.Policy.ThresholdCoverage},
		{"NEEDLEX_POLICY_THRESHOLD_CONFIDENCE", &c.Policy.ThresholdConfidence},
		{"NEEDLEX_SEMANTIC_SIMILARITY_THRESHOLD", &c.Semantic.SimilarityThreshold},
		{"NEEDLEX_SEMANTIC_DOMINANCE_DELTA", &c.Semantic.DominanceDelta},
	})
}

func (c *Config) applyStringEnv(env map[string]string) {
	applyStringEnv(env, []struct {
		key    string
		target *string
	}{
		{"NEEDLEX_FETCH_PROFILE", &c.Fetch.Profile},
		{"NEEDLEX_FETCH_RETRY_PROFILE", &c.Fetch.RetryProfile},
		{"NEEDLEX_MODELS_BACKEND", &c.Models.Backend},
		{"NEEDLEX_MODELS_BASE_URL", &c.Models.BaseURL},
		{"NEEDLEX_MODELS_API_KEY", &c.Models.APIKey},
		{"NEEDLEX_MODELS_ROUTER", &c.Models.Router},
		{"NEEDLEX_MODELS_JUDGE", &c.Models.Judge},
		{"NEEDLEX_MODELS_EXTRACTOR", &c.Models.Extractor},
		{"NEEDLEX_MODELS_FORMATTER", &c.Models.Formatter},
		{"NEEDLEX_DISCOVERY_PROVIDER_CHAIN", &c.Discovery.ProviderChain},
		{"BRAVE_SEARCH_API_KEY", &c.Discovery.BraveAPIKey},
		{"NEEDLEX_SEMANTIC_BACKEND", &c.Semantic.Backend},
		{"NEEDLEX_SEMANTIC_BASE_URL", &c.Semantic.BaseURL},
		{"NEEDLEX_SEMANTIC_MODEL", &c.Semantic.Model},
		{"NEEDLEX_MEMORY_BACKEND", &c.Memory.Backend},
		{"NEEDLEX_MEMORY_PATH", &c.Memory.Path},
		{"NEEDLEX_MEMORY_EMBEDDING_BACKEND", &c.Memory.EmbeddingBackend},
		{"NEEDLEX_MEMORY_EMBEDDING_MODEL", &c.Memory.EmbeddingModel},
		{"NEEDLEX_MEMORY_VECTOR_MODE", &c.Memory.VectorMode},
		{"NEEDLEX_MEMORY_VECTOR_ENGINE", &c.Memory.VectorEngine},
		{"NEEDLEX_MEMORY_PRUNE_POLICY", &c.Memory.PrunePolicy},
	})
}

func applyIntEnv(env map[string]string, setters []struct {
	key    string
	target *int
},
) error {
	for _, setter := range setters {
		if err := applyInt(env, setter.key, setter.target); err != nil {
			return err
		}
	}
	return nil
}

func applyInt64Env(env map[string]string, setters []struct {
	key    string
	target *int64
},
) error {
	for _, setter := range setters {
		if err := applyInt64(env, setter.key, setter.target); err != nil {
			return err
		}
	}
	return nil
}

func applyFloat64Env(env map[string]string, setters []struct {
	key    string
	target *float64
},
) error {
	for _, setter := range setters {
		if err := applyFloat64(env, setter.key, setter.target); err != nil {
			return err
		}
	}
	return nil
}

func applyStringEnv(env map[string]string, setters []struct {
	key    string
	target *string
},
) {
	for _, setter := range setters {
		applyString(env, setter.key, setter.target)
	}
}

func applyBool(env map[string]string, key string, target *bool) error {
	value, ok := env[key]
	if !ok || strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("parse %s: %w", key, err)
	}
	*target = parsed
	return nil
}

func envMap() map[string]string {
	env := map[string]string{}
	for _, raw := range os.Environ() {
		key, value, ok := strings.Cut(raw, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

func applyInt(env map[string]string, key string, target *int) error {
	value, ok := env[key]
	if !ok || strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("parse %s: %w", key, err)
	}
	*target = parsed
	return nil
}

func applyInt64(env map[string]string, key string, target *int64) error {
	value, ok := env[key]
	if !ok || strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf("parse %s: %w", key, err)
	}
	*target = parsed
	return nil
}

func applyFloat64(env map[string]string, key string, target *float64) error {
	value, ok := env[key]
	if !ok || strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("parse %s: %w", key, err)
	}
	*target = parsed
	return nil
}

func applyString(env map[string]string, key string, target *string) {
	value, ok := env[key]
	if ok && strings.TrimSpace(value) != "" {
		*target = value
	}
}
