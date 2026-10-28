package store

import (
	"errors"
	"fmt"

	"github.com/josepavese/needlex/internal/platform"
	"github.com/josepavese/needlex/internal/proof"
)

var ErrTraceNotFound = errors.New("trace not found")

type TraceStore struct {
	documents platform.JSONDocuments
}

func NewTraceStore(root string) TraceStore {
	return TraceStore{
		documents: platform.NewJSONDocuments(platform.NewStateLayout(root).TracesDir, "trace", "trace"),
	}
}

func (s TraceStore) SaveTrace(trace proof.RunTrace) (string, error) {
	if err := trace.Validate(); err != nil {
		return "", err
	}
	return s.documents.Save(sanitizeID(trace.TraceID), trace)
}

func (s TraceStore) LoadTrace(traceID string) (proof.RunTrace, error) {
	var trace proof.RunTrace
	if err := s.documents.Load(sanitizeID(traceID), &trace); err != nil {
		if errors.Is(err, platform.ErrDocumentNotFound) {
			return proof.RunTrace{}, fmt.Errorf("%w: %s", ErrTraceNotFound, traceID)
		}
		return proof.RunTrace{}, err
	}
	if err := trace.Validate(); err != nil {
		return proof.RunTrace{}, err
	}
	return trace, nil
}
