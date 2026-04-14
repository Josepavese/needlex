package analytics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/memory"
	"github.com/josepavese/needlex/internal/proof"
)

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
	rawFetchChars, rawFetchBytes := traceAcquireMetrics(trace)
	reducedChars, reducedNodes := traceReduceMetrics(trace)
	stages := stageEventsFromTrace(trace)
	selected := strings.TrimSpace(documentURL)
	if strings.TrimSpace(selectedURL) != "" {
		selected = strings.TrimSpace(selectedURL)
	}
	finalContextChars := 0
	for _, chunk := range pack.Chunks {
		finalContextChars += len(chunk.Text)
	}
	reasons := topMetadata(trace, "discover_web", "selected_reason_json")
	_ = reasons
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
		LocalMemoryUsed:      usesLocalMemory(provider),
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

func traceAcquireMetrics(trace proof.RunTrace) (int, int) {
	for _, stage := range trace.Stages {
		if stage.Stage != "acquire" {
			continue
		}
		return atoi(stage.Metadata["raw_chars"]), atoi(stage.Metadata["raw_bytes"])
	}
	return 0, 0
}

func traceReduceMetrics(trace proof.RunTrace) (int, int) {
	for _, stage := range trace.Stages {
		if stage.Stage != "reduce" {
			continue
		}
		return atoi(stage.Metadata["reduced_chars"]), atoi(stage.Metadata["reduced_nodes"])
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

func usesLocalMemory(provider string) bool {
	return strings.Contains(strings.TrimSpace(provider), "discovery_memory")
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

func topMetadata(trace proof.RunTrace, stageName, key string) string {
	for _, stage := range trace.Stages {
		if stage.Stage == stageName {
			return stage.Metadata[key]
		}
	}
	return ""
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
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	var value int
	_, _ = fmt.Sscanf(strings.TrimSpace(raw), "%d", &value)
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
