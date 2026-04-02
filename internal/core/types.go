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
	FetchModeReq    = "req"
	FetchModeRender = "render"
	MaxLane         = 4
	ProfileTiny     = "tiny"
	ProfileStandard = "standard"
	ProfileDeep     = "deep"
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
	ID               string   `json:"id"`
	DocID            string   `json:"doc_id"`
	Text             string   `json:"text"`
	HeadingPath      []string `json:"heading_path,omitempty"`
	ContextAlignment float64  `json:"context_alignment,omitempty"`
	Score            float64  `json:"score"`
	Fingerprint      string   `json:"fingerprint"`
	Confidence       float64  `json:"confidence"`
}

type SourceSpan struct {
	Selector  string `json:"selector"`
	CharStart int    `json:"char_start"`
	CharEnd   int    `json:"char_end"`
}

type ModelInvocation struct {
	Model            string   `json:"model"`
	Backend          string   `json:"backend,omitempty"`
	Task             string   `json:"task,omitempty"`
	Purpose          string   `json:"purpose"`
	TokensIn         int      `json:"tokens_in"`
	TokensOut        int      `json:"tokens_out"`
	LatencyMS        int64    `json:"latency_ms"`
	PatchID          string   `json:"patch_id,omitempty"`
	PatchEffect      string   `json:"patch_effect,omitempty"`
	PatchPreview     string   `json:"patch_preview,omitempty"`
	AffectedChunkIDs []string `json:"affected_chunk_ids,omitempty"`
	ValidatorOutcome string   `json:"validator_outcome,omitempty"`
	ValidatorMessage string   `json:"validator_message,omitempty"`
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
	Profile    string      `json:"profile,omitempty"`
	Chunks     []Chunk     `json:"chunks"`
	Sources    []SourceRef `json:"sources"`
	Outline    []string    `json:"outline,omitempty"`
	Links      []string    `json:"links,omitempty"`
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

func (d Document) Validate() error {
	errs := []error{
		RequireNonEmpty("document.id", d.ID),
		RequireNonEmpty("document.url", d.URL),
		RequireNonEmpty("document.final_url", d.FinalURL),
		RequireNonEmpty("document.raw_hash", d.RawHash),
		validateFetchMode(d.FetchMode),
	}
	if d.FetchedAt.IsZero() {
		errs = append(errs, fmt.Errorf("document.fetched_at is required"))
	}
	return JoinErrors(errs...)
}

func (c Chunk) Validate() error {
	errs := []error{
		RequireNonEmpty("chunk.id", c.ID),
		RequireNonEmpty("chunk.doc_id", c.DocID),
		RequireNonEmpty("chunk.text", c.Text),
		RequireNonEmpty("chunk.fingerprint", c.Fingerprint),
		validateUnitInterval("chunk.context_alignment", c.ContextAlignment),
		requireFinite("chunk.score", c.Score),
		validateUnitInterval("chunk.confidence", c.Confidence),
	}
	errs = append(errs, ValidateStringSlice("chunk.heading_path", c.HeadingPath)...)
	return JoinErrors(errs...)
}

func (s SourceSpan) Validate() error {
	errs := []error{
		RequireNonEmpty("proof.source_span.selector", s.Selector),
	}
	if s.CharStart < 0 {
		errs = append(errs, fmt.Errorf("proof.source_span.char_start must be >= 0"))
	}
	if s.CharEnd <= s.CharStart {
		errs = append(errs, fmt.Errorf("proof.source_span.char_end must be > char_start"))
	}
	return JoinErrors(errs...)
}

func (m ModelInvocation) Validate() error {
	errs := []error{
		RequireNonEmpty("proof.model_invocations.model", m.Model),
		RequireNonEmpty("proof.model_invocations.purpose", m.Purpose),
		RequireNonNegative("proof.model_invocations.tokens_in", m.TokensIn),
		RequireNonNegative("proof.model_invocations.tokens_out", m.TokensOut),
		RequireNonNegative("proof.model_invocations.latency_ms", m.LatencyMS),
	}
	errs = append(errs, ValidateStringSlice("proof.model_invocations.affected_chunk_ids", m.AffectedChunkIDs)...)
	return JoinErrors(errs...)
}

func (p Proof) Validate() error {
	errs := []error{
		RequireNonEmpty("proof.chunk_id", p.ChunkID),
		p.SourceSpan.Validate(),
	}
	if err := ValidateLane("proof.lane", p.Lane); err != nil {
		errs = append(errs, err)
	}
	if len(p.TransformChain) == 0 {
		errs = append(errs, fmt.Errorf("proof.transform_chain must not be empty"))
	}
	errs = append(errs, ValidateStringSlice("proof.transform_chain", p.TransformChain)...)
	errs = append(errs, ValidateIndexed("proof.model_invocations", p.ModelInvocations, func(invocation ModelInvocation) error { return invocation.Validate() })...)
	errs = append(errs, ValidateStringSlice("proof.risk_flags", p.RiskFlags)...)
	return JoinErrors(errs...)
}

func (s SourceRef) Validate() error {
	errs := []error{
		RequireNonEmpty("result_pack.sources.document_id", s.DocumentID),
		RequireNonEmpty("result_pack.sources.url", s.URL),
	}
	errs = append(errs, ValidateStringSlice("result_pack.sources.chunk_ids", s.ChunkIDs)...)
	return JoinErrors(errs...)
}

func (c CostReport) Validate() error {
	errs := []error{
		RequireNonNegative("result_pack.cost_report.latency_ms", c.LatencyMS),
		RequireNonNegative("result_pack.cost_report.token_in", c.TokenIn),
		RequireNonNegative("result_pack.cost_report.token_out", c.TokenOut),
	}
	if len(c.LanePath) == 0 {
		errs = append(errs, fmt.Errorf("result_pack.cost_report.lane_path must not be empty"))
	}
	errs = append(errs, ValidateIndexed("result_pack.cost_report.lane_path", c.LanePath, func(lane int) error { return ValidateLane("", lane) })...)
	return JoinErrors(errs...)
}

func (r ResultPack) Validate() error {
	errs := []error{r.CostReport.Validate()}
	if strings.TrimSpace(r.Query) == "" && strings.TrimSpace(r.Objective) == "" {
		errs = append(errs, fmt.Errorf("result_pack requires query or objective"))
	}
	if r.Profile != "" {
		if err := validateProfile("result_pack.profile", r.Profile); err != nil {
			errs = append(errs, err)
		}
	}
	if len(r.Chunks) == 0 {
		errs = append(errs, fmt.Errorf("result_pack.chunks must not be empty"))
	}
	if len(r.Sources) == 0 {
		errs = append(errs, fmt.Errorf("result_pack.sources must not be empty"))
	}
	errs = append(errs,
		ValidateIndexed("result_pack.chunks", r.Chunks, func(chunk Chunk) error { return chunk.Validate() })...,
	)
	errs = append(errs,
		ValidateIndexed("result_pack.sources", r.Sources, func(source SourceRef) error { return source.Validate() })...,
	)
	errs = append(errs,
		ValidateStringSlice("result_pack.proof_refs", r.ProofRefs)...,
	)
	errs = append(errs,
		ValidateStringSlice("result_pack.outline", r.Outline)...,
	)
	errs = append(errs,
		ValidateStringSlice("result_pack.links", r.Links)...,
	)
	return JoinErrors(errs...)
}

func RequireNonEmpty(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	return nil
}

func validateFetchMode(mode string) error {
	switch mode {
	case FetchModeHTTP, FetchModeReq, FetchModeRender:
		return nil
	default:
		return fmt.Errorf("document.fetch_mode must be one of %q, %q, %q", FetchModeHTTP, FetchModeReq, FetchModeRender)
	}
}

func ValidateLane(field string, lane int) error {
	if lane < 0 || lane > MaxLane {
		if field == "" {
			return fmt.Errorf("lane must be between 0 and %d", MaxLane)
		}
		return fmt.Errorf("%s must be between 0 and %d", field, MaxLane)
	}
	return nil
}

func validateProfile(field, profile string) error {
	switch profile {
	case ProfileTiny, ProfileStandard, ProfileDeep:
		return nil
	default:
		return fmt.Errorf("%s must be one of %q, %q, or %q", field, ProfileTiny, ProfileStandard, ProfileDeep)
	}
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

func JoinErrors(errs ...error) error {
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

func ValidateStringSlice(field string, values []string) []error {
	return ValidateIndexed(field, values, func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("must not be empty")
		}
		return nil
	})
}

func RequireNonNegative[T ~int | ~int64](field string, value T) error {
	if value < 0 {
		return fmt.Errorf("%s must be >= 0", field)
	}
	return nil
}

func ValidateIndexed[T any](field string, values []T, validate func(T) error) []error {
	errs := make([]error, 0, len(values))
	for i, value := range values {
		if err := validate(value); err != nil {
			errs = append(errs, fmt.Errorf("%s[%d] %w", field, i, err))
		}
	}
	return errs
}
