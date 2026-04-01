package schemas_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSchemaFilesContainValidJSON(t *testing.T) {
	files := []string{
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
