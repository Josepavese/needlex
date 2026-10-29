package service

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/rendering"
)

// This opt-in native harness stays in the service test package so passive
// observation can use the real contract without adding a test-only public API.
const (
	acquisitionCaseDeadline = 90 * time.Second
	acquisitionMeasurement  = "native-route-capture-v2"
)

type acquisitionCase struct {
	ID        string     `json:"id"`
	URL       string     `json:"url"`
	Objective string     `json:"objective"`
	Facts     [][]string `json:"fact_alternatives"`
	Family    string     `json:"family"`
	Partition string     `json:"partition"`
	Synthetic bool       `json:"synthetic"`
	Language  string     `json:"language"`
}

type acquisitionRoute struct {
	Action            string              `json:"action"`
	ActualAction      string              `json:"actual_action,omitempty"`
	Error             string              `json:"error,omitempty"`
	LatencyMS         int64               `json:"latency_ms"`
	SnapshotMS        int64               `json:"snapshot_ms"`
	FetchAttempts     int64               `json:"fetch_attempts"`
	BrowserHTTPEvents *int                `json:"browser_http_events"`
	BrowserUses       int                 `json:"browser_uses"`
	Complete          bool                `json:"capture_complete"`
	FactsFound        bool                `json:"facts_found"`
	TraceID           string              `json:"trace_id,omitempty"`
	TraceSHA          string              `json:"trace_sha256,omitempty"`
	Replay            *proof.ReplayReport `json:"replay,omitempty"`
	Render            []map[string]string `json:"render,omitempty"`
	Witnesses         []proof.ProofRecord `json:"witnesses,omitempty"`
}

type acquisitionReceipt struct {
	WarmupMS            int64                         `json:"warmup_ms,omitempty"`
	WarmupError         string                        `json:"warmup_error,omitempty"`
	Identity            string                        `json:"identity"`
	Case                acquisitionCase               `json:"case"`
	Input               intel.AcquisitionInput        `json:"input"`
	Routes              []acquisitionRoute            `json:"routes"`
	Observation         *intel.AcquisitionObservation `json:"observation,omitempty"`
	ObservationError    string                        `json:"observation_error,omitempty"`
	FailurePacingMS     int64                         `json:"failure_pacing_ms,omitempty"`
	ModelWallMS         int64                         `json:"model_wall_ms,omitempty"`
	ReferenceSimilarity *float64                      `json:"reference_similarity,omitempty"`
	ReferenceMS         int64                         `json:"reference_ms,omitempty"`
	ReferenceError      string                        `json:"reference_error,omitempty"`
}

type acquisitionRecorder struct{ input intel.AcquisitionInput }

func (r *acquisitionRecorder) ObserveAcquisition(_ context.Context, input intel.AcquisitionInput) (intel.AcquisitionObservation, error) {
	r.input = input
	metadata := input
	metadata.Evidence = nil
	return intel.AcquisitionObservation{Input: metadata, ObservationOnly: true, DecisionExecution: intel.DecisionExecution{Status: "snapshot_only", Specification: intel.AcquisitionSpecification}}, nil
}

