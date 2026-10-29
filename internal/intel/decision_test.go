package intel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/config"
)

func TestDecisionObserverLocalContractAndInvalidResponses(t *testing.T) {
	for _, mode := range []string{"valid", "foreign_model", "missing_question", "foreign_option", "unnormalized", "wrong_choice", "wrong_response_model", "upstream"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/models" {
					id := "Julia"
					if mode == "foreign_model" {
						id = "other"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": id, "meta": map[string]int{"n_ctx": 2048}}}})
					return
				}
				if r.URL.Path == "/tokenize" {
					_ = json.NewEncoder(w).Encode(map[string]any{"tokens": []int{1, 2, 3}})
					return
				}
				calls++
				if mode == "upstream" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				var req struct {
					State     string                      `json:"state"`
					Questions map[string]decisionQuestion `json:"questions"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req.State != "47 euro" || len(req.Questions) != 1 || !strings.Contains(req.Questions["support"].Instructions, "Prezzi") {
					t.Error("request lost its typed evidence or questions")
				}
				answers := map[string]DecisionAnswer{}
				for id, q := range req.Questions {
					p := map[string]float64{}
					winner := ""
					for option := range q.Criteria {
						p[option] = 0
						if winner == "" {
							winner = option
						}
					}
					p[winner] = 1
					if mode == "foreign_option" {
						p["foreign"] = 0
					}
					if mode == "unnormalized" {
						p[winner] = 0.7
					}
					if mode == "wrong_choice" {
						winner = "foreign"
					}
					answers[id] = DecisionAnswer{Choice: winner, Probabilities: p}
				}
				if mode == "missing_question" {
					delete(answers, "support")
				}
				model := "Julia"
				if mode == "wrong_response_model" {
					model = "other"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"model": model, "answers": answers})
			}))
			defer srv.Close()
			cfg := config.DecisionConfig{Mode: "observe", BaseURL: srv.URL, Model: "Julia", Revision: "pinned", TimeoutMS: 1000, MaxQuestionTokens: 256, MaxInputBytes: 1000}
			observer := NewDecisionObserver(cfg, srv.Client(), nil)
			result, err := observer.Observe(t.Context(), DecisionInput{Assertion: "Prezzi", Evidence: []DecisionEvidence{{ID: "chunk-1", Text: "47 euro"}}})
			if mode == "valid" {
				if err != nil || result.Status != "observed" || result.Calibration != "unqualified_in_domain" || len(result.EvidenceIDs) != 1 {
					t.Fatalf("%#v %v", result, err)
				}
			} else if err == nil {
				t.Fatal("invalid response accepted")
			}
			cfg.MaxInputBytes = 1
			result, err = NewDecisionObserver(cfg, srv.Client(), nil).Observe(t.Context(), DecisionInput{Assertion: "Prezzi", Evidence: []DecisionEvidence{{ID: "oversize", Text: "47 euro"}}})
			if err != nil || result.Status != "input_budget_exceeded" {
				t.Fatalf("oversized input must be reported, not truncated: %#v %v", result, err)
			}
			if calls > 1 {
				t.Fatal("oversized input invoked inference")
			}
		})
	}
}

func TestDecisionObserverRejectsPromptOverflowBeforeInference(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": "Julia", "meta": map[string]int{"n_ctx": 2048}}}})
		case "/tokenize":
			_ = json.NewEncoder(w).Encode(map[string]any{"tokens": make([]int, 257)})
		default:
			calls++
		}
	}))
	defer server.Close()
	observer := NewDecisionObserver(config.DecisionConfig{Mode: "observe", BaseURL: server.URL, Model: "Julia", Revision: "pinned", TimeoutMS: 1000, MaxInputBytes: 6000, MaxQuestionTokens: 256}, server.Client(), nil)
	result, err := observer.Observe(t.Context(), DecisionInput{Assertion: "Warranty", Evidence: []DecisionEvidence{{ID: "source", Text: "24 months"}}})
	if err != nil || result.Status != "prompt_budget_exceeded" || calls != 0 {
		t.Fatalf("overflow was inferred or hidden: %#v, %v, calls %d", result, err, calls)
	}
}

func TestDecisionObserverRefusesRedirects(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer server.Close()
	observer := NewDecisionObserver(config.DecisionConfig{Mode: "observe", BaseURL: server.URL, Model: "Julia", Revision: "pinned", TimeoutMS: 1000, MaxInputBytes: 6000, MaxQuestionTokens: 256}, server.Client(), nil)
	_, err := observer.Observe(t.Context(), DecisionInput{Assertion: "Warranty", Evidence: []DecisionEvidence{{ID: "source", Text: "24 months"}}})
	if err == nil || targetCalls != 0 {
		t.Fatal("local decision request followed a redirect")
	}
}
