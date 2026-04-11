package intel

import "strings"

const (
	BackendNoop             = "noop"
	BackendOpenAICompatible = "openai-compatible"
	BackendOllama           = "ollama"
)

type BackendSelection struct {
	Backend      string   `json:"backend"`
	EnabledTasks []string `json:"enabled_tasks"`
	HoldoutTasks []string `json:"holdout_tasks,omitempty"`
	Reason       string   `json:"reason"`
}

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

func SelectionForBackend(backend string) BackendSelection {
	backend = strings.TrimSpace(backend)
	switch backend {
	case BackendOpenAICompatible, BackendOllama:
		return BackendSelection{
			Backend:      backend,
			EnabledTasks: []string{TaskResolveAmbiguity, TaskQueryRewrite},
			HoldoutTasks: nil,
			Reason:       "keep only tasks with accepted backend interventions and acceptable benchmark quality",
		}
	case "", BackendNoop:
		return BackendSelection{
			Backend:      BackendNoop,
			EnabledTasks: nil,
			HoldoutTasks: []string{TaskResolveAmbiguity, TaskQueryRewrite},
			Reason:       "deterministic fallback mode",
		}
	default:
		return BackendSelection{
			Backend:      backend,
			EnabledTasks: nil,
			HoldoutTasks: []string{TaskResolveAmbiguity, TaskQueryRewrite},
			Reason:       "backend not in benchmark-backed selection profile",
		}
	}
}
