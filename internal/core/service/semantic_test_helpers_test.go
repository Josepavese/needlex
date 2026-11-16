package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/intel"
)

var (
	testDenseEmbeddingOnce sync.Once
	testDenseEmbeddingURL  string
)

type noScoreSemanticAligner struct{}

func (noScoreSemanticAligner) Align(_ context.Context, _ string, _ []intel.SemanticCandidate) (intel.SemanticAlignment, error) {
	return intel.SemanticAlignment{}, nil
}

func (noScoreSemanticAligner) Score(_ context.Context, _ string, _ []intel.SemanticCandidate) ([]intel.SemanticScore, error) {
	return nil, nil
}

func testConfig() config.Config {
	cfg := config.Defaults()
	cfg.Render.Enabled = false
	enableDiscoverSemantic(&cfg, "")
	return cfg
}

func enableDiscoverSemantic(cfg *config.Config, baseURL string) {
	cfg.Semantic.VectorSpace = intel.DenseSemanticVectorSpace
	cfg.Semantic.ProviderModel = "service-test-embedding"
	if baseURL == "" {
		baseURL = serviceTestEmbeddingURL()
	}
	cfg.Semantic.EmbeddingURL = baseURL
}

func serviceTestEmbeddingURL() string {
	testDenseEmbeddingOnce.Do(func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Input []string `json:"input"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			vectors := make([][]float32, 0, len(req.Input))
			for _, input := range req.Input {
				vectors = append(vectors, serviceTestEmbeddingVector(input))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": vectors})
		}))
		testDenseEmbeddingURL = server.URL
	})
	return testDenseEmbeddingURL
}

func serviceTestEmbeddingVector(text string) []float32 {
	vector := make([]float32, 20)
	if axis, weight, ok := serviceFixtureAxis(text); ok {
		vector[axis] = weight
	}
	return vector
}

func serviceFixtureAxis(text string) (int, float32, bool) {
	if axis, ok := serviceExactFixtureAxes[normalizeFixtureText(text)]; ok {
		return axis, 1, true
	}
	for _, rawURL := range extractFixtureURLs(text) {
		if axis, weight, ok := serviceURLFixtureAxis(rawURL); ok {
			return axis, weight, true
		}
	}
	if axis, weight, ok := serviceIdentityFixtureAxis(text); ok {
		return axis, weight, true
	}
	return 0, 0, false
}

var serviceExactFixtureAxes = map[string]int{
	"proof replay deterministic":                                  0,
	"proof replay deterministic context":                          0,
	"proof replay deterministic guide":                            0,
	"replay proof guide":                                          0,
	"replay proof guide replay proof guide":                       0,
	"replay proof guide replay proof guide replay proof guide":    0,
	"replay proof guide 127.0.0.1 / / html_like":                  0,
	"replay proof guide 127.0.0.1 . html_like":                    0,
	"replay proof guide replay proof guide 127.0.0.1 . html_like": 0,
	"replay proof guide replay proof guide 127.0.0.1 / /":         0,
	"replay proof guide replay proof guide replay proof guide replay proof guide replay proof guide replay proof guide 127.0.0.1 . html_like": 0,
	"replay guide":                                    0,
	"replay guide html_like":                          0,
	"replay guide replay guide":                       0,
	"replay guide replay guide replay guide":          0,
	"replay guide replay guide 127.0.0.1 . html_like": 0,
	"replay guide replay guide 127.0.0.1 / /":         0,
	"replay guide replay guide replay guide replay guide replay guide replay guide 127.0.0.1 . html_like": 0,
	"replay drift troubleshooting":       1,
	"forum replay drift troubleshooting": 1,
	"download":                           2,
	"download download":                  2,
	"playwright installation":            10,
	"playwright playwright is an end-to-end testing framework for modern apps.":                                        10,
	"installation install playwright with npm and run install browsers.":                                               10,
	"installation | playwright install playwright and then run the installation command to download browser binaries.": 10,
	"installation | playwright install playwright and browser binaries.":                                               10,
	"installation | playwright install playwright and run the installation command to download browser binaries.":      10,
	"playwright install": 10,
	"python packaging":   4,
	"python packaging user guide python packaging user guide python packaging user guide":                                 4,
	"python packaging user guide python packaging user guide python packaging user guide packaging python org python.org": 4,
	"mdn javascript guide":                                                               7,
	"mdn javascript overview":                                                            7,
	"mdn web docs javascript | mdn javascript | mdn html_like":                           7,
	"mdn web docs javascript | mdn javascript | mdn":                                     7,
	"javascript | mdn javascript | mdn":                                                  7,
	"introduction - javascript | mdn introduction - javascript | mdn":                    7,
	"distributed tracing metrics dashboard":                                              6,
	"distributed tracing metrics dashboard panels incident response observability guide": 6,
	"distributed tracing metrics dashboard panels incident response observability guide documentation html_like": 6,
	"distributed tracing metrics dashboard panels incident response observability guide documentation":           6,
	"openai api pricing":                                     9,
	"openai api reference":                                   9,
	"z ai coding plan base url":                              12,
	"z ai coding plan api endpoint":                          12,
	"read the initialize method":                             13,
	"asd charly brown dance school alessandria":              14,
	"official site for comitato olimpico nazionale italiano": 16,
	"capire l'identita complessiva del progetto":             17,
	"capire l'identità complessiva del progetto":             17,
}

func serviceURLFixtureAxis(raw string) (int, float32, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return 0, 0, false
	}
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.EscapedPath())
	switch host {
	case "docs.python.org", "packaging.python.org":
		return 4, 1, true
	case "developer.mozilla.org":
		return 7, 1, true
	case "javascript.com", "www.javascript.com":
		return 8, 0.5, true
	case "openai.com", "platform.openai.com":
		return 9, 1, true
	case "playwright.dev":
		return 10, 1, true
	case "sqlite.org", "www.sqlite.org":
		return 11, 1, true
	case "www.coni.it", "coni.it":
		return 16, 1, true
	}
	switch {
	case path == "/docs/replay":
		return 0, 1, true
	case path == "/download.html":
		return 2, 1, true
	case strings.HasSuffix(host, "halfpocket.it"):
		return 15, 1, true
	case strings.HasSuffix(host, "charlybrown.it"):
		return 14, 1, true
	case strings.HasSuffix(host, "z.ai"):
		return 12, 1, true
	}
	return 0, 0, false
}

func serviceIdentityFixtureAxis(text string) (int, float32, bool) {
	fields := strings.Fields(strings.ToLower(text))
	for _, field := range fields {
		field = strings.Trim(field, `"'()[]{}<>,.;`)
		switch field {
		case "/docs/replay":
			return 0, 1, true
		case "/download.html", "download.html":
			return 2, 1, true
		case "docs.python.org", "packaging.python.org":
			return 4, 1, true
		case "developer.mozilla.org":
			return 7, 1, true
		case "javascript.com", "www.javascript.com":
			return 8, 0.5, true
		case "openai.com", "platform.openai.com", "developers.openai.com":
			return 9, 1, true
		case "playwright.dev":
			return 10, 1, true
		case "sqlite.org", "www.sqlite.org":
			return 11, 1, true
		case "observability.example":
			return 6, 1, true
		case "coni.it", "www.coni.it":
			return 16, 1, true
		}
		if strings.HasSuffix(field, "halfpocket.it") {
			return 15, 1, true
		}
		if strings.HasSuffix(field, "charlybrown.it") {
			return 14, 1, true
		}
	}
	return 0, 0, false
}

func extractFixtureURLs(text string) []string {
	fields := strings.Fields(text)
	out := make([]string, 0, 2)
	for _, field := range fields {
		field = strings.Trim(field, `"'()[]{}<>,.;`)
		if strings.HasPrefix(field, "http://") || strings.HasPrefix(field, "https://") {
			out = append(out, field)
		}
	}
	return out
}

func normalizeFixtureText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.ReplaceAll(text, "à", "a")
	text = strings.Join(strings.Fields(text), " ")
	return text
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

	cfg := config.Defaults()
	cfg.Render.Enabled = false
	enableDiscoverSemantic(&cfg, "")
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
