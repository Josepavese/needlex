package intel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
)

type localDecisionObserver struct {
	cfg    config.DecisionConfig
	client *http.Client
}

type decisionQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

// NewDecisionObserver constructs the HTTP adapter; the caller owns the shared
// runtime and borrowed client. Disabled observation allocates no runtime.
func NewDecisionObserver(cfg config.DecisionConfig, client *http.Client) DecisionObserver {
	if cfg.Mode != "observe" {
		return nil
	}
	return localDecisionObserver{cfg, httpClientOrDefault(client, defaultUpstreamTimeout)}
}

func decisionQuestions(objective string) map[string]decisionQuestion {
	prefix := "Objective: " + objective + "\n"
	return map[string]decisionQuestion{
		"relevance": {"choice", prefix + "Is this source evidence about the objective?", map[string]string{"related": "The evidence concerns the objective's subject", "unrelated": "The evidence concerns a different subject", "unknown": "There is insufficient evidence to judge relevance"}},
		"coverage":  {"choice", prefix + "How much of the objective is answered by this source evidence?", map[string]string{"complete": "The evidence answers all requested aspects", "partial": "The evidence answers some requested aspects but leaves gaps", "absent": "The evidence does not answer the objective", "unknown": "The objective or evidence is insufficient to assess coverage"}},
		"support":   {"choice", prefix + "Can an answer to this objective be supported by this source evidence?", map[string]string{"supported": "An answer can be grounded in explicit evidence", "unsupported": "The evidence does not support an answer", "conflicting": "The evidence contains conflicting answers", "unknown": "Support cannot be determined from the supplied evidence"}},
	}
}

func (o localDecisionObserver) Observe(ctx context.Context, input DecisionInput) (DecisionObservation, error) {
	started := time.Now()
	result := DecisionObservation{Status: "unavailable", Model: o.cfg.Model, Revision: o.cfg.Revision, Specification: DecisionSpecification, Calibration: "unqualified_in_domain"}
	state, err := json.Marshal(struct {
		Evidence []DecisionEvidence `json:"evidence"`
	}{input.Evidence})
	if err != nil {
		return result, err
	}
	result.InputBytes = len(state) + len(input.Objective)
	for _, evidence := range input.Evidence {
		result.EvidenceIDs = append(result.EvidenceIDs, evidence.ID)
	}
	if result.InputBytes > o.cfg.MaxInputBytes {
		result.Status = "input_budget_exceeded"
		return result, nil
	}
	if strings.TrimSpace(input.Objective) == "" || len(input.Evidence) == 0 {
		result.Status = "insufficient_input"
		return result, nil
	}
	ctx, cancel := contextWithTimeoutMS(ctx, o.cfg.TimeoutMS)
	defer cancel()
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
	questions := decisionQuestions(input.Objective)
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
	result.Answers = response.Answers
	result.ElapsedMS = time.Since(started).Milliseconds()
	return result, nil
}

const decisionProbabilityTolerance = 0.0001

func (o localDecisionObserver) exchange(ctx context.Context, method, path string, body []byte, target any) error {
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
func (o localDecisionObserver) promptFits(ctx context.Context, head, state []byte, contextTokens int) (bool, error) {
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
