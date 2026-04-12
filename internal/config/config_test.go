package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsValidate(t *testing.T) {
	if err := Defaults().Validate(); err != nil {
		t.Fatalf("defaults should validate: %v", err)
	}
}

func TestDefaultsUseOperatorGradeRuntimeTimeout(t *testing.T) {
	cfg := Defaults()
	if cfg.Runtime.TimeoutMS != 8000 {
		t.Fatalf("expected runtime timeout default 8000ms, got %d", cfg.Runtime.TimeoutMS)
	}
}

func TestDefaultsUseBrowserLikeFetchProfile(t *testing.T) {
	cfg := Defaults()
	if cfg.Fetch.Profile != "browser_like" || cfg.Fetch.RetryProfile != "hardened" {
		t.Fatalf("unexpected fetch defaults: %+v", cfg.Fetch)
	}
	if cfg.Fetch.BlockedRetryBackoffMS != 400 || cfg.Fetch.BlockedRetryJitterMS != 200 || cfg.Fetch.TimeoutRetryBackoffMS != 150 || cfg.Fetch.TimeoutRetryJitterMS != 75 || cfg.Fetch.PerHostMinGapMS != 250 || cfg.Fetch.PerHostJitterMS != 100 {
		t.Fatalf("unexpected fetch pacing defaults: %+v", cfg.Fetch)
	}
}

func TestLoadMergesJSONWithDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "needlex.json")
	data := []byte(`{"runtime":{"max_pages":42},"models":{"router":"router-x"}}`)

	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Runtime.MaxPages != 42 {
		t.Fatalf("expected runtime.max_pages=42, got %d", cfg.Runtime.MaxPages)
	}
	if cfg.Models.Router != "router-x" {
		t.Fatalf("expected router override, got %q", cfg.Models.Router)
	}
	if cfg.Models.Judge == "" {
		t.Fatal("expected omitted fields to keep defaults")
	}
}

func TestApplyEnvOverridesValues(t *testing.T) {
	cfg := Defaults()
	env := map[string]string{
		"NEEDLEX_FETCH_PROFILE":                  "standard",
		"NEEDLEX_FETCH_RETRY_PROFILE":            "browser_like",
		"NEEDLEX_FETCH_BLOCKED_RETRY_BACKOFF_MS": "700",
		"NEEDLEX_FETCH_BLOCKED_RETRY_JITTER_MS":  "90",
		"NEEDLEX_FETCH_PER_HOST_MIN_GAP_MS":      "333",
		"NEEDLEX_FETCH_PER_HOST_JITTER_MS":       "55",
		"NEEDLEX_FETCH_TIMEOUT_RETRY_BACKOFF_MS": "120",
		"NEEDLEX_FETCH_TIMEOUT_RETRY_JITTER_MS":  "30",
		"NEEDLEX_MODELS_BACKEND":                 "openai-compatible",
		"NEEDLEX_MODELS_BASE_URL":                "http://localhost:11434/v1",
		"NEEDLEX_RUNTIME_MAX_DEPTH":              "7",
		"NEEDLEX_POLICY_THRESHOLD_CONFLICT":      "0.65",
		"NEEDLEX_MODELS_FORMATTER":               "formatter-x",
		"NEEDLEX_MODELS_MICRO_TIMEOUT_MS":        "1500",
	}

	if err := cfg.ApplyEnv(env); err != nil {
		t.Fatalf("apply env: %v", err)
	}
	if cfg.Runtime.MaxDepth != 7 {
		t.Fatalf("expected runtime.max_depth=7, got %d", cfg.Runtime.MaxDepth)
	}
	if cfg.Fetch.Profile != "standard" || cfg.Fetch.RetryProfile != "browser_like" {
		t.Fatalf("unexpected fetch override: %+v", cfg.Fetch)
	}
	if cfg.Fetch.BlockedRetryBackoffMS != 700 || cfg.Fetch.BlockedRetryJitterMS != 90 || cfg.Fetch.PerHostMinGapMS != 333 || cfg.Fetch.PerHostJitterMS != 55 || cfg.Fetch.TimeoutRetryBackoffMS != 120 || cfg.Fetch.TimeoutRetryJitterMS != 30 {
		t.Fatalf("unexpected fetch pacing override: %+v", cfg.Fetch)
	}
	if cfg.Policy.ThresholdConflict != 0.65 {
		t.Fatalf("expected threshold conflict override, got %f", cfg.Policy.ThresholdConflict)
	}
	if cfg.Models.Formatter != "formatter-x" {
		t.Fatalf("expected formatter override, got %q", cfg.Models.Formatter)
	}
	if cfg.Models.Backend != "openai-compatible" {
		t.Fatalf("expected backend override, got %q", cfg.Models.Backend)
	}
	if cfg.Models.BaseURL != "http://localhost:11434/v1" {
		t.Fatalf("expected base url override, got %q", cfg.Models.BaseURL)
	}
	if cfg.Models.MicroTimeoutMS != 1500 {
		t.Fatalf("expected micro timeout override, got %d", cfg.Models.MicroTimeoutMS)
	}
}

