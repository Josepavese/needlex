package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
)

type ReadRequest struct {
	URL       string
	Objective string
	UserAgent string
}

type ReadResponse struct {
	Document     core.Document       `json:"document"`
	ResultPack   core.ResultPack     `json:"result_pack"`
	ProofRecords []proof.ProofRecord `json:"proof_records"`
	Trace        proof.RunTrace      `json:"trace"`
	Replay       proof.ReplayReport  `json:"replay"`
}

type Service struct {
	cfg       config.Config
	acquirer  pipeline.Acquirer
	reducer   pipeline.Reducer
	segmenter pipeline.Segmenter
	now       func() time.Time
}

func New(cfg config.Config, client *http.Client) (*Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Service{
		cfg:      cfg,
		acquirer: pipeline.Acquirer{Client: client},
		reducer:  pipeline.Reducer{},
		segmenter: pipeline.Segmenter{
			MaxSegmentChars: 1200,
		},
		now: time.Now,
	}, nil
}

func (s *Service) Read(ctx context.Context, req ReadRequest) (ReadResponse, error) {
	if err := validateReadRequest(req); err != nil {
		return ReadResponse{}, err
	}

	startedAt := s.now().UTC()
	runID := prefixedHash("run", req.URL, startedAt.Format(time.RFC3339Nano))
	traceID := prefixedHash("trace", runID)
	recorder := proof.NewRecorder(runID, traceID, startedAt)

	rawPage, err := s.acquire(ctx, recorder, req, startedAt)
	if err != nil {
		return ReadResponse{}, err
	}

	dom, err := s.reduce(recorder, rawPage)
	if err != nil {
		return ReadResponse{}, err
	}

	document := buildDocument(rawPage, dom.Title)

	segments, err := s.segment(recorder, dom)
	if err != nil {
		return ReadResponse{}, err
	}

	resultPack, proofRecords, err := s.pack(recorder, req, document, segments)
	if err != nil {
		return ReadResponse{}, err
	}

	trace := recorder.Finish(s.now().UTC())
	replay, err := trace.ReplayReport()
	if err != nil {
		return ReadResponse{}, err
	}
	resultPack.CostReport = buildCostReport(trace)

	response := ReadResponse{
		Document:     document,
		ResultPack:   resultPack,
		ProofRecords: proofRecords,
		Trace:        trace,
		Replay:       replay,
	}
	if err := response.Validate(); err != nil {
		return ReadResponse{}, err
	}
	return response, nil
}

func (r ReadResponse) Validate() error {
	if err := r.Document.Validate(); err != nil {
		return err
	}
	if err := r.ResultPack.Validate(); err != nil {
		return err
	}
	for i, record := range r.ProofRecords {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("proof_records[%d]: %w", i, err)
		}
	}
	if err := r.Trace.Validate(); err != nil {
		return err
	}
	return nil
}

func (s *Service) acquire(ctx context.Context, recorder *proof.Recorder, req ReadRequest, at time.Time) (pipeline.RawPage, error) {
	const stage = "acquire"
	if err := recorder.StageStarted(stage, req, at); err != nil {
		return pipeline.RawPage{}, err
	}

	page, err := s.acquirer.Acquire(ctx, pipeline.AcquireInput{
		URL:       req.URL,
		Timeout:   time.Duration(s.cfg.Runtime.TimeoutMS) * time.Millisecond,
		MaxBytes:  s.cfg.Runtime.MaxBytes,
		UserAgent: req.UserAgent,
	})
	if err != nil {
		recorder.Error(stage, "NX_FETCH_FAILED", err.Error(), nil, s.now().UTC())
		return pipeline.RawPage{}, err
	}

	if err := recorder.StageCompleted(stage, page, 1, map[string]string{
		"fetch_mode": page.FetchMode,
		"final_url":  page.FinalURL,
	}, s.now().UTC()); err != nil {
		return pipeline.RawPage{}, err
	}
	return page, nil
}

func (s *Service) reduce(recorder *proof.Recorder, rawPage pipeline.RawPage) (pipeline.SimplifiedDOM, error) {
	const stage = "reduce"
	if err := recorder.StageStarted(stage, rawPage, s.now().UTC()); err != nil {
		return pipeline.SimplifiedDOM{}, err
	}

	dom, err := s.reducer.Reduce(rawPage)
	if err != nil {
		recorder.Error(stage, "NX_REDUCE_FAILED", err.Error(), nil, s.now().UTC())
		return pipeline.SimplifiedDOM{}, err
	}

	if err := recorder.StageCompleted(stage, dom, len(dom.Nodes), map[string]string{
		"title": dom.Title,
	}, s.now().UTC()); err != nil {
		return pipeline.SimplifiedDOM{}, err
	}
	return dom, nil
}

func (s *Service) segment(recorder *proof.Recorder, dom pipeline.SimplifiedDOM) ([]pipeline.Segment, error) {
	const stage = "segment"
	if err := recorder.StageStarted(stage, dom, s.now().UTC()); err != nil {
		return nil, err
	}

	segments := s.segmenter.Segment(dom)
	if len(segments) == 0 {
		err := fmt.Errorf("no segments produced")
		recorder.Error(stage, "NX_EMPTY_SEGMENTS", err.Error(), nil, s.now().UTC())
		return nil, err
	}

	if err := recorder.StageCompleted(stage, segments, len(segments), nil, s.now().UTC()); err != nil {
		return nil, err
	}
	return segments, nil
}

