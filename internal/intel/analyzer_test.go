package intel

import (
	"testing"

	"github.com/josepavese/needlex/internal/config"
)

func TestAnalyzeEscalatesAmbiguousSegment(t *testing.T) {
	analyzer := New(config.Defaults())
	summary := analyzer.Analyze([]Input{
		{
			Fingerprint:      "fp_1",
			Text:             "Short note.",
			HeadingPath:      []string{"Intro"},
			ContextAlignment: 0.18,
			Score:            0.65,
			Confidence:       0.70,
		},
	}, Hints{})

	decision := summary.Decisions["fp_1"]
	if decision.Lane != 1 {
		t.Fatalf("expected lane 1, got %d", decision.Lane)
	}
	if decision.ReasonCode == "" {
		t.Fatal("expected reason code")
	}
	if len(decision.ModelInvocations) != 2 {
		t.Fatalf("expected 2 policy invocations, got %d", len(decision.ModelInvocations))
	}
}

func TestAnalyzeKeepsClearSegmentDeterministic(t *testing.T) {
	analyzer := New(config.Defaults())
	summary := analyzer.Analyze([]Input{
		{
			Fingerprint:      "fp_1",
			Text:             "Proof and replay keep every extraction deterministic and auditable for local agents.",
			HeadingPath:      []string{"Deterministic Core"},
			ContextAlignment: 0.92,
			Score:            0.94,
			Confidence:       0.95,
		},
	}, Hints{})

	decision := summary.Decisions["fp_1"]
	if decision.Lane != 0 {
		t.Fatalf("expected lane 0, got %d", decision.Lane)
	}
	if decision.ReasonCode != "" {
		t.Fatalf("expected no reason code, got %q", decision.ReasonCode)
	}
}

func TestAnalyzeRespectsDomainForceLane(t *testing.T) {
	analyzer := New(config.Defaults())
	summary := analyzer.Analyze([]Input{
		{
			Fingerprint:      "fp_1",
			Text:             "Proof and replay keep every extraction deterministic and auditable for local agents.",
			HeadingPath:      []string{"Deterministic Core"},
			ContextAlignment: 0.92,
			Score:            0.94,
			Confidence:       0.95,
		},
	}, Hints{ForceLane: 1})

	decision := summary.Decisions["fp_1"]
	if decision.Lane != 1 {
		t.Fatalf("expected forced lane 1, got %d", decision.Lane)
	}
	if decision.ReasonCode != ReasonDomainForceLane {
		t.Fatalf("expected force lane reason, got %q", decision.ReasonCode)
	}
}

func TestAnalyzeEscalatesToExtractorAndFormatter(t *testing.T) {
	cfg := config.Defaults()
	analyzer := New(cfg)
	summary := analyzer.Analyze([]Input{
		{
			Fingerprint:      "fp_1",
			Text:             "Short proof. Replay deterministic context.",
			HeadingPath:      []string{"Overview"},
			ContextAlignment: 0.24,
			Score:            0.60,
			Confidence:       0.60,
		},
	}, Hints{ForceLane: 3, Profile: "tiny"})

	decision := summary.Decisions["fp_1"]
	if decision.Lane != 3 {
		t.Fatalf("expected lane 3, got %d", decision.Lane)
	}
	if decision.ReasonCode != ReasonDomainForceLane {
		t.Fatalf("expected domain force lane reason code, got %q", decision.ReasonCode)
	}
	if len(decision.TransformChain) < 4 {
		t.Fatalf("expected extended transform chain, got %#v", decision.TransformChain)
	}
}
