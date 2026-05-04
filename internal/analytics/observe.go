package analytics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/core/failure"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/memory"
	"github.com/josepavese/needlex/internal/proof"
)

type FailureObservation struct {
	Operation     string
	Surface       string
	Profile       string
	Goal          string
	SeedURL       string
	URL           string
	DiscoveryMode string
	Provider      string
	StartedAt     time.Time
	Err           error
	MemoryStats   memory.Stats
}

func ObserveRead(ctx context.Context, store SQLiteStore, surface string, req coreservice.ReadRequest, resp coreservice.ReadResponse, packetBytes int, memoryStats memory.Stats) error {
	run, stages := buildRunRecord("read", surface, strings.TrimSpace(req.Objective), "", req.Profile, "", false, "", resp.Trace, packetBytes, resp.Document.FinalURL, "", resp.ResultPack, len(resp.AgentContext.Candidates), memoryStats)
	return store.AppendRun(ctx, run, stages)
}

func ObserveQuery(ctx context.Context, store SQLiteStore, surface string, req coreservice.QueryRequest, resp coreservice.QueryResponse, packetBytes int, memoryStats memory.Stats) error {
	run, stages := buildRunRecord("query", surface, req.Goal, req.SeedURL, req.Profile, resp.Plan.DiscoveryMode, strings.TrimSpace(req.SeedURL) != "", resp.Plan.DiscoveryProvider, resp.Trace, packetBytes, resp.Document.FinalURL, resp.Plan.SelectedURL, resp.ResultPack, len(resp.AgentContext.Candidates), memoryStats)
	return store.AppendRun(ctx, run, stages)
}

func ObserveCrawl(ctx context.Context, store SQLiteStore, surface string, req coreservice.CrawlRequest, resp coreservice.CrawlResponse, packetBytes int, memoryStats memory.Stats) error {
	startedAt, completedAt := crawlWindow(resp)
	trace := proof.RunTrace{
		RunID:      prefixedHash("analytics", req.SeedURL, "crawl"),
		TraceID:    prefixedHash("analytics_trace", req.SeedURL, "crawl"),
		StartedAt:  startedAt,
		FinishedAt: completedAt,
	}
	run := RunRecord{
		RunID:                trace.RunID,
		StartedAt:            trace.StartedAt,
		CompletedAt:          trace.FinishedAt,
		Operation:            "crawl",
		Surface:              firstNonEmpty(surface, "cli"),
		Profile:              req.Profile,
		GoalHash:             prefixedHash("goal", req.SeedURL),
		GoalLengthChars:      len(strings.TrimSpace(req.SeedURL)),
		DiscoveryMode:        boolString(req.SameDomain, "same_site_links", "web_search"),
		SeedPresent:          true,
		SelectedURL:          strings.TrimSpace(req.SeedURL),
		Provider:             "crawl",
		Success:              true,
		TraceID:              trace.TraceID,
		LatencyMS:            completedAt.Sub(startedAt).Milliseconds(),
		PacketBytes:          packetBytes,
		FinalContextChars:    0,
		ChunkCount:           0,
		SourceCount:          len(resp.Documents),
		LinkCount:            0,
		ProofRefCount:        0,
		ProofUsable:          false,
		PublicBootstrapUsed:  false,
		LocalMemoryUsed:      false,
		TopicNodeUsed:        false,
		SameSiteRecoveryUsed: req.SameDomain,
		CandidateCount:       0,
		RawFetchChars:        0,
		RawFetchBytes:        0,
		ReducedChars:         0,
		ReducedNodeCount:     0,
		MemoryDocumentCount:  memoryStats.DocumentCount,
		MemoryEmbeddingCount: memoryStats.EmbeddingCount,
		MemoryTopicNodeCount: memoryStats.TopicNodeCount,
	}
	return store.AppendRun(ctx, run, nil)
}

