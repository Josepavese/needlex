package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/rendering"
)

func TestReadRetainsRenderedInlineContentAndProof(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>Knowledge</title><script src="/app.js"></script></head><body><main></main></body></html>`)
	}))
	defer server.Close()
	for _, mode := range []string{"auto", "required"} {
		t.Run(mode, func(t *testing.T) {
			cfg := testConfig()
			cfg.Render.Enabled = true
			svc, err := NewWithStateRoot(cfg, server.Client(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			svc.renderer = fakeRenderer{page: rendering.Page{
				URL: server.URL, FinalURL: server.URL,
				HTML:    `<html><head><title>Knowledge</title></head><body><h1>Knowledge</h1><div><span>La conoscenza resta condivisa e accessibile a tutti.</span><small>Autrice: Sofia</small><x-record>知識は共有される。</x-record></div></body></html>`,
				Browser: "fake", Duration: time.Millisecond, FetchedAt: time.Unix(1700000000, 0).UTC(),
			}}
			resp, err := svc.Read(context.Background(), ReadRequest{URL: server.URL, Profile: core.ProfileStandard, RenderMode: mode})
			if err != nil {
				t.Fatal(err)
			}
			if resp.Document.FetchMode != core.FetchModeRender {
				t.Fatalf("expected rendered content, got %s", resp.Document.FetchMode)
			}
			for _, want := range []string{"La conoscenza resta condivisa e accessibile a tutti.", "Autrice: Sofia", "知識は共有される。"} {
				if !containsChunkText(resp.ResultPack.Chunks, want) {
					t.Fatalf("missing rendered content %q", want)
				}
			}
			records := make(map[string]proof.ProofRecord, len(resp.ProofRecords))
			for _, record := range resp.ProofRecords {
				records[record.ID] = record
			}
			if len(resp.AgentContext.Chunks) == 0 {
				t.Fatal("missing agent context")
			}
			for _, chunk := range resp.AgentContext.Chunks {
				record, ok := records[chunk.ProofRef]
				if !ok || record.Proof.ChunkID != chunk.ID || chunk.SourceURL != server.URL || chunk.SourceSelector != record.Proof.SourceSpan.Selector {
					t.Fatalf("unresolved rendered content proof: %#v", chunk)
				}
				if !strings.Contains(chunk.SourceSelector, "/text()[1]") {
					t.Fatalf("missing text-node provenance: %s", chunk.SourceSelector)
				}
			}
		})
	}
}
