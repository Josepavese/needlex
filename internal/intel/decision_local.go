package intel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
)

type localDecisionObserver struct {
	cfg      config.DecisionConfig
	client   *http.Client
	semantic SemanticAligner
	memo     decisionMemo
}

type decisionQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

// NewDecisionObserver constructs the HTTP adapter; the caller owns the shared
// runtime and borrowed client. Disabled observation allocates no runtime.
func NewDecisionObserver(cfg config.DecisionConfig, client *http.Client, semantic SemanticAligner) DecisionObserver {
	if cfg.Mode != "observe" {
		return nil
	}
	return &localDecisionObserver{cfg: cfg, client: httpClientOrDefault(client, defaultUpstreamTimeout), semantic: semantic}
}

func decisionQuestions(assertion string) map[string]decisionQuestion {
	return map[string]decisionQuestion{"support": {"choice", "Using only the passage, assess this claim: " + assertion, map[string]string{
		"supported":       "The passage supports the entire claim and gives no incompatible fact",
		"contradicted":    "The passage states an incompatible fact and does not support the claim",
		"not_established": "The passage neither supports nor refutes the entire claim, or gives conflicting statements",
	}}}
}

func (o *localDecisionObserver) Observe(ctx context.Context, input DecisionInput) (DecisionObservation, error) {
	started := time.Now()
	result := DecisionObservation{Status: "unavailable", Model: o.cfg.Model, Revision: o.cfg.Revision, Specification: DecisionSpecification, Calibration: "unqualified_in_domain"}
	result.Assertion = input.Assertion
	result.UnassessedEvidenceCount = input.EvidenceCount()
	if err := o.cfg.Validate(); err != nil {
		return result, err
	}
	if strings.TrimSpace(input.Assertion) == "" || len(input.Evidence) == 0 {
		result.Status = "insufficient_input"
		return result, nil
	}
	ctx, cancel := contextWithTimeoutMS(ctx, o.cfg.TimeoutMS)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	unit, similarity, reason, err := o.selectEvidence(ctx, input)
	result.SelectionSimilarity, result.Reason = similarity, reason
	if err != nil {
		return result, err
	}
	if unit == nil {
		result.Status = reason
		return result, nil
	}
	result.EvidenceIDs = []string{unit.ID}
	state, err := json.Marshal(decisionEvidenceText(*unit))
	if err != nil {
		return result, err
	}
	result.InputBytes = len(state) + len(input.Assertion)
	questions := decisionQuestions(input.Assertion)
	cacheInput, _ := json.Marshal([]any{o.cfg.BaseURL, o.cfg.Model, o.cfg.Revision, DecisionSpecification, state, questions})
	key := sha256.Sum256(cacheInput)
	if answer, ok := o.memo.get(key); ok {
		result.Answers = map[string]DecisionAnswer{"support": answer}
		result.Status, result.CacheHit = "observed", true
		result.UnassessedEvidenceCount--
		result.ElapsedMS = time.Since(started).Milliseconds()
		return result, nil
	}
	var models struct {
		Data []struct {
			ID   string `json:"id"`
			Meta struct {
				ContextTokens int `json:"n_ctx"`
			} `json:"meta"`
		} `json:"data"`
	}
	if err := o.exchange(ctx, http.MethodGet, "/v1/models", nil, &models); err != nil {
		return result, err
	}
	if len(models.Data) != 1 || models.Data[0].ID != o.cfg.Model {
		return result, fmt.Errorf("local decision runtime model identity mismatch")
	}
	for _, question := range questions {
		head, _ := json.Marshal(question)
		fits, err := o.promptFits(ctx, head, state, models.Data[0].Meta.ContextTokens)
		if err != nil {
			return result, err
		}
		if !fits {
			result.Status = "prompt_budget_exceeded"
			return result, nil
		}
	}
	body, err := json.Marshal(struct {
		State     json.RawMessage             `json:"state"`
		Questions map[string]decisionQuestion `json:"questions"`
	}{state, questions})
	if err != nil {
		return result, err
	}
	var response struct {
		Model   string                    `json:"model"`
		Answers map[string]DecisionAnswer `json:"answers"`
	}
	if err := o.exchange(ctx, http.MethodPost, "/v1/systemone", body, &response); err != nil {
		return result, err
	}
	if response.Model != o.cfg.Model {
		return result, fmt.Errorf("decision response model identity mismatch")
	}
	if len(response.Answers) != len(questions) {
		return result, fmt.Errorf("decision response has missing or foreign questions")
	}
	for id, question := range questions {
		answer, ok := response.Answers[id]
		if !ok || len(answer.Probabilities) != len(question.Criteria) {
			return result, fmt.Errorf("invalid decision distribution for %s", id)
		}
		sum := 0.0
		best := -1.0
		for option := range question.Criteria {
			p, ok := answer.Probabilities[option]
			if !ok || math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
				return result, fmt.Errorf("invalid decision probability for %s", id)
			}
			sum += p
			best = max(best, p)
		}
		if p, ok := answer.Probabilities[answer.Choice]; !ok || p != best || math.Abs(sum-1) > decisionProbabilityTolerance {
			return result, fmt.Errorf("invalid decision choice or normalization for %s", id)
		}
	}
	result.Status = "observed"
	result.UnassessedEvidenceCount--
	o.memo.set(key, response.Answers["support"])
	result.Answers = response.Answers
	result.ElapsedMS = time.Since(started).Milliseconds()
	return result, nil
}

