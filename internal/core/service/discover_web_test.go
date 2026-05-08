package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/core/candidateintel"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/webdiscover"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/store"
)

func TestRefineWebCandidateStaysStructureAndProbeDriven(t *testing.T) {
	candidate := webdiscover.RefineCandidate(
		"OpenAI API pricing",
		DiscoverCandidate{URL: "https://developers.openai.com/api/pricing", Label: "OpenAI API pricing", Score: 0.5},
		"https://developers.openai.com/api/pricing",
		"OpenAI API pricing",
		core.WebIR{
			Title:     "OpenAI API pricing",
			NodeCount: 2,
			Nodes: []core.WebIRNode{
				{Text: "Pricing details for the API platform."},
				{Text: "Token usage and model costs."},
			},
		},
		nil,
	)
	if !containsReason(candidate.Reason, "page_title_probe") || !containsReason(candidate.Reason, "web_ir_probe") {
		t.Fatalf("expected probe-driven reasons, got %#v", candidate.Reason)
	}
	if containsReason(candidate.Reason, "host_page_coherence") {
		t.Fatalf("expected surface-form host-page coherence to be absent, got %#v", candidate.Reason)
	}
	if candidate.Metadata["web_ir_context"] == "" {
		t.Fatalf("expected probed web ir context metadata, got %#v", candidate.Metadata)
	}
}

