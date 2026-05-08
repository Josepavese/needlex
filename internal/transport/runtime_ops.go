package transport

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/analytics"
	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core/queryplan"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/memory"
	"github.com/josepavese/needlex/internal/platform"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/store"
)

func (r Runner) executeCrawlWithSurface(cfg config.Config, req coreservice.CrawlRequest, surface string) (coreservice.CrawlResponse, crawlArtifacts, error) {
	req = coreservice.PrepareCrawlRequestWithLocalState(r.storeRoot, req)

	startedAt := time.Now().UTC()
	cacheStart := intel.SnapshotEmbeddingCacheCounters()
	ctx, cancel := runtimeOperationContext(cfg, "crawl", req.MaxPages)
	defer cancel()
	resp, err := r.callCrawl(ctx, cfg, req)
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
		r.observeDiscoveryMemory(cfg, observationWithFingerprintEvidence(r.storeRoot, page.Document.FinalURL, memory.Observation{
			Document:     page.Document,
			ResultPack:   page.ResultPack,
			ProofRecords: page.ProofRecords,
			TraceID:      page.Trace.TraceID,
			SourceKind:   "crawl",
		}))
	}
	r.observeAnalyticsCrawl(cfg, surface, req, resp, storedRuns, analyticsEmbeddingCacheDelta(cacheStart))
	r.observeRuntimeCrawlDiagnostics(surface, req, resp, storedRuns)
	coreservice.ObserveCrawlResponseWithLocalState(r.storeRoot, req, resp)

	return resp, crawlArtifacts{StoredRuns: storedRuns}, nil
}

func (r Runner) executeReadWithSurface(cfg config.Config, req coreservice.ReadRequest, surface string) (coreservice.ReadResponse, artifactPaths, error) {
	req = coreservice.PrepareReadRequestWithLocalState(r.storeRoot, req)

	startedAt := time.Now().UTC()
	cacheStart := intel.SnapshotEmbeddingCacheCounters()
	ctx, cancel := runtimeOperationContext(cfg, "read", 1)
	defer cancel()
	resp, err := r.callRead(ctx, cfg, req)
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
	r.observeDiscoveryMemory(cfg, observationWithFingerprintEvidence(r.storeRoot, resp.Document.FinalURL, memory.Observation{
		Document:     resp.Document,
		ResultPack:   resp.ResultPack,
		ProofRecords: resp.ProofRecords,
		TraceID:      resp.Trace.TraceID,
		SourceKind:   "read",
	}))
	r.observeAnalyticsRead(cfg, surface, req, resp, analyticsEmbeddingCacheDelta(cacheStart))
	r.observeRuntimeReadDiagnostics(surface, req, resp)

	return resp, artifactPaths{
		TracePath:       tracePath,
		ProofPath:       proofPath,
		FingerprintPath: fingerprintPath,
	}, nil
}

func (r Runner) executeQueryWithSurface(cfg config.Config, req coreservice.QueryRequest, surface string) (coreservice.QueryResponse, artifactPaths, error) {
	req = coreservice.PrepareQueryRequestWithLocalState(r.storeRoot, req, cfg, r.semanticAligner(cfg))
	req.FingerprintEvidenceLoader = coreservice.NewFingerprintEvidenceLoader(r.storeRoot)
	startedAt := time.Now().UTC()
	cacheStart := intel.SnapshotEmbeddingCacheCounters()
	ctx, cancel := runtimeOperationContext(cfg, "query", 1)
	defer cancel()
	resp, err := r.callQuery(ctx, cfg, req)
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
	entityHints, localityHints, categoryHints := queryCompilerMemoryHints(resp.Plan.Compiler)
	r.observeDiscoveryMemory(cfg, observationWithFingerprintEvidence(r.storeRoot, resp.Document.FinalURL, memory.Observation{
		Document:      resp.Document,
		ResultPack:    resp.ResultPack,
		ProofRecords:  resp.ProofRecords,
		TraceID:       resp.TraceID,
		SourceKind:    "query",
		EntityHints:   entityHints,
		LocalityHints: localityHints,
		CategoryHints: categoryHints,
	}))
	r.observeAnalyticsQuery(cfg, surface, req, resp, analyticsEmbeddingCacheDelta(cacheStart))
	r.observeRuntimeQueryDiagnostics(surface, req, resp)

	return resp, artifactPaths{
		TracePath:       tracePath,
		ProofPath:       proofPath,
		FingerprintPath: fingerprintPath,
	}, nil
}

