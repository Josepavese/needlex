package transport

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/analytics"
	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/memory"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/store"
)

func (r Runner) executeCrawl(cfg config.Config, req coreservice.CrawlRequest) (coreservice.CrawlResponse, crawlArtifacts, error) {
	return r.executeCrawlWithSurface(cfg, req, "cli")
}

func (r Runner) executeCrawlWithSurface(cfg config.Config, req coreservice.CrawlRequest, surface string) (coreservice.CrawlResponse, crawlArtifacts, error) {
	req = coreservice.PrepareCrawlRequestWithLocalState(r.storeRoot, req)

	startedAt := time.Now().UTC()
	resp, err := r.callCrawl(context.Background(), cfg, req)
	if err != nil {
		r.observeRuntimeFailure("crawl", "crawl.failed", surface, startedAt, err, map[string]any{
			"seed_url":       req.SeedURL,
			"profile":        req.Profile,
			"discovery_mode": crawlDiscoveryMode(req),
		})
		r.observeAnalyticsFailure(cfg, analytics.FailureObservation{
			Operation:     "crawl",
			Surface:       surface,
			Profile:       req.Profile,
			Goal:          req.SeedURL,
			SeedURL:       req.SeedURL,
			DiscoveryMode: crawlDiscoveryMode(req),
			StartedAt:     startedAt,
			Err:           err,
		})
		return coreservice.CrawlResponse{}, crawlArtifacts{}, err
	}

	storedRuns := 0
	for _, page := range resp.Pages {
		if _, err := store.NewTraceStore(r.storeRoot).SaveTrace(page.Trace); err == nil {
			storedRuns++
		}
		_, _ = store.NewProofStore(r.storeRoot).SaveProofRecords(page.Trace.TraceID, page.ProofRecords)
		_, _ = store.NewFingerprintStore(r.storeRoot).SaveChunks(page.Trace.TraceID, page.ResultPack.Chunks)
		r.observeDiscoveryMemory(cfg, memory.Observation{
			Document:        page.Document,
			ResultPack:      page.ResultPack,
			ProofRecords:    page.ProofRecords,
			TraceID:         page.Trace.TraceID,
			SourceKind:      "crawl",
			StableRatio:     pageFingerprintStable(r.storeRoot, page.Document.FinalURL),
			NoveltyRatio:    pageFingerprintNovelty(r.storeRoot, page.Document.FinalURL),
			ChangedRecently: pageFingerprintChanged(r.storeRoot, page.Document.FinalURL),
		})
	}
	r.observeAnalyticsCrawl(cfg, surface, req, resp, storedRuns)
	r.observeRuntimeCrawlDiagnostics(surface, req, resp, storedRuns)
	coreservice.ObserveCrawlResponseWithLocalState(r.storeRoot, req, resp)

	return resp, crawlArtifacts{StoredRuns: storedRuns}, nil
}

func (r Runner) executeRead(cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, artifactPaths, error) {
	return r.executeReadWithSurface(cfg, req, "cli")
}

