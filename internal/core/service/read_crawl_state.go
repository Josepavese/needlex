package service

import "github.com/josepavese/needlex/internal/store"

func PrepareReadRequestWithLocalState(state store.LocalState, req ReadRequest) ReadRequest {
	loadGenomeHints(state.Genomes, req.URL, &req.ForceLane, &req.Profile, &req.FetchProfile, &req.FetchRetryProfile, &req.PruningProfile, &req.RenderHint)
	req.StableFingerprints = append(req.StableFingerprints, stableFingerprintsForURL(state.Fingerprints, req.URL)...)
	return req
}

func ObserveReadResponseWithLocalState(state store.LocalState, req ReadRequest, resp ReadResponse) {
	observeStoredPage(state, "read", resp.Document, resp.Trace.TraceID, resp.ResultPack.Chunks)
	observeGenome(state.Genomes, genomeObservation(resp.Document, resp.Trace, resp.ResultPack, req.PruningProfile, req.RenderHint))
}

func PrepareCrawlRequestWithLocalState(state store.LocalState, req CrawlRequest) CrawlRequest {
	loadGenomeHints(state.Genomes, req.SeedURL, &req.ForceLane, &req.Profile, &req.FetchProfile, &req.FetchRetryProfile, &req.PruningProfile, &req.RenderHint)
	return req
}

func ObserveCrawlResponseWithLocalState(state store.LocalState, req CrawlRequest, resp CrawlResponse) {
	for _, page := range resp.Pages {
		observeStoredPage(state, "crawl", page.Document, page.Trace.TraceID, page.ResultPack.Chunks)
		observeGenome(state.Genomes, genomeObservation(page.Document, page.Trace, page.ResultPack, req.PruningProfile, req.RenderHint))
	}
}
