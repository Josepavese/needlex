package main

import "testing"

func TestSummarize(t *testing.T) {
	rows := []caseResult{
		{Family: "docs", RuntimeOK: true, QualityPass: true, Pass: true, SelectedURLPass: true, ProofUsable: true, LatencyMS: 100, PacketBytes: 1000},
		{Family: "docs", RuntimeOK: true, QualityPass: false, Pass: false, SelectedURLPass: false, ProofUsable: true, LatencyMS: 200, PacketBytes: 2000, FailureClasses: []string{"wrong_selected_url"}},
		{Family: "corporate", RuntimeOK: false, QualityPass: false, Pass: false, SelectedURLPass: false, ProofUsable: false, LatencyMS: 0, PacketBytes: 0, FailureClasses: []string{"network_timeout"}},
	}

	s := summarize(rows)
	if s.CaseCount != 3 {
		t.Fatalf("expected 3 cases, got %d", s.CaseCount)
	}
	if s.FailureClassCounts["wrong_selected_url"] != 1 {
		t.Fatalf("expected wrong_selected_url count 1, got %#v", s.FailureClassCounts)
	}
	if s.FailureClassCounts["network_timeout"] != 1 {
		t.Fatalf("expected network_timeout count 1, got %#v", s.FailureClassCounts)
	}
	if s.RuntimeSuccessRate != 2.0/3.0 {
		t.Fatalf("expected runtime success rate 2/3, got %v", s.RuntimeSuccessRate)
	}
	if s.QualityPassRate != 1.0/3.0 {
		t.Fatalf("expected quality pass rate 1/3, got %v", s.QualityPassRate)
	}
	if len(s.FamilyBreakdown) != 2 {
		t.Fatalf("expected 2 family breakdown rows, got %d", len(s.FamilyBreakdown))
	}
}

func TestClassifyExecutionError(t *testing.T) {
	tests := []struct {
		errText string
		want    string
	}{
		{"read failed: context deadline exceeded", "network_timeout"},
		{"fetch page: tls handshake failure", "network_tls_error"},
		{"dial tcp: connection refused", "network_connect_error"},
		{"something else", "runtime_error"},
	}
	for _, tt := range tests {
		if got := classifyExecutionError(tt.errText); got != tt.want {
			t.Fatalf("classifyExecutionError(%q)=%q want %q", tt.errText, got, tt.want)
		}
	}
}