func TestDiscoverWebHostRootIdentityPrefersFirstPartyDocs(t *testing.T) {
	officialServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprint(w, `<html><head><title>OpenAI Platform</title></head><body><h1>OpenAI</h1><p>Official maintained API platform documentation.</p></body></html>`)
		case "/api/pricing":
			_, _ = fmt.Fprint(w, `<html><head><title>API pricing</title></head><body><h1>Pricing</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer officialServer.Close()

	thirdPartyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprint(w, `<html><head><title>Curlscape</title></head><body><h1>Curlscape</h1></body></html>`)
		case "/blog/openai-api-pricing-guide":
			_, _ = fmt.Fprint(w, `<html><head><title>OpenAI API pricing</title></head><body><h1>Guide</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer thirdPartyServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(
			w,
			`<html><body><a class="result__a" href="%s/blog/openai-api-pricing-guide">OpenAI API pricing</a><a class="result__a" href="%s/api/pricing">OpenAI API pricing</a></body></html>`,
			thirdPartyServer.URL,
			officialServer.URL,
		)
	}))
	defer searchServer.Close()

	cfg := testConfig()
	svc, err := New(cfg, searchServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "OpenAI API pricing",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != officialServer.URL+"/api/pricing" {
		t.Fatalf("expected first-party docs candidate to win, got %q", resp.SelectedURL)
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Metadata["host_root_title"] != "OpenAI Platform" {
		t.Fatalf("expected host root identity metadata on top candidate, got %#v", resp.Candidates)
	}
	if resp.Candidates[0].Metadata["host_root_context"] == "" {
		t.Fatalf("expected host root semantic context on top candidate, got %#v", resp.Candidates[0].Metadata)
	}
}

func TestDiscoverWebPromotesSameFamilyEmbeddedEndpoint(t *testing.T) {
	var docsServer *httptest.Server
	docsServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprint(w, `<html><head><title>Z AI Docs</title></head><body><h1>Z AI</h1></body></html>`)
		case "/coding-plan":
			_, _ = fmt.Fprintf(w, `<html><head><title>Z AI Coding Plan</title></head><body><h1>Coding Plan</h1><pre>%s/api/coding/paas/v4</pre></body></html>`, docsServer.URL)
		case "/api/coding/paas/v4":
			_, _ = fmt.Fprint(w, `{"ok":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer docsServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(
			w,
			`<html><body><a class="result__a" href="%s/coding-plan">Z AI coding plan</a></body></html>`,
			docsServer.URL,
		)
	}))
	defer searchServer.Close()

	cfg := testConfig()
	svc, err := New(cfg, searchServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "z ai coding plan base url",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != docsServer.URL+"/api/coding/paas/v4" {
		t.Fatalf("expected embedded endpoint candidate to win, got %q", resp.SelectedURL)
	}
}

func TestExtractEmbeddedURLCandidatesSkipsMediaAssetsFromHTMLPage(t *testing.T) {
	base := DiscoverCandidate{URL: "https://mdn.org.cn/en-US/docs/Web/JavaScript/Guide", Label: "JavaScript Guide"}
	dom := pipeline.SimplifiedDOM{Title: "JavaScript 指南 - JavaScript | MDN - MDN 文档"}
	rawHTML := `<html><head><title>JavaScript Guide</title></head><body><img src="https://mdn.org.cn/favicon.ico"><a href="https://mdn.org.cn/en-US/docs/Web/JavaScript/Guide">guide</a></body></html>`
	got := webdiscover.ExtractEmbeddedURLCandidates("MDN JavaScript guide", base, base.URL, rawHTML, dom, nil)
	for _, candidate := range got {
		if candidate.URL == "https://mdn.org.cn/favicon.ico" {
			t.Fatalf("expected media asset embedded URL to be skipped, got %#v", got)
		}
	}
}

func TestExtractEmbeddedURLCandidatesIgnoreRawHTMLNoiseForHTMLPages(t *testing.T) {
	base := DiscoverCandidate{URL: "https://www.w3schools.com/Js/", Label: "JavaScript Tutorial"}
	dom := pipeline.SimplifiedDOM{
		Title: "JavaScript Tutorial",
		Nodes: []pipeline.SimplifiedNode{
			{Text: "Learn JavaScript fundamentals."},
		},
	}
	rawHTML := `<html><head><title>JavaScript Tutorial</title><script>var x="https://www.w3schools.com/about/about_privacy.asp"</script></head><body><p>Learn JavaScript fundamentals.</p></body></html>`
	got := webdiscover.ExtractEmbeddedURLCandidates("MDN JavaScript guide", base, base.URL, rawHTML, dom, nil)
	for _, item := range got {
		if item.URL == "https://www.w3schools.com/about/about_privacy.asp" {
			t.Fatalf("expected raw html noise url to be ignored for html pages, got %#v", got)
		}
	}
}

func TestDiscoverWebEndpointExtractorPromotesEmbeddedURLFromShortlist(t *testing.T) {
	var docsServer *httptest.Server
	docsServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprint(w, `<html><head><title>Z AI Docs</title></head><body><h1>Z AI</h1></body></html>`)
		case "/coding-plan":
			_, _ = fmt.Fprintf(w, `<html><head><title>Z AI Coding Plan</title></head><body><h1>Coding Plan</h1><pre>%s/api/coding/paas/v4</pre></body></html>`, docsServer.URL)
		case "/api/coding/paas/v4":
			_, _ = fmt.Fprint(w, `{"ok":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer docsServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(
			w,
			`<html><body><a class="result__a" href="%s/coding-plan">Z AI coding plan</a><a class="result__a" href="%s/">Z AI</a></body></html>`,
			docsServer.URL,
			docsServer.URL,
		)
	}))
	defer searchServer.Close()

	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode model request: %v", err)
		}
		content := fmt.Sprintf(`{"selected_url":"%s/api/coding/paas/v4","evidence_page_url":"%s/coding-plan","kind":"native_api_endpoint","confidence":0.93}`, docsServer.URL, docsServer.URL)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"finish_reason": "stop", "message": map[string]any{"content": content}}},
			"usage":   map[string]any{"prompt_tokens": 64, "completion_tokens": 18},
		})
	}))
	defer modelServer.Close()

	cfg := testConfig()
	cfg.Models.Backend = "openai-compatible"
	cfg.Models.BaseURL = modelServer.URL
	cfg.Models.Router = "qwen-endpoint"
	svc := newTestService(t, cfg, searchServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "z ai coding plan api endpoint",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != docsServer.URL+"/api/coding/paas/v4" {
		t.Fatalf("expected llm endpoint extractor to win, got %q", resp.SelectedURL)
	}
}

func TestOrderEndpointCandidatesPrefersContextualHTMLCandidate(t *testing.T) {
	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc := newTestService(t, cfg, nil)

	ordered := svc.orderEndpointCandidates(context.Background(), "OpenAI API pricing", []DiscoverCandidate{
		{URL: "https://example.com/logo.png", Label: "OpenAI API pricing", Score: 1.0},
		{URL: "https://developers.openai.com/api/pricing", Label: "OpenAI API pricing", Score: 1.0},
	}, 1)
	if len(ordered) != 1 {
		t.Fatalf("expected one ordered candidate, got %d", len(ordered))
	}
	if ordered[0].URL != "https://developers.openai.com/api/pricing" {
		t.Fatalf("expected html-like candidate to win probe ordering, got %q", ordered[0].URL)
	}
	if ordered[0].Metadata["resource_class"] != "html_like" {
		t.Fatalf("expected resource class metadata, got %#v", ordered[0].Metadata)
	}
}

