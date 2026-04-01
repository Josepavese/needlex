package service

import (
	"encoding/json"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
)

func newDiscoverSemanticServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Input any `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		inputs := semanticInputsForTest(payload.Input)
		data := make([]map[string]any, 0, len(inputs))
		for i, input := range inputs {
			data = append(data, map[string]any{
				"object":    "embedding",
				"index":     i,
				"embedding": semanticVectorForTest(input),
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data":   data,
			"model":  "discover-test-embed",
		})
	}))
}

func semanticInputsForTest(raw any) []string {
	switch typed := raw.(type) {
	case string:
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func semanticVectorForTest(text string) []float64 {
	const dims = 64
	vector := make([]float64, dims)
	for _, token := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}) {
		if token == "" {
			continue
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(token))
		idx := int(h.Sum32() % dims)
		vector[idx] += 1
	}
	return vector
}

func enableDiscoverSemantic(cfg *config.Config, baseURL string) {
	cfg.Semantic.Enabled = true
	cfg.Semantic.Backend = "openai-embeddings"
	cfg.Semantic.BaseURL = baseURL
	cfg.Semantic.Model = "discover-test-embed"
}

func newTestService(tb testing.TB, cfg config.Config, client *http.Client) *Service {
	tb.Helper()

	svc, err := New(cfg, client)
	if err != nil {
		tb.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}
	return svc
}

func newSemanticService(tb testing.TB, client *http.Client) *Service {
	tb.Helper()

	semantic := newDiscoverSemanticServer()
	tb.Cleanup(semantic.Close)
	cfg := config.Defaults()
	enableDiscoverSemantic(&cfg, semantic.URL)
	return newTestService(tb, cfg, client)
}

func requireCompilerDecision(t *testing.T, decisions []QueryPlanDecision, reason string, match func(QueryPlanDecision) bool) QueryPlanDecision {
	t.Helper()

	for _, decision := range decisions {
		if decision.ReasonCode != reason {
			continue
		}
		if match == nil || match(decision) {
			return decision
		}
	}
	t.Fatalf("expected compiler reason %q in %#v", reason, decisions)
	return QueryPlanDecision{}
}

func forbidCompilerDecision(t *testing.T, decisions []QueryPlanDecision, reason string) {
	t.Helper()

	for _, decision := range decisions {
		if decision.ReasonCode == reason {
			t.Fatalf("unexpected compiler reason %q in %#v", reason, decisions)
		}
	}
}