func (r Runner) executeReadWithSurface(cfg config.Config, req coreservice.ReadRequest, surface string) (coreservice.ReadResponse, artifactPaths, error) {
	req = coreservice.PrepareReadRequestWithLocalState(r.storeRoot, req)

	startedAt := time.Now().UTC()
	resp, err := r.callRead(context.Background(), cfg, req)
	if err != nil {
		r.observeRuntimeFailure("read", "fetch.failed", surface, startedAt, err, map[string]any{
			"url":       req.URL,
			"profile":   req.Profile,
			"objective": req.Objective,
		})
		r.observeAnalyticsFailure(cfg, analytics.FailureObservation{
			Operation: "read",
			Surface:   surface,
			Profile:   req.Profile,
			Goal:      req.Objective,
			URL:       req.URL,
			StartedAt: startedAt,
			Err:       err,
		})
		return coreservice.ReadResponse{}, artifactPaths{}, err
	}

	tracePath, err := store.NewTraceStore(r.storeRoot).SaveTrace(resp.Trace)
	if err != nil {
		return coreservice.ReadResponse{}, artifactPaths{}, err
	}
	proofPath, err := store.NewProofStore(r.storeRoot).SaveProofRecords(resp.Trace.TraceID, resp.ProofRecords)
	if err != nil {
		return coreservice.ReadResponse{}, artifactPaths{}, err
	}
	fingerprintPath, err := store.NewFingerprintStore(r.storeRoot).SaveChunks(resp.Trace.TraceID, resp.ResultPack.Chunks)
	if err != nil {
		return coreservice.ReadResponse{}, artifactPaths{}, err
	}
	coreservice.ObserveReadResponseWithLocalState(r.storeRoot, req, resp)
	r.observeDiscoveryMemory(cfg, memory.Observation{
		Document:        resp.Document,
		ResultPack:      resp.ResultPack,
		ProofRecords:    resp.ProofRecords,
		TraceID:         resp.Trace.TraceID,
		SourceKind:      "read",
		StableRatio:     pageFingerprintStable(r.storeRoot, resp.Document.FinalURL),
		NoveltyRatio:    pageFingerprintNovelty(r.storeRoot, resp.Document.FinalURL),
		ChangedRecently: pageFingerprintChanged(r.storeRoot, resp.Document.FinalURL),
	})
	r.observeAnalyticsRead(cfg, surface, req, resp)
	r.observeRuntimeReadDiagnostics(surface, req, resp)

	return resp, artifactPaths{
		TracePath:       tracePath,
		ProofPath:       proofPath,
		FingerprintPath: fingerprintPath,
	}, nil
}

func (r Runner) executeQuery(cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, artifactPaths, error) {
	return r.executeQueryWithSurface(cfg, req, "cli")
}

func (r Runner) executeQueryWithSurface(cfg config.Config, req coreservice.QueryRequest, surface string) (coreservice.QueryResponse, artifactPaths, error) {
	req = coreservice.PrepareQueryRequestWithLocalState(r.storeRoot, req, cfg, intel.NewSemanticAligner(cfg, nil))
	req.FingerprintEvidenceLoader = coreservice.NewFingerprintEvidenceLoader(r.storeRoot)
	startedAt := time.Now().UTC()
	resp, err := r.callQuery(context.Background(), cfg, req)
	if err != nil {
		r.observeRuntimeFailure("query", "discovery_or_fetch.failed", surface, startedAt, err, map[string]any{
			"seed_url":       req.SeedURL,
			"profile":        req.Profile,
			"goal":           req.Goal,
			"discovery_mode": req.DiscoveryMode,
		})
		r.observeAnalyticsFailure(cfg, analytics.FailureObservation{
			Operation:     "query",
			Surface:       surface,
			Profile:       req.Profile,
			Goal:          req.Goal,
			SeedURL:       req.SeedURL,
			DiscoveryMode: req.DiscoveryMode,
			StartedAt:     startedAt,
			Err:           err,
		})
		return coreservice.QueryResponse{}, artifactPaths{}, err
	}

	tracePath, err := store.NewTraceStore(r.storeRoot).SaveTrace(resp.Trace)
	if err != nil {
		return coreservice.QueryResponse{}, artifactPaths{}, err
	}
	proofPath, err := store.NewProofStore(r.storeRoot).SaveProofRecords(resp.TraceID, resp.ProofRecords)
	if err != nil {
		return coreservice.QueryResponse{}, artifactPaths{}, err
	}
	fingerprintPath, err := store.NewFingerprintStore(r.storeRoot).SaveChunks(resp.TraceID, resp.ResultPack.Chunks)
	if err != nil {
		return coreservice.QueryResponse{}, artifactPaths{}, err
	}
	coreservice.ObserveQueryResponseWithLocalState(r.storeRoot, req, resp)
	r.observeDiscoveryMemory(cfg, memory.Observation{
		Document:        resp.Document,
		ResultPack:      resp.ResultPack,
		ProofRecords:    resp.ProofRecords,
		TraceID:         resp.TraceID,
		SourceKind:      "query",
		EntityHints:     queryCompilerEntityHints(resp.Plan.Compiler),
		LocalityHints:   queryCompilerListMetadata(resp.Plan.Compiler, "locality_hints"),
		CategoryHints:   queryCompilerListMetadata(resp.Plan.Compiler, "category_hints"),
		StableRatio:     pageFingerprintStable(r.storeRoot, resp.Document.FinalURL),
		NoveltyRatio:    pageFingerprintNovelty(r.storeRoot, resp.Document.FinalURL),
		ChangedRecently: pageFingerprintChanged(r.storeRoot, resp.Document.FinalURL),
	})
	r.observeAnalyticsQuery(cfg, surface, req, resp)
	r.observeRuntimeQueryDiagnostics(surface, req, resp)

	return resp, artifactPaths{
		TracePath:       tracePath,
		ProofPath:       proofPath,
		FingerprintPath: fingerprintPath,
	}, nil
}