func (s *Service) pack(recorder *proof.Recorder, req ReadRequest, document core.Document, segments []pipeline.Segment) (core.ResultPack, []proof.ProofRecord, error) {
	const stage = "pack"
	if err := recorder.StageStarted(stage, segments, s.now().UTC()); err != nil {
		return core.ResultPack{}, nil, err
	}

	chunks := buildChunks(document.ID, segments)
	proofRecords, err := buildProofRecords(chunks, segments)
	if err != nil {
		recorder.Error(stage, "NX_PROOF_BUILD_FAILED", err.Error(), nil, s.now().UTC())
		return core.ResultPack{}, nil, err
	}

	resultPack := core.ResultPack{
		Objective: objectiveOrDefault(req.Objective),
		Chunks:    chunks,
		Sources: []core.SourceRef{
			{
				DocumentID: document.ID,
				URL:        document.FinalURL,
				Title:      document.Title,
				ChunkIDs:   chunkIDs(chunks),
			},
		},
		ProofRefs:  proofIDs(proofRecords),
		CostReport: core.CostReport{LanePath: []int{0}},
	}

	if err := resultPack.Validate(); err != nil {
		return core.ResultPack{}, nil, err
	}

	if err := recorder.StageCompleted(stage, resultPack, len(chunks), map[string]string{
		"chunks": fmt.Sprintf("%d", len(chunks)),
	}, s.now().UTC()); err != nil {
		return core.ResultPack{}, nil, err
	}

	return resultPack, proofRecords, nil
}

func validateReadRequest(req ReadRequest) error {
	if strings.TrimSpace(req.URL) == "" {
		return fmt.Errorf("read request url must not be empty")
	}
	return nil
}

func buildDocument(page pipeline.RawPage, title string) core.Document {
	return core.Document{
		ID:        prefixedHash("doc", page.FinalURL, page.HTML),
		URL:       page.URL,
		FinalURL:  page.FinalURL,
		Title:     strings.TrimSpace(title),
		FetchedAt: page.FetchedAt,
		FetchMode: page.FetchMode,
		RawHash:   prefixedHash("sha256", page.HTML),
	}
}

func buildChunks(docID string, segments []pipeline.Segment) []core.Chunk {
	chunks := make([]core.Chunk, 0, len(segments))
	for index, segment := range segments {
		fingerprint := prefixedHash("fp", docID, strings.Join(segment.HeadingPath, "/"), segment.Text)
		score := segmentScore(segment, index, len(segments))
		confidence := segmentConfidence(segment)
		chunks = append(chunks, core.Chunk{
			ID:          prefixedHash("chk", docID, fingerprint),
			DocID:       docID,
			Text:        segment.Text,
			HeadingPath: append([]string{}, segment.HeadingPath...),
			Score:       score,
			Fingerprint: fingerprint,
			Confidence:  confidence,
		})
	}
	return chunks
}

func buildProofRecords(chunks []core.Chunk, segments []pipeline.Segment) ([]proof.ProofRecord, error) {
	if len(chunks) != len(segments) {
		return nil, fmt.Errorf("chunks and segments length mismatch")
	}

	records := make([]proof.ProofRecord, 0, len(chunks))
	for i := range chunks {
		record, err := proof.BuildProofRecord(proof.BuildInput{
			Chunk:          chunks[i],
			Segment:        segments[i],
			Lane:           0,
			TransformChain: []string{"acquire:v1", "reduce:v1", "segment:v1", "pack:v1"},
		})
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func buildCostReport(trace proof.RunTrace) core.CostReport {
	latency := int64(0)
	if !trace.FinishedAt.IsZero() {
		latency = trace.FinishedAt.Sub(trace.StartedAt).Milliseconds()
	}
	return core.CostReport{
		LatencyMS: latency,
		TokenIn:   0,
		TokenOut:  0,
		LanePath:  []int{0},
	}
}

func proofIDs(records []proof.ProofRecord) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, record.ID)
	}
	return out
}

func chunkIDs(chunks []core.Chunk) []string {
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, chunk.ID)
	}
	return out
}

func objectiveOrDefault(objective string) string {
	if strings.TrimSpace(objective) == "" {
		return "read"
	}
	return objective
}

func segmentScore(segment pipeline.Segment, index, total int) float64 {
	kindWeight := map[string]float64{
		"paragraph":  0.82,
		"list_item":  0.76,
		"table_cell": 0.72,
		"code":       0.70,
	}
	base := kindWeight[segment.Kind]
	if base == 0 {
		base = 0.65
	}
	positionBoost := 0.0
	if total > 1 {
		positionBoost = float64(total-index-1) / float64(total-1) * 0.08
	}
	if len(segment.HeadingPath) > 0 {
		base += 0.05
	}
	return clamp(base+positionBoost, 0, 1)
}

func segmentConfidence(segment pipeline.Segment) float64 {
	confidence := 0.78
	if len(segment.NodePaths) > 0 {
		confidence += 0.1
	}
	if len(segment.Text) > 40 {
		confidence += 0.07
	}
	if len(segment.HeadingPath) > 0 {
		confidence += 0.03
	}
	return clamp(confidence, 0, 0.99)
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func prefixedHash(prefix string, parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + "_" + hex.EncodeToString(digest[:])[:16]
}
