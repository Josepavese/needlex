package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
)

const testHTML = `
<html>
  <head><title>Needle Runtime</title></head>
  <body>
    <nav>ignored nav</nav>
    <article>
      <h1>Needle Runtime</h1>
      <p>Needle-X compiles noisy pages into compact context.</p>
      <h2>Details</h2>
      <p>Proof and replay are emitted for every run.</p>
      <ul><li>Local-first</li><li>Deterministic</li></ul>
    </article>
    <footer>ignored footer</footer>
  </body>
</html>
`

func TestReadRunsDeterministicPipelineEndToEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	resp, err := svc.Read(context.Background(), ReadRequest{
		URL: server.URL,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if resp.Document.FinalURL != server.URL {
		t.Fatalf("expected final url %q, got %q", server.URL, resp.Document.FinalURL)
	}
	if len(resp.ResultPack.Chunks) == 0 {
		t.Fatal("expected chunks to be produced")
	}
	if len(resp.ProofRecords) != len(resp.ResultPack.Chunks) {
		t.Fatalf("expected proof count to match chunks, got %d proofs and %d chunks", len(resp.ProofRecords), len(resp.ResultPack.Chunks))
	}
	if resp.Replay.StageCount != 4 {
		t.Fatalf("expected 4 stages, got %d", resp.Replay.StageCount)
	}
	if len(resp.Trace.Events) < 8 {
		t.Fatalf("expected stage start/completion events, got %d", len(resp.Trace.Events))
	}
	if resp.ResultPack.CostReport.LanePath[0] != 0 {
		t.Fatalf("expected deterministic lane path, got %#v", resp.ResultPack.CostReport.LanePath)
	}
}

func TestReadRejectsEmptyURL(t *testing.T) {
	svc, err := New(config.Defaults(), nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.Read(context.Background(), ReadRequest{})
	if err == nil {
		t.Fatal("expected empty URL to fail")
	}
}
