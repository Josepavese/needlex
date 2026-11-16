package document

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/pipeline"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../testdata/pdf/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func extractFixture(t *testing.T, name string, limits pipeline.DocumentLimits) (pipeline.SimplifiedDOM, error) {
	t.Helper()
	return Extract(context.Background(), pipeline.RawPage{
		FinalURL:    "https://example.test/" + name,
		ContentType: "application/pdf",
		Body:        readFixture(t, name),
	}, limits)
}

func defaultDocumentLimits() pipeline.DocumentLimits {
	return pipeline.DocumentLimits{MaxBytes: 4_000_000, MaxPages: 100, MaxOutputChars: 200_000, TimeoutMS: 5000}
}

func TestExtractPDFKeepsPageAndReadingOrder(t *testing.T) {
	dom, err := extractFixture(t, "sample.pdf", defaultDocumentLimits())
	if err != nil {
		t.Fatal(err)
	}
	if dom.Title != "Sample PDF" {
		t.Fatalf("title = %q, want Sample PDF", dom.Title)
	}
	if dom.Document == nil || dom.Document.PagesTotal != 1 || dom.Document.PagesProcessed != 1 || dom.Document.PagesWithText != 1 {
		t.Fatalf("unexpected document info: %+v", dom.Document)
	}
	if dom.Document.RawSHA256 != "229defbb0cee6f02673a5cde290d0673e75a0dc31cec43989c8ab2a4eca7e1bb" || dom.Document.SourceBytes != 18810 {
		t.Fatalf("unexpected raw identity: %+v", dom.Document)
	}
	if len(dom.Nodes) == 0 || dom.Nodes[0].Page != 1 {
		t.Fatalf("expected page-tagged nodes, got %+v", dom.Nodes)
	}
	body := strings.Join(nodeTexts(dom.Nodes), "\n")
	if !strings.Contains(body, "This is a simple PDF file. Fun fun fun.") {
		t.Fatalf("first sentence missing from %q", body)
	}
	if !strings.HasPrefix(dom.Nodes[0].Path, "/pdf/page[1]/block[") {
		t.Fatalf("unexpected selector %q", dom.Nodes[0].Path)
	}
}

func TestExtractPDFNoTextIsDistinct(t *testing.T) {
	_, err := extractFixture(t, "blank.pdf", defaultDocumentLimits())
	if code := ErrorCode(err); code != CodeNoText {
		t.Fatalf("error = %v, code = %q, want %q", err, code, CodeNoText)
	}
}

func TestExtractPDFEncryptedIsDistinct(t *testing.T) {
	_, err := extractFixture(t, "encrypted.pdf", defaultDocumentLimits())
	if code := ErrorCode(err); code != CodeEncrypted {
		t.Fatalf("error = %v, code = %q, want %q", err, code, CodeEncrypted)
	}
}

func TestExtractPDFMalformedIsDistinct(t *testing.T) {
	_, err := Extract(context.Background(), pipeline.RawPage{FinalURL: "https://example.test/bad.pdf", ContentType: "application/pdf", Body: []byte("%PDF-1.7\nnot a PDF")}, defaultDocumentLimits())
	if code := ErrorCode(err); code != CodeMalformed {
		t.Fatalf("error = %v, code = %q, want %q", err, code, CodeMalformed)
	}
}

func TestExtractPDFByteLimit(t *testing.T) {
	limits := defaultDocumentLimits()
	limits.MaxBytes = 100
	_, err := extractFixture(t, "sample.pdf", limits)
	if code := ErrorCode(err); code != CodeLimit {
		t.Fatalf("error = %v, code = %q, want %q", err, code, CodeLimit)
	}
}

func TestExtractPDFOutputLimitIsReported(t *testing.T) {
	limits := defaultDocumentLimits()
	limits.MaxOutputChars = 100
	dom, err := extractFixture(t, "multi.pdf", limits)
	if err != nil {
		t.Fatal(err)
	}
	if dom.Document == nil || !dom.Document.Truncated || dom.Document.Coverage != "partial" {
		t.Fatalf("expected truncated partial extraction, got %+v", dom.Document)
	}
	total := 0
	for _, node := range dom.Nodes {
		total += len([]rune(node.Text))
	}
	if total > limits.MaxOutputChars {
		t.Fatalf("extracted %d chars, exceeds limit %d", total, limits.MaxOutputChars)
	}
}

func TestExtractPDFPageLimitIsReported(t *testing.T) {
	limits := defaultDocumentLimits()
	limits.MaxPages = 2
	dom, err := extractFixture(t, "multi.pdf", limits)
	if err != nil {
		t.Fatal(err)
	}
	if dom.Document.PagesTotal != 3 || dom.Document.PagesProcessed != 2 || !dom.Document.Truncated {
		t.Fatalf("unexpected page metadata: %+v", dom.Document)
	}
	for _, node := range dom.Nodes {
		if node.Page > 2 {
			t.Fatalf("extracted page %d beyond limit", node.Page)
		}
	}
}

func TestExtractPDFHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Extract(ctx, pipeline.RawPage{FinalURL: "https://example.test/sample.pdf", ContentType: "application/pdf", Body: readFixture(t, "sample.pdf")}, defaultDocumentLimits())
	if code := ErrorCode(err); code != CodeTimeout {
		t.Fatalf("error = %v, code = %q, want %q", err, code, CodeTimeout)
	}
}

func nodeTexts(nodes []pipeline.SimplifiedNode) []string {
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, node.Text)
	}
	return out
}
