package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/platform"
)

var ErrFingerprintNotFound = errors.New("fingerprint not found")

type FingerprintRecord struct {
	Fingerprint string    `json:"fingerprint"`
	ChunkID     string    `json:"chunk_id"`
	DocumentID  string    `json:"document_id"`
	TraceID     string    `json:"trace_id"`
	HeadingPath []string  `json:"heading_path,omitempty"`
	SavedAt     time.Time `json:"saved_at"`
}

type FingerprintStore struct {
	root string
	now  func() time.Time
}

func NewFingerprintStore(root string) FingerprintStore {
	if strings.TrimSpace(root) == "" {
		root = platform.DefaultStateRoot()
	}
	return FingerprintStore{
		root: root,
		now:  time.Now,
	}
}

func (s FingerprintStore) SaveChunks(traceID string, chunks []core.Chunk) (string, error) {
	cleanTraceID := sanitizeID(traceID)
	if cleanTraceID == "" {
		return "", fmt.Errorf("trace id must not be empty")
	}
	records := make([]FingerprintRecord, 0, len(chunks))
	for i, chunk := range chunks {
		if err := chunk.Validate(); err != nil {
			return "", fmt.Errorf("chunks[%d]: %w", i, err)
		}
		records = append(records, FingerprintRecord{
			Fingerprint: chunk.Fingerprint,
			ChunkID:     chunk.ID,
			DocumentID:  chunk.DocID,
			TraceID:     cleanTraceID,
			HeadingPath: append([]string{}, chunk.HeadingPath...),
			SavedAt:     s.now().UTC(),
		})
	}

	dir := filepath.Join(s.root, "fingerprints")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create fingerprint dir: %w", err)
	}

	path := filepath.Join(dir, cleanTraceID+".json")
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode fingerprints: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write fingerprints: %w", err)
	}
	return path, nil
}

func (s FingerprintStore) Load(traceID string) ([]FingerprintRecord, error) {
	path := filepath.Join(s.root, "fingerprints", sanitizeID(traceID)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrFingerprintNotFound, traceID)
		}
		return nil, fmt.Errorf("read fingerprints: %w", err)
	}
	var records []FingerprintRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode fingerprints: %w", err)
	}
	for i, record := range records {
		if err := record.Validate(); err != nil {
			return nil, fmt.Errorf("fingerprints[%d]: %w", i, err)
		}
	}
	return records, nil
}

func (r FingerprintRecord) Validate() error {
	if strings.TrimSpace(r.Fingerprint) == "" {
		return fmt.Errorf("fingerprint is required")
	}
	if strings.TrimSpace(r.ChunkID) == "" {
		return fmt.Errorf("chunk_id is required")
	}
	if strings.TrimSpace(r.DocumentID) == "" {
		return fmt.Errorf("document_id is required")
	}
	if strings.TrimSpace(r.TraceID) == "" {
		return fmt.Errorf("trace_id is required")
	}
	if r.SavedAt.IsZero() {
		return fmt.Errorf("saved_at is required")
	}
	for i, part := range r.HeadingPath {
		if strings.TrimSpace(part) == "" {
			return fmt.Errorf("heading_path[%d] must not be empty", i)
		}
	}
	return nil
}
