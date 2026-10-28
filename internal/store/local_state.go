package store

import (
	"context"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
)

// LocalState supplies the repositories used by request preparation and result
// observation. Its caller borrows operation-scoped handles synchronously;
// adapters own native resources and complete writes before returning.
type LocalState struct {
	Candidates   Candidates
	Domains      DomainGraph
	Genomes      Genomes
	Fingerprints FingerprintGraphs
}

type Candidates interface {
	Search(context.Context, string, int, intel.SemanticAligner) ([]CandidateMatch, error)
	Observe(CandidateObservation) (CandidateRecord, string, error)
}

type DomainGraph interface {
	Expand([]string, int) ([]DomainMatch, error)
	Observe(string, string, string) (DomainTransition, string, error)
}

type Genomes interface {
	LoadByURL(string) (DomainGenome, error)
	Observe(GenomeObservation) (DomainGenome, string, error)
}

type FingerprintGraphs interface {
	Load(string) (FingerprintGraph, error)
	Observe(string, string, []core.Chunk) (FingerprintGraphDelta, string, error)
}

func NewLocalState(root string) LocalState {
	return LocalState{
		Candidates: NewCandidateStore(root), Domains: NewDomainGraphStore(root),
		Genomes: NewGenomeStore(root), Fingerprints: NewFingerprintGraphStore(root),
	}
}
