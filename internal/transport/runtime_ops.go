package transport

import (
	"context"

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
			Document:     page.Document,
			ResultPack:   page.ResultPack,
			ProofRecords: page.ProofRecords,
			TraceID:      page.Trace.TraceID,
			SourceKind:   "crawl",
		})
	}
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
		Document:     resp.Document,
		ResultPack:   resp.ResultPack,
		ProofRecords: resp.ProofRecords,
		TraceID:      resp.Trace.TraceID,
		SourceKind:   "read",
	})

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
		Document:     resp.Document,
		ResultPack:   resp.ResultPack,
		ProofRecords: resp.ProofRecords,
		TraceID:      resp.TraceID,
		SourceKind:   "query",
	})

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