func (r Runner) callRead(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
	if r.read != nil {
		return r.read(ctx, cfg, req)
	}
	svc, err := coreservice.NewWithStateRoot(cfg, nil, r.storeRoot)
	if err != nil {
		return coreservice.ReadResponse{}, err
	}
	return svc.Read(ctx, req)
}

func (r Runner) callQuery(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
	if r.query != nil {
		return r.query(ctx, cfg, req)
	}
	svc, err := coreservice.NewWithStateRoot(cfg, nil, r.storeRoot)
	if err != nil {
		return coreservice.QueryResponse{}, err
	}
	return svc.Query(ctx, req)
}

func (r Runner) callCrawl(ctx context.Context, cfg config.Config, req coreservice.CrawlRequest) (coreservice.CrawlResponse, error) {
	if r.crawl != nil {
		return r.crawl(ctx, cfg, req)
	}
	svc, err := coreservice.NewWithStateRoot(cfg, nil, r.storeRoot)
	if err != nil {
		return coreservice.CrawlResponse{}, err
	}
	return svc.Crawl(ctx, req)
}

func (r Runner) observeDiscoveryMemory(cfg config.Config, observation memory.Observation) {
	if !cfg.Memory.Enabled {
		return
	}
	store := memory.NewSQLiteStore(r.storeRoot, cfg.Memory.Path)
	service := memory.NewService(cfg.Memory, store, intel.NewTextEmbedder(cfg, nil))
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Semantic.TimeoutMS)*time.Millisecond)
	defer cancel()
	_ = service.Observe(ctx, observation)
}

func (r Runner) observeAnalyticsRead(cfg config.Config, surface string, req coreservice.ReadRequest, resp coreservice.ReadResponse) {
	stats := r.analyticsMemoryStats(cfg)
	packetBytes := compactJSONSize(compactReadResponse(resp))
	_ = analytics.ObserveRead(context.Background(), analytics.NewSQLiteStore(r.storeRoot), surface, req, resp, packetBytes, stats)
}

func (r Runner) observeAnalyticsQuery(cfg config.Config, surface string, req coreservice.QueryRequest, resp coreservice.QueryResponse) {
	stats := r.analyticsMemoryStats(cfg)
	packetBytes := compactJSONSize(compactQueryResponse(resp))
	_ = analytics.ObserveQuery(context.Background(), analytics.NewSQLiteStore(r.storeRoot), surface, req, resp, packetBytes, stats)
}

func (r Runner) observeRuntimeReadDiagnostics(surface string, req coreservice.ReadRequest, resp coreservice.ReadResponse) {
	r.observeRuntimeFetchCompleted("read", surface, req.URL, resp.Document.FinalURL, resp.Trace)
}

func (r Runner) observeRuntimeQueryDiagnostics(surface string, req coreservice.QueryRequest, resp coreservice.QueryResponse) {
	r.logRuntimeInfo("query", "discovery.completed", "query discovery completed", map[string]any{
		"surface":         surface,
		"goal":            req.Goal,
		"seed_url":        req.SeedURL,
		"discovery_mode":  resp.Plan.DiscoveryMode,
		"provider":        resp.Plan.DiscoveryProvider,
		"candidate_count": len(resp.AgentContext.Candidates),
		"selected_url":    resp.Plan.SelectedURL,
		"trace_id":        resp.TraceID,
	})
	r.observeRuntimeFetchCompleted("query", surface, resp.Plan.SelectedURL, resp.Document.FinalURL, resp.Trace)
	if strings.TrimSpace(req.SeedURL) != "" {
		return
	}
	candidates := resp.AgentContext.Candidates
	if len(candidates) < 3 {
		return
	}
	topScore := candidates[0].Score
	secondScore := candidates[1].Score
	scoreDelta := topScore - secondScore
	if len(candidates) < 5 && scoreDelta > 0.08 {
		return
	}
	r.logRuntimeWarning("query", "seedless.ambiguous_candidates", "seedless discovery returned a dense or low-margin candidate set", map[string]any{
		"surface":         surface,
		"goal":            req.Goal,
		"discovery_mode":  resp.Plan.DiscoveryMode,
		"provider":        resp.Plan.DiscoveryProvider,
		"candidate_count": len(candidates),
		"selected_url":    resp.Plan.SelectedURL,
		"top_score":       topScore,
		"second_score":    secondScore,
		"score_delta":     scoreDelta,
		"candidate_urls":  firstCandidateURLs(candidates, 8),
		"trace_id":        resp.TraceID,
	})
}

