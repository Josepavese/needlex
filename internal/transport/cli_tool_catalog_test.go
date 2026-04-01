package transport

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunToolCatalogOpenAI(t *testing.T) {
	r := NewRunner()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := r.runToolCatalog([]string{"--provider", "openai"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%s", code, stderr.String())
	}
	var cat openAIToolCatalog
	if err := json.Unmarshal(stdout.Bytes(), &cat); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if cat.Provider != "openai" {
		t.Fatalf("unexpected provider %q", cat.Provider)
	}
	if len(cat.Tools) == 0 {
		t.Fatalf("expected non-empty catalog")
	}
	foundRead := false
	foundQuery := false
	for _, tool := range cat.Tools {
		if tool.Function.Name == "web_read" {
			foundRead = true
		}
		if tool.Function.Name == "web_query" {
			foundQuery = true
		}
	}
	if !foundRead || !foundQuery {
		t.Fatalf("expected web_read and web_query in catalog")
	}
}

func TestRunToolCatalogOpenAIStrict(t *testing.T) {
	r := NewRunner()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := r.runToolCatalog([]string{"--provider", "openai", "--strict"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%s", code, stderr.String())
	}
	var cat openAIToolCatalog
	if err := json.Unmarshal(stdout.Bytes(), &cat); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !cat.Strict {
		t.Fatalf("expected strict catalog")
	}
	for _, tool := range cat.Tools {
		if !tool.Function.Strict {
			t.Fatalf("tool %q not strict", tool.Function.Name)
		}
	}
}

func TestRunToolCatalogAnthropic(t *testing.T) {
	r := NewRunner()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := r.runToolCatalog([]string{"--provider", "anthropic"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%s", code, stderr.String())
	}
	var cat anthropicToolCatalog
	if err := json.Unmarshal(stdout.Bytes(), &cat); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if cat.Provider != "anthropic" {
		t.Fatalf("unexpected provider %q", cat.Provider)
	}
	foundProof := false
	for _, tool := range cat.Tools {
		if tool.Name == "web_proof" {
			foundProof = true
			break
		}
	}
	if !foundProof {
		t.Fatalf("expected web_proof in catalog")
	}
}

func TestRunToolCatalogRejectsUnknownProvider(t *testing.T) {
	r := NewRunner()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := r.runToolCatalog([]string{"--provider", "unknown"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected code 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), `unsupported provider "unknown"`) {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}
