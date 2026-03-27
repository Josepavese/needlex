package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Runtime RuntimeConfig `json:"runtime"`
	Policy  PolicyConfig  `json:"policy"`
	Budget  BudgetConfig  `json:"budget"`
	Models  ModelsConfig  `json:"models"`
}

type RuntimeConfig struct {
	MaxPages  int   `json:"max_pages"`
	MaxDepth  int   `json:"max_depth"`
	TimeoutMS int64 `json:"timeout_ms"`
	MaxBytes  int64 `json:"max_bytes"`
	LaneMax   int   `json:"lane_max"`
}

type PolicyConfig struct {
	ThresholdConflict   float64 `json:"threshold_conflict"`
	ThresholdAmbiguity  float64 `json:"threshold_ambiguity"`
	ThresholdCoverage   float64 `json:"threshold_coverage"`
	ThresholdConfidence float64 `json:"threshold_confidence"`
}

type BudgetConfig struct {
	MaxTokens    int   `json:"max_tokens"`
	MaxLatencyMS int64 `json:"max_latency_ms"`
}

type ModelsConfig struct {
	Router    string `json:"router"`
	Judge     string `json:"judge"`
	Extractor string `json:"extractor"`
	Formatter string `json:"formatter"`
}

func Defaults() Config {
	return Config{
		Runtime: RuntimeConfig{
			MaxPages:  20,
			MaxDepth:  2,
			TimeoutMS: 2000,
			MaxBytes:  4_000_000,
			LaneMax:   3,
		},
		Policy: PolicyConfig{
			ThresholdConflict:   0.42,
			ThresholdAmbiguity:  0.37,
			ThresholdCoverage:   0.15,
			ThresholdConfidence: 0.78,
		},
		Budget: BudgetConfig{
			MaxTokens:    8000,
			MaxLatencyMS: 1800,
		},
		Models: ModelsConfig{
			Router:    "local-slm-router",
			Judge:     "local-slm-judge",
			Extractor: "local-slm-extractor",
			Formatter: "local-slm-formatter",
		},
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

func (c *Config) ApplyEnv(env map[string]string) error {
	intSetters := []struct {
		key    string
		target *int
	}{
		{"NEEDLEX_RUNTIME_MAX_PAGES", &c.Runtime.MaxPages},
		{"NEEDLEX_RUNTIME_MAX_DEPTH", &c.Runtime.MaxDepth},
		{"NEEDLEX_RUNTIME_LANE_MAX", &c.Runtime.LaneMax},
		{"NEEDLEX_BUDGET_MAX_TOKENS", &c.Budget.MaxTokens},
	}
	for _, setter := range intSetters {
		if err := applyInt(env, setter.key, setter.target); err != nil {
			return err
		}
	}

	int64Setters := []struct {
		key    string
		target *int64
	}{
		{"NEEDLEX_RUNTIME_TIMEOUT_MS", &c.Runtime.TimeoutMS},
		{"NEEDLEX_RUNTIME_MAX_BYTES", &c.Runtime.MaxBytes},
		{"NEEDLEX_BUDGET_MAX_LATENCY_MS", &c.Budget.MaxLatencyMS},
	}
	for _, setter := range int64Setters {
		if err := applyInt64(env, setter.key, setter.target); err != nil {
			return err
		}
	}

	floatSetters := []struct {
		key    string
		target *float64
	}{
		{"NEEDLEX_POLICY_THRESHOLD_CONFLICT", &c.Policy.ThresholdConflict},
		{"NEEDLEX_POLICY_THRESHOLD_AMBIGUITY", &c.Policy.ThresholdAmbiguity},
		{"NEEDLEX_POLICY_THRESHOLD_COVERAGE", &c.Policy.ThresholdCoverage},
		{"NEEDLEX_POLICY_THRESHOLD_CONFIDENCE", &c.Policy.ThresholdConfidence},
	}
	for _, setter := range floatSetters {
		if err := applyFloat64(env, setter.key, setter.target); err != nil {
			return err
		}
	}

	stringSetters := []struct {
		key    string
		target *string
	}{
		{"NEEDLEX_MODELS_ROUTER", &c.Models.Router},
		{"NEEDLEX_MODELS_JUDGE", &c.Models.Judge},
		{"NEEDLEX_MODELS_EXTRACTOR", &c.Models.Extractor},
		{"NEEDLEX_MODELS_FORMATTER", &c.Models.Formatter},
	}
	for _, setter := range stringSetters {
		applyString(env, setter.key, setter.target)
	}

	return nil
}

func (c Config) Validate() error {
	errs := []error{}
	if c.Runtime.MaxPages <= 0 {
		errs = append(errs, fmt.Errorf("runtime.max_pages must be > 0"))
	}
	if c.Runtime.MaxDepth <= 0 {
		errs = append(errs, fmt.Errorf("runtime.max_depth must be > 0"))
	}
	if c.Runtime.TimeoutMS <= 0 {
		errs = append(errs, fmt.Errorf("runtime.timeout_ms must be > 0"))
	}
	if c.Runtime.MaxBytes <= 0 {
		errs = append(errs, fmt.Errorf("runtime.max_bytes must be > 0"))
	}
	if c.Runtime.LaneMax < 0 || c.Runtime.LaneMax > 4 {
		errs = append(errs, fmt.Errorf("runtime.lane_max must be between 0 and 4"))
	}

	if err := validateRatio("policy.threshold_conflict", c.Policy.ThresholdConflict); err != nil {
		errs = append(errs, err)
	}
	if err := validateRatio("policy.threshold_ambiguity", c.Policy.ThresholdAmbiguity); err != nil {
		errs = append(errs, err)
	}
	if err := validateRatio("policy.threshold_coverage", c.Policy.ThresholdCoverage); err != nil {
		errs = append(errs, err)
	}
	if err := validateRatio("policy.threshold_confidence", c.Policy.ThresholdConfidence); err != nil {
		errs = append(errs, err)
	}

	if c.Budget.MaxTokens <= 0 {
		errs = append(errs, fmt.Errorf("budget.max_tokens must be > 0"))
	}
	if c.Budget.MaxLatencyMS <= 0 {
		errs = append(errs, fmt.Errorf("budget.max_latency_ms must be > 0"))
	}

	if strings.TrimSpace(c.Models.Router) == "" {
		errs = append(errs, fmt.Errorf("models.router must not be empty"))
	}
	if strings.TrimSpace(c.Models.Judge) == "" {
		errs = append(errs, fmt.Errorf("models.judge must not be empty"))
	}
	if strings.TrimSpace(c.Models.Extractor) == "" {
		errs = append(errs, fmt.Errorf("models.extractor must not be empty"))
	}
	if strings.TrimSpace(c.Models.Formatter) == "" {
		errs = append(errs, fmt.Errorf("models.formatter must not be empty"))
	}

	if len(errs) == 0 {
		return nil
	}
	return errorsJoin(errs...)
}

func validateRatio(field string, value float64) error {
	if value < 0 || value > 1 {
		return fmt.Errorf("%s must be between 0 and 1", field)
	}
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
