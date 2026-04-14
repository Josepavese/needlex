package transport

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/josepavese/needlex/internal/analytics"
	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/memory"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/store"
)

func (r Runner) executeCrawl(cfg config.Config, req coreservice.CrawlRequest) (coreservice.CrawlResponse, crawlArtifacts, error) {
	req = coreservice.PrepareCrawlRequestWithLocalState(r.storeRoot, req)

	resp, err := r.crawl(context.Background(), cfg, req)
	if err != nil {
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
	r.observeAnalyticsCrawl(cfg, req, resp, storedRuns)
	coreservice.ObserveCrawlResponseWithLocalState(r.storeRoot, req, resp)

	return resp, crawlArtifacts{StoredRuns: storedRuns}, nil
}

func (r Runner) executeRead(cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, artifactPaths, error) {
	req = coreservice.PrepareReadRequestWithLocalState(r.storeRoot, req)

	resp, err := r.read(context.Background(), cfg, req)
	if err != nil {
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
	r.observeAnalyticsRead(cfg, req, resp)

	return resp, artifactPaths{
		TracePath:       tracePath,
		ProofPath:       proofPath,
		FingerprintPath: fingerprintPath,
	}, nil
}

func (r Runner) executeQuery(cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, artifactPaths, error) {
	req = coreservice.PrepareQueryRequestWithLocalState(r.storeRoot, req, cfg, intel.NewSemanticAligner(cfg, nil))
	req.FingerprintEvidenceLoader = coreservice.NewFingerprintEvidenceLoader(r.storeRoot)
	resp, err := r.query(context.Background(), cfg, req)
	if err != nil {
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
	r.observeAnalyticsQuery(cfg, req, resp)

	return resp, artifactPaths{
		TracePath:       tracePath,
		ProofPath:       proofPath,
		FingerprintPath: fingerprintPath,
	}, nil
}

func (r Runner) observeDiscoveryMemory(cfg config.Config, observation memory.Observation) {
	if !cfg.Memory.Enabled {
		return
	}
	store := memory.NewSQLiteStore(r.storeRoot, cfg.Memory.Path)
	service := memory.NewService(cfg.Memory, store, intel.NewTextEmbedder(cfg, nil))
	_ = service.Observe(context.Background(), observation)
}

func (r Runner) observeAnalyticsRead(cfg config.Config, req coreservice.ReadRequest, resp coreservice.ReadResponse) {
	stats := r.analyticsMemoryStats(cfg)
	packetBytes := compactJSONSize(compactReadResponse(resp))
	_ = analytics.ObserveRead(context.Background(), analytics.NewSQLiteStore(r.storeRoot), "cli", req, resp, packetBytes, stats)
}

func (r Runner) observeAnalyticsQuery(cfg config.Config, req coreservice.QueryRequest, resp coreservice.QueryResponse) {
	stats := r.analyticsMemoryStats(cfg)
	packetBytes := compactJSONSize(compactQueryResponse(resp))
	_ = analytics.ObserveQuery(context.Background(), analytics.NewSQLiteStore(r.storeRoot), "cli", req, resp, packetBytes, stats)
}

func (r Runner) observeAnalyticsCrawl(cfg config.Config, req coreservice.CrawlRequest, resp coreservice.CrawlResponse, storedRuns int) {
	stats := r.analyticsMemoryStats(cfg)
	packetBytes := compactJSONSize(compactCrawlResponse(resp, crawlArtifacts{StoredRuns: storedRuns}))
	_ = analytics.ObserveCrawl(context.Background(), analytics.NewSQLiteStore(r.storeRoot), "cli", req, resp, packetBytes, stats)
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
