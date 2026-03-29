package service

import "github.com/josepavese/needlex/internal/store"

func PrepareReadRequestWithLocalState(storeRoot string, req ReadRequest) ReadRequest {
	if genome, err := store.NewGenomeStore(storeRoot).LoadByURL(req.URL); err == nil {
		applyGenomeHints(&req.ForceLane, &req.Profile, &req.PruningProfile, &req.RenderHint, genome)
	}
	if graph, err := store.NewFingerprintGraphStore(storeRoot).Load(req.URL); err == nil {
		for _, node := range graph.LatestNodes {
			req.StableFingerprints = append(req.StableFingerprints, node.Fingerprint)
		}
	}
	return req
}

func ObserveReadResponseWithLocalState(storeRoot string, req ReadRequest, resp ReadResponse) {
	genomeStore := store.NewGenomeStore(storeRoot)
	_ = observeCandidate(store.NewCandidateStore(storeRoot), resp.Document, "read")
	if resp.Document.FinalURL != "" && resp.Trace.TraceID != "" && len(resp.ResultPack.Chunks) > 0 {
		_, _, _ = store.NewFingerprintGraphStore(storeRoot).Observe(resp.Document.FinalURL, resp.Trace.TraceID, resp.ResultPack.Chunks)
	}
	_, _, _ = genomeStore.Observe(store.GenomeObservation{
		URL:              resp.Document.FinalURL,
		ObservedLane:     maxLane(resp.ResultPack.CostReport.LanePath),
		PreferredProfile: resp.ResultPack.Profile,
		PruningProfile:   req.PruningProfile,
		RenderNeeded:     req.RenderHint,
		FetchMode:        resp.Document.FetchMode,
		NoiseLevel:       packMetadata(resp.Trace, "noise_level"),
		PageType:         packMetadata(resp.Trace, "page_type"),
	})
}

func PrepareCrawlRequestWithLocalState(storeRoot string, req CrawlRequest) CrawlRequest {
	if genome, err := store.NewGenomeStore(storeRoot).LoadByURL(req.SeedURL); err == nil {
		applyGenomeHints(&req.ForceLane, &req.Profile, &req.PruningProfile, &req.RenderHint, genome)
	}
	return req
}

func ObserveCrawlResponseWithLocalState(storeRoot string, req CrawlRequest, resp CrawlResponse) {
	candidateStore := store.NewCandidateStore(storeRoot)
	genomeStore := store.NewGenomeStore(storeRoot)
	for _, page := range resp.Pages {
		_ = observeCandidate(candidateStore, page.Document, "crawl")
		if page.Document.FinalURL != "" && page.Trace.TraceID != "" && len(page.ResultPack.Chunks) > 0 {
			_, _, _ = store.NewFingerprintGraphStore(storeRoot).Observe(page.Document.FinalURL, page.Trace.TraceID, page.ResultPack.Chunks)
		}
		_, _, _ = genomeStore.Observe(store.GenomeObservation{
			URL:              page.Document.FinalURL,
			ObservedLane:     maxLane(page.ResultPack.CostReport.LanePath),
			PreferredProfile: page.ResultPack.Profile,
			PruningProfile:   req.PruningProfile,
			RenderNeeded:     req.RenderHint,
			FetchMode:        page.Document.FetchMode,
			NoiseLevel:       packMetadata(page.Trace, "noise_level"),
			PageType:         packMetadata(page.Trace, "page_type"),
		})
	}
}

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
