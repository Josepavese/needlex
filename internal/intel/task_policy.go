package intel

import (
	"fmt"

	"github.com/josepavese/needlex/internal/config"
)

const (
	FailureClassAmbiguityConflict = "ambiguity_conflict"
)

type TaskBudget struct {
	MaxInputTokens  int   `json:"max_input_tokens"`
	MaxOutputTokens int   `json:"max_output_tokens"`
	TimeoutMS       int64 `json:"timeout_ms"`
}

type TaskRoute struct {
	Task         string     `json:"task"`
	FailureClass string     `json:"failure_class"`
	ModelClass   string     `json:"model_class"`
	Budget       TaskBudget `json:"budget"`
	ReasonCode   string     `json:"reason_code"`
}

func (r TaskRoute) Validate() error {
	if !validTask(r.Task) {
		return fmt.Errorf("task route task %q is not supported", r.Task)
	}
	if !validModelClass(r.ModelClass) {
		return fmt.Errorf("task route model_class %q is not supported", r.ModelClass)
	}
	if r.Budget.MaxInputTokens <= 0 || r.Budget.MaxOutputTokens <= 0 || r.Budget.TimeoutMS <= 0 {
		return fmt.Errorf("task route budget must be positive")
	}
	if r.FailureClass == "" {
		return fmt.Errorf("task route failure_class must not be empty")
	}
	if r.ReasonCode == "" {
		return fmt.Errorf("task route reason_code must not be empty")
	}
	return nil
}

func PlanResolveAmbiguityTask(cfg config.Config, input ResolveAmbiguityInput) (TaskRoute, bool) {
	if err := input.Validate(); err != nil {
		return TaskRoute{}, false
	}
	if len(input.Candidates) < 2 {
		return TaskRoute{}, false
	}
	first, second := input.Candidates[0].Score, input.Candidates[1].Score
	if first < second {
		first, second = second, first
	}
	if first-second > cfg.Policy.ThresholdConflict {
		return TaskRoute{}, false
	}
	route := TaskRoute{
		Task:         TaskResolveAmbiguity,
		FailureClass: FailureClassAmbiguityConflict,
		ModelClass:   ModelClassMicroSolver,
		Budget: TaskBudget{
			MaxInputTokens:  minInt(cfg.Budget.MaxTokens, 256),
			MaxOutputTokens: 64,
			TimeoutMS:       minInt64(cfg.Budget.MaxLatencyMS, cfg.Models.MicroTimeoutMS),
		},
		ReasonCode: ReasonAmbiguityTriggered,
	}
	return route, route.Validate() == nil
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
