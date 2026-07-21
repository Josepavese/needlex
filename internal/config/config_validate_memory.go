package config

import (
	"errors"
	"fmt"
	"strings"
)

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

func validateAgent(agent AgentConfig) []error {
	errs := []error{}
	if agent.MaxCandidates < 0 {
		errs = append(errs, fmt.Errorf("agent.max_candidates must be >= 0"))
	}
	return errs
}

func validateRender(render RenderConfig) []error {
	errs := []error{}
	switch strings.TrimSpace(render.Provider) {
	case "", "exec-dump-dom":
	default:
		errs = append(errs, fmt.Errorf("render.provider must be exec-dump-dom"))
	}
	if render.TimeoutMS < 0 {
		errs = append(errs, fmt.Errorf("render.timeout_ms must be >= 0"))
	}
	if render.MaxConcurrency < 0 {
		errs = append(errs, fmt.Errorf("render.max_concurrency must be >= 0"))
	}
	if render.NetworkIdleMS < 0 {
		errs = append(errs, fmt.Errorf("render.network_idle_ms must be >= 0"))
	}
	if render.NetworkMaxBytes < 0 {
		errs = append(errs, fmt.Errorf("render.network_max_bytes must be >= 0"))
	}
	if render.NetworkResourceMaxBytes < 0 {
		errs = append(errs, fmt.Errorf("render.network_resource_max_bytes must be >= 0"))
	}
	if render.NetworkMaxResources < 0 {
		errs = append(errs, fmt.Errorf("render.network_max_resources must be >= 0"))
	}
	if render.NetworkMaxMessages < 0 {
		errs = append(errs, fmt.Errorf("render.network_max_messages must be >= 0"))
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
