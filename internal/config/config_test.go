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
		"NEEDLEX_RUNTIME_MAX_DEPTH":         "7",
		"NEEDLEX_POLICY_THRESHOLD_CONFLICT": "0.65",
		"NEEDLEX_MODELS_FORMATTER":          "formatter-x",
	}

	if err := cfg.ApplyEnv(env); err != nil {
		t.Fatalf("apply env: %v", err)
	}
	if cfg.Runtime.MaxDepth != 7 {
		t.Fatalf("expected runtime.max_depth=7, got %d", cfg.Runtime.MaxDepth)
	}
	if cfg.Policy.ThresholdConflict != 0.65 {
		t.Fatalf("expected threshold conflict override, got %f", cfg.Policy.ThresholdConflict)
	}
	if cfg.Models.Formatter != "formatter-x" {
		t.Fatalf("expected formatter override, got %q", cfg.Models.Formatter)
	}
}

func TestLoadRejectsInvalidEnvValue(t *testing.T) {
	t.Setenv("NEEDLEX_RUNTIME_MAX_PAGES", "oops")

	if _, err := Load(""); err == nil {
		t.Fatal("expected invalid env value to fail")
	}
}
