package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/targetkind"
	"github.com/josepavese/needlex/internal/intel"
)

type targetKindTestAligner struct {
	target string
}

func (a targetKindTestAligner) Align(context.Context, string, []intel.SemanticCandidate) (intel.SemanticAlignment, error) {
	return intel.SemanticAlignment{}, nil
}

func (a targetKindTestAligner) Score(_ context.Context, _ string, candidates []intel.SemanticCandidate) ([]intel.SemanticScore, error) {
	out := make([]intel.SemanticScore, 0, len(candidates))
	for _, candidate := range candidates {
		score := 0.10
		if strings.HasPrefix(candidate.ID, "http://") || strings.HasPrefix(candidate.ID, "https://") {
			out = append(out, intel.SemanticScore{ID: candidate.ID, Similarity: score})
			continue
		}
		if targetkind.PrototypeKind(candidate.ID) == a.target {
			score = 0.92
		}
		out = append(out, intel.SemanticScore{ID: candidate.ID, Similarity: score})
	}
	return out, nil
}

type localFirstSeedTestAligner struct {
	prototypeScores map[string]float64
	candidateScores map[string]float64
}

func (a localFirstSeedTestAligner) Align(context.Context, string, []intel.SemanticCandidate) (intel.SemanticAlignment, error) {
	return intel.SemanticAlignment{}, nil
}

func (a localFirstSeedTestAligner) Score(_ context.Context, _ string, candidates []intel.SemanticCandidate) ([]intel.SemanticScore, error) {
	out := make([]intel.SemanticScore, 0, len(candidates))
	for _, candidate := range candidates {
		score := 0.10
		if value, ok := a.candidateScores[candidate.ID]; ok {
			score = value
		} else if kind := targetkind.PrototypeKind(candidate.ID); kind != "" {
			if value, ok := a.prototypeScores[kind]; ok {
				score = value
			}
		}
		out = append(out, intel.SemanticScore{ID: candidate.ID, Similarity: score})
	}
	return out, nil
}

func TestExtractLinkCandidatesPreservesLabels(t *testing.T) {
	links := extractLinkCandidates(
		`<html><body><a href="/docs/replay">Replay Guide</a><a href="https://external.example/page">External</a></body></html>`,
		"https://example.com",
		true,
	)
	if len(links) != 1 {
		t.Fatalf("expected 1 same-domain candidate, got %d", len(links))
	}
	if links[0].URL != "https://example.com/docs/replay" {
		t.Fatalf("unexpected candidate url %q", links[0].URL)
	}
	if links[0].Label != "Replay Guide" {
		t.Fatalf("unexpected candidate label %q", links[0].Label)
	}
}

func TestDiscoverChoosesBestGoalMatch(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Portal</title></head><body><article><h1>Portal</h1><a href="%s/blog">Blog</a><a href="%s/docs/replay">Replay Guide</a></article></body></html>`, serverURL, serverURL)
		case "/docs/replay":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Guide</title></head><body><article><h1>Replay</h1></article></body></html>`)
		case "/blog":
			_, _ = fmt.Fprint(w, `<html><head><title>Blog</title></head><body><article><h1>Blog</h1></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL
	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc, err := New(cfg, server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	resp, err := svc.Discover(context.Background(), DiscoverRequest{
		Goal:       "proof replay deterministic",
		SeedURL:    server.URL,
		SameDomain: true,
	})
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if resp.SelectedURL != server.URL+"/docs/replay" {
		t.Fatalf("expected replay guide to win, got %q", resp.SelectedURL)
	}
	if len(resp.Candidates) < 3 {
		t.Fatalf("expected seed plus links, got %#v", resp.Candidates)
	}
}

func TestDiscoverSameSiteSpecificityPriorRoutesWithSemanticEndpoint(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Portal</title></head><body><article><h1>Portal</h1><a href="%s/blog">Blog</a><a href="%s/docs/replay">Replay Guide</a></article></body></html>`, serverURL, serverURL)
		case "/docs/replay":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Guide</title></head><body><article><h1>Replay</h1></article></body></html>`)
		case "/blog":
			_, _ = fmt.Fprint(w, `<html><head><title>Blog</title></head><body><article><h1>Blog</h1></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc, err := New(cfg, server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	resp, err := svc.Discover(context.Background(), DiscoverRequest{
		Goal:                   "proof replay deterministic",
		SeedURL:                server.URL,
		SameDomain:             true,
		PreferSpecificSameSite: true,
	})
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if resp.SelectedURL != server.URL+"/docs/replay" {
		t.Fatalf("expected same-site route to beat seed without semantic support, got %q", resp.SelectedURL)
	}
	if !containsReason(resp.Candidates[0].Reason, "same_site_specific_route") {
		t.Fatalf("expected same-site specificity reason, got %#v", resp.Candidates[0].Reason)
	}
}

