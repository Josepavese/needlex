package proof

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/pipeline"
)

const (
	EventStageStarted        = "stage_started"
	EventStageCompleted      = "stage_completed"
	EventEscalationTriggered = "escalation_triggered"
	EventBudgetWarning       = "budget_warning"
	EventModelIntervention   = "model_intervention"
	EventError               = "error"
)

type ProofRecord struct {
	ID    string     `json:"id"`
	Proof core.Proof `json:"proof"`
}

type BuildInput struct {
	Chunk            core.Chunk
	Segment          pipeline.Segment
	Lane             int
	TransformChain   []string
	ModelInvocations []core.ModelInvocation
	RiskFlags        []string
}

type TraceEvent struct {
	Type       string            `json:"type"`
	Stage      string            `json:"stage"`
	Timestamp  time.Time         `json:"timestamp"`
	ReasonCode string            `json:"reason_code,omitempty"`
	Message    string            `json:"message,omitempty"`
	Lane       int               `json:"lane,omitempty"`
	Data       map[string]string `json:"data,omitempty"`
}

type StageSnapshot struct {
	Stage       string            `json:"stage"`
	StartedAt   time.Time         `json:"started_at"`
	CompletedAt time.Time         `json:"completed_at,omitempty"`
	InputHash   string            `json:"input_hash,omitempty"`
	OutputHash  string            `json:"output_hash,omitempty"`
	ItemCount   int               `json:"item_count,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type RunTrace struct {
	RunID      string          `json:"run_id"`
	TraceID    string          `json:"trace_id"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt time.Time       `json:"finished_at,omitempty"`
	Stages     []StageSnapshot `json:"stages"`
	Events     []TraceEvent    `json:"events"`
}

type ReplayReport struct {
	RunID           string   `json:"run_id"`
	TraceID         string   `json:"trace_id"`
	StageCount      int      `json:"stage_count"`
	EventCount      int      `json:"event_count"`
	CompletedStages []string `json:"completed_stages"`
	Deterministic   bool     `json:"deterministic"`
}

type StageDiff struct {
	Stage      string `json:"stage"`
	Status     string `json:"status"`
	BeforeHash string `json:"before_hash,omitempty"`
	AfterHash  string `json:"after_hash,omitempty"`
}

type DiffReport struct {
	TraceA        string      `json:"trace_a"`
	TraceB        string      `json:"trace_b"`
	ChangedStages []StageDiff `json:"changed_stages"`
}

func BuildProofRecord(input BuildInput) (ProofRecord, error) {
	proof := core.Proof{
		ChunkID:          input.Chunk.ID,
		SourceSpan:       buildSourceSpan(input.Segment, input.Chunk),
		TransformChain:   append([]string{}, input.TransformChain...),
		Lane:             input.Lane,
		ModelInvocations: append([]core.ModelInvocation{}, input.ModelInvocations...),
		RiskFlags:        append([]string{}, input.RiskFlags...),
	}
	if err := proof.Validate(); err != nil {
		return ProofRecord{}, err
	}

	record := ProofRecord{
		ID:    proofRecordID(proof),
		Proof: proof,
	}
	return record, record.Validate()
}

func (r ProofRecord) Validate() error {
	return core.JoinErrors(core.RequireNonEmpty("proof_record.id", r.ID), r.Proof.Validate())
}

func (e TraceEvent) Validate() error {
	errs := []error{
		core.RequireNonEmpty("trace_event.type", e.Type),
		core.RequireNonEmpty("trace_event.stage", e.Stage),
	}
	if e.Timestamp.IsZero() {
		errs = append(errs, fmt.Errorf("trace_event.timestamp is required"))
	}
	switch e.Type {
	case EventStageStarted, EventStageCompleted, EventEscalationTriggered, EventBudgetWarning, EventModelIntervention, EventError:
	default:
		errs = append(errs, fmt.Errorf("trace_event.type %q is not supported", e.Type))
	}
	if e.Lane != 0 {
		if err := core.ValidateLane("trace_event.lane", e.Lane); err != nil {
			errs = append(errs, err)
		}
	}
	errs = append(errs, validateStringMap("trace_event.data", e.Data)...)
	return core.JoinErrors(errs...)
}

func (s StageSnapshot) Validate() error {
	errs := []error{
		core.RequireNonEmpty("stage_snapshot.stage", s.Stage),
	}
	if s.StartedAt.IsZero() {
		errs = append(errs, fmt.Errorf("stage_snapshot.started_at is required"))
	}
	if !s.CompletedAt.IsZero() && s.CompletedAt.Before(s.StartedAt) {
		errs = append(errs, fmt.Errorf("stage_snapshot.completed_at must be >= started_at"))
	}
	errs = append(errs, core.RequireNonNegative("stage_snapshot.item_count", s.ItemCount))
	return core.JoinErrors(errs...)
}

