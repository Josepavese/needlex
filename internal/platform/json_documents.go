package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrDocumentNotFound provides a portable missing-document identity.
var ErrDocumentNotFound = fs.ErrNotExist

// JSONDocuments stores one JSON value per logical ID. IDs are supplied by the
// owning store; List returns them in filename order. Reads and writes are
// synchronous, preserve the existing indented JSON format and do not own locks.
type JSONDocuments interface {
	Load(id string, target any) error
	Save(id string, value any) (string, error)
	List() ([]string, error)
}

type jsonDocuments struct {
	directory, valueName, directoryName string
}

// NewJSONDocuments composes the filesystem backend. Names provide the existing
// store-specific diagnostic context; schema validation stays with the caller.
func NewJSONDocuments(directory, valueName, directoryName string) JSONDocuments {
	return jsonDocuments{directory, valueName, directoryName}
}

func (s jsonDocuments) Load(id string, target any) error {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return fmt.Errorf("read %s: %w", s.valueName, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", s.valueName, err)
	}
	return nil
}

func (s jsonDocuments) Save(id string, value any) (string, error) {
	if err := os.MkdirAll(s.directory, 0o755); err != nil {
		return "", fmt.Errorf("create %s dir: %w", s.directoryName, err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", s.valueName, err)
	}
	path := s.path(id)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", s.valueName, err)
	}
	return path, nil
}

func (s jsonDocuments) List() ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(s.directory, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", s.valueName, err)
	}
	ids := make([]string, 0, len(paths))
	for _, path := range paths {
		ids = append(ids, strings.TrimSuffix(filepath.Base(path), ".json"))
	}
	return ids, nil
}

func (s jsonDocuments) path(id string) string {
	return filepath.Join(s.directory, id+".json")
}