func TestDiscoverTargetKindKeepsBroadIdentityOnHome(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Atlas Project</title></head><body><article><h1>Atlas Project</h1><a href="%s/products/cloud">Cloud</a><a href="%s/resources/case-studies">Case studies</a></article></body></html>`, serverURL, serverURL)
		case "/products/cloud":
			_, _ = fmt.Fprint(w, `<html><head><title>Atlas Cloud</title></head><body><article><h1>Cloud</h1></article></body></html>`)
		case "/resources/case-studies":
			_, _ = fmt.Fprint(w, `<html><head><title>Atlas Case Studies</title></head><body><article><h1>Case studies</h1></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	cfg := testConfig()
	svc := newTestService(t, cfg, server.Client())
	svc.semantic = targetKindTestAligner{target: targetkind.CanonicalHome}
	resp, err := svc.Discover(context.Background(), DiscoverRequest{
		Goal:                   "capire l'identità complessiva del progetto",
		SeedURL:                server.URL,
		SameDomain:             true,
		PreferSpecificSameSite: true,
	})
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if resp.SelectedURL != server.URL {
		t.Fatalf("expected broad identity goal to keep root page, got %q with candidates %#v", resp.SelectedURL, resp.Candidates)
	}
	if !containsReason(resp.Candidates[0].Reason, "target_kind_topology_alignment") {
		t.Fatalf("expected target-kind topology reason, got %#v", resp.Candidates[0].Reason)
	}
}

func TestDiscoverSameSiteUsesNativeContextFallbackForSiblingChoice(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>SQLite</title></head><body><a href="%s/fiddle/index.html">Fiddle</a><a href="%s/download.html">Download</a></body></html>`, serverURL, serverURL)
		case "/download.html":
			_, _ = fmt.Fprint(w, `<html><head><title>Download</title></head><body><article><h1>Download</h1></article></body></html>`)
		case "/fiddle/index.html":
			_, _ = fmt.Fprint(w, `<html><head><title>Fiddle</title></head><body><article><h1>Fiddle</h1></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc, err := New(cfg, server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	resp, err := svc.Discover(context.Background(), DiscoverRequest{
		Goal:                   "download",
		SeedURL:                server.URL,
		SameDomain:             true,
		PreferSpecificSameSite: true,
	})
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if resp.SelectedURL != server.URL+"/download.html" {
		t.Fatalf("expected semantic context to select download route, got %q with candidates %#v", resp.SelectedURL, resp.Candidates)
	}
	if !containsReason(resp.Candidates[0].Reason, "semantic_goal_alignment") {
		t.Fatalf("expected semantic alignment reason, got %#v", resp.Candidates[0].Reason)
	}
}

func TestScoreDiscoveryCandidatesBoostsDomainHint(t *testing.T) {
	candidates := discoverycore.ScoreStructuralCandidates(
		"",
		"",
		[]discoverycore.LinkCandidate{
			{URL: "https://other.example.com/company", Label: "Official site"},
			{URL: "https://preferred.example.com/company", Label: "Official site"},
		},
		[]string{"preferred.example.com"},
	)
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].URL != "https://preferred.example.com/company" {
		t.Fatalf("expected preferred domain to be boosted, got %q", candidates[0].URL)
	}
	if !containsReason(candidates[0].Reason, "domain_identity_match") {
		t.Fatalf("expected domain_identity_match reason, got %#v", candidates[0].Reason)
	}
}