func TestExportAcquisitionPolicy(t *testing.T) {
	out := os.Getenv("NEEDLEX_ACQUISITION_OUT")
	if out == "" {
		t.Skip("explicit acquisition qualification only")
	}
	cfg, err := config.Load(os.Getenv("NEEDLEX_ACQUISITION_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	phase := os.Getenv("NEEDLEX_ACQUISITION_PHASE")
	if phase == "collect" {
		collectAcquisitionPolicy(t, cfg, out)
		return
	}
	if phase != "development" && phase != "heldout" {
		t.Fatal("phase must be collect/development/heldout")
	}
	evaluateAcquisitionPolicy(t, cfg, out, phase)
}

func readAcquisitionReceipts(t *testing.T, path string) []acquisitionReceipt {
	t.Helper()
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows := []acquisitionReceipt{}
	decoder := json.NewDecoder(bufio.NewReader(file))
	for decoder.More() {
		var row acquisitionReceipt
		if err := decoder.Decode(&row); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	return rows
}

func collectAcquisitionPolicy(t *testing.T, cfg config.Config, out string) {
	t.Helper()
	corpus := filepath.Join("..", "..", "..", "benchmarks", "corpora", "acquisition-route-corpus-v1.json")
	data, err := os.ReadFile(corpus)
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Cases []acquisitionCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	cfgBytes, _ := json.Marshal(cfg)
	identity := fmt.Sprintf("%x", sha256.Sum256(append(append(data, cfgBytes...), acquisitionMeasurement...)))
	rows := readAcquisitionReceipts(t, out)
	done := map[string]bool{}
	for _, row := range rows {
		if row.Identity != identity {
			t.Fatal("resume inputs changed")
		}
		done[row.Case.ID] = true
	}
	file, err := os.OpenFile(out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	fixture := httptest.NewServer(http.HandlerFunc(acquisitionFixture))
	defer fixture.Close()
	runtime, err := rendering.NewRuntime(cfg.Render)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	state := t.TempDir()
	for _, item := range input.Cases {
		if done[item.ID] {
			continue
		}
		target := item.URL
		if strings.HasPrefix(target, "fixture://") {
			target = fixture.URL + "/" + strings.TrimPrefix(target, "fixture://")
		}
		snapshot := &acquisitionRecorder{}
		baseline := runAcquisitionRoute(t, cfg, runtime, state, item, target, "baseline", snapshot)
		row := acquisitionReceipt{Identity: identity, Case: item, Input: snapshot.input, Routes: []acquisitionRoute{baseline}}
		if snapshot.input.URL != "" {
			for _, action := range []string{intel.AcquisitionServe, intel.AcquisitionDeclared, intel.AcquisitionRender} {
				if !slices.Contains(snapshot.input.AllowedActions, action) {
					continue
				}
				row.Routes = append(row.Routes, runAcquisitionRoute(t, cfg, runtime, state, item, target, action, nil))
			}
		}
		if err := encoder.Encode(row); err != nil {
			t.Fatal(err)
		}
		if err := file.Sync(); err != nil {
			t.Fatal(err)
		}
		fmt.Printf("ACQUISITION_COLLECT id=%s routes=%d baseline_facts=%t error=%s\n", item.ID, len(row.Routes), baseline.FactsFound, baseline.Error)
	}
}

func runAcquisitionRoute(t *testing.T, cfg config.Config, renderer rendering.Renderer, state string, item acquisitionCase, target, action string, snapshot *acquisitionRecorder) acquisitionRoute {
	t.Helper()
	req := ReadRequest{URL: target, Objective: item.Objective, Profile: core.ProfileDeep, RenderMode: "auto"}
	switch action {
	case intel.AcquisitionServe:
		req.RenderMode = "off"
		req.AgentReadableMode = "off"
	case intel.AcquisitionDeclared:
		req.RenderMode = "off"
		req.AgentReadableMode = "declared"
	case intel.AcquisitionRender:
		req.RenderMode = "required"
		req.AgentReadableMode = "off"
	}
	svc, err := NewWithRenderer(cfg, nil, state, renderer)
	if err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int64
	svc.acquirer.ObserveFetch = func() { attempts.Add(1) }
	if snapshot != nil {
		req.ObserveAcquisition = true
		svc.acquisitionObserver = snapshot
	}
	ctx, cancel := context.WithTimeout(t.Context(), acquisitionCaseDeadline)
	defer cancel()
	fmt.Printf("ACQUISITION_SAMPLE start id=%s action=%s\n", item.ID, action)
	defer fmt.Printf("ACQUISITION_SAMPLE end id=%s action=%s\n", item.ID, action)
	started := time.Now()
	response, err := svc.Read(ctx, req)
	result := acquisitionRoute{Action: action, LatencyMS: time.Since(started).Milliseconds(), FetchAttempts: attempts.Load()}
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.ActualAction = acquisitionBaseline(response)
	result.TraceID = response.Trace.TraceID
	result.Replay = &response.Replay
	raw, _ := json.Marshal(response.Trace)
	result.TraceSHA = fmt.Sprintf("%x", sha256.Sum256(raw))
	result.Complete = true
	requests := 0
	result.BrowserHTTPEvents = &requests
	for _, stage := range response.Trace.Stages {
		if stage.Stage == "acquire" {
			result.Complete = stage.Metadata["partial_body"] == "false"
		}
		if stage.Stage == "agent_readable" && stage.Metadata["selected"] == "true" {
			result.Complete = stage.Metadata["selected_partial"] == "false"
		}
		if stage.Stage != "render" {
			continue
		}
		result.BrowserUses++
		result.Render = append(result.Render, stage.Metadata)
		if stage.Metadata["render_path"] != "cdp" || stage.Metadata["rendered"] != "true" {
			result.BrowserHTTPEvents = nil
			result.Complete = false
			continue
		}
		if result.BrowserHTTPEvents != nil {
			var count int
			_, _ = fmt.Sscan(stage.Metadata["network_http_requests"], &count)
			requests += count
		}
		result.Complete = stage.Metadata["network_truncated"] == "false" && stage.Metadata["partial"] == "false" && stage.Metadata["network_body_missing"] == "0" && stage.Metadata["render_degraded"] != "true"
	}
	if response.AgentContext.Acquisition != nil {
		result.SnapshotMS = response.AgentContext.Acquisition.ElapsedMS
		result.LatencyMS = max(result.LatencyMS-result.SnapshotMS, 0)
	}
	text := normalizeAcquisitionText(mergeAcquisitionChunks(response))
	for _, alternative := range item.Facts {
		all := len(alternative) > 0
		for _, fact := range alternative {
			all = all && strings.Contains(text, normalizeAcquisitionText(fact))
		}
		if !all {
			continue
		}
		result.FactsFound = true
		for _, record := range response.ProofRecords {
			for _, fact := range alternative {
				if strings.Contains(normalizeAcquisitionText(acquisitionProofText(response, record.Proof.ChunkID)), normalizeAcquisitionText(fact)) {
					result.Witnesses = append(result.Witnesses, record)
					break
				}
			}
		}
		break
	}
	return result
}

func acquisitionProofText(response ReadResponse, id string) string {
	for _, chunk := range response.ResultPack.Chunks {
		if chunk.ID == id {
			return chunk.Text
		}
	}
	return ""
}

func mergeAcquisitionChunks(response ReadResponse) string {
	parts := make([]string, 0, len(response.ResultPack.Chunks))
	for _, chunk := range response.ResultPack.Chunks {
		parts = append(parts, chunk.Text)
	}
	return strings.Join(parts, "\n")
}
func normalizeAcquisitionText(text string) string { return strings.Join(strings.Fields(text), " ") }

func evaluateAcquisitionPolicy(t *testing.T, cfg config.Config, out, partition string) {
	t.Helper()
	source := os.Getenv("NEEDLEX_ACQUISITION_INPUT")
	if source == "" {
		t.Fatal("NEEDLEX_ACQUISITION_INPUT required")
	}
	rows := readAcquisitionReceipts(t, source)
	sourceBytes, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	configBytes, _ := json.Marshal(cfg)
	identity := fmt.Sprintf("%x", sha256.Sum256(append(sourceBytes, configBytes...))) + ":" + intel.AcquisitionSpecification + ":independent-model-cases-v1:" + partition + ":warm=" + os.Getenv("NEEDLEX_ACQUISITION_WARM")
	existing := readAcquisitionReceipts(t, out)
	done := map[string]bool{}
	for _, r := range existing {
		if r.Identity != identity {
			t.Fatal("inference resume inputs changed")
		}
		done[r.Case.ID] = true
	}
	model := intel.NewDecisionObserver(cfg.Decision, nil, intel.NewSemanticAligner(cfg, nil))
	if model == nil {
		t.Fatal("local decision observation is disabled")
	}
	semantic := intel.NewSemanticAligner(cfg, nil)
	file, err := os.OpenFile(out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, row := range rows {
		if row.Case.Partition != partition || done[row.Case.ID] {
			continue
		}
		row.Identity = identity
		if row.Input.URL == "" {
			row.ObservationError = "initial acquisition snapshot unavailable"
		} else {
			if os.Getenv("NEEDLEX_ACQUISITION_WARM") == "1" {
				started := time.Now()
				if err := warmAcquisitionEvidence(t.Context(), cfg, semantic, row.Input); err != nil {
					row.WarmupError = err.Error()
				}
				row.WarmupMS = time.Since(started).Milliseconds()
			}
			fmt.Printf("ACQUISITION_SAMPLE start id=%s action=model\n", row.Case.ID)
			started := time.Now()
			observation, err := model.ObserveAcquisition(t.Context(), row.Input)
			row.ModelWallMS = time.Since(started).Milliseconds()
			row.Observation = &observation
			if err != nil {
				row.ObservationError = err.Error()
			}
			if observation.SelectionSimilarity != nil {
				row.ReferenceSimilarity = observation.SelectionSimilarity
			} else if len(observation.Input.Evidence) == 1 {
				evidence := observation.Input.Evidence[0]
				started = time.Now()
				scores, err := semantic.Score(t.Context(), row.Input.Objective, []intel.SemanticCandidate{{ID: evidence.ID, Heading: evidence.Heading, Text: evidence.Text}})
				row.ReferenceMS = time.Since(started).Milliseconds()
				if err != nil {
					row.ReferenceError = err.Error()
				} else if len(scores) == 1 && scores[0].ID == evidence.ID && !math.IsNaN(scores[0].Similarity) && !math.IsInf(scores[0].Similarity, 0) {
					score := scores[0].Similarity
					row.ReferenceSimilarity = &score
				} else {
					row.ReferenceError = "dense reference unavailable"
				}
			}
		}
		fmt.Printf("ACQUISITION_SAMPLE end id=%s action=model\n", row.Case.ID)
		// Independent corpus cases must not all become a fast loop through one
		// earlier request's circuit breaker. Retain the failed case, keep its
		// deadlines, and account separately for the configured recovery interval.
		if row.ObservationError != "" {
			row.FailurePacingMS = cfg.Semantic.FailureCooldownMS
		}
		// Preserve only the exact selected state needed to reproduce the local
		// classifier, not copies of entire public documents in final receipts.
		if row.Observation != nil {
			row.Input = row.Observation.Input
		} else {
			row.Input.Evidence = nil
		}
		if err := encoder.Encode(row); err != nil {
			t.Fatal(err)
		}
		if err := file.Sync(); err != nil {
			t.Fatal(err)
		}
		status := "unavailable"
		choice := ""
		if row.Observation != nil {
			status = row.Observation.Status
			choice = row.Observation.ProposedAction
		}
		fmt.Printf("ACQUISITION_MODEL partition=%s id=%s status=%s choice=%s\n", partition, row.Case.ID, status, choice)
		if row.FailurePacingMS > 0 {
			select {
			case <-time.After(time.Duration(row.FailurePacingMS) * time.Millisecond):
			case <-t.Context().Done():
				t.Fatal(t.Context().Err())
			}
		}
	}
}

// Warm cache qualification is separate from cold admission. It uses real dense
// vectors and unchanged per-request deadlines, not fabricated scores or labels.
func warmAcquisitionEvidence(ctx context.Context, cfg config.Config, semantic intel.SemanticAligner, input intel.AcquisitionInput) error {
	batch := make([]intel.SemanticCandidate, 0, cfg.Semantic.MaxCandidates)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		scores, err := semantic.Score(ctx, input.Objective, batch)
		if err != nil {
			return err
		}
		if len(scores) != len(batch) {
			return fmt.Errorf("warmup dense scores unavailable")
		}
		batch = batch[:0]
		return nil
	}
	for _, unit := range input.Evidence {
		if len(strings.Join(unit.Heading, "\n"))+len(unit.Text) > cfg.Decision.MaxInputBytes {
			continue
		}
		batch = append(batch, intel.SemanticCandidate{ID: unit.ID, Heading: unit.Heading, Text: unit.Text})
		if len(batch) >= cfg.Semantic.MaxCandidates {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func acquisitionFixture(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	kind := parts[0]
	if len(parts) > 1 {
		switch parts[1] {
		case "evidence.md":
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = fmt.Fprint(w, "# Warranty\nThe product warranty lasts for 37 months after purchase.")
			return
		case "data.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"warranty":"37 months"}`)
			return
		}
	}
	if !slices.Contains([]string{"static", "declared", "equivalent", "inline", "fetch", "rich_fetch", "unavailable"}, kind) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	body := `<title>Product information</title><main><h1>Product information</h1>`
	switch kind {
	case "static", "equivalent":
		body += `<p>The product warranty lasts for 37 months after purchase.</p>`
	case "rich_fetch", "declared", "unavailable":
		body += strings.Repeat(`<p>Ancient astronomers documented movements of stars and seasonal constellations. This historical archive describes celestial observations across the Mediterranean.</p>`, 15)
	default:
		body += `<p id="data">Waiting for application data</p>`
	}
	if kind == "declared" || kind == "equivalent" {
		body += fmt.Sprintf(`<link rel="alternate" href="/%s/evidence.md">`, kind)
	}
	if kind == "inline" {
		body += `<script>setTimeout(()=>document.getElementById('data').textContent='The product warranty lasts for 37 months after purchase.',250)</script>`
	}
	if kind == "fetch" || kind == "rich_fetch" {
		body += fmt.Sprintf(`<script>fetch('/%s/data.json').then(r=>r.json()).then(d=>{let p=document.createElement('p');p.textContent='The product warranty lasts for '+d.warranty+' after purchase.';document.body.appendChild(p)})</script>`, kind)
	}
	_, _ = fmt.Fprint(w, body+`</main>`)
}

func TestAcquisitionQualificationDoesNotCallPartialHTTPComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<title>Warranty</title><main><p>The product warranty lasts for 37 months after purchase.</p>`+strings.Repeat(`<p>Additional terms.</p>`, 30)+`</main>`)
	}))
	defer server.Close()
	cfg := testConfig()
	cfg.Runtime.MaxBytes = 200
	item := acquisitionCase{ID: "partial", Objective: "Warranty", Facts: [][]string{{"37 months"}}}
	result := runAcquisitionRoute(t, cfg, nil, t.TempDir(), item, server.URL, intel.AcquisitionServe, nil)
	if result.Error != "" || !result.FactsFound || result.Complete {
		t.Fatalf("partial HTTP response disguised as complete: %+v", result)
	}
}