func ObserveFailure(ctx context.Context, store SQLiteStore, obs FailureObservation) error {
	completedAt := time.Now().UTC()
	startedAt := obs.StartedAt
	if startedAt.IsZero() {
		startedAt = completedAt
	}
	if completedAt.Before(startedAt) {
		completedAt = startedAt
	}
	errText := failureMessage(obs.Err)
	failureClass := failure.Classify(obs.Err).String()
	operation := firstNonEmpty(obs.Operation, "unknown")
	selectedURL := firstNonEmpty(obs.URL, obs.SeedURL)
	runID := prefixedHash("analytics_failure", operation, obs.Surface, obs.Goal, obs.SeedURL, obs.URL, completedAt.Format(time.RFC3339Nano), errText)
	run := RunRecord{
		RunID:                runID,
		StartedAt:            startedAt,
		CompletedAt:          completedAt,
		Operation:            operation,
		Surface:              firstNonEmpty(obs.Surface, "cli"),
		Profile:              firstNonEmpty(obs.Profile, "standard"),
		GoalHash:             prefixedHash("goal", firstNonEmpty(obs.Goal, obs.SeedURL, obs.URL)),
		GoalLengthChars:      len(strings.TrimSpace(firstNonEmpty(obs.Goal, obs.SeedURL, obs.URL))),
		DiscoveryMode:        firstNonEmpty(obs.DiscoveryMode, "off"),
		SeedPresent:          strings.TrimSpace(obs.SeedURL) != "",
		Host:                 hostFromURL(selectedURL),
		SelectedURL:          selectedURL,
		Provider:             firstNonEmpty(obs.Provider, "error:"+failureClass),
		FailureClass:         failureClass,
		Success:              false,
		TraceID:              prefixedHash("analytics_failure_trace", runID),
		LatencyMS:            completedAt.Sub(startedAt).Milliseconds(),
		MemoryDocumentCount:  obs.MemoryStats.DocumentCount,
		MemoryEmbeddingCount: obs.MemoryStats.EmbeddingCount,
		MemoryTopicNodeCount: obs.MemoryStats.TopicNodeCount,
	}
	stages := []StageEvent{{
		RunID:       run.RunID,
		Stage:       "runtime.error",
		StartedAt:   startedAt,
		CompletedAt: completedAt,
		LatencyMS:   completedAt.Sub(startedAt).Milliseconds(),
		Status:      "failed",
		Metadata: map[string]string{
			"failure_class": failureClass,
			"error":         truncate(errText, 512),
		},
	}}
	return store.AppendRun(ctx, run, stages)
}