func TestCanonicalizeCandidateFamiliesPrefersSameHostRootWhenScoresClose(t *testing.T) {
	candidates := []DiscoverCandidate{
		{URL: "https://playwright.dev/community/welcome", Score: 1.20},
		{URL: "https://playwright.dev/", Score: 1.05},
	}
	got := webdiscover.CanonicalizeCandidateFamilies(candidates)
	if got[0].URL != "https://playwright.dev/" {
		t.Fatalf("expected host root to win after canonicalization, got %q", got[0].URL)
	}
}

func TestDampenWeakCanonicalRootTrapsUsesFamilyProvenance(t *testing.T) {
	candidates := []DiscoverCandidate{
		{
			URL:   "https://portal.example/",
			Score: 2.80,
			Reason: []string{
				"structure_hint",
				"resource_class_hint",
				"host_compactness",
				"same_host_canonical_root",
			},
		},
		{
			URL:   "https://origin.example/docs",
			Score: 2.10,
			Reason: []string{
				"same_family_child_recovery",
				"page_expand",
				"page_expand_child_context",
				"host_root_identity_probe",
			},
		},
	}
	got := webdiscover.DampenWeakProvenanceTraps(candidates)
	if got[0].URL != "https://origin.example/docs" {
		t.Fatalf("expected provenanced family to outrank weak canonical root, got %#v", got)
	}
	if !containsReason(got[1].Reason, "weak_canonical_root_context_penalty") {
		t.Fatalf("expected weak root penalty reason, got %#v", got[1].Reason)
	}
}

func TestDampenWeakCanonicalRootTrapsKeepsVerifiedRoot(t *testing.T) {
	candidates := []DiscoverCandidate{
		{
			URL:   "https://origin.example/",
			Score: 1.20,
			Reason: []string{
				"same_host_canonical_root",
				"host_root_candidate",
			},
		},
		{
			URL:   "https://origin.example/docs",
			Score: 1.10,
		},
	}
	got := webdiscover.DampenWeakProvenanceTraps(candidates)
	if got[0].URL != "https://origin.example/" {
		t.Fatalf("expected verified root to remain first, got %#v", got)
	}
	if containsReason(got[0].Reason, "weak_canonical_root_context_penalty") {
		t.Fatalf("did not expect verified root penalty, got %#v", got[0].Reason)
	}
}

func TestDampenWeakProvenanceTrapsPenalizesRecoveredFamilyWithoutEvidence(t *testing.T) {
	candidates := []DiscoverCandidate{
		{
			URL:   "https://unverified.example/app",
			Score: 2.40,
			Reason: []string{
				"same_family_child_recovery",
				"page_expand",
				"page_expand_child_context",
			},
		},
		{
			URL:   "https://origin.example/docs",
			Score: 2.12,
			Reason: []string{
				"same_family_child_recovery",
				"page_expand",
				"page_expand_child_context",
				"host_root_identity_probe",
			},
		},
	}
	got := webdiscover.DampenWeakProvenanceTraps(candidates)
	if got[0].URL != "https://origin.example/docs" {
		t.Fatalf("expected provenanced recovered family to win, got %#v", got)
	}
	if !containsReason(got[1].Reason, "weak_recovered_family_context_penalty") {
		t.Fatalf("expected weak recovered family penalty, got %#v", got[1].Reason)
	}
}

func TestExpandedRecoveryKeepsScopeRegressionSubordinate(t *testing.T) {
	svc := newTestService(t, testConfig(), nil)
	source := DiscoverCandidate{URL: "https://platform.example/org/project", Score: 1.0}
	expanded := []DiscoverCandidate{
		{URL: "https://platform.example/", Score: 1.0},
		{URL: "https://platform.example/org/project/docs", Score: 0.8},
	}

	got := svc.selectExpandedRecoveryCandidates(context.Background(), "recover contextual resource", source, expanded)
	if len(got) != 1 {
		t.Fatalf("expected expanded candidates, got %#v", got)
	}
	if got[0].URL != "https://platform.example/org/project/docs" {
		t.Fatalf("expected deeper same-family resource to stay above root regression, got %#v", got)
	}
	regressed := svc.selectExpandedRecoveryCandidates(context.Background(), "recover contextual resource", source, []DiscoverCandidate{
		{URL: "https://platform.example/", Score: 1.0},
	})
	if len(regressed) != 1 || !containsReason(regressed[0].Reason, "same_family_scope_regression") {
		t.Fatalf("expected root-only candidate to be retained as scope context, got %#v", regressed)
	}
}

