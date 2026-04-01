package intel

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

var forbiddenInputKeys = []string{"html", "raw_html", "full_html"}

func (r ModelRequest) Validate() error {
	errs := []error{}
	if !validTask(r.Task) {
		errs = append(errs, fmt.Errorf("task %q is not supported", r.Task))
	}
	if !validModelClass(r.ModelClass) {
		errs = append(errs, fmt.Errorf("model_class %q is not supported", r.ModelClass))
	}
	if strings.TrimSpace(r.SchemaName) == "" {
		errs = append(errs, fmt.Errorf("schema_name must not be empty"))
	}
	if r.MaxInputTokens <= 0 {
		errs = append(errs, fmt.Errorf("max_input_tokens must be > 0"))
	}
	if r.MaxOutputTokens <= 0 {
		errs = append(errs, fmt.Errorf("max_output_tokens must be > 0"))
	}
	if r.TimeoutMS <= 0 {
		errs = append(errs, fmt.Errorf("timeout_ms must be > 0"))
	}
	if len(r.Input) == 0 {
		errs = append(errs, fmt.Errorf("input must not be empty"))
	}
	if err := validateInputPayload(r.Input); err != nil {
		errs = append(errs, err)
	}
	return joinErrors(errs...)
}

func (r ModelResponse) Validate() error {
	errs := []error{}
	if strings.TrimSpace(r.Model) == "" {
		errs = append(errs, fmt.Errorf("model must not be empty"))
	}
	if !validTask(r.Task) {
		errs = append(errs, fmt.Errorf("task %q is not supported", r.Task))
	}
	if r.TokensIn < 0 {
		errs = append(errs, fmt.Errorf("tokens_in must be >= 0"))
	}
	if r.TokensOut < 0 {
		errs = append(errs, fmt.Errorf("tokens_out must be >= 0"))
	}
	if r.LatencyMS < 0 {
		errs = append(errs, fmt.Errorf("latency_ms must be >= 0"))
	}
	if !validFinishReason(r.FinishReason) {
		errs = append(errs, fmt.Errorf("finish_reason %q is not supported", r.FinishReason))
	}
	if r.Confidence < 0 || r.Confidence > 1 {
		errs = append(errs, fmt.Errorf("confidence must be between 0 and 1"))
	}
	return joinErrors(errs...)
}

func validateInputPayload(payload map[string]any) error {
	for _, key := range slices.Sorted(maps.Keys(payload)) {
		if err := validateInputEntry(strings.TrimSpace(key), payload[key]); err != nil {
			return err
		}
	}
	return nil
}

func validateInputEntry(path string, value any) error {
	lower := strings.ToLower(strings.TrimSpace(path))
	if slices.Contains(forbiddenInputKeys, lower) {
		return fmt.Errorf("input key %q is forbidden: raw page payloads must not enter model runtime directly", path)
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(typed)) {
			if err := validateInputEntry(path+"."+strings.TrimSpace(key), typed[key]); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range typed {
			if err := validateInputEntry(fmt.Sprintf("%s[%d]", path, i), item); err != nil {
				return err
			}
		}
	}
	return nil
}

func joinErrors(errs ...error) error {
	filtered := make([]error, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			continue
		}
		filtered = append(filtered, err)
	}
	if len(filtered) == 0 {
		return nil
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	message := filtered[0].Error()
	for _, err := range filtered[1:] {
		message += "; " + err.Error()
	}
	return fmt.Errorf("%s", message)
}