func (r Runner) observeAnalyticsCrawl(cfg config.Config, surface string, req coreservice.CrawlRequest, resp coreservice.CrawlResponse, storedRuns int) {
	stats := r.analyticsMemoryStats(cfg)
	packetBytes := compactJSONSize(compactCrawlResponse(resp, crawlArtifacts{StoredRuns: storedRuns}))
	_ = analytics.ObserveCrawl(context.Background(), analytics.NewSQLiteStore(r.storeRoot), surface, req, resp, packetBytes, stats)
}

func (r Runner) observeRuntimeCrawlDiagnostics(surface string, req coreservice.CrawlRequest, resp coreservice.CrawlResponse, storedRuns int) {
	r.logRuntimeInfo("crawl", "crawl.completed", "crawl completed", map[string]any{
		"surface":        surface,
		"seed_url":       req.SeedURL,
		"same_domain":    req.SameDomain,
		"max_pages":      req.MaxPages,
		"max_depth":      req.MaxDepth,
		"page_count":     len(resp.Pages),
		"stored_runs":    storedRuns,
		"discovery_mode": crawlDiscoveryMode(req),
	})
	for _, page := range resp.Pages {
		r.observeRuntimeFetchCompleted("crawl", surface, page.Document.URL, page.Document.FinalURL, page.Trace)
	}
}

func (r Runner) observeAnalyticsFailure(cfg config.Config, failure analytics.FailureObservation) {
	failure.MemoryStats = r.analyticsMemoryStats(cfg)
	_ = analytics.ObserveFailure(context.Background(), analytics.NewSQLiteStore(r.storeRoot), failure)
}

func (r Runner) observeRuntimeFailure(operation, eventName, surface string, startedAt time.Time, err error, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["surface"] = surface
	fields["latency_ms"] = time.Since(startedAt).Milliseconds()
	r.logRuntimeError(operation, eventName, err, fields)
}

func crawlDiscoveryMode(req coreservice.CrawlRequest) string {
	if req.SameDomain {
		return "same_site_links"
	}
	return "web_search"
}

func (r Runner) analyticsMemoryStats(cfg config.Config) memory.Stats {
	if !cfg.Memory.Enabled {
		return memory.Stats{}
	}
	stats, err := memory.NewSQLiteStore(r.storeRoot, cfg.Memory.Path).GetStats(context.Background())
	if err != nil {
		return memory.Stats{}
	}
	return stats
}

func compactJSONSize(value any) int {
	data, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return len(data)
}

func firstCandidateURLs(candidates []coreservice.AgentCandidate, limit int) []string {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	out := make([]string, 0, min(len(candidates), limit))
	for i, candidate := range candidates {
		if i >= limit {
			break
		}
		out = append(out, candidate.URL)
	}
	return out
}

func (r Runner) observeRuntimeFetchCompleted(operation, surface, requestedURL, finalURL string, trace proof.RunTrace) {
	stage, ok := traceStage(trace, "acquire")
	if !ok {
		return
	}
	metadata := stage.Metadata
	r.logRuntimeInfo(operation, "fetch.completed", "fetch completed", map[string]any{
		"surface":        surface,
		"requested_url":  requestedURL,
		"final_url":      firstNonEmptyRuntimeString(finalURL, metadata["final_url"]),
		"trace_id":       trace.TraceID,
		"run_id":         trace.RunID,
		"latency_ms":     stageLatencyMS(stage),
		"fetch_mode":     metadata["fetch_mode"],
		"fetch_profile":  metadata["fetch_profile"],
		"retry_profile":  metadata["retry_profile"],
		"retry_count":    intMetadata(metadata, "retry_count"),
		"retry_reason":   metadata["retry_reason"],
		"host_pacing_ms": intMetadata(metadata, "host_pacing_ms"),
		"raw_chars":      intMetadata(metadata, "raw_chars"),
		"raw_bytes":      intMetadata(metadata, "raw_bytes"),
		"content_type":   metadata["content_type"],
	})
}

