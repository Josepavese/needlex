package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/core/failure"
	"github.com/josepavese/needlex/internal/rendering"
)

func TestReadPDFReturnsPageAwareProof(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/pdf/sample.pdf")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(data)
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.Render.Enabled = false
	svc, err := NewWithRenderer(cfg, nil, t.TempDir(), rendering.NoopRenderer{})
	if err != nil {
		t.Fatal(err)
	}
	svc.semantic = noScoreSemanticAligner{}
	resp, err := svc.Read(context.Background(), ReadRequest{
		URL:        server.URL + "/sample.pdf",
		Objective:  "Extract the title and first sentence from page one",
		RenderMode: "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.DocumentExtraction == nil || resp.DocumentExtraction.PagesTotal != 1 || resp.DocumentExtraction.PagesWithText != 1 {
		t.Fatalf("unexpected extraction: %+v", resp.DocumentExtraction)
	}
	if !strings.Contains(strings.Join(chunkTexts(resp.ResultPack.Chunks), " "), "Sample PDF") {
		t.Fatalf("title missing from chunks: %#v", chunkTexts(resp.ResultPack.Chunks))
	}
	if !strings.Contains(strings.Join(chunkTexts(resp.ResultPack.Chunks), " "), "This is a simple PDF file. Fun fun fun.") {
		t.Fatalf("first sentence missing from chunks: %#v", chunkTexts(resp.ResultPack.Chunks))
	}
	if len(resp.ProofRecords) == 0 || resp.ProofRecords[0].Proof.SourceSpan.Page != 1 {
		t.Fatalf("proof missing page: %+v", resp.ProofRecords)
	}
}

func chunkTexts(chunks []core.Chunk) []string {
	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		texts = append(texts, chunk.Text)
	}
	return texts
}

func TestReadPDFErrorsAreClassified(t *testing.T) {
	cases := []struct {
		name string
		file string
		want failure.Class
	}{
		{name: "encrypted", file: "encrypted.pdf", want: failure.ClassDocumentEncrypted},
		{name: "no text", file: "blank.pdf", want: failure.ClassDocumentNoText},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile("../../../testdata/pdf/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/pdf")
				_, _ = w.Write(data)
			}))
			defer server.Close()
			cfg := config.Defaults()
			cfg.Render.Enabled = false
			svc, err := NewWithRenderer(cfg, nil, t.TempDir(), rendering.NoopRenderer{})
			if err != nil {
				t.Fatal(err)
			}
			svc.semantic = noScoreSemanticAligner{}
			if _, err := svc.Read(context.Background(), ReadRequest{URL: server.URL + "/" + tc.file, RenderMode: "off"}); err == nil {
				t.Fatal("expected read error")
			} else if got := failure.Classify(err); got != tc.want {
				t.Fatalf("class = %q, want %q (err=%v)", got, tc.want, err)
			}
		})
	}
}

func TestReadPDFRenderRequiredFailsClearly(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/pdf/sample.pdf")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(data)
	}))
	defer server.Close()
	cfg := config.Defaults()
	cfg.Render.Enabled = false
	svc, err := NewWithRenderer(cfg, nil, t.TempDir(), rendering.NoopRenderer{})
	if err != nil {
		t.Fatal(err)
	}
	svc.semantic = noScoreSemanticAligner{}
	_, err = svc.Read(context.Background(), ReadRequest{URL: server.URL + "/sample.pdf", RenderMode: "required"})
	if err == nil {
		t.Fatal("expected render required error")
	}
	if got := failure.Classify(err); got != failure.ClassRenderNotApplicable {
		t.Fatalf("class = %q, want %q (err=%v)", got, failure.ClassRenderNotApplicable, err)
	}
}

func TestReadPDFOutputLimitAppearsInResponse(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/pdf/multi.pdf")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(data)
	}))
	defer server.Close()
	cfg := config.Defaults()
	cfg.Render.Enabled = false
	cfg.Document.MaxOutputChars = 100
	svc, err := NewWithRenderer(cfg, nil, t.TempDir(), rendering.NoopRenderer{})
	if err != nil {
		t.Fatal(err)
	}
	svc.semantic = noScoreSemanticAligner{}
	resp, err := svc.Read(context.Background(), ReadRequest{URL: server.URL + "/multi.pdf", RenderMode: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.DocumentExtraction == nil || !resp.DocumentExtraction.Truncated || resp.DocumentExtraction.Coverage != "partial" {
		t.Fatalf("unexpected extraction: %+v", resp.DocumentExtraction)
	}
	if resp.DocumentExtraction.Limits.MaxOutputChars != 100 {
		t.Fatalf("limits not reported: %+v", resp.DocumentExtraction.Limits)
	}
}

func TestReadPDFByteLimitIsClassified(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/pdf/sample.pdf")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(data)
	}))
	defer server.Close()
	cfg := config.Defaults()
	cfg.Render.Enabled = false
	cfg.Runtime.MaxBytes = 100
	svc, err := NewWithRenderer(cfg, nil, t.TempDir(), rendering.NoopRenderer{})
	if err != nil {
		t.Fatal(err)
	}
	svc.semantic = noScoreSemanticAligner{}
	_, err = svc.Read(context.Background(), ReadRequest{URL: server.URL + "/sample.pdf", RenderMode: "off"})
	if err == nil {
		t.Fatal("expected byte limit error")
	}
	if got := failure.Classify(err); got != failure.ClassDocumentLimit {
		t.Fatalf("class = %q, want %q (err=%v)", got, failure.ClassDocumentLimit, err)
	}
}
