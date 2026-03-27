package core

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	FetchModeHTTP   = "http"
	FetchModeRender = "render"
	MaxLane         = 4
)

type Document struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	FinalURL  string    `json:"final_url"`
	Title     string    `json:"title,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	FetchMode string    `json:"fetch_mode"`
	RawHash   string    `json:"raw_hash"`
}

type Chunk struct {
	ID          string   `json:"id"`
	DocID       string   `json:"doc_id"`
	Text        string   `json:"text"`
	HeadingPath []string `json:"heading_path,omitempty"`
	Score       float64  `json:"score"`
	Fingerprint string   `json:"fingerprint"`
	Confidence  float64  `json:"confidence"`
}

type SourceSpan struct {
	Selector  string `json:"selector"`
	CharStart int    `json:"char_start"`
	CharEnd   int    `json:"char_end"`
}

type ModelInvocation struct {
	Model     string `json:"model"`
	Purpose   string `json:"purpose"`
	TokensIn  int    `json:"tokens_in"`
	TokensOut int    `json:"tokens_out"`
	LatencyMS int64  `json:"latency_ms"`
}

type Proof struct {
	ChunkID          string            `json:"chunk_id"`
	SourceSpan       SourceSpan        `json:"source_span"`
	TransformChain   []string          `json:"transform_chain"`
	Lane             int               `json:"lane"`
	ModelInvocations []ModelInvocation `json:"model_invocations,omitempty"`
	RiskFlags        []string          `json:"risk_flags,omitempty"`
}

type SourceRef struct {
	DocumentID string   `json:"document_id"`
	URL        string   `json:"url"`
	Title      string   `json:"title,omitempty"`
	ChunkIDs   []string `json:"chunk_ids,omitempty"`
}

type CostReport struct {
	LatencyMS int64 `json:"latency_ms"`
	TokenIn   int   `json:"token_in"`
	TokenOut  int   `json:"token_out"`
	LanePath  []int `json:"lane_path"`
}

type ResultPack struct {
	Query      string      `json:"query,omitempty"`
	Objective  string      `json:"objective,omitempty"`
	Chunks     []Chunk     `json:"chunks"`
	Sources    []SourceRef `json:"sources"`
	ProofRefs  []string    `json:"proof_refs,omitempty"`
	CostReport CostReport  `json:"cost_report"`
}

type Budget struct {
	MaxTokens    int   `json:"max_tokens"`
	MaxLatencyMS int64 `json:"max_latency_ms"`
	MaxPages     int   `json:"max_pages"`
	MaxDepth     int   `json:"max_depth"`
	MaxBytes     int64 `json:"max_bytes"`
}

type RunContext struct {
	RunID       string    `json:"run_id"`
	TraceID     string    `json:"trace_id"`
	Objective   string    `json:"objective,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	LaneMax     int       `json:"lane_max"`
	Budget      Budget    `json:"budget"`
	DomainHints []string  `json:"domain_hints,omitempty"`
}

func (d Document) Validate() error {
	errs := []error{
		requireNonEmpty("document.id", d.ID),
		requireNonEmpty("document.url", d.URL),
		requireNonEmpty("document.final_url", d.FinalURL),
		requireNonEmpty("document.raw_hash", d.RawHash),
		validateFetchMode(d.FetchMode),
	}
	if d.FetchedAt.IsZero() {
		errs = append(errs, fmt.Errorf("document.fetched_at is required"))
	}
	return joinErrors(errs...)
}

func (c Chunk) Validate() error {
	errs := []error{
		requireNonEmpty("chunk.id", c.ID),
		requireNonEmpty("chunk.doc_id", c.DocID),
		requireNonEmpty("chunk.text", c.Text),
		requireNonEmpty("chunk.fingerprint", c.Fingerprint),
		requireFinite("chunk.score", c.Score),
		validateUnitInterval("chunk.confidence", c.Confidence),
	}
	for i, part := range c.HeadingPath {
		if strings.TrimSpace(part) == "" {
			errs = append(errs, fmt.Errorf("chunk.heading_path[%d] must not be empty", i))
		}
	}
	return joinErrors(errs...)
}

func (s SourceSpan) Validate() error {
	errs := []error{
		requireNonEmpty("proof.source_span.selector", s.Selector),
	}
	if s.CharStart < 0 {
		errs = append(errs, fmt.Errorf("proof.source_span.char_start must be >= 0"))
	}
	if s.CharEnd <= s.CharStart {
		errs = append(errs, fmt.Errorf("proof.source_span.char_end must be > char_start"))
	}
	return joinErrors(errs...)
}

func (m ModelInvocation) Validate() error {
	errs := []error{
		requireNonEmpty("proof.model_invocations.model", m.Model),
		requireNonEmpty("proof.model_invocations.purpose", m.Purpose),
	}
	if m.TokensIn < 0 {
		errs = append(errs, fmt.Errorf("proof.model_invocations.tokens_in must be >= 0"))
	}
	if m.TokensOut < 0 {
		errs = append(errs, fmt.Errorf("proof.model_invocations.tokens_out must be >= 0"))
	}
	if m.LatencyMS < 0 {
		errs = append(errs, fmt.Errorf("proof.model_invocations.latency_ms must be >= 0"))
	}
	return joinErrors(errs...)
}

