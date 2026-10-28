package platform

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestJSONDocumentsPreserveExistingFilesAndFormat(t *testing.T) {
	dir := t.TempDir()
	documents := NewJSONDocuments(dir, "records", "record")
	// Existing files must be readable directly, without rewriting their schema.
	if err := os.WriteFile(filepath.Join(dir, "old.json"), []byte(`{"count":7,"extra":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var record struct {
		Count int `json:"count"`
	}
	if err := documents.Load("old", &record); err != nil || record.Count != 7 {
		t.Fatalf("read existing record: %+v, %v", record, err)
	}
	path, err := documents.Save("new", record)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "{\n  \"count\": 7\n}" || path != filepath.Join(dir, "new.json") {
		t.Fatalf("persisted format/path changed: %s, %q, %v", data, path, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ids, err := documents.List()
	if err != nil || !reflect.DeepEqual(ids, []string{"new", "old"}) {
		t.Fatalf("ordered JSON IDs: %v, %v", ids, err)
	}
}

func TestJSONDocumentsMissingReadsHaveNoSideEffects(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "absent")
	documents := NewJSONDocuments(dir, "records", "record")
	var value any
	if err := documents.Load("missing", &value); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("missing document: %v", err)
	}
	if ids, err := documents.List(); err != nil || len(ids) != 0 {
		t.Fatalf("missing collection: %v, %v", ids, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read created collection: %v", err)
	}
}

func TestJSONDocumentsFailureStagesPreserveData(t *testing.T) {
	dir := t.TempDir()
	documents := NewJSONDocuments(dir, "records", "record")
	path, err := documents.Save("value", "original")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := documents.Save("value", make(chan int)); err == nil || !strings.HasPrefix(err.Error(), "encode records:") {
		t.Fatalf("encoding failure: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != `"original"` {
		t.Fatalf("failed encoding overwrote data: %s, %v", data, err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	var value any
	if err := documents.Load("value", &value); err == nil || !strings.HasPrefix(err.Error(), "decode records:") {
		t.Fatalf("decoding failure: %v", err)
	}
	blocked := NewJSONDocuments(path, "records", "record")
	if _, err := blocked.Save("value", true); err == nil || !strings.HasPrefix(err.Error(), "create record dir:") {
		t.Fatalf("directory failure: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "directory.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := documents.Load("directory", &value); err == nil || !strings.HasPrefix(err.Error(), "read records:") {
		t.Fatalf("read failure: %v", err)
	}
	if _, err := documents.Save("directory", true); err == nil || !strings.HasPrefix(err.Error(), "write records:") {
		t.Fatalf("write failure: %v", err)
	}
}
