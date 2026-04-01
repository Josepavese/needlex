package service

import (
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/store"
)

func applyGenomeHints(forceLane *int, profile, pruningProfile *string, renderHint *bool, genome store.DomainGenome) {
	if *forceLane == 0 {
		*forceLane = genome.ForceLane
	}
	if *profile == "" && genome.PreferredProfile != "" {
		*profile = genome.PreferredProfile
	}
	if *pruningProfile == "" && genome.PruningProfile != "" {
		*pruningProfile = genome.PruningProfile
	}
	if !*renderHint && genome.RenderNeeded {
		*renderHint = true
	}
}

func loadGenomeHints(storeRoot, rawURL string, forceLane *int, profile, pruningProfile *string, renderHint *bool) {
	if genome, err := store.NewGenomeStore(storeRoot).LoadByURL(rawURL); err == nil {
		applyGenomeHints(forceLane, profile, pruningProfile, renderHint, genome)
	}
}

func stableFingerprintsForURL(storeRoot, rawURL string) []string {
	graph, err := store.NewFingerprintGraphStore(storeRoot).Load(rawURL)
	if err != nil || len(graph.LatestNodes) == 0 {
		return nil
	}
	out := make([]string, 0, len(graph.LatestNodes))
	for _, node := range graph.LatestNodes {
		out = append(out, node.Fingerprint)
	}
	return out
}

func observeStoredPage(storeRoot, source string, document core.Document, traceID string, chunks []core.Chunk) {
	_ = observeCandidate(store.NewCandidateStore(storeRoot), document, source)
	if strings.TrimSpace(document.FinalURL) != "" && strings.TrimSpace(traceID) != "" && len(chunks) > 0 {
		_, _, _ = store.NewFingerprintGraphStore(storeRoot).Observe(document.FinalURL, traceID, chunks)
	}
}

func observeGenome(storeRoot string, observation store.GenomeObservation) {
	_, _, _ = store.NewGenomeStore(storeRoot).Observe(observation)
}

func genomeObservation(document core.Document, trace proof.RunTrace, pack core.ResultPack, pruningProfile string, renderHint bool) store.GenomeObservation {
	return store.GenomeObservation{
		URL:              document.FinalURL,
		ObservedLane:     maxLane(pack.CostReport.LanePath),
		PreferredProfile: pack.Profile,
		PruningProfile:   pruningProfile,
		RenderNeeded:     renderHint,
		FetchMode:        document.FetchMode,
		NoiseLevel:       packMetadata(trace, "noise_level"),
		PageType:         packMetadata(trace, "page_type"),
	}
}
