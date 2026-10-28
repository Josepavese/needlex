package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/memory"
	"github.com/josepavese/needlex/internal/store"
)

type stateGenomes struct {
	store.Genomes
	genome store.DomainGenome
	err    error
}

func (g stateGenomes) LoadByURL(string) (store.DomainGenome, error) { return g.genome, g.err }

type stateFingerprints struct {
	store.FingerprintGraphs
	graph store.FingerprintGraph
	err   error
}

func (g stateFingerprints) Load(string) (store.FingerprintGraph, error) { return g.graph, g.err }

type stateCandidates struct {
	store.Candidates
	search func(context.Context, string, int, intel.SemanticAligner) ([]store.CandidateMatch, error)
}

func (s stateCandidates) Search(ctx context.Context, goal string, limit int, semantic intel.SemanticAligner) ([]store.CandidateMatch, error) {
	return s.search(ctx, goal, limit, semantic)
}

type stateDomains struct {
	store.DomainGraph
	expand func([]string, int) ([]store.DomainMatch, error)
}

func (s stateDomains) Expand(hints []string, limit int) ([]store.DomainMatch, error) {
	return s.expand(hints, limit)
}

type stateRecall func(context.Context, string, memory.SearchOptions) ([]memory.Candidate, error)

func (f stateRecall) Search(ctx context.Context, goal string, opts memory.SearchOptions) ([]memory.Candidate, error) {
	return f(ctx, goal, opts)
}

func TestStatePreparationUsesInjectedRepositoriesAndHonorsExplicitOptions(t *testing.T) {
	at := time.Unix(1700000000, 0).UTC()
	state := store.LocalState{
		Genomes: stateGenomes{genome: store.DomainGenome{Domain: "example.com", SeenCount: 1, LastSeenAt: at, UpdatedAt: at, ForceLane: 2, PreferredProfile: "deep", FetchProfile: "hardened", RenderNeeded: true}},
		Fingerprints: stateFingerprints{graph: store.FingerprintGraph{
			URL: "https://example.com", LatestTraceID: "trace_prior", UpdatedAt: at,
			LatestNodes: []store.FingerprintGraphNode{{Fingerprint: "persisted", ChunkID: "chunk_prior"}},
		}},
	}
	req := PrepareReadRequestWithLocalState(state, ReadRequest{
		URL: "https://example.com", ForceLane: 1, Profile: "tiny", StableFingerprints: []string{"caller"},
	})
	if req.ForceLane != 1 || req.Profile != "tiny" || req.FetchProfile != "hardened" || !req.RenderHint {
		t.Fatalf("injected genome or caller precedence lost: %+v", req)
	}
	if len(req.StableFingerprints) != 2 || req.StableFingerprints[0] != "caller" || req.StableFingerprints[1] != "persisted" {
		t.Fatalf("injected fingerprints lost: %v", req.StableFingerprints)
	}
}

func TestStrictStatePreparationDoesNotSearchAndToleratesUnavailableHints(t *testing.T) {
	failure := errors.New("state unavailable")
	state := store.LocalState{Genomes: stateGenomes{err: failure}, Fingerprints: stateFingerprints{err: failure}}
	cfg := testConfig()
	cfg.Memory.Enabled = true
	// Search repositories and recall are deliberately absent: strict mode must
	// not consult them or fall back to a concrete backend.
	req := PrepareQueryRequestWithLocalState(state, QueryRequest{
		Goal: "prezzi", SeedURL: "https://example.com/page", DiscoveryMode: QueryDiscoveryOff, Profile: "tiny",
	}, cfg, nil, nil)
	if req.SeedURL != "https://example.com/page" || req.Profile != "tiny" || len(req.MemoryCandidates) != 0 || req.SeedTraceID != "" {
		t.Fatalf("strict request changed after hint failure: %+v", req)
	}
}

func TestQueryStateKeepsMemoryPriorityAndFallsBackToCandidatesOnRecallFailure(t *testing.T) {
	for _, recallFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "candidate"}[recallFails], func(t *testing.T) {
			candidateSearches := 0
			at := time.Unix(1700000000, 0).UTC()
			state := store.LocalState{
				Genomes: stateGenomes{err: store.ErrGenomeNotFound},
				Fingerprints: stateFingerprints{graph: store.FingerprintGraph{
					URL: "https://candidate.example/page", UpdatedAt: at, LatestTraceID: "trace_prior", LatestNodes: []store.FingerprintGraphNode{{Fingerprint: "a", ChunkID: "current_a"}, {Fingerprint: "b", ChunkID: "current_b"}},
					History: []store.FingerprintGraphDelta{{PreviousTraceID: "trace_previous", CurrentTraceID: "trace_prior", ObservedAt: at, Retained: []store.FingerprintGraphRelation{{Fingerprint: "a", PreviousChunkID: "previous_a", CurrentChunkID: "current_a"}}, Added: []string{"b"}}},
				}},
				Candidates: stateCandidates{search: func(ctx context.Context, goal string, limit int, _ intel.SemanticAligner) ([]store.CandidateMatch, error) {
					candidateSearches++
					if _, ok := ctx.Deadline(); !ok || limit != 3 || goal != "prezzi" {
						t.Fatalf("candidate request lost budget/intent: %s, %d", goal, limit)
					}
					return []store.CandidateMatch{{URL: "https://candidate.example/page", Score: 0.8, Reason: []string{"semantic_goal_alignment"}}}, nil
				}},
				Domains: stateDomains{expand: func(_ []string, limit int) ([]store.DomainMatch, error) {
					if limit != 2 {
						t.Fatalf("graph budget changed: %d", limit)
					}
					return nil, nil
				}},
			}
			recalls := 0
			recall := stateRecall(func(ctx context.Context, goal string, opts memory.SearchOptions) ([]memory.Candidate, error) {
				recalls++
				if _, ok := ctx.Deadline(); !ok || goal != "prezzi" || opts.Limit != 5 || opts.ExpandLimit != 3 {
					t.Fatalf("memory request lost budget/intent: %s, %+v", goal, opts)
				}
				if recallFails {
					return nil, errors.New("recall unavailable")
				}
				return []memory.Candidate{{URL: "https://memory.example/page", Score: 0.9}}, nil
			})
			cfg := testConfig()
			cfg.Memory.Enabled = true
			req := PrepareQueryRequestWithLocalState(state, QueryRequest{Goal: "prezzi", DiscoveryMode: QueryDiscoveryWeb}, cfg, nil, recall)
			if recalls != 1 || candidateSearches != 1 {
				t.Fatalf("providers bypassed: recall=%d candidates=%d", recalls, candidateSearches)
			}
			if recallFails {
				if req.SeedURL != "https://candidate.example/page" || req.SeedTraceID != "trace_prior" || req.SeedStable != 0.5 || req.SeedNovelty != 0.5 || !req.SeedChanged {
					t.Fatalf("candidate recovery/evidence changed: %+v", req)
				}
			} else if req.SeedURL != "" || len(req.MemoryCandidates) != 1 || req.MemoryCandidates[0].URL != "https://memory.example/page" {
				t.Fatalf("candidate store displaced semantic memory: %+v", req)
			}
		})
	}
}