func (r Runner) semanticAligner(cfg config.Config) intel.SemanticAligner {
	if r.newSemanticAligner != nil {
		return r.newSemanticAligner(cfg)
	}
	return intel.NewSemanticAlignerWithCacheDir(cfg, nil, platform.NewStateLayout(r.storeRoot).EmbeddingCacheDir)
}

func (r Runner) textEmbedder(cfg config.Config) intel.TextEmbedder {
	return intel.NewTextEmbedderWithCacheDir(cfg, nil, platform.NewStateLayout(r.storeRoot).EmbeddingCacheDir)
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
	service := memory.NewService(cfg.Memory, store, r.textEmbedder(cfg))
	ctx, cancel := persistenceOperationContext(cfg)
	defer cancel()
	if err := service.Observe(ctx, observation); err != nil {
		r.logRuntimeWarning("memory", "memory.observe_failed", "memory observation failed", map[string]any{
			"source_kind": observation.SourceKind,
			"url":         firstNonEmptyRuntimeString(observation.Document.FinalURL, observation.Document.URL),
			"trace_id":    observation.TraceID,
			"error":       err.Error(),
		})
	}
}

func (r Runner) observeAnalyticsRead(cfg config.Config, surface string, req coreservice.ReadRequest, resp coreservice.ReadResponse, cacheCounters analytics.EmbeddingCacheCounters) {
	stats := r.analyticsMemoryStats(cfg)
	packetBytes := compactJSONSize(compactReadResponse(resp))
	ctx, cancel := persistenceOperationContext(cfg)
	defer cancel()
	if err := analytics.ObserveRead(ctx, analytics.NewSQLiteStore(r.storeRoot), surface, req, resp, packetBytes, stats, cacheCounters); err != nil {
		r.logRuntimeWarning("analytics", "analytics.observe_failed", "analytics read observation failed", map[string]any{
			"operation": "read",
			"surface":   surface,
			"url":       req.URL,
			"trace_id":  resp.Trace.TraceID,
			"error":     err.Error(),
		})
	}
}