func TestSemanticDisambiguateCandidateFamiliesBoostsBrandAlignedFamily(t *testing.T) {
	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc := newTestService(t, cfg, nil)

	candidates := []DiscoverCandidate{
		{
			URL:   "https://packaging-guide.openastronomy.org/en/latest/",
			Score: 1.00,
			Label: "Packaging guide",
			Metadata: map[string]string{
				"host_root_title": "OpenAstronomy packaging guide",
				"page_title":      "Packaging guide",
			},
		},
		{
			URL:   "https://packaging.python.org/en/latest/",
			Score: 0.92,
			Label: "Python Packaging User Guide",
			Metadata: map[string]string{
				"host_root_title": "Python Packaging User Guide",
				"page_title":      "Python Packaging User Guide",
			},
		},
	}

	got := svc.semanticDisambiguateCandidateFamilies(context.Background(), "python packaging", candidates)
	if got[0].URL != "https://packaging.python.org/en/latest/" {
		t.Fatalf("expected semantic family scorer to prefer python packaging family, got %q", got[0].URL)
	}
}

func TestSemanticRerankUsesProbedWebIRContext(t *testing.T) {
	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc := newTestService(t, cfg, nil)

	candidates := []DiscoverCandidate{
		{
			URL:   "https://generic.example/",
			Score: 1.10,
			Label: "Generic platform",
		},
		{
			URL:   "https://observability.example/docs",
			Score: 0.92,
			Label: "Documentation",
			Metadata: map[string]string{
				"web_ir_context": "distributed tracing metrics dashboard panels incident response observability guide",
			},
		},
	}

	got := svc.semanticRerankDiscoverCandidates(context.Background(), "distributed tracing metrics dashboard", candidates)
	if got[0].URL != "https://observability.example/docs" {
		t.Fatalf("expected probed web ir context to drive semantic rerank, got %q", got[0].URL)
	}
}

func TestApplyCandidateIntelligenceAddsSemanticGroundingMetadata(t *testing.T) {
	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc := newTestService(t, cfg, nil)

	candidates := []DiscoverCandidate{
		{
			URL:   "https://www.javascript.com/resources",
			Score: 1.92,
			Label: "JavaScript.com Resources",
			Metadata: map[string]string{
				"host_root_title": "JavaScript resources",
				"page_title":      "JavaScript.com | Resources",
				"resource_class":  "html_like",
			},
		},
		{
			URL:   "https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide",
			Score: 1.64,
			Label: "JavaScript Guide - JavaScript | MDN",
			Metadata: map[string]string{
				"host_root_title": "MDN Web Docs",
				"page_title":      "JavaScript Guide - JavaScript | MDN",
				"resource_class":  "html_like",
			},
		},
	}

	got := svc.applyCandidateIntelligence(context.Background(), "MDN JavaScript guide", candidates)
	if got[0].Metadata["candidate_goal_similarity"] == "" || got[0].Metadata["cluster_id"] == "" {
		t.Fatalf("expected candidate intelligence metadata, got %#v", got[0].Metadata)
	}
	if !containsReason(got[0].Reason, "candidate_intelligence") {
		t.Fatalf("expected candidate intelligence reason, got %#v", got[0].Reason)
	}
	if got[1].Metadata["candidate_goal_similarity"] == "" || got[1].Metadata["cluster_id"] == "" {
		t.Fatalf("expected runner-up semantic metadata, got %#v", got[1].Metadata)
	}
}

