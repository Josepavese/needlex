package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
)

type hardCaseMetrics struct {
	maxLane        int
	invocations    int
	fidelity       float64
	objectiveScore float64
	signalDensity  float64
	tokens         int
}

func TestHardCaseMatrix(t *testing.T) {
	t.Run("embedded_lane3_beats_lane0_on_controlled_fullstack_path", func(t *testing.T) {
		html := `<html><head><title>App Shell</title></head><body><app-root></app-root><script>window._a2s={"configuration":{"blog":[{"title":"Needle Runtime","description":"<p>Needle-X compiles noisy pages into compact proof-carrying context for agents.</p>"}]}}</script></body></html>`
		expected := []string{"Needle-X compiles noisy pages into compact proof-carrying context for agents"}
		lane0 := runHardCaseMetrics(t, html, expected, ReadRequest{Objective: "company profile", Profile: core.ProfileStandard})
		lane3 := runHardCaseMetrics(t, html, expected, ReadRequest{Objective: "company profile", Profile: core.ProfileStandard, ForceLane: 3})
		if lane3.maxLane < 3 {
			t.Fatalf("expected lane3 path, got %#v", lane3)
		}
		if lane3.invocations <= lane0.invocations {
			t.Fatalf("expected more invocations at lane3, lane0=%#v lane3=%#v", lane0, lane3)
		}
		if lane3.fidelity < lane0.fidelity {
			t.Fatalf("expected lane3 fidelity >= lane0, lane0=%#v lane3=%#v", lane0, lane3)
		}
		if lane3.signalDensity < lane0.signalDensity {
			t.Fatalf("expected lane3 signal density >= lane0, lane0=%#v lane3=%#v", lane0, lane3)
		}
		if lane3.objectiveScore < lane0.objectiveScore {
			t.Fatalf("expected lane3 objective score >= lane0, lane0=%#v lane3=%#v", lane0, lane3)
		}
		if lane3.fidelity < 1.0 {
			t.Fatalf("expected lane3 fidelity to retain the embedded signal, got %#v", lane3)
		}
	})

	t.Run("forum_lane2_beats_lane0_on_troubleshooting_focus", func(t *testing.T) {
		html := loadGoldenHTML(t, "forum.html")
		expected := []string{"Compare stage hashes first", "inspect proof records by chunk id", "replay became deterministic again"}
		lane0 := runHardCaseMetrics(t, html, expected, ReadRequest{Objective: "forum replay drift troubleshooting", Profile: core.ProfileDeep})
		lane2 := runHardCaseMetrics(t, html, expected, ReadRequest{Objective: "forum replay drift troubleshooting", Profile: core.ProfileDeep, ForceLane: 2})
		if lane2.maxLane < 2 {
			t.Fatalf("expected lane2 path, got %#v", lane2)
		}
		if lane2.invocations <= lane0.invocations {
			t.Fatalf("expected more invocations at lane2, lane0=%#v lane2=%#v", lane0, lane2)
		}
		if lane2.fidelity < 0.66 {
			t.Fatalf("expected lane2 fidelity to preserve most troubleshooting anchors, got %#v", lane2)
		}
		if lane2.signalDensity < lane0.signalDensity {
			t.Fatalf("expected lane2 signal density >= lane0, lane0=%#v lane2=%#v", lane0, lane2)
		}
		if lane2.objectiveScore < lane0.objectiveScore {
			t.Fatalf("expected lane2 objective score >= lane0, lane0=%#v lane2=%#v", lane0, lane2)
		}
	})

	t.Run("tiny_lane3_compacts_without_losing_signal", func(t *testing.T) {
		html := `<html><head><title>Needle Runtime</title></head><body><article><h1>Needle Runtime</h1><p>Needle-X compiles noisy public pages into compact context for agents and keeps proof and replay auditable for operators.</p></article></body></html>`
		expected := []string{
			"Needle-X compiles noisy public pages compact context agents",
			"proof replay auditable operators",
		}
		lane0 := runHardCaseMetrics(t, html, expected, ReadRequest{Objective: "company profile", Profile: core.ProfileTiny})
		lane3 := runHardCaseMetrics(t, html, expected, ReadRequest{Objective: "company profile", Profile: core.ProfileTiny, ForceLane: 3})
		if lane3.maxLane < 3 {
			t.Fatalf("expected lane3 path, got %#v", lane3)
		}
		if lane3.tokens > lane0.tokens {
			t.Fatalf("expected tiny lane3 not to expand token count, lane0=%#v lane3=%#v", lane0, lane3)
		}
		if lane3.signalDensity < lane0.signalDensity {
			t.Fatalf("expected tiny lane3 signal density >= lane0, lane0=%#v lane3=%#v", lane0, lane3)
		}
		if lane3.objectiveScore < lane0.objectiveScore {
			t.Fatalf("expected tiny lane3 objective score >= lane0, lane0=%#v lane3=%#v", lane0, lane3)
		}
		if lane3.fidelity < lane0.fidelity {
			t.Fatalf("expected tiny lane3 to preserve tiny-profile fidelity, lane0=%#v lane3=%#v", lane0, lane3)
		}
	})
}

func runHardCaseMetrics(t *testing.T, html string, expected []string, req ReadRequest) hardCaseMetrics {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, html)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	req.URL = server.URL

	resp, err := svc.Read(context.Background(), req)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	joined := joinChunkText(resp.ResultPack.Chunks)
	return hardCaseMetrics{
		maxLane:        maxLane(resp.ResultPack.CostReport.LanePath),
		invocations:    len(resp.ProofRecords[0].Proof.ModelInvocations),
		fidelity:       fidelityScore(resp.ResultPack.Chunks, expected),
		objectiveScore: objectiveSignalDensity(joined, req.Objective),
		signalDensity:  signalDensity(joined, expected),
		tokens:         tokenCount(joined),
	}
}
