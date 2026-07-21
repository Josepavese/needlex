package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/josepavese/needlex/internal/config/modelbaseline"
	"github.com/josepavese/needlex/internal/platform"
)

const (
	DefaultSemanticEmbeddingURL  = "http://127.0.0.1:11434/api/embed"
	DefaultSemanticProviderModel = "embeddinggemma:latest"
	DefaultSemanticVectorSpace   = "ollama-embeddinggemma-v1"
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
		Agent:     defaultAgentConfig(),
		Render:    defaultRenderConfig(),
	}
}

func DefaultsWithEnv() (Config, error) {
	cfg := Defaults()
	if err := cfg.ApplyEnv(envMap()); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
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
	embeddingURL := firstNonEmptyConfig(baseline.Semantic.EmbeddingURL, DefaultSemanticEmbeddingURL)
	providerModel := firstNonEmptyConfig(baseline.Semantic.ProviderModel, DefaultSemanticProviderModel)
	vectorSpace := firstNonEmptyConfig(baseline.Semantic.VectorSpace, DefaultSemanticVectorSpace)
	return SemanticConfig{
		EmbeddingURL:        embeddingURL,
		ProviderModel:       providerModel,
		VectorSpace:         vectorSpace,
		TimeoutMS:           baseline.Semantic.TimeoutMS,
		FailureCooldownMS:   5000,
		SimilarityThreshold: 0.55,
		DominanceDelta:      0.08,
		MaxCandidates:       4,
		EmbeddingCache: SemanticEmbeddingCacheConfig{
			MaxEntries:   200000,
			MaxBytes:     2147483648,
			TTL:          "720h",
			NegativeTTL:  "2m",
			StaleIfError: boolPtr(true),
		},
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func firstNonEmptyConfig(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func defaultMemoryConfig(baseline modelbaseline.Manifest) MemoryConfig {
	return MemoryConfig{
		Enabled:       true,
		Backend:       "sqlite",
		Path:          "discovery/discovery.db",
		MaxDocuments:  4000,
		MaxEdges:      12000,
		MaxEmbeddings: 4000,
		VectorMode:    "fallback-linear",
		VectorEngine:  "sqlite-vec",
		PrunePolicy:   "lru",
	}
}

func defaultAgentConfig() AgentConfig {
	return AgentConfig{
		ReadableEnabled: true,
		MaxCandidates:   16,
	}
}

func defaultRenderConfig() RenderConfig {
	return RenderConfig{
		Enabled:                 true,
		Provider:                "exec-dump-dom",
		TimeoutMS:               30000,
		MaxConcurrency:          1,
		NetworkIdleMS:           1500,
		NetworkMaxBytes:         64_000_000,
		NetworkResourceMaxBytes: 64_000_000,
		NetworkMaxResources:     32,
		NetworkMaxMessages:      4096,
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	if resolved := ResolvePath(path); resolved != "" {
		data, err := os.ReadFile(resolved)
		if err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
		if err := decodeConfigStrict(data, &cfg); err != nil {
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

func ResolvePath(path string) string {
	if trimmed := strings.TrimSpace(path); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(os.Getenv(platform.EnvConfig))
}

func DefaultPath() string {
	return platform.NewStateLayout(platform.DefaultStateRoot()).ConfigPath
}

func Write(path string, cfg Config) error {
	resolved := strings.TrimSpace(path)
	if resolved == "" {
		resolved = DefaultPath()
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(resolved, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func decodeConfigStrict(data []byte, cfg *Config) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(cfg); err != nil {
		return err
	}
	return nil
}