func TestExtractSearchResultsResolvesRedirectTargets(t *testing.T) {
	results := discoverycore.ExtractSearchResults(
		`<html><body><a class="result__a" href="/l/?uddg=https%3A%2F%2Fdocs.example.com%2Freplay">Replay Docs</a></body></html>`,
		"https://html.duckduckgo.com/html/?q=replay",
	)
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].URL != "https://docs.example.com/replay" {
		t.Fatalf("unexpected resolved url %q", results[0].URL)
	}
	if results[0].Label != "Replay Docs" {
		t.Fatalf("unexpected label %q", results[0].Label)
	}
}

func TestExtractSearchResultsIgnoresNonResultAnchors(t *testing.T) {
	results := discoverycore.ExtractSearchResults(
		`<html><body><a href="https://duckduckgo.com/duckduckgo-help-pages/company/contact-us">Contact</a><a class="result__a" href="/l/?uddg=https%3A%2F%2Ftangotoday.net%2Fscuole%2Fasd-charly-brown%2F">ASD Charly Brown - TangoToday</a><a href="https://duckduckgo.com/duckduckgo-help-pages/resources/opt-out-guides">Opt out</a></body></html>`,
		"https://html.duckduckgo.com/html/?q=asd+charly+brown",
	)
	if len(results) != 1 {
		t.Fatalf("expected only one real result, got %d: %#v", len(results), results)
	}
	if results[0].URL != "https://tangotoday.net/scuole/asd-charly-brown/" {
		t.Fatalf("unexpected resolved url %q", results[0].URL)
	}
}

func TestExtractSearchResultsParsesLiteRedirectResults(t *testing.T) {
	results := discoverycore.ExtractSearchResults(
		`<html><body><a href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fprolocosantagostino.it%2Fasd%2Dcharly%2Dbrown%2F&amp;rut=abc">ASD Charly Brown - Cassine, Provincia di Alessandria</a><a href="/lite/?q=next">Next Page</a></body></html>`,
		"https://lite.duckduckgo.com/lite/?q=asd+charly+brown",
	)
	if len(results) != 1 {
		t.Fatalf("expected one lite result, got %d: %#v", len(results), results)
	}
	if results[0].URL != "https://prolocosantagostino.it/asd-charly-brown/" {
		t.Fatalf("unexpected lite resolved url %q", results[0].URL)
	}
}

func TestDiscoverWebChoosesBestCrossSiteCandidate(t *testing.T) {
	docsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Replay Proof Guide</title></head><body><article><h1>Replay Proof Guide</h1></article></body></html>`)
	}))
	defer docsServer.Close()

	blogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Blog</title></head><body><article><h1>Blog</h1></article></body></html>`)
	}))
	defer blogServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Company Blog</a><a class="result__a" href="%s">Replay Proof Guide</a></body></html>`, blogServer.URL, docsServer.URL)
	}))
	defer searchServer.Close()

	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc, err := New(cfg, searchServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://seed.example/root",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != docsServer.URL {
		t.Fatalf("expected docs candidate to win, got %q", resp.SelectedURL)
	}
	if resp.Provider == "" {
		t.Fatal("expected provider name")
	}
}

func TestDiscoverWebReranksUsingFetchedPageTitle(t *testing.T) {
	docsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Replay Proof Guide</title></head><body><article><h1>Guide</h1></article></body></html>`)
	}))
	defer docsServer.Close()

	blogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Company Updates</title></head><body><article><h1>Blog</h1></article></body></html>`)
	}))
	defer blogServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Official Site</a><a class="result__a" href="%s">Official Site</a></body></html>`, blogServer.URL, docsServer.URL)
	}))
	defer searchServer.Close()

	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc, err := New(cfg, searchServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://seed.example/root",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != docsServer.URL {
		t.Fatalf("expected fetched page title rerank to prefer docs candidate, got %q", resp.SelectedURL)
	}
	if len(resp.Candidates) == 0 || !containsReason(resp.Candidates[0].Reason, "page_title_probe") {
		t.Fatalf("expected top candidate to include page_title_probe reason, got %#v", resp.Candidates)
	}
	if resp.Candidates[0].Metadata["web_ir_node_count"] == "" {
		t.Fatalf("expected web ir metadata on top candidate, got %#v", resp.Candidates[0].Metadata)
	}
}

