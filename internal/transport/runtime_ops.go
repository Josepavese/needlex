package transport

import (
	"context"

	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/store"
)

type readArtifacts struct {
	TracePath       string `json:"trace_path"`
	ProofPath       string `json:"proof_path"`
	FingerprintPath string `json:"fingerprint_path"`
}

func (r Runner) executeRead(cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, readArtifacts, error) {
	genomeStore := store.NewGenomeStore(r.storeRoot)
	if genome, err := genomeStore.LoadByURL(req.URL); err == nil {
		req.ForceLane = genome.ForceLane
	}

	resp, err := r.read(context.Background(), cfg, req)
	if err != nil {
		return coreservice.ReadResponse{}, readArtifacts{}, err
	}

	tracePath, err := store.NewTraceStore(r.storeRoot).SaveTrace(resp.Trace)
	if err != nil {
		return coreservice.ReadResponse{}, readArtifacts{}, err
	}
	proofPath, err := store.NewProofStore(r.storeRoot).SaveProofRecords(resp.Trace.TraceID, resp.ProofRecords)
	if err != nil {
		return coreservice.ReadResponse{}, readArtifacts{}, err
	}
	fingerprintPath, err := store.NewFingerprintStore(r.storeRoot).SaveChunks(resp.Trace.TraceID, resp.ResultPack.Chunks)
	if err != nil {
		return coreservice.ReadResponse{}, readArtifacts{}, err
	}
	_, _, _ = genomeStore.Observe(store.GenomeObservation{
		URL:              resp.Document.FinalURL,
		ObservedLane:     maxLane(resp.ResultPack.CostReport.LanePath),
		PreferredProfile: resp.ResultPack.Profile,
		FetchMode:        resp.Document.FetchMode,
		NoiseLevel:       packMetadata(resp.Trace, "noise_level"),
		PageType:         packMetadata(resp.Trace, "page_type"),
	})

	return resp, readArtifacts{
		TracePath:       tracePath,
		ProofPath:       proofPath,
		FingerprintPath: fingerprintPath,
	}, nil
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

	record, traceID, findErr := proofStore.FindProofByChunkID(lookup)
	if findErr != nil {
		return proofLookupResult{}, findErr
	}
	result.TraceID = traceID
	result.Records = []proof.ProofRecord{record}
	return result, nil
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

func maxLane(path []int) int {
	max := 0
	for _, lane := range path {
		if lane > max {
			max = lane
		}
	}
	return max
}
