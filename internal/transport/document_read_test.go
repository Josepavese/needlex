package transport

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	coreservice "github.com/josepavese/needlex/internal/core/service"
)

func pdfReadResponse() coreservice.ReadResponse {
	resp := fakeResponse()
	resp.DocumentExtraction = &core.DocumentExtraction{
		Schema:         core.DocumentReadSchemaV1,
		Format:         "pdf",
		RawSHA256:      strings.Repeat("a", 64),
		SourceBytes:    18810,
		PagesTotal:     1,
		PagesProcessed: 1,
		PagesWithText:  1,
		Coverage:       "complete",
		Limits: core.DocumentLimits{
			MaxBytes: 4_000_000, MaxPages: 100, MaxOutputChars: 200_000, TimeoutMS: 5000,
		},
	}
	resp.ResultPack.Chunks[0].PageStart = 1
	resp.ResultPack.Chunks[0].PageEnd = 1
	resp.AgentContext.Chunks[0].PageStart = 1
	resp.AgentContext.Chunks[0].PageEnd = 1
	resp.ProofRecords[0].Proof.SourceSpan.Page = 1
	resp.ProofRecords[0].Proof.SourceSpan.Selector = "/pdf/page[1]/block[1]"
	return resp
}

func TestCompactPDFReadExposesDocumentContract(t *testing.T) {
	compact := compactReadResponse(pdfReadResponse())
	if compact.DocumentExtraction == nil || compact.DocumentExtraction.Schema != core.DocumentReadSchemaV1 {
		t.Fatalf("missing document extraction: %+v", compact.DocumentExtraction)
	}
	if compact.Signals.ContentSource != "pdf_text" {
		t.Fatalf("content source = %q, want pdf_text", compact.Signals.ContentSource)
	}
	if len(compact.Chunks) == 0 || compact.Chunks[0].PageStart != 1 || compact.Chunks[0].PageEnd != 1 {
		t.Fatalf("missing page metadata: %+v", compact.Chunks)
	}
}

func TestCLIAndMCPReadPDFCarryDocumentContract(t *testing.T) {
	runner := Runner{
		loadConfig: func(string) (config.Config, error) { return config.Defaults(), nil },
		read: func(context.Context, config.Config, coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			return pdfReadResponse(), nil
		},
		storeRoot: t.TempDir(),
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runner.Run([]string{"read", "https://example.com/sample.pdf", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("CLI exit %d: %s", code, stderr.String())
	}
	for _, want := range []string{`"document_extraction"`, `"schema": "needlex.document.read.v1"`, `"page_start": 1`, `"pdf_text"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("CLI output missing %s: %s", want, stdout.String())
		}
	}

	result, err := runner.callMCPReadTool(map[string]any{"url": "https://example.com/sample.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("missing structured content: %#v", result)
	}
	if _, ok := structured["document_extraction"].(map[string]any); !ok {
		t.Fatalf("MCP compact output missing document extraction: %#v", structured)
	}

	result, err = runner.callMCPReadTool(map[string]any{"url": "https://example.com/sample.pdf", "include_diagnostics": true})
	if err != nil {
		t.Fatal(err)
	}
	structured = result["structuredContent"].(map[string]any)
	diagnostics, ok := structured["diagnostics"].(map[string]any)
	if !ok {
		t.Fatalf("missing diagnostics: %#v", structured)
	}
	if diagnostics["document_extraction"] == nil {
		t.Fatalf("MCP diagnostics missing document extraction: %#v", diagnostics)
	}
}
