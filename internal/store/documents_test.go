package store

import (
	"errors"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/platform"
	"github.com/josepavese/needlex/internal/proof"
)

func TestJSONStoresKeepNativeIOInPAL(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate store sources")
	}
	for _, name := range []string{
		"candidates.go", "discovery_provider_state.go", "domain_graph.go",
		"fingerprint_graph.go", "fingerprints.go", "genome.go", "proofs.go", "traces.go",
	} {
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(filepath.Dir(current), name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			switch strings.Trim(spec.Path.Value, `"`) {
			case "os", "io/fs", "path/filepath", "encoding/json", "syscall", "os/exec":
				t.Errorf("%s bypasses the JSON document contract through %s", name, spec.Path.Value)
			}
		}
	}
}

type proofDocuments struct {
	platform.JSONDocuments
	records []proof.ProofRecord
	err     error
}

func (d proofDocuments) Load(_ string, target any) error {
	if d.err != nil {
		return d.err
	}
	*target.(*[]proof.ProofRecord) = d.records
	return nil
}

func TestProofStoreValidatesThroughDocumentContract(t *testing.T) {
	records := fakeProofRecords()
	records[0].ID = ""
	store := ProofStore{documents: proofDocuments{records: records}}
	if _, err := store.LoadProofRecords("trace_1"); err == nil || !strings.HasPrefix(err.Error(), "proof_records[0]:") {
		t.Fatalf("invalid persisted proof accepted: %v", err)
	}
	// No Save implementation is needed: validation must reject this before I/O.
	if _, err := store.SaveProofRecords("trace_1", records); err == nil {
		t.Fatal("invalid proof written")
	}
	store.documents = proofDocuments{err: platform.ErrDocumentNotFound}
	if _, err := store.LoadProofRecords("trace_1"); !errors.Is(err, ErrProofNotFound) {
		t.Fatalf("missing document did not map to proof identity: %v", err)
	}
	failure := errors.New("storage unavailable")
	store.documents = proofDocuments{err: failure}
	if _, err := store.LoadProofRecords("trace_1"); !errors.Is(err, failure) {
		t.Fatalf("storage failure lost: %v", err)
	}
}

func TestProofLookupsPreserveOrderingValidationAndIdentity(t *testing.T) {
	for _, lookup := range []struct {
		name, id string
		find     func(ProofStore, string) (proof.ProofRecord, string, error)
	}{
		{"proof", "proof_1", ProofStore.FindProofByID},
		{"chunk", "chk_1", ProofStore.FindProofByChunkID},
	} {
		t.Run(lookup.name, func(t *testing.T) {
			store := NewProofStore(t.TempDir())
			for _, traceID := range []string{"trace_b", "trace_a"} {
				if _, err := store.SaveProofRecords(traceID, fakeProofRecords()); err != nil {
					t.Fatal(err)
				}
			}
			if _, traceID, err := lookup.find(store, " "+lookup.id+" "); err != nil || traceID != "trace_a" {
				t.Fatalf("ordered lookup: %q, %v", traceID, err)
			}
			if _, _, err := lookup.find(store, " "); err == nil || err.Error() != lookup.name+" id must not be empty" {
				t.Fatalf("empty ID: %v", err)
			}
			if _, _, err := lookup.find(store, "missing"); !errors.Is(err, ErrProofNotFound) {
				t.Fatalf("missing proof: %v", err)
			}
			invalid := fakeProofRecords()
			invalid[0].ID = ""
			if _, err := store.documents.Save("trace_0", invalid); err != nil {
				t.Fatal(err)
			}
			if _, _, err := lookup.find(store, lookup.id); err == nil || !strings.HasPrefix(err.Error(), "proof_records[0]:") {
				t.Fatalf("lookup ignored earlier invalid file: %v", err)
			}
		})
	}
}
