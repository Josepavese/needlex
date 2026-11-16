package schemas_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/pipeline"
)

func TestSchemaFilesContainValidJSON(t *testing.T) {
	files := []string{
		"needlex-document-read.v1.json",
		"needlex-fetch-receipt.v1.json",
		"needlex-tools.anthropic.json",
		"needlex-tools.openai.json",
		"proof.schema.json",
		"resultpack.schema.json",
		"slm-task.schema.json",
		"slm-patch.schema.json",
	}

	for _, name := range files {
		name := name
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(".", name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read schema: %v", err)
			}

			var decoded map[string]any
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("invalid json: %v", err)
			}
		})
	}
}

func TestFetchReceiptSchemaDeclaresCurrentContract(t *testing.T) {
	data, err := os.ReadFile("needlex-fetch-receipt.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Required   []string                  `json:"required"`
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Properties["schema"]["const"] != pipeline.DownloadReceiptSchemaV1 {
		t.Fatalf("schema const = %v, want %q", schema.Properties["schema"]["const"], pipeline.DownloadReceiptSchemaV1)
	}
	required := map[string]bool{}
	for _, name := range schema.Required {
		required[name] = true
	}
	for _, name := range []string{"schema", "directory", "primary"} {
		if !required[name] {
			t.Fatalf("schema does not require %q", name)
		}
	}
}

func TestDocumentReadSchemaDeclaresPageAndRawContract(t *testing.T) {
	data, err := os.ReadFile("needlex-document-read.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Properties["schema"]["const"] != core.DocumentReadSchemaV1 {
		t.Fatalf("schema const = %v, want %q", schema.Properties["schema"]["const"], core.DocumentReadSchemaV1)
	}
	for _, name := range []string{"raw_sha256", "source_bytes", "pages_total", "pages_processed", "pages_with_text", "truncated", "coverage", "limits"} {
		if _, ok := schema.Properties[name]; !ok {
			t.Fatalf("schema does not declare %q", name)
		}
	}
}

func TestResultPackSchemaDeclaresContextAlignment(t *testing.T) {
	data, err := os.ReadFile("resultpack.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties struct {
			Chunks struct {
				Items struct {
					Properties map[string]any `json:"properties"`
				} `json:"items"`
			} `json:"chunks"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	if _, ok := schema.Properties.Chunks.Items.Properties["context_alignment"]; !ok {
		t.Fatal("resultpack schema does not declare context_alignment")
	}
	for _, name := range []string{"page_start", "page_end"} {
		if _, ok := schema.Properties.Chunks.Items.Properties[name]; !ok {
			t.Fatalf("resultpack schema does not declare %q", name)
		}
	}
}

func TestSLMSchemasMatchRuntimeTasks(t *testing.T) {
	for _, name := range []string{"slm-task.schema.json", "slm-patch.schema.json"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Properties struct {
				Task struct {
					Enum []string `json:"enum"`
				} `json:"task"`
				ModelClass struct {
					Enum []string `json:"enum"`
				} `json:"model_class"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(data, &schema); err != nil {
			t.Fatal(err)
		}
		wantTasks := []string{"resolve_ambiguity", "query_rewrite", "endpoint_extract"}
		if strings.Join(schema.Properties.Task.Enum, ",") != strings.Join(wantTasks, ",") {
			t.Fatalf("%s tasks = %v, want %v", name, schema.Properties.Task.Enum, wantTasks)
		}
		if len(schema.Properties.ModelClass.Enum) != 0 {
			if strings.Join(schema.Properties.ModelClass.Enum, ",") != "micro_solver" {
				t.Fatalf("%s model classes = %v", name, schema.Properties.ModelClass.Enum)
			}
		}
	}
}
