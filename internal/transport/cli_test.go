package transport

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/store"
)

func TestRunnerReadJSON(t *testing.T) {
	var captured coreservice.ReadRequest
	root := t.TempDir()
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			captured = req
			return fakeResponse(), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runner.Run([]string{"read", "https://example.com", "--json", "--profile", "tiny"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"document"`) {
		t.Fatalf("expected json document payload, got %q", stdout.String())
	}
	if captured.Profile != "tiny" {
		t.Fatalf("expected profile to be forwarded, got %q", captured.Profile)
	}
}

func TestRunnerQueryJSON(t *testing.T) {
	root := t.TempDir()
	var captured coreservice.QueryRequest
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		query: func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
			captured = req
			return fakeQueryResponse(req), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"query", "https://example.com", "--goal", "proof replay deterministic", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"result_pack"`) {
		t.Fatalf("expected query json payload, got %q", stdout.String())
	}
	if captured.Goal != "proof replay deterministic" {
		t.Fatalf("expected goal to be forwarded, got %q", captured.Goal)
	}
}

func TestRunnerReadText(t *testing.T) {
	root := t.TempDir()
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			return fakeResponse(), nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runner.Run([]string{"read", "https://example.com"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d with stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Title: Needle Runtime") {
		t.Fatalf("expected text output, got %q", stdout.String())
	}
}

func TestRunnerUnknownCommand(t *testing.T) {
	runner := NewRunner()
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runner.Run([]string{"unknown"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("expected usage exit 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("expected unknown command message, got %q", stderr.String())
	}
}

func TestRunnerReplayAndDiff(t *testing.T) {
	root := t.TempDir()
	runner := Runner{
		loadConfig: config.Load,
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			resp := fakeResponse()
			resp.Trace.TraceID = req.URL
			resp.Trace.RunID = req.URL
			resp.Trace.Stages = []proof.StageSnapshot{
				{
					Stage:       "acquire",
					StartedAt:   time.Unix(1700000000, 0).UTC(),
					CompletedAt: time.Unix(1700000001, 0).UTC(),
					OutputHash:  req.URL,
				},
			}
			return resp, nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := runner.Run([]string{"read", "trace_a"}, &stdout, &stderr); code != 0 {
		t.Fatalf("seed trace a failed: %d %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"read", "trace_b"}, &stdout, &stderr); code != 0 {
		t.Fatalf("seed trace b failed: %d %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()

	if code := runner.Run([]string{"replay", "trace_a", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("replay failed: %d %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"trace_id": "trace_a"`) {
		t.Fatalf("expected replay json, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"diff", "trace_a", "trace_b"}, &stdout, &stderr); code != 0 {
		t.Fatalf("diff failed: %d %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Changed Stages: 1") {
		t.Fatalf("expected diff output, got %q", stdout.String())
	}
}

func TestRunnerProofByTraceAndChunk(t *testing.T) {
	root := t.TempDir()
	runner := Runner{
		loadConfig: config.Load,
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			resp := fakeResponse()
			resp.Trace.TraceID = "trace_for_proof"
			return resp, nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := runner.Run([]string{"read", "https://example.com"}, &stdout, &stderr); code != 0 {
		t.Fatalf("seed proof records failed: %d %q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"proof", "trace_for_proof", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("proof by trace failed: %d %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"trace_id": "trace_for_proof"`) {
		t.Fatalf("expected trace id in output, got %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"proof", "chk_1"}, &stdout, &stderr); code != 0 {
		t.Fatalf("proof by chunk failed: %d %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Chunk ID: chk_1") {
		t.Fatalf("expected chunk proof output, got %q", stdout.String())
	}
}

func TestRunnerPruneAll(t *testing.T) {
	root := t.TempDir()
	runner := Runner{
		loadConfig: config.Load,
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			resp := fakeResponse()
			resp.Trace.TraceID = "trace_for_prune"
			return resp, nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if code := runner.Run([]string{"read", "https://example.com"}, &stdout, &stderr); code != 0 {
		t.Fatalf("seed state failed: %d %q", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"prune", "--all", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("prune failed: %d %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"removed_files": 4`) {
		t.Fatalf("expected 4 removed files, got %q", stdout.String())
	}
}

func TestRunnerReadUsesGenomeForceLane(t *testing.T) {
	root := t.TempDir()
	genomeStore := store.NewGenomeStore(root)
	if _, _, err := genomeStore.Observe(store.GenomeObservation{
		URL:          "https://example.com/docs",
		ObservedLane: 1,
	}); err != nil {
		t.Fatalf("seed genome: %v", err)
	}

	var captured coreservice.ReadRequest
	runner := Runner{
		loadConfig: func(path string) (config.Config, error) {
			return config.Defaults(), nil
		},
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			captured = req
			resp := fakeResponse()
			resp.Document.FinalURL = "https://example.com/docs"
			resp.ResultPack.CostReport.LanePath = []int{0, 1}
			return resp, nil
		},
		storeRoot: root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := runner.Run([]string{"read", "https://example.com/docs"}, &stdout, &stderr); code != 0 {
		t.Fatalf("read failed: %d %q", code, stderr.String())
	}
	if captured.ForceLane != 1 {
		t.Fatalf("expected force lane 1 from genome, got %d", captured.ForceLane)
	}
}

func fakeResponse() coreservice.ReadResponse {
	return coreservice.ReadResponse{
		Document: core.Document{
			ID:        "doc_1",
			URL:       "https://example.com",
			FinalURL:  "https://example.com",
			Title:     "Needle Runtime",
			FetchedAt: time.Unix(1700000000, 0).UTC(),
			FetchMode: core.FetchModeHTTP,
			RawHash:   "sha256_abc",
		},
		ResultPack: core.ResultPack{
			Objective: "read",
			Profile:   core.ProfileStandard,
			Chunks: []core.Chunk{
				{
					ID:          "chk_1",
					DocID:       "doc_1",
					Text:        "Compact context.",
					HeadingPath: []string{"Needle Runtime"},
					Score:       0.9,
					Fingerprint: "fp_1",
					Confidence:  0.95,
				},
			},
			Sources: []core.SourceRef{
				{
					DocumentID: "doc_1",
					URL:        "https://example.com",
					ChunkIDs:   []string{"chk_1"},
				},
			},
			ProofRefs: []string{"proof_1"},
			CostReport: core.CostReport{
				LatencyMS: 10,
				TokenIn:   0,
				TokenOut:  0,
				LanePath:  []int{0},
			},
		},
		ProofRecords: []proof.ProofRecord{
			{
				ID: "proof_1",
				Proof: core.Proof{
					ChunkID:        "chk_1",
					SourceSpan:     core.SourceSpan{Selector: "/article/p[1]", CharStart: 0, CharEnd: 16},
					TransformChain: []string{"reduce:v1", "segment:v1", "pack:v1"},
					Lane:           0,
				},
			},
		},
		Trace: proof.RunTrace{
			RunID:      "run_1",
			TraceID:    "trace_1",
			StartedAt:  time.Unix(1700000000, 0).UTC(),
			FinishedAt: time.Unix(1700000001, 0).UTC(),
			Stages: []proof.StageSnapshot{
				{Stage: "acquire", StartedAt: time.Unix(1700000000, 0).UTC(), CompletedAt: time.Unix(1700000000, 0).UTC(), OutputHash: "hash"},
			},
			Events: []proof.TraceEvent{
				{Type: proof.EventStageStarted, Stage: "acquire", Timestamp: time.Unix(1700000000, 0).UTC()},
			},
		},
		Replay: proof.ReplayReport{
			RunID:      "run_1",
			TraceID:    "trace_1",
			StageCount: 1,
			EventCount: 1,
		},
	}
}

func fakeQueryResponse(req coreservice.QueryRequest) coreservice.QueryResponse {
	read := fakeResponse()
	read.Trace.TraceID = "trace_query"
	read.ResultPack.Query = req.Goal
	return coreservice.QueryResponse{
		Plan: coreservice.QueryPlan{
			Goal:    req.Goal,
			SeedURL: req.SeedURL,
			Profile: core.ProfileStandard,
			Budget: core.Budget{
				MaxTokens:    8000,
				MaxLatencyMS: 1800,
				MaxPages:     20,
				MaxDepth:     2,
				MaxBytes:     4_000_000,
			},
			LaneMax: 3,
		},
		Document:     read.Document,
		ResultPack:   read.ResultPack,
		ProofRefs:    read.ResultPack.ProofRefs,
		ProofRecords: read.ProofRecords,
		Trace:        read.Trace,
		TraceID:      read.Trace.TraceID,
		CostReport:   read.ResultPack.CostReport,
	}
}
