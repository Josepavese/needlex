package modelbaseline

import "testing"

func TestDefault(t *testing.T) {
	m := Default()
	if m.CPUBaselineID != "gemma3_1b_all" {
		t.Fatalf("unexpected baseline id: %q", m.CPUBaselineID)
	}
	if m.Models.Router == "" || m.Models.Extractor == "" || m.Models.Formatter == "" {
		t.Fatal("expected populated model defaults")
	}
	if m.Timeouts.MicroMS <= 0 || m.Timeouts.StructuredMS <= 0 || m.Timeouts.SpecialistMS <= 0 {
		t.Fatal("expected positive timeout defaults")
	}
}
