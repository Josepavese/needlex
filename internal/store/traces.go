package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/josepavese/needlex/internal/platform"
	"github.com/josepavese/needlex/internal/proof"
)

var ErrTraceNotFound = errors.New("trace not found")

type TraceStore struct {
	root string
}

func NewTraceStore(root string) TraceStore {
	if strings.TrimSpace(root) == "" {
		root = platform.DefaultStateRoot()
	}
	return TraceStore{root: root}
}

func (s TraceStore) SaveTrace(trace proof.RunTrace) (string, error) {
	if err := trace.Validate(); err != nil {
		return "", err
	}
	dir := filepath.Join(s.root, "traces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create trace dir: %w", err)
	}

	path := s.tracePath(trace.TraceID)
	data, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode trace: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write trace: %w", err)
	}
	return path, nil
}

func (s TraceStore) LoadTrace(traceID string) (proof.RunTrace, error) {
	path := s.tracePath(traceID)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return proof.RunTrace{}, fmt.Errorf("%w: %s", ErrTraceNotFound, traceID)
		}
		return proof.RunTrace{}, fmt.Errorf("read trace: %w", err)
	}

	var trace proof.RunTrace
	if err := json.Unmarshal(data, &trace); err != nil {
		return proof.RunTrace{}, fmt.Errorf("decode trace: %w", err)
	}
	if err := trace.Validate(); err != nil {
		return proof.RunTrace{}, err
	}
	return trace, nil
}

func (s TraceStore) tracePath(traceID string) string {
	clean := strings.TrimSpace(traceID)
	clean = strings.ReplaceAll(clean, "/", "_")
	clean = strings.ReplaceAll(clean, "\\", "_")
	return filepath.Join(s.root, "traces", clean+".json")
}