func traceStage(trace proof.RunTrace, name string) (proof.StageSnapshot, bool) {
	for _, stage := range trace.Stages {
		if stage.Stage == name {
			return stage, true
		}
	}
	return proof.StageSnapshot{}, false
}

func stageLatencyMS(stage proof.StageSnapshot) int64 {
	if stage.StartedAt.IsZero() || stage.CompletedAt.IsZero() {
		return 0
	}
	return stage.CompletedAt.Sub(stage.StartedAt).Milliseconds()
}

func intMetadata(metadata map[string]string, key string) int {
	value, err := strconv.Atoi(strings.TrimSpace(metadata[key]))
	if err != nil {
		return 0
	}
	return value
}

func pageFingerprintStable(storeRoot, rawURL string) float64 {
	evidence, ok := coreservice.NewFingerprintEvidenceLoader(storeRoot)(rawURL)
	if !ok {
		return 0
	}
	return evidence.Stable
}

func pageFingerprintNovelty(storeRoot, rawURL string) float64 {
	evidence, ok := coreservice.NewFingerprintEvidenceLoader(storeRoot)(rawURL)
	if !ok {
		return 0
	}
	return evidence.Novelty
}

func pageFingerprintChanged(storeRoot, rawURL string) bool {
	evidence, ok := coreservice.NewFingerprintEvidenceLoader(storeRoot)(rawURL)
	if !ok {
		return false
	}
	return evidence.Changed
}

func queryCompilerEntityHints(plan coreservice.QueryCompiler) []string {
	entity := ""
	for _, decision := range plan.Decisions {
		if decision.Stage != "plan.query_rewrite" {
			continue
		}
		if value := decision.Metadata["canonical_entity"]; value != "" {
			entity = value
			break
		}
	}
	if entity == "" {
		return nil
	}
	return []string{entity}
}

func queryCompilerListMetadata(plan coreservice.QueryCompiler, key string) []string {
	for _, decision := range plan.Decisions {
		if decision.Stage != "plan.query_rewrite" {
			continue
		}
		if raw := decision.Metadata[key]; raw != "" {
			return splitCommaMetadata(raw)
		}
	}
	return nil
}

func splitCommaMetadata(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	return out
}

func (r Runner) loadReplay(traceID string) (proof.ReplayReport, error) {
	trace, err := store.NewTraceStore(r.storeRoot).LoadTrace(traceID)
	if err != nil {
		return proof.ReplayReport{}, err
	}
	return trace.ReplayReport()
}

func (r Runner) loadDiff(traceA, traceB string) (proof.DiffReport, error) {
	left, err := store.NewTraceStore(r.storeRoot).LoadTrace(traceA)
	if err != nil {
		return proof.DiffReport{}, err
	}
	right, err := store.NewTraceStore(r.storeRoot).LoadTrace(traceB)
	if err != nil {
		return proof.DiffReport{}, err
	}
	return proof.Diff(left, right)
}

func (r Runner) loadProof(lookup string) (proofLookupResult, error) {
	proofStore := store.NewProofStore(r.storeRoot)
	result := proofLookupResult{Lookup: lookup}

	records, err := proofStore.LoadProofRecords(lookup)
	if err == nil {
		result.TraceID = lookup
		result.Records = records
		return result, nil
	}

	record, traceID, findErr := proofStore.FindProofByID(lookup)
	if findErr == nil {
		result.TraceID = traceID
		result.Records = []proof.ProofRecord{record}
		return result, nil
	}

	record, traceID, findErr = proofStore.FindProofByChunkID(lookup)
	if findErr != nil {
		return proofLookupResult{}, findErr
	}
	result.TraceID = traceID
	result.Records = []proof.ProofRecord{record}
	return result, nil
}
