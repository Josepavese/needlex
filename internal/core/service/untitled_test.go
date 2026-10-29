package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/rendering"
)

func TestUntitledPageRetainsFactsAndValidTrace(t *testing.T) {
	html := `<main><p>The product warranty lasts for 37 months after purchase. Repairs remain available throughout this period.</p></main>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(html))
	}))
	defer server.Close()
	for _, mode := range []string{"off", "required"} {
		t.Run(mode, func(t *testing.T) {
			cfg := testConfig()
			cfg.Agent.ReadableEnabled = false
			cfg.Render.Enabled = true
			svc := newTestService(t, cfg, server.Client())
			svc.renderer = fakeRenderer{page: rendering.Page{URL: server.URL, FinalURL: server.URL, HTML: html, Browser: "fixture cdp", FetchedAt: time.Unix(1700000000, 0)}}
			response, err := svc.Read(t.Context(), ReadRequest{URL: server.URL, Objective: "Warranty conditions", Profile: "deep", RenderMode: mode})
			if err != nil {
				t.Fatal(err)
			}
			if response.Document.Title != "" {
				t.Fatal("missing title was fabricated")
			}
			found := false
			for _, chunk := range response.ResultPack.Chunks {
				found = found || strings.Contains(chunk.Text, "37 months")
			}
			if !found || len(response.ProofRecords) == 0 || !response.Replay.Deterministic {
				t.Fatal("untitled source lost facts, proof or replay")
			}
			for _, stage := range response.Trace.Stages {
				if stage.Stage == "reduce" {
					if _, ok := stage.Metadata["title"]; ok {
						t.Fatal("absent title was emitted as required nonempty metadata")
					}
				}
			}
		})
	}
}
