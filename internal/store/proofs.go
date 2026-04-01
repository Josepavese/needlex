package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/josepavese/needlex/internal/proof"
)

var ErrProofNotFound = errors.New("proof not found")

type ProofStore struct {
	root string
}

func NewProofStore(root string) ProofStore {
	if strings.TrimSpace(root) == "" {
		root = ".needlex"
	}
	return ProofStore{root: root}
}

func (s ProofStore) SaveProofRecords(traceID string, records []proof.ProofRecord) (string, error) {
	cleanTraceID := sanitizeID(traceID)
	if cleanTraceID == "" {
		return "", fmt.Errorf("trace id must not be empty")
	}
	for i, record := range records {
		if err := record.Validate(); err != nil {
			return "", fmt.Errorf("proof_records[%d]: %w", i, err)
		}
	}

	dir := filepath.Join(s.root, "proofs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create proof dir: %w", err)
	}

	path := filepath.Join(dir, cleanTraceID+".json")
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode proofs: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write proofs: %w", err)
	}
	return path, nil
}

func (s ProofStore) LoadProofRecords(traceID string) ([]proof.ProofRecord, error) {
	path := filepath.Join(s.root, "proofs", sanitizeID(traceID)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrProofNotFound, traceID)
		}
		return nil, fmt.Errorf("read proofs: %w", err)
	}
	return decodeRecords(data)
}

func (s ProofStore) FindProofByChunkID(chunkID string) (proof.ProofRecord, string, error) {
	cleanChunkID := strings.TrimSpace(chunkID)
	if cleanChunkID == "" {
		return proof.ProofRecord{}, "", fmt.Errorf("chunk id must not be empty")
	}

	pattern := filepath.Join(s.root, "proofs", "*.json")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return proof.ProofRecord{}, "", fmt.Errorf("glob proofs: %w", err)
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return proof.ProofRecord{}, "", fmt.Errorf("read proofs: %w", err)
		}
		records, err := decodeRecords(data)
		if err != nil {
			return proof.ProofRecord{}, "", err
		}
		for _, record := range records {
			if record.Proof.ChunkID == cleanChunkID {
				traceID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
				return record, traceID, nil
			}
		}
	}

	return proof.ProofRecord{}, "", fmt.Errorf("%w: %s", ErrProofNotFound, chunkID)
}

func (s ProofStore) FindProofByID(proofID string) (proof.ProofRecord, string, error) {
	cleanProofID := strings.TrimSpace(proofID)
	if cleanProofID == "" {
		return proof.ProofRecord{}, "", fmt.Errorf("proof id must not be empty")
	}

	pattern := filepath.Join(s.root, "proofs", "*.json")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return proof.ProofRecord{}, "", fmt.Errorf("glob proofs: %w", err)
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return proof.ProofRecord{}, "", fmt.Errorf("read proofs: %w", err)
		}
		records, err := decodeRecords(data)
		if err != nil {
			return proof.ProofRecord{}, "", err
		}
		for _, record := range records {
			if record.ID == cleanProofID {
				traceID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
				return record, traceID, nil
			}
		}
	}

	return proof.ProofRecord{}, "", fmt.Errorf("%w: %s", ErrProofNotFound, proofID)
}

func decodeRecords(data []byte) ([]proof.ProofRecord, error) {
	var records []proof.ProofRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode proofs: %w", err)
	}
	for i, record := range records {
		if err := record.Validate(); err != nil {
			return nil, fmt.Errorf("proof_records[%d]: %w", i, err)
		}
	}
	return records, nil
}

func sanitizeID(value string) string {
	clean := strings.TrimSpace(value)
	clean = strings.ReplaceAll(clean, "/", "_")
	clean = strings.ReplaceAll(clean, "\\", "_")
	return clean
}
