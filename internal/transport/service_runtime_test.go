package transport

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
)

func TestMCPServiceRetainsDecisionMemoAndInvalidatesChangedConfig(t *testing.T) {
	modelCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = fmt.Fprint(w, `{"data":[{"id":"Julia","meta":{"n_ctx":2048}}]}`)
		case "/tokenize":
			_, _ = fmt.Fprint(w, `{"tokens":[1,2,3]}`)
		case "/v1/systemone":
			modelCalls++
			_, _ = fmt.Fprint(w, `{"model":"Julia","answers":{"support":{"choice":"supported","probabilities":{"supported":1,"contradicted":0,"not_established":0}}}}`)
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<title>Conditions</title><p>The warranty for this product lasts for 37 months from purchase. All repairs are covered throughout this period.</p>`)
		}
	}))
	defer server.Close()
	cfg := config.Defaults()
	cfg.Decision = config.DecisionConfig{Mode: "observe", BaseURL: server.URL, Model: "Julia", Revision: "first", TimeoutMS: 1000, MaxInputBytes: 1000, MaxQuestionTokens: 256}
	runtime := &serviceRuntime{}
	root := t.TempDir()
	read := func(assertion string) coreservice.ReadResponse {
		svc, err := runtime.get(cfg, root, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := svc.Read(t.Context(), coreservice.ReadRequest{URL: server.URL, Assertion: assertion, RenderMode: "off"})
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	ordinary := read("")
	if ordinary.AgentContext.Decision != nil || modelCalls != 0 {
		t.Fatal("ordinary retrieval spent advisory inference")
	}
	first := read("The warranty lasts37months")
	second := read("The warranty lasts37months")
	if modelCalls != 1 || first.AgentContext.Decision.CacheHit || !second.AgentContext.Decision.CacheHit {
		t.Fatal("serial MCP calls did not retain their bounded observer memo")
	}
	first.AgentContext.Decision.Answers["support"].Probabilities["supported"] = 0
	third := read("The warranty lasts37months")
	if third.AgentContext.Decision.Answers["support"].Probabilities["supported"] != 1 {
		t.Fatal("a caller mutated cached probability storage")
	}
	cfg.Decision.Revision = "second"
	fourth := read("The warranty lasts37months")
	if modelCalls != 2 || fourth.AgentContext.Decision.CacheHit {
		t.Fatal("a changed model revision reused an old inference")
	}
	before, _ := json.Marshal(ordinary.ResultPack.Chunks)
	after, _ := json.Marshal(fourth.ResultPack.Chunks)
	if string(before) != string(after) {
		t.Fatal("assertion observation changed compiled source text")
	}
}
