package intel

import "strings"

const (
	BackendNoop             = "noop"
	BackendOpenAICompatible = "openai-compatible"
	BackendOllama           = "ollama"
)

func TaskAllowedForBackend(backend, task string) (bool, string) {
	backend = strings.TrimSpace(backend)
	task = strings.TrimSpace(task)
	switch backend {
	case "", BackendNoop:
		return false, "backend_disabled"
	case BackendOpenAICompatible, BackendOllama:
		switch task {
		case TaskResolveAmbiguity, TaskQueryRewrite:
			return true, "benchmark_proven"
		default:
			return false, "not_benchmark_proven"
		}
	default:
		return false, "backend_unrecognized"
	}
}
