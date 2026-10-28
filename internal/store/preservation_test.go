package store

import (
	"bytes"
	"os"
	"testing"

	"github.com/josepavese/needlex/internal/core"
)

func TestObservationDoesNotReplaceInvalidExistingState(t *testing.T) {
	for _, kind := range []string{"candidates", "domains", "fingerprints"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			var update func() (string, error)
			switch kind {
			case "candidates":
				s := NewCandidateStore(root)
				update = func() (string, error) {
					_, path, err := s.Observe(CandidateObservation{URL: "https://example.com", Source: "fixture"})
					return path, err
				}
			case "domains":
				s := NewDomainGraphStore(root)
				update = func() (string, error) {
					_, path, err := s.Observe("https://from.example", "https://to.example", "fixture")
					return path, err
				}
			case "fingerprints":
				s := NewFingerprintGraphStore(root)
				update = func() (string, error) {
					_, path, err := s.Observe("https://example.com", "trace_fixture", []core.Chunk{{
						ID: "chunk_fixture", DocID: "doc_fixture", Text: "State evidence", Fingerprint: "fp_fixture", Score: 0.9, Confidence: 0.9,
					}})
					return path, err
				}
			}
			path, err := update()
			if err != nil {
				t.Fatal(err)
			}
			original := []byte("{ interrupted or invalid state: preserve these bytes")
			if err := os.WriteFile(path, original, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := update(); err == nil {
				t.Fatal("invalid state was accepted as an empty repository")
			}
			retained, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(retained, original) {
				t.Fatalf("existing state replaced: %q, %v", retained, err)
			}
		})
	}
}