func (r RunTrace) Validate() error {
	errs := []error{
		core.RequireNonEmpty("run_trace.run_id", r.RunID),
		core.RequireNonEmpty("run_trace.trace_id", r.TraceID),
	}
	if r.StartedAt.IsZero() {
		errs = append(errs, fmt.Errorf("run_trace.started_at is required"))
	}
	if !r.FinishedAt.IsZero() && r.FinishedAt.Before(r.StartedAt) {
		errs = append(errs, fmt.Errorf("run_trace.finished_at must be >= started_at"))
	}
	errs = append(errs, core.ValidateIndexed("run_trace.stages", r.Stages, func(stage StageSnapshot) error { return stage.Validate() })...)
	errs = append(errs, core.ValidateIndexed("run_trace.events", r.Events, func(event TraceEvent) error { return event.Validate() })...)
	return core.JoinErrors(errs...)
}

func (r RunTrace) ReplayReport() (ReplayReport, error) {
	if err := r.Validate(); err != nil {
		return ReplayReport{}, err
	}

	completed := make([]string, 0, len(r.Stages))
	deterministic := true
	for _, stage := range r.Stages {
		if stage.OutputHash == "" {
			deterministic = false
			continue
		}
		if stage.CompletedAt.IsZero() {
			deterministic = false
			continue
		}
		completed = append(completed, stage.Stage)
	}

	return ReplayReport{
		RunID:           r.RunID,
		TraceID:         r.TraceID,
		StageCount:      len(r.Stages),
		EventCount:      len(r.Events),
		CompletedStages: completed,
		Deterministic:   deterministic,
	}, nil
}

func Diff(a, b RunTrace) (DiffReport, error) {
	if err := a.Validate(); err != nil {
		return DiffReport{}, fmt.Errorf("trace_a invalid: %w", err)
	}
	if err := b.Validate(); err != nil {
		return DiffReport{}, fmt.Errorf("trace_b invalid: %w", err)
	}

	left := snapshotMap(a.Stages)
	right := snapshotMap(b.Stages)
	ordered := stageOrder(left, right)

	report := DiffReport{
		TraceA:        a.TraceID,
		TraceB:        b.TraceID,
		ChangedStages: []StageDiff{},
	}

	for _, stage := range ordered {
		before, okA := left[stage]
		after, okB := right[stage]

		switch {
		case !okA && okB:
			report.ChangedStages = append(report.ChangedStages, StageDiff{
				Stage:     stage,
				Status:    "added",
				AfterHash: after.OutputHash,
			})
		case okA && !okB:
			report.ChangedStages = append(report.ChangedStages, StageDiff{
				Stage:      stage,
				Status:     "removed",
				BeforeHash: before.OutputHash,
			})
		case before.OutputHash != after.OutputHash:
			report.ChangedStages = append(report.ChangedStages, StageDiff{
				Stage:      stage,
				Status:     "changed",
				BeforeHash: before.OutputHash,
				AfterHash:  after.OutputHash,
			})
		}
	}

	return report, nil
}

func buildSourceSpan(segment pipeline.Segment, chunk core.Chunk) core.SourceSpan {
	selector := ""
	if len(segment.NodePaths) > 0 {
		selector = strings.Join(segment.NodePaths, " | ")
	}
	return core.SourceSpan{
		Selector:  selector,
		CharStart: 0,
		CharEnd:   len(chunk.Text),
	}
}

func proofRecordID(proof core.Proof) string {
	digest := sha256Digest(proof)
	return "proof_" + digest[:16]
}

func sha256Digest(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func snapshotMap(stages []StageSnapshot) map[string]StageSnapshot {
	out := make(map[string]StageSnapshot, len(stages))
	for _, stage := range stages {
		out[stage.Stage] = stage
	}
	return out
}

func stageOrder(left, right map[string]StageSnapshot) []string {
	seen := map[string]struct{}{}
	ordered := make([]string, 0, len(left)+len(right))
	for _, table := range []map[string]StageSnapshot{left, right} {
		for key := range table {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			ordered = append(ordered, key)
		}
	}
	return ordered
}

func validateStringMap(field string, values map[string]string) []error {
	errs := make([]error, 0, len(values))
	for key, value := range values {
		if strings.TrimSpace(key) == "" {
			errs = append(errs, fmt.Errorf("%s key must not be empty", field))
		}
		if strings.TrimSpace(value) == "" {
			errs = append(errs, fmt.Errorf("%s[%s] must not be empty", field, key))
		}
	}
	return errs
}
