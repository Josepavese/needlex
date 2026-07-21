package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/core/fetchpolicy"
	"github.com/josepavese/needlex/internal/core/webirbuilder"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/platform"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/rendering"
	"github.com/josepavese/needlex/internal/store"
)

type ReadRequest struct {
	URL, Objective, Profile, UserAgent, PruningProfile string
	FetchProfile, FetchRetryProfile                    string
	ForceLane                                          int
	RenderHint                                         bool
	RenderMode                                         string
	AgentReadableMode                                  string
	StableFingerprints                                 []string
}

type ReadResponse struct {
	Document     core.Document       `json:"document"`
	WebIR        core.WebIR          `json:"web_ir"`
	ResultPack   core.ResultPack     `json:"result_pack"`
	AgentContext AgentContext        `json:"agent_context,omitempty"`
	ProofRecords []proof.ProofRecord `json:"proof_records"`
	Trace        proof.RunTrace      `json:"trace"`
	Replay       proof.ReplayReport  `json:"replay"`
}

type Service struct {
	cfg                config.Config
	httpClient         *http.Client
	acquirer           pipeline.Acquirer
	reducer            pipeline.Reducer
	segmenter          pipeline.Segmenter
	renderer           rendering.Renderer
	runtime            intel.ModelRuntime
	semantic           intel.SemanticAligner
	now                func() time.Time
	storeRoot          string
	webDiscoverBaseURL string
	discoveryProviders store.DiscoveryProviderStateStore
}

func New(cfg config.Config, client *http.Client) (*Service, error) {
	return NewWithStateRoot(cfg, client, platform.DefaultStateRoot())
}

func NewWithStateRoot(cfg config.Config, client *http.Client, storeRoot string) (*Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cleanRoot := strings.TrimSpace(storeRoot)
	if cleanRoot == "" {
		cleanRoot = platform.DefaultStateRoot()
	}
	return &Service{
		cfg:        cfg,
		httpClient: client,
		acquirer:   pipeline.Acquirer{Client: client},
		reducer:    pipeline.Reducer{},
		segmenter: pipeline.Segmenter{
			MaxSegmentChars: 1200,
		},
		renderer:           rendering.New(cfg.Render),
		runtime:            intel.NewRuntime(cfg, client),
		semantic:           intel.NewSemanticAlignerWithCacheDir(cfg, client, platform.NewStateLayout(cleanRoot).EmbeddingCacheDir),
		now:                time.Now,
		storeRoot:          cleanRoot,
		webDiscoverBaseURL: strings.TrimSpace(cfg.Discovery.ProviderChain),
		discoveryProviders: store.NewDiscoveryProviderStateStore(cleanRoot),
	}, nil
}

func (s *Service) SetWebDiscoverBaseURL(baseURL string) {
	s.webDiscoverBaseURL = strings.TrimSpace(baseURL)
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
	resolver := s.sourceResolver()
	resolutionReq := sourceResolutionRequest(req)
	rawPage, err = resolver.ResolveAgentReadable(ctx, recorder, resolutionReq, rawPage)
	if err != nil {
		return ReadResponse{}, err
	}

	dom, err := s.reduce(recorder, rawPage, req)
	if err != nil {
		return ReadResponse{}, err
	}
	rawPage, dom, err = resolver.MaybeRender(ctx, recorder, resolutionReq, rawPage, dom)
	if err != nil {
		return ReadResponse{}, err
	}
	dom = webirbuilder.EnsureMinimum(dom)
	webIR := webirbuilder.Build(dom)
	if err := webIR.Validate(); err != nil {
		return ReadResponse{}, err
	}

	document := buildDocument(rawPage, dom.Title)

	segments, err := s.segment(recorder, dom)
	if err != nil {
		return ReadResponse{}, err
	}

	resultPack, proofRecords, err := s.pack(ctx, recorder, req, document, dom, webIR, segments)
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
		WebIR:        webIR,
		ResultPack:   resultPack,
		AgentContext: buildAgentContext(document, resultPack, proofRecords, nil),
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
	if err := r.WebIR.Validate(); err != nil {
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

	page, err := s.acquirer.Acquire(ctx, fetchpolicy.Input(s.cfg, req.URL, pipeline.EffectiveUserAgent(req.UserAgent, req.RenderHint), req.FetchProfile, req.FetchRetryProfile, ""))
	if err != nil {
		recorder.Error(stage, "NX_FETCH_FAILED", err.Error(), nil, s.now().UTC())
		return pipeline.RawPage{}, err
	}

	effectiveRetryProfile := req.FetchRetryProfile
	if effectiveRetryProfile == "" {
		effectiveRetryProfile = s.cfg.Fetch.RetryProfile
	}
	metadata := map[string]string{
		"fetch_mode":     page.FetchMode,
		"fetch_profile":  page.FetchProfile,
		"retry_profile":  effectiveRetryProfile,
		"final_url":      page.FinalURL,
		"content_type":   page.ContentType,
		"retry_count":    fmt.Sprintf("%d", page.RetryCount),
		"retry_sleep_ms": fmt.Sprintf("%d", page.RetrySleepMS),
		"host_pacing_ms": fmt.Sprintf("%d", page.HostPacingMS),
		"raw_chars":      fmt.Sprintf("%d", len(page.HTML)),
		"raw_bytes":      fmt.Sprintf("%d", len([]byte(page.HTML))),
		"partial_body":   fmt.Sprintf("%t", page.Partial),
	}
	if page.RetryReason != "" {
		metadata["retry_reason"] = page.RetryReason
	}
	if err := recorder.StageCompleted(stage, page, 1, metadata, s.now().UTC()); err != nil {
		return pipeline.RawPage{}, err
	}
	return page, nil
}
