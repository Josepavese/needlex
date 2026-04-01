package service

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/memory"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/store"
)

const graphExpansionMinScore = 2.0

func PrepareQueryRequestWithLocalState(storeRoot string, req QueryRequest, cfg config.Config, semantic intel.SemanticAligner) QueryRequest {
	candidateStore := store.NewCandidateStore(storeRoot)
	domainGraphStore := store.NewDomainGraphStore(storeRoot)
	autoSeed := strings.TrimSpace(strings.ToLower(req.DiscoveryMode)) != QueryDiscoveryOff

	if autoSeed {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Semantic.TimeoutMS)*time.Millisecond)
		defer cancel()
		if cfg.Memory.Enabled {
			memoryStore := memory.NewSQLiteStore(storeRoot, cfg.Memory.Path)
			memoryService := memory.NewService(cfg.Memory, memoryStore, intel.NewTextEmbedder(cfg, nil))
			if matches, err := memoryService.Search(ctx, req.Goal, memory.SearchOptions{
				Limit:       3,
				ExpandLimit: 2,
				MinScore:    0.15,
				DomainHints: req.DomainHints,
			}); err == nil && len(matches) > 0 {
				req.MemoryCandidates = memoryCandidatesToDiscover(matches)
				req.DomainHints = mergeDomainHints(req.DomainHints, domainHintsFromDiscoverCandidates(req.MemoryCandidates)...)
			}
		}
		if matches, err := candidateStore.Search(ctx, req.Goal, 3, semantic); err == nil && len(matches) > 0 {
			req.DomainHints = mergeDomainHints(req.DomainHints, domainHintsFromCandidateMatches(matches)...)
			if strings.TrimSpace(req.SeedURL) == "" && len(req.MemoryCandidates) == 0 {
				req.SeedURL = matches[0].URL
			}
		}
	}
	if strings.TrimSpace(req.SeedURL) != "" {
		if host := hostFromURLString(req.SeedURL); host != "" {
			req.DomainHints = mergeDomainHints(req.DomainHints, host)
		}
	}
	if autoSeed && len(req.DomainHints) > 0 {
		if matches, err := domainGraphStore.Expand(req.DomainHints, 2); err == nil && len(matches) > 0 {
			req.DomainHints = mergeDomainHints(req.DomainHints, domainsFromGraphMatches(selectGraphExpansionMatches(matches))...)
		}
	}
	if evidence, ok := loadQueryFingerprintEvidence(storeRoot, req.SeedURL); ok {
		req.SeedTraceID, req.SeedStable, req.SeedNovelty, req.SeedChanged = evidence.TraceID, evidence.Stable, evidence.Novelty, evidence.Changed
	}
	loadGenomeHints(storeRoot, req.SeedURL, &req.ForceLane, &req.Profile, &req.PruningProfile, &req.RenderHint)
	return req
}

func ObserveQueryResponseWithLocalState(storeRoot string, req QueryRequest, resp QueryResponse) {
	candidateStore := store.NewCandidateStore(storeRoot)
	domainGraphStore := store.NewDomainGraphStore(storeRoot)
	observeStoredPage(storeRoot, "query", resp.Document, resp.TraceID, resp.ResultPack.Chunks)
	for _, candidateURL := range resp.Plan.CandidateURLs {
		_ = observeCandidate(candidateStore, core.Document{FinalURL: candidateURL}, "query_discovery")
		if strings.TrimSpace(req.SeedURL) != "" {
			_, _, _ = domainGraphStore.Observe(req.SeedURL, candidateURL, "query_discovery")
		}
		if strings.TrimSpace(resp.Plan.SelectedURL) != "" {
			_, _, _ = domainGraphStore.Observe(resp.Plan.SelectedURL, candidateURL, "query_related")
		}
	}
	observeGenome(storeRoot, genomeObservation(resp.Document, resp.Trace, resp.ResultPack, req.PruningProfile, req.RenderHint))
}

