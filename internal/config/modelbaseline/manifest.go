package modelbaseline

import (
	_ "embed"
	"encoding/json"
	"sync"
)

type Manifest struct {
	Version            string            `json:"version"`
	CPUBaselineID      string            `json:"cpu_baseline_id"`
	RecommendedBackend string            `json:"recommended_backend"`
	RecommendedBaseURL string            `json:"recommended_base_url"`
	Discovery          DiscoveryManifest `json:"discovery"`
	Semantic           SemanticManifest  `json:"semantic"`
	Models             ModelsManifest    `json:"models"`
	Timeouts           TimeoutManifest   `json:"timeouts"`
	OverrideEnv        []string          `json:"override_env"`
}

type DiscoveryManifest struct {
	RecommendedProviderChain string `json:"recommended_provider_chain"`
}

type SemanticManifest struct {
	RecommendedBackend string `json:"recommended_backend"`
	RecommendedBaseURL string `json:"recommended_base_url"`
	Model              string `json:"model"`
	TimeoutMS          int64  `json:"timeout_ms"`
}

type ModelsManifest struct {
	Router    string `json:"router"`
	Judge     string `json:"judge"`
	Extractor string `json:"extractor"`
	Formatter string `json:"formatter"`
}

type TimeoutManifest struct {
	MicroMS      int64 `json:"micro_timeout_ms"`
	StructuredMS int64 `json:"structured_timeout_ms"`
	SpecialistMS int64 `json:"specialist_timeout_ms"`
}

//go:embed model-baseline.json
var raw []byte

var (
	once sync.Once
	cfg  Manifest
)

func Default() Manifest {
	once.Do(func() {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			panic(err)
		}
	})
	return cfg
}
