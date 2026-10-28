package config

import "testing"

func TestDecisionConfigRejectsExternalRuntimeAndIncompleteIdentity(t *testing.T) {
	valid := DecisionConfig{Mode: "observe", BaseURL: "http://127.0.0.1:11941", Model: "Julia", Revision: "sha256:pinned", TimeoutMS: 1000, MaxQuestionTokens: 256, MaxInputBytes: 6000}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"https://example.com", "http://localhost:11941", "http://127.0.0.1@example.com", "http://127.0.0.1:11941?upstream=cloud", "http://127.0.0.1:11941#ignored"} {
		cfg := valid
		cfg.BaseURL = endpoint
		if err := cfg.Validate(); err == nil {
			t.Fatalf("accepted endpoint %q", endpoint)
		}
	}
	for _, mode := range []string{"active", "fallback"} {
		cfg := valid
		cfg.Mode = mode
		if err := cfg.Validate(); err == nil {
			t.Fatal("accepted an operational decision mode")
		}
	}
	valid.Revision = ""
	if err := valid.Validate(); err == nil {
		t.Fatal("accepted unpinned model identity")
	}
	if err := (DecisionConfig{}).Validate(); err != nil {
		t.Fatal(err)
	}
}