func observeCandidate(candidateStore store.CandidateStore, document core.Document, source string) error {
	url := strings.TrimSpace(document.FinalURL)
	if url == "" {
		url = strings.TrimSpace(document.URL)
	}
	if url == "" {
		return nil
	}
	_, _, err := candidateStore.Observe(store.CandidateObservation{URL: url, Title: document.Title, Source: source})
	return err
}

func domainHintsFromCandidateMatches(matches []store.CandidateMatch) []string {
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if host := hostFromURLString(match.URL); host != "" {
			out = append(out, host)
		}
	}
	return out
}

func memoryCandidatesToDiscover(matches []memory.Candidate) []DiscoverCandidate {
	out := make([]DiscoverCandidate, 0, len(matches))
	for _, match := range matches {
		metadata := map[string]string{}
		if strings.TrimSpace(match.Source) != "" {
			metadata["memory_source"] = match.Source
		}
		if match.Distance > 0 {
			metadata["memory_distance"] = strconv.FormatFloat(match.Distance, 'f', 4, 64)
		}
		out = append(out, DiscoverCandidate{
			URL:      match.URL,
			Label:    match.Title,
			Score:    match.Score,
			Reason:   append([]string{}, match.Reasons...),
			Metadata: metadata,
		})
	}
	return out
}

func domainHintsFromDiscoverCandidates(matches []DiscoverCandidate) []string {
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if host := hostFromURLString(match.URL); host != "" {
			out = append(out, host)
		}
	}
	return out
}

func domainsFromGraphMatches(matches []store.DomainMatch) []string {
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		domain := strings.TrimSpace(strings.ToLower(match.Domain))
		if domain == "" {
			continue
		}
		out = append(out, domain)
	}
	return out
}

func selectGraphExpansionMatches(matches []store.DomainMatch) []store.DomainMatch {
	out := make([]store.DomainMatch, 0, len(matches))
	for _, match := range matches {
		if !graphExpansionConfident(match) {
			continue
		}
		out = append(out, match)
	}
	return out
}

func graphExpansionConfident(match store.DomainMatch) bool {
	if match.Score >= 4 {
		return true
	}
	return match.Score >= graphExpansionMinScore && slices.Contains(match.Reason, "outbound_transition")
}

func maxLane(path []int) int {
	max := 0
	for _, lane := range path {
		if lane > max {
			max = lane
		}
	}
	return max
}

func packMetadata(trace proof.RunTrace, key string) string {
	for _, stage := range trace.Stages {
		if stage.Stage != "pack" {
			continue
		}
		return stage.Metadata[key]
	}
	return ""
}

func NewFingerprintEvidenceLoader(storeRoot string) func(string) (QueryFingerprintEvidence, bool) {
	return func(url string) (QueryFingerprintEvidence, bool) { return loadQueryFingerprintEvidence(storeRoot, url) }
}

func loadQueryFingerprintEvidence(storeRoot, seedURL string) (QueryFingerprintEvidence, bool) {
	cleanURL := strings.TrimSpace(seedURL)
	if cleanURL == "" {
		return QueryFingerprintEvidence{}, false
	}
	graph, err := store.NewFingerprintGraphStore(storeRoot).Load(cleanURL)
	if err != nil || len(graph.LatestNodes) == 0 {
		return QueryFingerprintEvidence{}, false
	}
	evidence := QueryFingerprintEvidence{TraceID: graph.LatestTraceID}
	if len(graph.History) == 0 {
		return evidence, true
	}
	last := graph.History[len(graph.History)-1]
	retainedCount := len(last.Retained)
	changeCount := len(last.Added) + len(last.Removed)
	if len(graph.LatestNodes) > 0 {
		evidence.Stable = float64(retainedCount) / float64(len(graph.LatestNodes))
		evidence.Novelty = float64(changeCount) / float64(len(graph.LatestNodes))
	}
	evidence.Changed = changeCount > 0
	return evidence, true
}