func TestDiscoverWebExpandsLandingPageToBetterChild(t *testing.T) {
	var portalURL string
	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Portal</title></head><body><article><h1>Portal</h1><a href="%s/docs/replay-proof">Replay Proof Guide</a></article></body></html>`, portalURL)
		case "/docs/replay-proof":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Proof Guide</title></head><body><article><h1>Replay Proof Guide</h1></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer portalServer.Close()
	portalURL = portalServer.URL

	blogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Blog</title></head><body><article><h1>Blog</h1></article></body></html>`)
	}))
	defer blogServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Official Site</a><a class="result__a" href="%s">News</a></body></html>`, portalServer.URL, blogServer.URL)
	}))
	defer searchServer.Close()

	svc, err := New(testConfig(), searchServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://seed.example/root",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != portalServer.URL+"/docs/replay-proof" {
		t.Fatalf("expected expansion to choose child docs page, got %q", resp.SelectedURL)
	}
	if len(resp.Candidates) == 0 || !containsReason(resp.Candidates[0].Reason, "page_expand") {
		t.Fatalf("expected top candidate to include page_expand reason, got %#v", resp.Candidates)
	}
}

func TestDiscoverWebMergesMultipleProviders(t *testing.T) {
	docsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Replay Proof Guide</title></head><body><article><h1>Replay Proof Guide</h1></article></body></html>`)
	}))
	defer docsServer.Close()

	blogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Blog</title></head><body><article><h1>Blog</h1></article></body></html>`)
	}))
	defer blogServer.Close()

	searchOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Company Blog</a></body></html>`, blogServer.URL)
	}))
	defer searchOne.Close()

	searchTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Replay Proof Guide</a></body></html>`, docsServer.URL)
	}))
	defer searchTwo.Close()

	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc, err := New(cfg, searchOne.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	svc.SetWebDiscoverBaseURL(searchOne.URL + "," + searchTwo.URL)

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://seed.example/root",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != docsServer.URL {
		t.Fatalf("expected merged providers to surface docs candidate, got %q", resp.SelectedURL)
	}
	if !strings.Contains(resp.Provider, discoverycore.ProviderName(searchOne.URL)) || !strings.Contains(resp.Provider, discoverycore.ProviderName(searchTwo.URL)) {
		t.Fatalf("expected combined provider names, got %q", resp.Provider)
	}
}

func TestDiscoverWebUsesLocalSubstrateBeforeWebBootstrap(t *testing.T) {
	var seedURL string
	seedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><head><title>Portal</title></head><body><article><a href="%s/docs/replay">Replay Guide</a></article></body></html>`, seedURL)
	}))
	defer seedServer.Close()
	seedURL = seedServer.URL

	searchHits := 0
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchHits++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><body><a class="result__a" href="https://external.example/replay">Replay</a></body></html>`)
	}))
	defer searchServer.Close()

	cfg := testConfig()
	enableDiscoverSemantic(&cfg, "")
	svc, err := New(cfg, seedServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       seedServer.URL,
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.Provider != "local_same_site" {
		t.Fatalf("expected local substrate provider, got %q", resp.Provider)
	}
	if resp.SelectedURL != seedServer.URL+"/docs/replay" {
		t.Fatalf("expected local docs selection, got %q", resp.SelectedURL)
	}
	if searchHits != 0 {
		t.Fatalf("expected web bootstrap not used, got hits=%d", searchHits)
	}
}

func TestDiscoverWebLocalFirstPreservesNonRootSeedForWeakHomeIntent(t *testing.T) {
	var seedURL string
	seedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/docs/Web/JavaScript":
			_, _ = fmt.Fprintf(w, `<html><head><title>JavaScript | MDN</title></head><body><article><h1>JavaScript</h1><a href="%s/docs/Web/JavaScript/Reference/Classes/Private_elements">Private elements</a></article></body></html>`, seedURL)
		case "/docs/Web/JavaScript/Reference/Classes/Private_elements":
			_, _ = fmt.Fprint(w, `<html><head><title>Private elements</title></head><body><article><h1>Private elements</h1></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer seedServer.Close()
	seedURL = seedServer.URL

	searchHits := 0
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchHits++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><body><a class="result__a" href="https://external.example/javascript">JavaScript</a></body></html>`)
	}))
	defer searchServer.Close()

	cfg := testConfig()
	svc := newTestService(t, cfg, seedServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)
	svc.semantic = localFirstSeedTestAligner{
		prototypeScores: map[string]float64{
			targetkind.CanonicalHome: 0.48,
			targetkind.LearningPath:  0.44,
		},
		candidateScores: map[string]float64{
			seedServer.URL + "/docs/Web/JavaScript":                                    0.62,
			seedServer.URL + "/docs/Web/JavaScript/Reference/Classes/Private_elements": 0.62,
		},
	}

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "understand the JavaScript landing page",
		SeedURL:       seedServer.URL + "/docs/Web/JavaScript",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != seedServer.URL+"/docs/Web/JavaScript" {
		t.Fatalf("expected weak home-like intent to preserve non-root seed, got %q", resp.SelectedURL)
	}
	if len(resp.Candidates) == 0 || !containsReason(resp.Candidates[0].Reason, "semantic_seed_preserved") {
		t.Fatalf("expected semantic seed preservation reason, got %#v", resp.Candidates)
	}
	if searchHits != 0 {
		t.Fatalf("expected web bootstrap not used, got hits=%d", searchHits)
	}
}

func TestDiscoverWebLocalFirstPromotesSemanticallyStrongerChild(t *testing.T) {
	var seedURL string
	seedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/docs":
			_, _ = fmt.Fprintf(w, `<html><head><title>Docs</title></head><body><article><h1>Docs</h1><a href="%s/docs/replay">Replay Guide</a></article></body></html>`, seedURL)
		case "/docs/replay":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Guide</title></head><body><article><h1>Replay Guide</h1></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer seedServer.Close()
	seedURL = seedServer.URL

	searchHits := 0
	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchHits++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><body><a class="result__a" href="https://external.example/replay">Replay</a></body></html>`)
	}))
	defer searchServer.Close()

	cfg := testConfig()
	svc := newTestService(t, cfg, seedServer.Client())
	svc.SetWebDiscoverBaseURL(searchServer.URL)
	svc.semantic = localFirstSeedTestAligner{
		candidateScores: map[string]float64{
			seedServer.URL + "/docs":        0.22,
			seedServer.URL + "/docs/replay": 0.74,
		},
	}

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       seedServer.URL + "/docs",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != seedServer.URL+"/docs/replay" {
		t.Fatalf("expected semantically stronger child to win, got %q", resp.SelectedURL)
	}
	if searchHits != 0 {
		t.Fatalf("expected web bootstrap not used, got hits=%d", searchHits)
	}
}

func containsReason(reasons []string, needle string) bool {
	for _, reason := range reasons {
		if reason == needle {
			return true
		}
	}
	return false
}
