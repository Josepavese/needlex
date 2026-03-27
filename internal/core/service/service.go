package service

import (
	"context"
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
	URL            string
	Objective      string
	Profile        string
	UserAgent      string
	ForceLane      int
	PruningProfile string
	RenderHint     bool
}

type ReadResponse struct {
	Document     core.Document       `json:"document"`
	ResultPack   core.ResultPack     `json:"result_pack"`
	ProofRecords []proof.ProofRecord `json:"proof_records"`
	Trace        proof.RunTrace      `json:"trace"`
	Replay       proof.ReplayReport  `json:"replay"`
}

type Service struct {
	cfg                config.Config
	acquirer           pipeline.Acquirer
	reducer            pipeline.Reducer
	segmenter          pipeline.Segmenter
	now                func() time.Time
	webDiscoverBaseURL string
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
	var err error
	req.Profile, err = resolveProfile(req.Profile)
	if err != nil {
		return ReadResponse{}, err
	}
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

	dom, err := s.reduce(recorder, rawPage, req)
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
	resultPack.CostReport = buildCostReport(trace, resultPack.CostReport.LanePath)

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
		UserAgent: effectiveUserAgent(req.UserAgent, req.RenderHint),
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

func (s *Service) reduce(recorder *proof.Recorder, rawPage pipeline.RawPage, req ReadRequest) (pipeline.SimplifiedDOM, error) {
	const stage = "reduce"
	if err := recorder.StageStarted(stage, rawPage, s.now().UTC()); err != nil {
		return pipeline.SimplifiedDOM{}, err
	}

	dom, err := s.reducer.ReduceProfile(rawPage, req.PruningProfile)
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

func effectiveUserAgent(userAgent string, renderHint bool) string {
	if strings.TrimSpace(userAgent) != "" {
		return userAgent
	}
	if renderHint {
		return "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36"
	}
	return ""
}