func (p Proof) Validate() error {
	errs := []error{
		requireNonEmpty("proof.chunk_id", p.ChunkID),
		p.SourceSpan.Validate(),
	}
	if err := validateLane("proof.lane", p.Lane); err != nil {
		errs = append(errs, err)
	}
	if len(p.TransformChain) == 0 {
		errs = append(errs, fmt.Errorf("proof.transform_chain must not be empty"))
	}
	for i, step := range p.TransformChain {
		if strings.TrimSpace(step) == "" {
			errs = append(errs, fmt.Errorf("proof.transform_chain[%d] must not be empty", i))
		}
	}
	for i, invocation := range p.ModelInvocations {
		if err := invocation.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("proof.model_invocations[%d]: %w", i, err))
		}
	}
	for i, flag := range p.RiskFlags {
		if strings.TrimSpace(flag) == "" {
			errs = append(errs, fmt.Errorf("proof.risk_flags[%d] must not be empty", i))
		}
	}
	return joinErrors(errs...)
}

func (s SourceRef) Validate() error {
	errs := []error{
		requireNonEmpty("result_pack.sources.document_id", s.DocumentID),
		requireNonEmpty("result_pack.sources.url", s.URL),
	}
	for i, chunkID := range s.ChunkIDs {
		if strings.TrimSpace(chunkID) == "" {
			errs = append(errs, fmt.Errorf("result_pack.sources.chunk_ids[%d] must not be empty", i))
		}
	}
	return joinErrors(errs...)
}

func (c CostReport) Validate() error {
	errs := []error{}
	if c.LatencyMS < 0 {
		errs = append(errs, fmt.Errorf("result_pack.cost_report.latency_ms must be >= 0"))
	}
	if c.TokenIn < 0 {
		errs = append(errs, fmt.Errorf("result_pack.cost_report.token_in must be >= 0"))
	}
	if c.TokenOut < 0 {
		errs = append(errs, fmt.Errorf("result_pack.cost_report.token_out must be >= 0"))
	}
	if len(c.LanePath) == 0 {
		errs = append(errs, fmt.Errorf("result_pack.cost_report.lane_path must not be empty"))
	}
	for i, lane := range c.LanePath {
		if err := validateLane(fmt.Sprintf("result_pack.cost_report.lane_path[%d]", i), lane); err != nil {
			errs = append(errs, err)
		}
	}
	return joinErrors(errs...)
}

func (r ResultPack) Validate() error {
	errs := []error{r.CostReport.Validate()}
	if strings.TrimSpace(r.Query) == "" && strings.TrimSpace(r.Objective) == "" {
		errs = append(errs, fmt.Errorf("result_pack requires query or objective"))
	}
	if len(r.Chunks) == 0 {
		errs = append(errs, fmt.Errorf("result_pack.chunks must not be empty"))
	}
	if len(r.Sources) == 0 {
		errs = append(errs, fmt.Errorf("result_pack.sources must not be empty"))
	}
	for i, chunk := range r.Chunks {
		if err := chunk.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("result_pack.chunks[%d]: %w", i, err))
		}
	}
	for i, source := range r.Sources {
		if err := source.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("result_pack.sources[%d]: %w", i, err))
		}
	}
	for i, ref := range r.ProofRefs {
		if strings.TrimSpace(ref) == "" {
			errs = append(errs, fmt.Errorf("result_pack.proof_refs[%d] must not be empty", i))
		}
	}
	return joinErrors(errs...)
}

func (b Budget) Validate() error {
	errs := []error{}
	if b.MaxTokens <= 0 {
		errs = append(errs, fmt.Errorf("run_context.budget.max_tokens must be > 0"))
	}
	if b.MaxLatencyMS <= 0 {
		errs = append(errs, fmt.Errorf("run_context.budget.max_latency_ms must be > 0"))
	}
	if b.MaxPages <= 0 {
		errs = append(errs, fmt.Errorf("run_context.budget.max_pages must be > 0"))
	}
	if b.MaxDepth <= 0 {
		errs = append(errs, fmt.Errorf("run_context.budget.max_depth must be > 0"))
	}
	if b.MaxBytes <= 0 {
		errs = append(errs, fmt.Errorf("run_context.budget.max_bytes must be > 0"))
	}
	return joinErrors(errs...)
}

func (r RunContext) Validate() error {
	errs := []error{
		requireNonEmpty("run_context.run_id", r.RunID),
		requireNonEmpty("run_context.trace_id", r.TraceID),
		r.Budget.Validate(),
	}
	if r.StartedAt.IsZero() {
		errs = append(errs, fmt.Errorf("run_context.started_at is required"))
	}
	if err := validateLane("run_context.lane_max", r.LaneMax); err != nil {
		errs = append(errs, err)
	}
	for i, hint := range r.DomainHints {
		if strings.TrimSpace(hint) == "" {
			errs = append(errs, fmt.Errorf("run_context.domain_hints[%d] must not be empty", i))
		}
	}
	return joinErrors(errs...)
}

func requireNonEmpty(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	return nil
}

func validateFetchMode(mode string) error {
	switch mode {
	case FetchModeHTTP, FetchModeRender:
		return nil
	default:
		return fmt.Errorf("document.fetch_mode must be one of %q or %q", FetchModeHTTP, FetchModeRender)
	}
}

func validateLane(field string, lane int) error {
	if lane < 0 || lane > MaxLane {
		return fmt.Errorf("%s must be between 0 and %d", field, MaxLane)
	}
	return nil
}

func validateUnitInterval(field string, value float64) error {
	if err := requireFinite(field, value); err != nil {
		return err
	}
	if value < 0 || value > 1 {
		return fmt.Errorf("%s must be between 0 and 1", field)
	}
	return nil
}

func requireFinite(field string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%s must be finite", field)
	}
	return nil
}

func joinErrors(errs ...error) error {
	filtered := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return errors.Join(filtered...)
}