func TestLoadRejectsInvalidEnvValue(t *testing.T) {
	t.Setenv("NEEDLEX_RUNTIME_MAX_PAGES", "oops")

	if _, err := Load(""); err == nil {
		t.Fatal("expected invalid env value to fail")
	}
}

func TestDefaultsUseModelBaselineSSOT(t *testing.T) {
	cfg := Defaults()
	if cfg.Models.Router != "gemma3:1b-it-q8_0" {
		t.Fatalf("expected gemma router baseline, got %q", cfg.Models.Router)
	}
	if cfg.Models.Extractor != "gemma3:1b-it-q8_0" {
		t.Fatalf("expected gemma extractor baseline, got %q", cfg.Models.Extractor)
	}
	if cfg.Models.BaseURL != "http://127.0.0.1:11434/v1" {
		t.Fatalf("expected baseline base url, got %q", cfg.Models.BaseURL)
	}
	if cfg.Models.MicroTimeoutMS != 8000 || cfg.Models.StructuredTimeoutMS != 20000 || cfg.Models.SpecialistTimeoutMS != 8000 {
		t.Fatalf("unexpected SSOT timeout defaults: %+v", cfg.Models)
	}
	if cfg.Semantic.Backend != "openai-embeddings" || cfg.Semantic.BaseURL != "http://127.0.0.1:18180" || cfg.Semantic.Model != "intfloat/multilingual-e5-small" || cfg.Semantic.TimeoutMS != 1200 {
		t.Fatalf("unexpected semantic SSOT defaults: %+v", cfg.Semantic)
	}
	if cfg.Semantic.Enabled {
		t.Fatal("expected semantic gate disabled by default")
	}
	if cfg.Discovery.ProviderChain != "brave://search,https://lite.duckduckgo.com/lite/,https://html.duckduckgo.com/html/" {
		t.Fatalf("unexpected discovery SSOT defaults: %+v", cfg.Discovery)
	}
	if cfg.Memory.Backend != "sqlite" || cfg.Memory.Path != "discovery/discovery.db" || cfg.Memory.EmbeddingBackend != "openai-embeddings" || cfg.Memory.EmbeddingModel != "intfloat/multilingual-e5-small" || cfg.Memory.VectorEngine != "sqlite-vec" {
		t.Fatalf("unexpected memory SSOT defaults: %+v", cfg.Memory)
	}
	if cfg.Memory.Enabled {
		t.Fatal("expected discovery memory disabled by default")
	}
}

