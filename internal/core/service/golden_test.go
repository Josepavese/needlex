package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
)

func TestReadGoldenArticleStandard(t *testing.T) {
	resp := runGoldenRead(t, "article.html", ReadRequest{
		Profile:   core.ProfileStandard,
		Objective: "proof replay deterministic context",
	})

	if resp.Document.Title != "Needle-X Runtime Deep Dive" {
		t.Fatalf("unexpected title %q", resp.Document.Title)
	}
	if len(resp.ResultPack.Chunks) == 0 {
		t.Fatal("expected chunks to be populated")
	}
	if len(resp.ResultPack.Chunks) > 6 {
		t.Fatalf("standard profile should keep at most 6 chunks, got %d", len(resp.ResultPack.Chunks))
	}
	if len(resp.ResultPack.Outline) < 2 {
		t.Fatalf("expected outline to preserve headings, got %#v", resp.ResultPack.Outline)
	}
	if !containsLine(resp.ResultPack.Outline, "Deterministic Core") {
		t.Fatalf("expected deterministic core in outline, got %#v", resp.ResultPack.Outline)
	}
	if containsChunkText(resp.ResultPack.Chunks, "newsletter noise") {
		t.Fatal("expected footer noise to be pruned")
	}
	if containsChunkText(resp.ResultPack.Chunks, "Home Docs Pricing") {
		t.Fatal("expected nav noise to be pruned")
	}
	if len(resp.ProofRecords) != len(resp.ResultPack.Chunks) {
		t.Fatalf("expected proof count to match chunks, got %d and %d", len(resp.ProofRecords), len(resp.ResultPack.Chunks))
	}
	if len(resp.ResultPack.Links) != 1 {
		t.Fatalf("expected one canonical link, got %d", len(resp.ResultPack.Links))
	}
}

func TestReadGoldenForumDeep(t *testing.T) {
	resp := runGoldenRead(t, "forum.html", ReadRequest{
		Profile: core.ProfileDeep,
	})

	if resp.ResultPack.Profile != core.ProfileDeep {
		t.Fatalf("expected deep profile, got %q", resp.ResultPack.Profile)
	}
	if len(resp.ResultPack.Chunks) != 3 {
		t.Fatalf("expected deep profile to keep all 3 forum chunks, got %d", len(resp.ResultPack.Chunks))
	}
	if !containsLine(resp.ResultPack.Outline, "Troubleshooting") {
		t.Fatalf("expected troubleshooting heading, got %#v", resp.ResultPack.Outline)
	}
	if !containsChunkText(resp.ResultPack.Chunks, "Compare stage hashes first.") {
		t.Fatal("expected troubleshooting text in chunks")
	}
	if resp.Replay.StageCount != 4 {
		t.Fatalf("expected 4 stages, got %d", resp.Replay.StageCount)
	}
	if !resp.Replay.Deterministic {
		t.Fatal("expected replay report to remain deterministic")
	}
}

func BenchmarkReadGoldenArticle(b *testing.B) {
	html := loadGoldenHTML(b, "article.html")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, html)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		b.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := svc.Read(context.Background(), ReadRequest{
			URL:     server.URL,
			Profile: core.ProfileStandard,
		})
		if err != nil {
			b.Fatalf("read failed: %v", err)
		}
	}
}

func runGoldenRead(t *testing.T, fixture string, req ReadRequest) ReadResponse {
	t.Helper()

	html := loadGoldenHTML(t, fixture)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, html)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	req.URL = server.URL
	resp, err := svc.Read(context.Background(), req)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	return resp
}

type testingReader interface {
	Helper()
	Fatalf(format string, args ...any)
}

func loadGoldenHTML(t testingReader, fixture string) string {
	t.Helper()

	path := filepath.Join("..", "..", "..", "testdata", "golden", fixture)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixture, err)
	}
	return string(data)
}

func containsChunkText(chunks []core.Chunk, needle string) bool {
	for _, chunk := range chunks {
		if strings.Contains(chunk.Text, needle) {
			return true
		}
	}
	return false
}

func containsLine(lines []string, needle string) bool {
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}