func TestDiscoverWebRecoversCanonicalFamilyFromIdentityReferences(t *testing.T) {
	officialServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprint(w, `<html><head><title>MDN Web Docs</title></head><body><h1>MDN Web Docs</h1></body></html>`)
		case "/en-US/docs/Web/JavaScript/Guide":
			_, _ = fmt.Fprint(w, `<html><head><title>JavaScript Guide - JavaScript | MDN</title></head><body><h1>Guide</h1><p>Guide body.</p></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer officialServer.Close()

	mirrorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/guide":
			_, _ = fmt.Fprintf(w, `<html><head><title>JavaScript Guide</title><link rel="canonical" href="%s/en-US/docs/Web/JavaScript/Guide"><link rel="alternate" href="%s/en-US/docs/Web/JavaScript/Guide"></head><body><h1>Guide</h1></body></html>`, officialServer.URL, officialServer.URL)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mirrorServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s/guide">JavaScript Guide</a></body></html>`, mirrorServer.URL)
	}))
	defer searchServer.Close()

	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc := newTestService(t, cfg, searchServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "MDN JavaScript guide",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != officialServer.URL+"/en-US/docs/Web/JavaScript/Guide" {
		t.Fatalf("expected canonical family recovery to win, got %q", resp.SelectedURL)
	}
}

func TestDiscoverWebSurfacesOfficialFamilyFromExternalEntityLinks(t *testing.T) {
	officialServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprint(w, `<html><head><title>Home - Comitato Olimpico Nazionale</title></head><body><h1>CONI</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer officialServer.Close()

	entityServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/entity":
			_, _ = fmt.Fprintf(w, `<html><head><title>Comitato Olimpico Nazionale Italiano</title></head><body><h1>CONI</h1><a href="%s">sito istituzionale</a></body></html>`, officialServer.URL)
		default:
			http.NotFound(w, r)
		}
	}))
	defer entityServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s/entity">Comitato Olimpico Nazionale Italiano</a></body></html>`, entityServer.URL)
	}))
	defer searchServer.Close()

	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc := newTestService(t, cfg, searchServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "official site for Comitato Olimpico Nazionale Italiano",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	found := false
	for _, candidate := range resp.Candidates {
		if candidate.URL == officialServer.URL {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected official host to surface in candidates, got %#v", resp.Candidates)
	}
}

func TestApplyCandidateIntelligencePenalizesIdentityMismatchMirror(t *testing.T) {
	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc := newTestService(t, cfg, nil)

	candidates := []DiscoverCandidate{
		{
			URL:   "https://devdoc.net/web/developer.mozilla.org/en-US/docs/Web/JavaScript/Guide/JavaScript_Overview.html",
			Score: 1.10,
			Label: "Introduction - JavaScript | MDN",
			Metadata: map[string]string{
				"host_root_title": "Developer's Documentation Collections",
				"page_title":      "Introduction - JavaScript | MDN",
				"resource_class":  "html_like",
			},
		},
		{
			URL:   "https://developer.mozilla.org/en-US/docs/Web/JavaScript",
			Score: 1.02,
			Label: "JavaScript | MDN",
			Metadata: map[string]string{
				"host_root_title": "MDN Web Docs",
				"page_title":      "JavaScript | MDN",
				"resource_class":  "html_like",
			},
		},
	}

	got := svc.applyCandidateIntelligence(context.Background(), "MDN JavaScript guide", candidates)
	if got[0].URL != "https://developer.mozilla.org/en-US/docs/Web/JavaScript" {
		t.Fatalf("expected official family to beat mirror after identity mismatch penalty, got %q", got[0].URL)
	}
	if got[1].Metadata["candidate_host_similarity"] == "" || got[1].Metadata["candidate_page_similarity"] == "" {
		t.Fatalf("expected mirror identity metadata, got %#v", got[1].Metadata)
	}
}

func TestOrderedDiscoveryProvidersPrefersHealthyProvider(t *testing.T) {
	cfg := testConfig()
	svc := newTestService(t, cfg, nil)
	svc.discoveryProviders = store.NewDiscoveryProviderStateStore(t.TempDir())
	svc.discoveryProviders.Observe(store.DiscoveryProviderObservation{
		Name:            "lite.duckduckgo.com",
		Outcome:         store.DiscoveryProviderOutcomeBlocked,
		BlockedCooldown: 10 * time.Minute,
	})
	svc.discoveryProviders.Observe(store.DiscoveryProviderObservation{
		Name:    "html.duckduckgo.com",
		Outcome: store.DiscoveryProviderOutcomeSuccess,
	})

	got := svc.orderedDiscoveryProviders([]string{
		"https://lite.duckduckgo.com/lite/",
		"https://html.duckduckgo.com/html/",
	})
	if got[0] != "https://html.duckduckgo.com/html/" {
		t.Fatalf("expected healthy provider first, got %#v", got)
	}
}

func TestDiscoverWebSkipsRemainingQueriesAfterProviderLevelFailure(t *testing.T) {
	searchHits := 0
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchHits++
		http.Error(w, "blocked", http.StatusForbidden)
	}))
	defer searchServer.Close()

	cfg := testConfig()
	svc := newTestService(t, cfg, searchServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)
	svc.discoveryProviders = store.NewDiscoveryProviderStateStore(t.TempDir())

	_, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "OpenAI API pricing",
		Queries:       []string{"openai api pricing", "openai pricing docs"},
		MaxCandidates: 5,
	})
	if err == nil {
		t.Fatal("expected discover web to fail")
	}
	if searchHits >= 6 {
		t.Fatalf("expected provider-level failure to reduce provider hits, got %d", searchHits)
	}
}

