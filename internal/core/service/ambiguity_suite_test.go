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

func TestAmbiguitySuite(t *testing.T) {
	cases := []struct {
		name             string
		html             string
		objective        string
		forceLane        int
		profile          string
		threshold        float64
		minMaxLane       int
		minInvocations   int
		expectedContains string
	}{
		{
			name:             "deterministic_docs_stays_lane0",
			html:             `<html><head><title>Replay Guide</title></head><body><article><h1>Replay Guide</h1><p>Proof replay deterministic context for operators.</p></article></body></html>`,
			objective:        "proof replay deterministic",
			profile:          core.ProfileTiny,
			threshold:        config.Defaults().Policy.ThresholdAmbiguity,
			minMaxLane:       0,
			minInvocations:   0,
			expectedContains: "Proof replay deterministic context",
		},
		{
			name:             "ambiguous_forum_escalates",
			html:             `<html><head><title>Forum Thread</title></head><body><article><h1>Regression incident</h1><p>Users report replay mismatch during a regression incident.</p><p>Operators compare stage hashes first.</p></article></body></html>`,
			objective:        "forum thread regression incident",
			profile:          core.ProfileTiny,
			threshold:        0.10,
			minMaxLane:       1,
			minInvocations:   1,
			expectedContains: "regression incident",
		},
		{
			name:             "forced_lane3_runs_full_stack",
			html:             `<html><head><title>Company</title></head><body><article><h1>Needle Runtime</h1><p>Needle-X compiles noisy public pages into compact context.</p></article></body></html>`,
			objective:        "company profile",
			profile:          core.ProfileTiny,
			forceLane:        3,
			threshold:        config.Defaults().Policy.ThresholdAmbiguity,
			minMaxLane:       3,
			minInvocations:   4,
			expectedContains: "Needle-X compiles noisy public pages",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = fmt.Fprint(w, tc.html)
			}))
			defer server.Close()

			cfg := config.Defaults()
			cfg.Policy.ThresholdAmbiguity = tc.threshold
			svc, err := New(cfg, server.Client())
			if err != nil {
				t.Fatalf("new service: %v", err)
			}
			svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

			resp, err := svc.Read(context.Background(), ReadRequest{URL: server.URL, Objective: tc.objective, Profile: tc.profile, ForceLane: tc.forceLane})
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}
			if maxLane(resp.ResultPack.CostReport.LanePath) < tc.minMaxLane {
				t.Fatalf("expected max lane >= %d, got %#v", tc.minMaxLane, resp.ResultPack.CostReport.LanePath)
			}
			if tc.minInvocations > 0 && len(resp.ProofRecords[0].Proof.ModelInvocations) < tc.minInvocations {
				t.Fatalf("expected at least %d model invocations, got %d", tc.minInvocations, len(resp.ProofRecords[0].Proof.ModelInvocations))
			}
			if tc.expectedContains != "" && !containsChunkText(resp.ResultPack.Chunks, tc.expectedContains) {
				t.Fatalf("expected packed chunks to contain %q, got %#v", tc.expectedContains, resp.ResultPack.Chunks)
			}
		})
	}
}
