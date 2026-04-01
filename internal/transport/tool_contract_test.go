package transport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProviderToolCatalogsMatchMCPTools(t *testing.T) {
	root := filepath.Join("..", "..")
	mcp := mcpTools()

	t.Run("openai", func(t *testing.T) {
		var cat openAIToolCatalog
		loadJSON(t, filepath.Join(root, "schemas", "needlex-tools.openai.json"), &cat)
		if cat.Provider != "openai" {
			t.Fatalf("unexpected provider %q", cat.Provider)
		}
		if cat.Strict {
			t.Fatalf("expected non-strict schema catalog file")
		}
		if len(cat.Tools) != len(mcp) {
			t.Fatalf("tool count mismatch: got %d want %d", len(cat.Tools), len(mcp))
		}
		for i, tool := range mcp {
			got := cat.Tools[i]
			if got.Type != "function" {
				t.Fatalf("tool %d: type = %q, want function", i, got.Type)
			}
			if got.Function.Name != tool.Name {
				t.Fatalf("tool %d: name = %q, want %q", i, got.Function.Name, tool.Name)
			}
			if got.Function.Description != tool.Description {
				t.Fatalf("tool %s: description mismatch", tool.Name)
			}
			if got.Function.Strict {
				t.Fatalf("tool %s: expected non-strict function in schema file", tool.Name)
			}
			assertJSONEqual(t, tool.InputSchema, got.Function.Parameters, tool.Name)
		}
	})

	t.Run("anthropic", func(t *testing.T) {
		var cat anthropicToolCatalog
		loadJSON(t, filepath.Join(root, "schemas", "needlex-tools.anthropic.json"), &cat)
		if cat.Provider != "anthropic" {
			t.Fatalf("unexpected provider %q", cat.Provider)
		}
		if len(cat.Tools) != len(mcp) {
			t.Fatalf("tool count mismatch: got %d want %d", len(cat.Tools), len(mcp))
		}
		for i, tool := range mcp {
			got := cat.Tools[i]
			if got.Name != tool.Name {
				t.Fatalf("tool %d: name = %q, want %q", i, got.Name, tool.Name)
			}
			if got.Description != tool.Description {
				t.Fatalf("tool %s: description mismatch", tool.Name)
			}
			assertJSONEqual(t, tool.InputSchema, got.InputSchema, tool.Name)
		}
	})
}

func TestOpenAIStrictCatalog(t *testing.T) {
	cat := buildOpenAIToolCatalog(true)
	if !cat.Strict {
		t.Fatalf("expected strict catalog")
	}
	for _, tool := range cat.Tools {
		if !tool.Function.Strict {
			t.Fatalf("expected tool %q to be strict", tool.Function.Name)
		}
	}
}

func loadJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func assertJSONEqual(t *testing.T, want, got any, label string) {
	t.Helper()
	wantData, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("%s: marshal want: %v", label, err)
	}
	gotData, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("%s: marshal got: %v", label, err)
	}
	if string(wantData) != string(gotData) {
		t.Fatalf("%s: schema mismatch\nwant=%s\ngot=%s", label, wantData, gotData)
	}
}