func crawlWindow(resp coreservice.CrawlResponse) (time.Time, time.Time) {
	var startedAt time.Time
	var completedAt time.Time
	for _, page := range resp.Pages {
		if page.Trace.StartedAt.IsZero() {
			continue
		}
		if startedAt.IsZero() || page.Trace.StartedAt.Before(startedAt) {
			startedAt = page.Trace.StartedAt
		}
		finishedAt := page.Trace.FinishedAt
		if finishedAt.IsZero() {
			finishedAt = page.Trace.StartedAt
		}
		if completedAt.IsZero() || finishedAt.After(completedAt) {
			completedAt = finishedAt
		}
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	if completedAt.IsZero() || completedAt.Before(startedAt) {
		completedAt = startedAt
	}
	return startedAt, completedAt
}

func buildRunRecord(operation, surface, goal, seedURL, profile, discoveryMode string, seedPresent bool, provider string, trace proof.RunTrace, packetBytes int, documentURL, selectedURL string, pack core.ResultPack, candidateCount int, memoryStats memory.Stats) (RunRecord, []StageEvent) {
	rawFetchChars, rawFetchBytes := traceMetrics(trace, "acquire", "raw_chars", "raw_bytes")
	reducedChars, reducedNodes := traceMetrics(trace, "reduce", "reduced_chars", "reduced_nodes")
	stages := stageEventsFromTrace(trace)
	selected := strings.TrimSpace(documentURL)
	if strings.TrimSpace(selectedURL) != "" {
		selected = strings.TrimSpace(selectedURL)
	}
	finalContextChars := 0
	for _, chunk := range pack.Chunks {
		finalContextChars += len(chunk.Text)
	}
	return RunRecord{
		RunID:                trace.RunID,
		StartedAt:            trace.StartedAt,
		CompletedAt:          trace.FinishedAt,
		Operation:            operation,
		Surface:              firstNonEmpty(surface, "cli"),
		Profile:              firstNonEmpty(pack.Profile, profile),
		GoalHash:             prefixedHash("goal", goal),
		GoalLengthChars:      len(strings.TrimSpace(goal)),
		DiscoveryMode:        firstNonEmpty(discoveryMode, "off"),
		SeedPresent:          seedPresent,
		Host:                 hostFromURL(firstNonEmpty(selected, documentURL)),
		SelectedURL:          firstNonEmpty(selected, documentURL),
		Provider:             strings.TrimSpace(provider),
		Success:              true,
		TraceID:              trace.TraceID,
		LatencyMS:            pack.CostReport.LatencyMS,
		PacketBytes:          packetBytes,
		FinalContextChars:    finalContextChars,
		ChunkCount:           len(pack.Chunks),
		SourceCount:          len(pack.Sources),
		LinkCount:            len(pack.Links),
		ProofRefCount:        len(pack.ProofRefs),
		ProofUsable:          len(pack.ProofRefs) > 0,
		PublicBootstrapUsed:  usesPublicBootstrap(provider),
		LocalMemoryUsed:      strings.Contains(strings.TrimSpace(provider), "discovery_memory"),
		TopicNodeUsed:        traceReasonPresent(trace, "topic_node_retrieval"),
		SameSiteRecoveryUsed: strings.Contains(provider, "same_site"),
		CandidateCount:       candidateCount,
		RawFetchChars:        rawFetchChars,
		RawFetchBytes:        rawFetchBytes,
		ReducedChars:         reducedChars,
		ReducedNodeCount:     reducedNodes,
		MemoryDocumentCount:  memoryStats.DocumentCount,
		MemoryEmbeddingCount: memoryStats.EmbeddingCount,
		MemoryTopicNodeCount: memoryStats.TopicNodeCount,
	}, stages
}

func stageEventsFromTrace(trace proof.RunTrace) []StageEvent {
	out := make([]StageEvent, 0, len(trace.Stages))
	for _, stage := range trace.Stages {
		status := "completed"
		if stage.CompletedAt.IsZero() {
			status = "started"
		}
		out = append(out, StageEvent{
			RunID:       trace.RunID,
			Stage:       stage.Stage,
			StartedAt:   stage.StartedAt,
			CompletedAt: stage.CompletedAt,
			LatencyMS:   stage.CompletedAt.Sub(stage.StartedAt).Milliseconds(),
			ItemCount:   stage.ItemCount,
			Status:      status,
			Metadata:    cloneMetadata(stage.Metadata),
		})
	}
	return out
}

func traceMetrics(trace proof.RunTrace, stageName, firstKey, secondKey string) (int, int) {
	for _, stage := range trace.Stages {
		if stage.Stage != stageName {
			continue
		}
		return atoi(stage.Metadata[firstKey]), atoi(stage.Metadata[secondKey])
	}
	return 0, 0
}

func usesPublicBootstrap(provider string) bool {
	for _, part := range strings.Split(strings.TrimSpace(provider), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.HasPrefix(part, "local_") && !strings.HasPrefix(part, "discovery_memory") {
			return true
		}
	}
	return false
}

func traceReasonPresent(trace proof.RunTrace, needle string) bool {
	for _, stage := range trace.Stages {
		for key, value := range stage.Metadata {
			if strings.Contains(key, "reason") && strings.Contains(value, needle) {
				return true
			}
		}
	}
	return false
}

func cloneMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func prefixedHash(prefix string, parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return prefix + "_" + hex.EncodeToString(sum[:8])
}

func atoi(raw string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(raw))
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func boolString(flag bool, yes, no string) string {
	if flag {
		return yes
	}
	return no
}

func failureMessage(err error) string {
	if err == nil {
		return "unknown error"
	}
	return strings.TrimSpace(err.Error())
}

func truncate(value string, max int) string {
	if max > 0 && len(value) > max {
		return value[:max]
	}
	return value
}