func (r Runner) observeAnalyticsQuery(cfg config.Config, surface string, req coreservice.QueryRequest, resp coreservice.QueryResponse, cacheCounters analytics.EmbeddingCacheCounters) {
	stats := r.analyticsMemoryStats(cfg)
	packetBytes := compactJSONSize(compactQueryResponse(resp))
	ctx, cancel := persistenceOperationContext(cfg)
	defer cancel()
	if err := analytics.ObserveQuery(ctx, analytics.NewSQLiteStore(r.storeRoot), surface, req, resp, packetBytes, stats, cacheCounters); err != nil {
		r.logRuntimeWarning("analytics", "analytics.observe_failed", "analytics query observation failed", map[string]any{
			"operation":  "query",
			"surface":    surface,
			"goal_chars": len(strings.TrimSpace(req.Goal)),
			"trace_id":   resp.TraceID,
			"error":      err.Error(),
		})
	}
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

func (r Runner) observeAnalyticsCrawl(cfg config.Config, surface string, req coreservice.CrawlRequest, resp coreservice.CrawlResponse, storedRuns int, cacheCounters analytics.EmbeddingCacheCounters) {
	stats := r.analyticsMemoryStats(cfg)
	packetBytes := compactJSONSize(compactCrawlResponse(resp, crawlArtifacts{StoredRuns: storedRuns}))
	ctx, cancel := persistenceOperationContext(cfg)
	defer cancel()
	if err := analytics.ObserveCrawl(ctx, analytics.NewSQLiteStore(r.storeRoot), surface, req, resp, packetBytes, stats, cacheCounters); err != nil {
		r.logRuntimeWarning("analytics", "analytics.observe_failed", "analytics crawl observation failed", map[string]any{
			"operation": "crawl",
			"surface":   surface,
			"seed_url":  req.SeedURL,
			"error":     err.Error(),
		})
	}
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
	ctx, cancel := persistenceOperationContext(cfg)
	defer cancel()
	if err := analytics.ObserveFailure(ctx, analytics.NewSQLiteStore(r.storeRoot), failure); err != nil {
		r.logRuntimeWarning("analytics", "analytics.failure_observe_failed", "analytics failure observation failed", map[string]any{
			"operation": failure.Operation,
			"surface":   failure.Surface,
			"error":     err.Error(),
		})
	}
}

func analyticsEmbeddingCacheDelta(before intel.EmbeddingCacheCounters) analytics.EmbeddingCacheCounters {
	delta := intel.DiffEmbeddingCacheCounters(before, intel.SnapshotEmbeddingCacheCounters())
	return analytics.EmbeddingCacheCounters{
		Hits:         delta.Hits,
		Misses:       delta.Misses,
		Writes:       delta.Writes,
		NegativeHits: delta.NegativeHits,
		StaleHits:    delta.StaleHits,
		Evictions:    delta.Evictions,
		EvictedBytes: delta.EvictedBytes,
	}
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
	ctx, cancel := persistenceOperationContext(cfg)
	defer cancel()
	stats, err := memory.NewSQLiteStore(r.storeRoot, cfg.Memory.Path).GetStats(ctx)
	if err != nil {
		r.logRuntimeWarning("memory", "memory.stats_failed", "memory stats unavailable for analytics enrichment", map[string]any{
			"error": err.Error(),
		})
		return memory.Stats{}
	}
	return stats
}

func persistenceOperationContext(cfg config.Config) (context.Context, context.CancelFunc) {
	timeout := maxDuration(
		2*time.Second,
		time.Duration(cfg.Semantic.TimeoutMS)*time.Millisecond,
		time.Duration(cfg.Runtime.TimeoutMS/2)*time.Millisecond,
	)
	timeout = min(timeout, 10*time.Second)
	return context.WithTimeout(context.Background(), timeout)
}

func compactJSONSize(value any) int {
	data, _ := json.Marshal(value)
	return len(data)
}

func runtimeOperationContext(cfg config.Config, operation string, workItems int) (context.Context, context.CancelFunc) {
	if timeout := runtimeOperationTimeout(cfg, operation, workItems); timeout > 0 {
		return context.WithTimeout(context.Background(), timeout)
	}
	return context.WithCancel(context.Background())
}

func runtimeOperationTimeout(cfg config.Config, operation string, workItems int) time.Duration {
	unit := time.Duration(cfg.Runtime.TimeoutMS) * time.Millisecond
	if unit <= 0 {
		return 0
	}
	model := maxDuration(
		time.Duration(cfg.Models.MicroTimeoutMS)*time.Millisecond,
		time.Duration(cfg.Models.StructuredTimeoutMS)*time.Millisecond,
		time.Duration(cfg.Models.SpecialistTimeoutMS)*time.Millisecond,
		time.Duration(cfg.Semantic.TimeoutMS)*time.Millisecond,
	)
	readBudget := unit*4 + model + 2*time.Second
	if operation == "query" {
		return readBudget * 2
	}
	if operation == "crawl" {
		if workItems <= 0 {
			workItems = cfg.Runtime.MaxPages
		}
		if workItems <= 0 {
			workItems = 1
		}
		return readBudget * time.Duration(min(workItems*2, 80))
	}
	return readBudget
}

func maxDuration(values ...time.Duration) time.Duration {
	var out time.Duration
	for _, value := range values {
		out = max(out, value)
	}
	return out
}

func firstCandidateURLs(candidates []coreservice.AgentCandidate, limit int) []string {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	limit = min(limit, len(candidates))
	out := make([]string, 0, limit)
	for _, candidate := range candidates[:limit] {
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
	value, _ := strconv.Atoi(strings.TrimSpace(metadata[key]))
	return value
}

func observationWithFingerprintEvidence(storeRoot, rawURL string, obs memory.Observation) memory.Observation {
	evidence, ok := coreservice.NewFingerprintEvidenceLoader(storeRoot)(rawURL)
	if !ok {
		return obs
	}
	obs.StableRatio = evidence.Stable
	obs.NoveltyRatio = evidence.Novelty
	obs.ChangedRecently = evidence.Changed
	return obs
}

func queryCompilerMemoryHints(plan queryplan.QueryCompiler) ([]string, []string, []string) {
	for _, decision := range plan.Decisions {
		if decision.Stage != "plan.query_rewrite" {
			continue
		}
		var entities []string
		if entity := strings.TrimSpace(decision.Metadata["canonical_entity"]); entity != "" {
			entities = []string{entity}
		}
		return entities, splitCSV(decision.Metadata["locality_hints"]), splitCSV(decision.Metadata["category_hints"])
	}
	return nil, nil, nil
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
