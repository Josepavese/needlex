package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/josepavese/needlex/internal/config/modelbaseline"
)

func Defaults() Config {
	baseline := modelbaseline.Default()
	return Config{
		Runtime:   defaultRuntimeConfig(),
		Fetch:     defaultFetchConfig(),
		Policy:    defaultPolicyConfig(),
		Budget:    defaultBudgetConfig(),
		Models:    defaultModelsConfig(baseline),
		Discovery: defaultDiscoveryConfig(baseline),
		Semantic:  defaultSemanticConfig(baseline),
		Memory:    defaultMemoryConfig(baseline),
	}
}

func defaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		MaxPages:  20,
		MaxDepth:  2,
		TimeoutMS: 8000,
		MaxBytes:  4_000_000,
		LaneMax:   3,
	}
}

func defaultFetchConfig() FetchConfig {
	return FetchConfig{
		Profile:               "browser_like",
		RetryProfile:          "hardened",
		BlockedRetryBackoffMS: 400,
		BlockedRetryJitterMS:  200,
		TimeoutRetryBackoffMS: 150,
		TimeoutRetryJitterMS:  75,
		PerHostMinGapMS:       250,
		PerHostJitterMS:       100,
	}
}

func defaultPolicyConfig() PolicyConfig {
	return PolicyConfig{
		ThresholdConflict:   0.42,
		ThresholdAmbiguity:  0.37,
		ThresholdCoverage:   0.15,
		ThresholdConfidence: 0.78,
	}
}

func defaultBudgetConfig() BudgetConfig {
	return BudgetConfig{
		MaxTokens:    8000,
		MaxLatencyMS: 1800,
	}
}

func defaultModelsConfig(baseline modelbaseline.Manifest) ModelsConfig {
	return ModelsConfig{
		Backend:             "noop",
		BaseURL:             baseline.RecommendedBaseURL,
		Router:              baseline.Models.Router,
		Judge:               baseline.Models.Judge,
		Extractor:           baseline.Models.Extractor,
		Formatter:           baseline.Models.Formatter,
		MicroTimeoutMS:      baseline.Timeouts.MicroMS,
		StructuredTimeoutMS: baseline.Timeouts.StructuredMS,
		SpecialistTimeoutMS: baseline.Timeouts.SpecialistMS,
	}
}

func defaultDiscoveryConfig(baseline modelbaseline.Manifest) DiscoveryConfig {
	return DiscoveryConfig{
		ProviderChain:                 baseline.Discovery.RecommendedProviderChain,
		ProviderFailureCooldownMS:     120_000,
		ProviderBlockedCooldownMS:     900_000,
		ProviderTimeoutCooldownMS:     300_000,
		ProviderUnavailableCooldownMS: 60_000,
	}
}

func defaultSemanticConfig(baseline modelbaseline.Manifest) SemanticConfig {
	return SemanticConfig{
		Enabled:             false,
		Backend:             baseline.Semantic.RecommendedBackend,
		BaseURL:             baseline.Semantic.RecommendedBaseURL,
		Model:               baseline.Semantic.Model,
		TimeoutMS:           baseline.Semantic.TimeoutMS,
		SimilarityThreshold: 0.55,
		DominanceDelta:      0.08,
		MaxCandidates:       4,
	}
}

func defaultMemoryConfig(baseline modelbaseline.Manifest) MemoryConfig {
	return MemoryConfig{
		Enabled:          false,
		Backend:          "sqlite",
		Path:             "discovery/discovery.db",
		MaxDocuments:     4000,
		MaxEdges:         12000,
		MaxEmbeddings:    4000,
		EmbeddingBackend: baseline.Semantic.RecommendedBackend,
		EmbeddingModel:   baseline.Semantic.Model,
		VectorMode:       "fallback-linear",
		VectorEngine:     "sqlite-vec",
		PrunePolicy:      "lru",
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("decode config: %w", err)
		}
	}
	if err := cfg.ApplyEnv(envMap()); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
