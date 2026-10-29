package service

import (
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/store"
)

func applyGenomeHints(forceLane *int, profile, fetchProfile, fetchRetryProfile *string, renderHint *bool, genome store.DomainGenome) {
	if *forceLane == 0 {
		*forceLane = genome.ForceLane
	}
	if *profile == "" && genome.PreferredProfile != "" {
		*profile = genome.PreferredProfile
	}
	if *fetchProfile == "" && genome.FetchProfile != "" {
		*fetchProfile = genome.FetchProfile
	}
	if *fetchRetryProfile == "" && genome.FetchRetryProfile != "" {
		*fetchRetryProfile = genome.FetchRetryProfile
	}
	if !*renderHint && genome.RenderNeeded {
		*renderHint = true
	}
}

func loadGenomeHints(genomes store.Genomes, rawURL string, forceLane *int, profile, fetchProfile, fetchRetryProfile *string, renderHint *bool) {
	if genome, err := genomes.LoadByURL(rawURL); err == nil {
		applyGenomeHints(forceLane, profile, fetchProfile, fetchRetryProfile, renderHint, genome)
	}
}

func stableFingerprintsForURL(fingerprints store.FingerprintGraphs, rawURL string) []string {
	graph, err := fingerprints.Load(rawURL)
	if err != nil || len(graph.LatestNodes) == 0 {
		return nil
	}
	out := make([]string, 0, len(graph.LatestNodes))
	for _, node := range graph.LatestNodes {
		out = append(out, node.Fingerprint)
	}
	return out
}

func observeStoredPage(state store.LocalState, source string, document core.Document, traceID string, chunks []core.Chunk) {
	_ = observeCandidate(state.Candidates, document, source)
	if strings.TrimSpace(document.FinalURL) != "" && strings.TrimSpace(traceID) != "" && len(chunks) > 0 {
		_, _, _ = state.Fingerprints.Observe(document.FinalURL, traceID, chunks)
	}
}

func observeGenome(genomes store.Genomes, observation store.GenomeObservation) {
	_, _, _ = genomes.Observe(observation)
}

func genomeObservation(document core.Document, trace proof.RunTrace, pack core.ResultPack, renderHint bool) store.GenomeObservation {
	return store.GenomeObservation{
		URL:               document.FinalURL,
		ObservedLane:      maxLane(pack.CostReport.LanePath),
		PreferredProfile:  pack.Profile,
		FetchProfile:      traceMetadata(trace, "acquire", "fetch_profile"),
		FetchRetryProfile: traceMetadata(trace, "acquire", "retry_profile"),
		RenderNeeded:      renderHint,
		FetchMode:         document.FetchMode,
		NoiseLevel:        traceMetadata(trace, "pack", "noise_level"),
	}
}
