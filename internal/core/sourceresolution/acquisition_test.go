package sourceresolution

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
)

func TestAcquisitionMenuRespectsStrictModesFormatsAndBudget(t *testing.T) {
	cfg := config.Defaults()
	cfg.Render.Enabled, cfg.Agent.ReadableEnabled = true, true
	r := Resolver{Config: cfg}
	page := pipeline.RawPage{URL: "https://example.com/docs", FinalURL: "https://example.com/docs", ContentType: "text/html", HTML: `<title>Conditions</title><link rel="alternate" href="/docs.md"><h1>Warranty</h1><p>La garanzia dura 37 mesi dalla data di acquisto.</p>`}
	expired := time.Duration(0)
	for _, item := range []struct {
		name, mode, contentType string
		budget                  *time.Duration
		render, declared        bool
	}{
		{"auto", "auto", "text/html", nil, true, true},
		{"off", "off", "text/html", nil, false, true},
		{"json", "auto", "application/json", nil, false, false},
		{"expired", "auto", "text/html", &expired, false, false},
		{"required", "required", "text/html", nil, true, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			original := page
			original.ContentType = item.contentType
			input, err := r.AcquisitionInput(Request{Objective: "Warranty conditions", RenderMode: item.mode}, original, item.budget)
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(input.AllowedActions, intel.AcquisitionRender) != item.render || slices.Contains(input.AllowedActions, intel.AcquisitionDeclared) != item.declared {
				t.Fatalf("illegal menu: %#v", input.AllowedActions)
			}
			if item.mode == "required" && !reflect.DeepEqual(input.AllowedActions, []string{intel.AcquisitionRender}) {
				t.Fatal("strict required can be weakened")
			}
			if input.SourceHash == "" || input.URL != page.URL {
				t.Fatal("unattributed original surface")
			}
		})
	}
}

func TestRequiredRenderCannotBeReplacedByDeclaredMarkdown(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# Terms\nThe warranty lasts37months."))
	}))
	defer server.Close()
	page := pipeline.RawPage{URL: server.URL, FinalURL: server.URL, ContentType: "text/html", HTML: `<link rel="alternate" href="/terms.md"><p>Original page</p>`}
	r := Resolver{Config: config.Defaults(), Acquirer: pipeline.Acquirer{Client: server.Client()}}
	r.Config.Agent.ReadableEnabled = true
	recorder := proof.NewRecorder("run-required", "trace-required", time.Now())
	resolved, err := r.ResolveAgentReadable(t.Context(), recorder, Request{RenderMode: "required"}, page)
	if err != nil || requests != 0 || resolved.HTML != page.HTML {
		t.Fatalf("required browser read was replaced: %v requests=%d %#v", err, requests, resolved)
	}
}
