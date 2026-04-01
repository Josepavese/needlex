package service

func PrepareReadRequestWithLocalState(storeRoot string, req ReadRequest) ReadRequest {
	loadGenomeHints(storeRoot, req.URL, &req.ForceLane, &req.Profile, &req.PruningProfile, &req.RenderHint)
	req.StableFingerprints = append(req.StableFingerprints, stableFingerprintsForURL(storeRoot, req.URL)...)
	return req
}

func ObserveReadResponseWithLocalState(storeRoot string, req ReadRequest, resp ReadResponse) {
	observeStoredPage(storeRoot, "read", resp.Document, resp.Trace.TraceID, resp.ResultPack.Chunks)
	observeGenome(storeRoot, genomeObservation(resp.Document, resp.Trace, resp.ResultPack, req.PruningProfile, req.RenderHint))
}

func PrepareCrawlRequestWithLocalState(storeRoot string, req CrawlRequest) CrawlRequest {
	loadGenomeHints(storeRoot, req.SeedURL, &req.ForceLane, &req.Profile, &req.PruningProfile, &req.RenderHint)
	return req
}

func ObserveCrawlResponseWithLocalState(storeRoot string, req CrawlRequest, resp CrawlResponse) {
	for _, page := range resp.Pages {
		observeStoredPage(storeRoot, "crawl", page.Document, page.Trace.TraceID, page.ResultPack.Chunks)
		observeGenome(storeRoot, genomeObservation(page.Document, page.Trace, page.ResultPack, req.PruningProfile, req.RenderHint))
	}
}
