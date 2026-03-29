package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
)

func TestHardCaseSuite(t *testing.T) {
	cases := []struct {
		name             string
		html             string
		objective        string
		profile          string
		forceLane        int
		minMaxLane       int
		minInvocations   int
		expectedContains string
	}{
		{
			name:             "embedded_app_shell_force_lane3",
			html:             `<html><head><title>App Shell</title></head><body><app-root></app-root><script>window._a2s={"configuration":{"blog":[{"title":"Needle Runtime","description":"<p>Needle-X compiles noisy pages into compact proof-carrying context for agents.</p>"}]}}</script></body></html>`,
			objective:        "company profile",
			profile:          core.ProfileStandard,
			forceLane:        3,
			minMaxLane:       3,
			minInvocations:   4,
			expectedContains: "Needle-X compiles noisy pages into compact proof-carrying context",
		},
		{
			name:             "forum_replay_investigation_force_lane2",
			html:             loadGoldenHTML(t, "forum.html"),
			objective:        "forum replay drift troubleshooting",
			profile:          core.ProfileDeep,
			forceLane:        2,
			minMaxLane:       2,
			minInvocations:   3,
			expectedContains: "Compare stage hashes first",
		},
		{
			name:             "embedded_state_without_force_still_yields_signal",
			html:             `<html><head><title>App Shell</title></head><body><app-root></app-root><script>window._a2s={"configuration":{"blog":[{"title":"Needle Runtime","description":"<p>Needle-X compiles noisy pages into compact context for agents.</p>"}]}}</script></body></html>`,
			objective:        "Needle Runtime summary",
			profile:          core.ProfileStandard,
			minMaxLane:       0,
			minInvocations:   0,
			expectedContains: "Needle-X compiles noisy pages into compact context for agents",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = fmt.Fprint(w, tc.html)
			}))
			defer server.Close()

			svc, err := New(config.Defaults(), server.Client())
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
			if !containsChunkText(resp.ResultPack.Chunks, tc.expectedContains) {
				t.Fatalf("expected packed chunks to contain %q, got %#v", tc.expectedContains, resp.ResultPack.Chunks)
			}
		})
	}
}

func BenchmarkHardCaseEmbeddedLane3(b *testing.B) {
	benchmarkHardCaseRead(b, `<html><head><title>App Shell</title></head><body><app-root></app-root><script>window._a2s={"configuration":{"blog":[{"title":"Needle Runtime","description":"<p>Needle-X compiles noisy pages into compact proof-carrying context for agents.</p>"}]}}</script></body></html>`, ReadRequest{Objective: "company profile", Profile: core.ProfileStandard, ForceLane: 3})
}

func BenchmarkHardCaseForumLane2(b *testing.B) {
	benchmarkHardCaseRead(b, mustLoadHardCaseFixture(b, "forum.html"), ReadRequest{Objective: "forum replay drift troubleshooting", Profile: core.ProfileDeep, ForceLane: 2})
}

func benchmarkHardCaseRead(b *testing.B, html string, req ReadRequest) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, html)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		b.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	req.URL = server.URL

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.Read(context.Background(), req); err != nil {
			b.Fatalf("read failed: %v", err)
		}
	}
}

func mustLoadHardCaseFixture(tb testing.TB, name string) string {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "golden", name))
	if err != nil {
		tb.Fatalf("load fixture %s: %v", name, err)
	}
	return string(data)
}
