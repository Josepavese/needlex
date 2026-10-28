package store

import (
	"errors"
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/platform"
	"github.com/josepavese/needlex/internal/proof"
)

var ErrProofNotFound = errors.New("proof not found")

type ProofStore struct {
	documents platform.JSONDocuments
}

func NewProofStore(root string) ProofStore {
	return ProofStore{
		documents: platform.NewJSONDocuments(platform.NewStateLayout(root).ProofsDir, "proofs", "proof"),
	}
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

	return s.documents.Save(cleanTraceID, records)
}

func (s ProofStore) LoadProofRecords(traceID string) ([]proof.ProofRecord, error) {
	records, err := s.loadRecords(sanitizeID(traceID))
	if errors.Is(err, platform.ErrDocumentNotFound) {
		return nil, fmt.Errorf("%w: %s", ErrProofNotFound, traceID)
	}
	return records, err
}

func (s ProofStore) FindProofByChunkID(chunkID string) (proof.ProofRecord, string, error) {
	return s.findProof(chunkID, "chunk", func(record proof.ProofRecord) string { return record.Proof.ChunkID })
}

func (s ProofStore) FindProofByID(proofID string) (proof.ProofRecord, string, error) {
	return s.findProof(proofID, "proof", func(record proof.ProofRecord) string { return record.ID })
}

func (s ProofStore) findProof(id, kind string, identity func(proof.ProofRecord) string) (proof.ProofRecord, string, error) {
	cleanID := strings.TrimSpace(id)
	if cleanID == "" {
		return proof.ProofRecord{}, "", fmt.Errorf("%s id must not be empty", kind)
	}
	ids, err := s.documents.List()
	if err != nil {
		return proof.ProofRecord{}, "", err
	}
	for _, traceID := range ids {
		records, err := s.loadRecords(traceID)
		if err != nil {
			if errors.Is(err, platform.ErrDocumentNotFound) {
				err = fmt.Errorf("read proofs: %w", err)
			}
			return proof.ProofRecord{}, "", err
		}
		for _, record := range records {
			if identity(record) == cleanID {
				return record, traceID, nil
			}
		}
	}
	return proof.ProofRecord{}, "", fmt.Errorf("%w: %s", ErrProofNotFound, id)
}

func (s ProofStore) loadRecords(traceID string) ([]proof.ProofRecord, error) {
	var records []proof.ProofRecord
	if err := s.documents.Load(traceID, &records); err != nil {
		return nil, err
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