func TestDiscoverWebPersistsProviderOutcome(t *testing.T) {
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "blocked", http.StatusForbidden)
	}))
	defer searchServer.Close()

	cfg := testConfig()
	svc := newTestService(t, cfg, searchServer.Client())
	root := t.TempDir()
	svc.discoveryProviders = store.NewDiscoveryProviderStateStore(root)
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	_, _ = svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "OpenAI API pricing",
		MaxCandidates: 5,
	})
	state, err := svc.discoveryProviders.Load(discoverycore.ProviderName(searchServer.URL))
	if err != nil {
		t.Fatalf("expected provider state to be written: %v", err)
	}
	if state.BlockedCount == 0 && state.FailureCount == 0 && state.UnavailableCount == 0 {
		t.Fatalf("expected provider state to record a failure outcome, got %+v", state)
	}
}

func TestCandidateIntelligenceWindowSkipsClearLeaderWithoutProvenanceConflict(t *testing.T) {
	candidates := []DiscoverCandidate{
		{URL: "https://example.com/a", Score: 3.3},
		{URL: "https://example.com/b", Score: 1.9},
		{URL: "https://example.com/c", Score: 1.7},
	}
	if got := candidateintel.Window(candidates); got != 0 {
		t.Fatalf("expected clear leader to skip semantic review, got window=%d", got)
	}
}

func TestCandidateIntelligenceWindowKeepsProvenanceConflict(t *testing.T) {
	candidates := []DiscoverCandidate{
		{URL: "https://related.example/", Score: 3.3, Reason: []string{"same_family_canonical_root"}},
		{URL: "https://origin.example/docs", Score: 2.2, Reason: []string{"host_root_identity_probe"}},
		{URL: "https://example.com/c", Score: 1.7},
	}
	if got := candidateintel.Window(candidates); got != 3 {
		t.Fatalf("expected provenance conflict to trigger semantic review, got window=%d", got)
	}
}

func TestApplyCandidateIntelligenceChoosesClusterRepresentative(t *testing.T) {
	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc := newTestService(t, cfg, nil)

	candidates := []DiscoverCandidate{
		{
			URL:   "https://developers.openai.com/api",
			Score: 1.30,
			Label: "OpenAI API",
			Metadata: map[string]string{
				"host_root_title": "OpenAI Platform",
				"page_title":      "API Reference",
				"resource_class":  "html_like",
			},
		},
		{
			URL:   "https://developers.openai.com/api/reference/responses",
			Score: 1.31,
			Label: "Responses API",
			Metadata: map[string]string{
				"host_root_title": "OpenAI Platform",
				"page_title":      "Responses API Reference",
				"resource_class":  "html_like",
			},
		},
	}

	got := svc.applyCandidateIntelligence(context.Background(), "OpenAI API reference", candidates)
	if got[0].URL != "https://developers.openai.com/api" {
		t.Fatalf("expected shallower central family representative to win, got %q", got[0].URL)
	}
	if !containsReason(got[0].Reason, "candidate_cluster_representative") {
		t.Fatalf("expected representative reason, got %#v", got[0].Reason)
	}
	if !containsReason(got[1].Reason, "candidate_cluster_redundant") {
		t.Fatalf("expected redundant cluster reason, got %#v", got[1].Reason)
	}
}