func TestApplyEnvOverridesSemanticValues(t *testing.T) {
	cfg := Defaults()
	env := map[string]string{
		"NEEDLEX_SEMANTIC_ENABLED":              "true",
		"NEEDLEX_SEMANTIC_BACKEND":              "ollama-embed",
		"NEEDLEX_SEMANTIC_BASE_URL":             "http://localhost:11434",
		"NEEDLEX_SEMANTIC_MODEL":                "embed-x",
		"NEEDLEX_SEMANTIC_TIMEOUT_MS":           "1500",
		"NEEDLEX_SEMANTIC_SIMILARITY_THRESHOLD": "0.66",
		"NEEDLEX_SEMANTIC_DOMINANCE_DELTA":      "0.11",
		"NEEDLEX_SEMANTIC_MAX_CANDIDATES":       "5",
	}
	if err := cfg.ApplyEnv(env); err != nil {
		t.Fatalf("apply env: %v", err)
	}
	if !cfg.Semantic.Enabled || cfg.Semantic.Model != "embed-x" || cfg.Semantic.MaxCandidates != 5 {
		t.Fatalf("unexpected semantic config override: %+v", cfg.Semantic)
	}
}

func TestApplyEnvOverridesDiscoveryValues(t *testing.T) {
	cfg := Defaults()
	env := map[string]string{
		"NEEDLEX_DISCOVERY_PROVIDER_CHAIN":                   "https://example.com/search,https://backup.example/search",
		"BRAVE_SEARCH_API_KEY":                               "brave-test",
		"NEEDLEX_DISCOVERY_PROVIDER_FAILURE_COOLDOWN_MS":     "1000",
		"NEEDLEX_DISCOVERY_PROVIDER_BLOCKED_COOLDOWN_MS":     "2000",
		"NEEDLEX_DISCOVERY_PROVIDER_TIMEOUT_COOLDOWN_MS":     "3000",
		"NEEDLEX_DISCOVERY_PROVIDER_UNAVAILABLE_COOLDOWN_MS": "4000",
	}
	if err := cfg.ApplyEnv(env); err != nil {
		t.Fatalf("apply env: %v", err)
	}
	if cfg.Discovery.ProviderChain != "https://example.com/search,https://backup.example/search" {
		t.Fatalf("unexpected discovery override: %+v", cfg.Discovery)
	}
	if cfg.Discovery.BraveAPIKey != "brave-test" {
		t.Fatalf("unexpected discovery api key override: %+v", cfg.Discovery)
	}
	if cfg.Discovery.ProviderFailureCooldownMS != 1000 || cfg.Discovery.ProviderBlockedCooldownMS != 2000 || cfg.Discovery.ProviderTimeoutCooldownMS != 3000 || cfg.Discovery.ProviderUnavailableCooldownMS != 4000 {
		t.Fatalf("unexpected discovery cooldown override: %+v", cfg.Discovery)
	}
}

func TestApplyEnvOverridesMemoryValues(t *testing.T) {
	cfg := Defaults()
	env := map[string]string{
		"NEEDLEX_MEMORY_ENABLED":           "true",
		"NEEDLEX_MEMORY_BACKEND":           "sqlite",
		"NEEDLEX_MEMORY_PATH":              "memory/custom.db",
		"NEEDLEX_MEMORY_MAX_DOCUMENTS":     "500",
		"NEEDLEX_MEMORY_MAX_EDGES":         "900",
		"NEEDLEX_MEMORY_MAX_EMBEDDINGS":    "500",
		"NEEDLEX_MEMORY_EMBEDDING_BACKEND": "openai-embeddings",
		"NEEDLEX_MEMORY_EMBEDDING_MODEL":   "embed-y",
		"NEEDLEX_MEMORY_VECTOR_MODE":       "embedded",
		"NEEDLEX_MEMORY_VECTOR_ENGINE":     "vec1",
		"NEEDLEX_MEMORY_PRUNE_POLICY":      "lru",
	}
	if err := cfg.ApplyEnv(env); err != nil {
		t.Fatalf("apply env: %v", err)
	}
	if !cfg.Memory.Enabled || cfg.Memory.Path != "memory/custom.db" || cfg.Memory.MaxDocuments != 500 || cfg.Memory.VectorEngine != "vec1" || cfg.Memory.VectorMode != "embedded" {
		t.Fatalf("unexpected memory config override: %+v", cfg.Memory)
	}
}
