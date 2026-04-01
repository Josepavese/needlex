package proof

import (
	"encoding/json"
	"fmt"
	"time"
)

type Recorder struct {
	trace  RunTrace
	active map[string]int
}

func NewRecorder(runID, traceID string, startedAt time.Time) *Recorder {
	return &Recorder{
		trace: RunTrace{
			RunID:     runID,
			TraceID:   traceID,
			StartedAt: startedAt.UTC(),
			Stages:    []StageSnapshot{},
			Events:    []TraceEvent{},
		},
		active: map[string]int{},
	}
}

func (r *Recorder) StageStarted(stage string, input any, at time.Time) error {
	if _, exists := r.active[stage]; exists {
		return fmt.Errorf("stage %q already active", stage)
	}
	snapshot := StageSnapshot{
		Stage:     stage,
		StartedAt: at.UTC(),
		InputHash: payloadHash(input),
	}
	r.trace.Stages = append(r.trace.Stages, snapshot)
	r.active[stage] = len(r.trace.Stages) - 1
	r.trace.Events = append(r.trace.Events, TraceEvent{
		Type:      EventStageStarted,
		Stage:     stage,
		Timestamp: at.UTC(),
	})
	return nil
}

func (r *Recorder) StageCompleted(stage string, output any, itemCount int, metadata map[string]string, at time.Time) error {
	index, ok := r.active[stage]
	if !ok {
		return fmt.Errorf("stage %q not active", stage)
	}
	snapshot := r.trace.Stages[index]
	snapshot.CompletedAt = at.UTC()
	snapshot.OutputHash = payloadHash(output)
	snapshot.ItemCount = itemCount
	snapshot.Metadata = cloneMap(metadata)
	r.trace.Stages[index] = snapshot
	delete(r.active, stage)

	r.trace.Events = append(r.trace.Events, TraceEvent{
		Type:      EventStageCompleted,
		Stage:     stage,
		Timestamp: at.UTC(),
		Data:      cloneMap(metadata),
	})
	return nil
}

func (r *Recorder) EscalationTriggered(stage, reasonCode, message string, lane int, data map[string]string, at time.Time) {
	r.trace.Events = append(r.trace.Events, TraceEvent{
		Type:       EventEscalationTriggered,
		Stage:      stage,
		Timestamp:  at.UTC(),
		ReasonCode: reasonCode,
		Message:    message,
		Lane:       lane,
		Data:       cloneMap(data),
	})
}

func (r *Recorder) BudgetWarning(stage, reasonCode, message string, data map[string]string, at time.Time) {
	r.trace.Events = append(r.trace.Events, TraceEvent{
		Type:       EventBudgetWarning,
		Stage:      stage,
		Timestamp:  at.UTC(),
		ReasonCode: reasonCode,
		Message:    message,
		Data:       cloneMap(data),
	})
}

func (r *Recorder) ModelIntervention(stage, reasonCode, message string, lane int, data map[string]string, at time.Time) {
	r.trace.Events = append(r.trace.Events, TraceEvent{
		Type:       EventModelIntervention,
		Stage:      stage,
		Timestamp:  at.UTC(),
		ReasonCode: reasonCode,
		Message:    message,
		Lane:       lane,
		Data:       cloneMap(data),
	})
}

func (r *Recorder) Error(stage, reasonCode, message string, data map[string]string, at time.Time) {
	r.trace.Events = append(r.trace.Events, TraceEvent{
		Type:       EventError,
		Stage:      stage,
		Timestamp:  at.UTC(),
		ReasonCode: reasonCode,
		Message:    message,
		Data:       cloneMap(data),
	})
}

func (r *Recorder) Finish(at time.Time) RunTrace {
	r.trace.FinishedAt = at.UTC()
	return r.Trace()
}

func (r *Recorder) Trace() RunTrace {
	out := r.trace
	out.Stages = append([]StageSnapshot{}, r.trace.Stages...)
	out.Events = append([]TraceEvent{}, r.trace.Events...)
	for i := range out.Stages {
		out.Stages[i].Metadata = cloneMap(out.Stages[i].Metadata)
	}
	for i := range out.Events {
		out.Events[i].Data = cloneMap(out.Events[i].Data)
	}
	return out
}

func payloadHash(value any) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return sha256Digest(json.RawMessage(data))
}

func cloneMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