const decisionProbabilityTolerance = 0.0001

func (o *localDecisionObserver) exchange(ctx context.Context, method, path string, body []byte, target any) error {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(o.cfg.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// Never follow a local runtime redirect into an external service.
	client := *o.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("local decision runtime returned HTTP %d", resp.StatusCode)
	}
	return decodeJSONLimited(resp.Body, maxModelResponseBytes, "decision", target)
}

// Tokenize conservative envelopes before inference: the engine can silently cut
// a long question to its trained head budget. Reject instead of auditing a
// different question or an incomplete source. The deployment pins that budget.
func (o *localDecisionObserver) promptFits(ctx context.Context, head, state []byte, contextTokens int) (bool, error) {
	for _, part := range []struct {
		text  string
		limit int
	}{{string(head), o.cfg.MaxQuestionTokens}, {string(head) + "\n" + string(state), contextTokens}} {
		body, err := json.Marshal(map[string]any{"content": part.text, "add_special": true, "parse_special": false})
		if err != nil {
			return false, err
		}
		var result struct {
			Tokens []int `json:"tokens"`
		}
		if err := o.exchange(ctx, http.MethodPost, "/tokenize", body, &result); err != nil {
			return false, err
		}
		if len(result.Tokens) == 0 {
			return false, fmt.Errorf("local decision tokenizer returned no tokens")
		}
		if len(result.Tokens) > part.limit {
			return false, nil
		}
	}
	return true, nil
}

// Select whole evidence units; no byte slicing or lexical approximation of relevance.
func (o *localDecisionObserver) selectEvidence(ctx context.Context, input DecisionInput) (*DecisionEvidence, *float64, string, error) {
	eligible := []DecisionEvidence{}
	seen := map[string]string{}
	for _, evidence := range input.Evidence {
		if evidence.ID == "" {
			return nil, nil, "", fmt.Errorf("decision evidence requires an identity")
		}
		if previous, ok := seen[evidence.ID]; ok {
			if previous != decisionEvidenceText(evidence) {
				return nil, nil, "", fmt.Errorf("conflicting decision evidence identity")
			}
			continue
		}
		seen[evidence.ID] = decisionEvidenceText(evidence)
		state, _ := json.Marshal(decisionEvidenceText(evidence))
		if strings.TrimSpace(evidence.Text) != "" && len(state)+len(input.Assertion) <= o.cfg.MaxInputBytes {
			eligible = append(eligible, evidence)
		}
	}
	if len(eligible) == 0 {
		return nil, nil, "input_budget_exceeded", nil
	}
	if len(eligible) == 1 {
		return &eligible[0], nil, "single_eligible_evidence", nil
	}
	if o.semantic == nil {
		return nil, nil, "selection_unavailable", nil
	}
	candidates := make([]SemanticCandidate, len(eligible))
	for i, evidence := range eligible {
		candidates[i] = SemanticCandidate{ID: evidence.ID, Heading: evidence.Heading, Text: evidence.Text}
	}
	scores, err := o.semantic.Score(ctx, input.Assertion, candidates)
	if err != nil {
		return nil, nil, "", err
	}
	byID := map[string]float64{}
	for _, score := range scores {
		if !math.IsNaN(score.Similarity) && !math.IsInf(score.Similarity, 0) {
			byID[score.ID] = score.Similarity
		}
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		a, aok := byID[eligible[i].ID]
		b, bok := byID[eligible[j].ID]
		return aok && (!bok || a > b)
	})
	similarity, ok := byID[eligible[0].ID]
	if !ok {
		return nil, nil, "selection_unavailable", nil
	}
	return &eligible[0], &similarity, "dense_semantic_selection", nil
}

func decisionEvidenceText(evidence DecisionEvidence) string {
	if len(evidence.Heading) == 0 {
		return evidence.Text
	}
	return strings.Join(evidence.Heading, "\n") + "\n" + evidence.Text
}
