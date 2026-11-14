package schemas_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/josepavese/needlex/internal/pipeline"
)

func TestSchemaFilesContainValidJSON(t *testing.T) {
	files := []string{
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
