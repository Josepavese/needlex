package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/pipeline"
)

var wordPattern = regexp.MustCompile(`\S+`)

func TestReadGoldenArticleStandard(t *testing.T) {
	resp := runGoldenRead(t, "article.html", ReadRequest{
		Profile:   core.ProfileStandard,
		Objective: "proof replay deterministic context",
	})

	if resp.Document.Title != "Needle-X Runtime Deep Dive" {
		t.Fatalf("unexpected title %q", resp.Document.Title)
	}
	if len(resp.ResultPack.Chunks) == 0 {
		t.Fatal("expected chunks to be populated")
	}
	if len(resp.ResultPack.Chunks) > 6 {
		t.Fatalf("standard profile should keep at most 6 chunks, got %d", len(resp.ResultPack.Chunks))
	}
	if len(resp.ResultPack.Outline) < 2 {
		t.Fatalf("expected outline to preserve headings, got %#v", resp.ResultPack.Outline)
	}
	if !containsLine(resp.ResultPack.Outline, "Deterministic Core") {
		t.Fatalf("expected deterministic core in outline, got %#v", resp.ResultPack.Outline)
	}
	if containsChunkText(resp.ResultPack.Chunks, "newsletter noise") {
		t.Fatal("expected footer noise to be pruned")
	}
	if containsChunkText(resp.ResultPack.Chunks, "Home Docs Pricing") {
		t.Fatal("expected nav noise to be pruned")
	}
	if len(resp.ProofRecords) != len(resp.ResultPack.Chunks) {
		t.Fatalf("expected proof count to match chunks, got %d and %d", len(resp.ProofRecords), len(resp.ResultPack.Chunks))
	}
	if len(resp.ResultPack.Links) != 1 {
		t.Fatalf("expected one canonical link, got %d", len(resp.ResultPack.Links))
	}
}

func TestReadGoldenForumDeep(t *testing.T) {
	resp := runGoldenRead(t, "forum.html", ReadRequest{
		Profile: core.ProfileDeep,
	})

	if resp.ResultPack.Profile != core.ProfileDeep {
		t.Fatalf("expected deep profile, got %q", resp.ResultPack.Profile)
	}
	if len(resp.ResultPack.Chunks) != 3 {
		t.Fatalf("expected deep profile to keep all 3 forum chunks, got %d", len(resp.ResultPack.Chunks))
	}
	if !containsLine(resp.ResultPack.Outline, "Troubleshooting") {
		t.Fatalf("expected troubleshooting heading, got %#v", resp.ResultPack.Outline)
	}
	if !containsChunkText(resp.ResultPack.Chunks, "Compare stage hashes first.") {
		t.Fatal("expected troubleshooting text in chunks")
	}
	if resp.Replay.StageCount != 4 {
		t.Fatalf("expected 4 stages, got %d", resp.Replay.StageCount)
	}
	if !resp.Replay.Deterministic {
		t.Fatal("expected replay report to remain deterministic")
	}
}

func TestGoldenNFRDeterminismArticle(t *testing.T) {
	html := loadGoldenHTML(t, "article.html")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, html)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	req := ReadRequest{
		URL:       server.URL,
		Profile:   core.ProfileStandard,
		Objective: "proof replay deterministic context",
	}
	first, err := svc.Read(context.Background(), req)
	if err != nil {
		t.Fatalf("first read failed: %v", err)
	}
	second, err := svc.Read(context.Background(), req)
	if err != nil {
		t.Fatalf("second read failed: %v", err)
	}

	if !reflect.DeepEqual(first.ResultPack, second.ResultPack) {
		t.Fatal("expected identical result packs across repeated runs")
	}
	if !reflect.DeepEqual(first.ProofRecords, second.ProofRecords) {
		t.Fatal("expected identical proof records across repeated runs")
	}
	if !first.Replay.Deterministic || !second.Replay.Deterministic {
		t.Fatal("expected replay reports to remain deterministic")
	}
}

func TestGoldenNFRCompressionRatioTiny(t *testing.T) {
	resp := runGoldenRead(t, "article.html", ReadRequest{
		Profile:   core.ProfileTiny,
		Objective: "proof replay deterministic context",
	})
	rawHTML := loadGoldenHTML(t, "article.html")
	packed := joinChunkText(resp.ResultPack.Chunks)

	ratio := compressionRatio(rawHTML, packed)
	if ratio < 3.0 {
		t.Fatalf("expected compression ratio baseline >= 3.0, got %.2f", ratio)
	}
}

func TestGoldenNFRFidelityScoreStandard(t *testing.T) {
	resp := runGoldenRead(t, "article.html", ReadRequest{
		Profile:   core.ProfileStandard,
		Objective: "proof replay deterministic context",
	})

	score := fidelityScore(resp.ResultPack.Chunks, []string{
		"Needle-X compiles noisy public pages into compact proof-carrying context for agents.",
		"The runtime reduces HTML into a stable intermediate representation before ranking and packing.",
		"Replay and diff keep every extraction auditable and locally inspectable without a backend.",
		"Small language models activate only when ambiguity survives deterministic pruning.",
	})
	if score < 1.0 {
		t.Fatalf("expected fidelity score 1.0 on golden article, got %.2f", score)
	}
}

func TestGoldenQueryDiscoveryImprovesSignal(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Portal</title></head><body><article><h1>Portal</h1><p>Overview page.</p><a href="%s/docs/replay-proof">Replay Proof Guide</a><a href="%s/blog">Blog</a></article></body></html>`, serverURL, serverURL)
		case "/docs/replay-proof":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Proof Guide</title></head><body><article><h1>Replay Proof Guide</h1><p>Proof replay deterministic context for operators.</p><p>Inspect proof records and compare replay traces.</p></article></body></html>`)
		case "/blog":
			_, _ = fmt.Fprint(w, `<html><head><title>Blog</title></head><body><article><h1>Blog</h1><p>Company updates.</p></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	seedOnly, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       server.URL,
		Profile:       core.ProfileTiny,
		DiscoveryMode: QueryDiscoveryOff,
	})
	if err != nil {
		t.Fatalf("seed-only query failed: %v", err)
	}

	discoverFirst, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       server.URL,
		Profile:       core.ProfileTiny,
		DiscoveryMode: QueryDiscoverySameSite,
	})
	if err != nil {
		t.Fatalf("discover-first query failed: %v", err)
	}

	expected := []string{
		"Proof replay deterministic context for operators.",
		"Inspect proof records and compare replay traces.",
	}
	seedScore := fidelityScore(seedOnly.ResultPack.Chunks, expected)
	discoverScore := fidelityScore(discoverFirst.ResultPack.Chunks, expected)

	if discoverFirst.Plan.SelectedURL == seedOnly.Plan.SelectedURL {
		t.Fatalf("expected discovery to choose a different page, got %q", discoverFirst.Plan.SelectedURL)
	}
	if discoverScore <= seedScore {
		t.Fatalf("expected discover-first fidelity %.2f to exceed seed-only %.2f", discoverScore, seedScore)
	}
}

func TestGoldenNeedlexBeatsNaiveBaselineOnSignalDensity(t *testing.T) {
	rawHTML := loadGoldenHTML(t, "article.html")
	objective := "proof replay deterministic context"
	needle := runGoldenRead(t, "article.html", ReadRequest{
		Profile:   core.ProfileTiny,
		Objective: objective,
	})
	needleText := joinChunkText(needle.ResultPack.Chunks)
	baselineText := naiveBaselineText(rawHTML)

	needleDensity := objectiveSignalDensity(needleText, objective)
	baselineDensity := objectiveSignalDensity(baselineText, objective)

	if needleDensity <= baselineDensity {
		t.Fatalf("expected needle signal density %.4f to exceed baseline %.4f", needleDensity, baselineDensity)
	}
	if compressionRatio(rawHTML, needleText) <= compressionRatio(rawHTML, baselineText) {
		t.Fatal("expected needle compression ratio to exceed naive baseline")
	}
}

func TestGoldenNeedlexBeatsReducedBaselineOnSignalDensity(t *testing.T) {
	rawHTML := loadGoldenHTML(t, "article.html")
	objective := "proof replay deterministic context"
	needle := runGoldenRead(t, "article.html", ReadRequest{
		Profile:   core.ProfileTiny,
		Objective: objective,
	})
	needleText := joinChunkText(needle.ResultPack.Chunks)
	reducedText := reducedBaselineText(rawHTML)

	needleDensity := objectiveSignalDensity(needleText, objective)
	reducedDensity := objectiveSignalDensity(reducedText, objective)

	if needleDensity <= reducedDensity {
		t.Fatalf("expected needle signal density %.4f to exceed reduced baseline %.4f", needleDensity, reducedDensity)
	}
	if compressionRatio(rawHTML, needleText) <= compressionRatio(rawHTML, reducedText) {
		t.Fatal("expected needle compression ratio to exceed reduced baseline")
	}
}

func BenchmarkReadGoldenArticle(b *testing.B) {
	html := loadGoldenHTML(b, "article.html")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, html)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		b.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := svc.Read(context.Background(), ReadRequest{
			URL:     server.URL,
			Profile: core.ProfileStandard,
		})
		if err != nil {
			b.Fatalf("read failed: %v", err)
		}
	}
}

func BenchmarkNaiveBaselineGoldenArticle(b *testing.B) {
	htmlText := loadGoldenHTML(b, "article.html")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		text := naiveBaselineText(htmlText)
		if text == "" {
			b.Fatal("baseline extract returned empty text")
		}
	}
}

func BenchmarkReducedBaselineGoldenArticle(b *testing.B) {
	htmlText := loadGoldenHTML(b, "article.html")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		text := reducedBaselineText(htmlText)
		if text == "" {
			b.Fatal("reduced baseline extract returned empty text")
		}
	}
}

func BenchmarkExternalBaselineGoldenArticle(b *testing.B) {
	htmlText := loadGoldenHTML(b, "article.html")

	if _, ok := externalBaselineCommand(); !ok {
		b.Skip("external baseline command not configured")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		text, err := externalBaselineText(htmlText)
		if err != nil {
			b.Fatalf("external baseline failed: %v", err)
		}
		if text == "" {
			b.Fatal("external baseline extract returned empty text")
		}
	}
}

func BenchmarkQueryGoldenArticle(b *testing.B) {
	html := loadGoldenHTML(b, "article.html")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, html)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		b.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := svc.Query(context.Background(), QueryRequest{
			Goal:    "proof replay deterministic",
			SeedURL: server.URL,
			Profile: core.ProfileStandard,
		})
		if err != nil {
			b.Fatalf("query failed: %v", err)
		}
	}
}

func BenchmarkQuerySeedOnly(b *testing.B) {
	benchmarkQueryModes(b, QueryDiscoveryOff)
}

func BenchmarkQueryDiscoverFirst(b *testing.B) {
	benchmarkQueryModes(b, QueryDiscoverySameSite)
}

func BenchmarkCrawlGoldenArticle(b *testing.B) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Home</title></head><body><article><h1>Home</h1><p>Seed page.</p><a href="%s/docs">Docs</a></article></body></html>`, serverURL)
		case "/docs":
			_, _ = fmt.Fprint(w, `<html><head><title>Docs</title></head><body><article><h1>Docs</h1><p>Linked docs page.</p></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		b.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := svc.Crawl(context.Background(), CrawlRequest{
			SeedURL:    server.URL,
			Profile:    core.ProfileTiny,
			MaxPages:   2,
			MaxDepth:   1,
			SameDomain: true,
		})
		if err != nil {
			b.Fatalf("crawl failed: %v", err)
		}
	}
}

func runGoldenRead(t *testing.T, fixture string, req ReadRequest) ReadResponse {
	t.Helper()

	html := loadGoldenHTML(t, fixture)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, html)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	req.URL = server.URL
	resp, err := svc.Read(context.Background(), req)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	return resp
}

func benchmarkQueryModes(b *testing.B, mode string) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Portal</title></head><body><article><h1>Portal</h1><p>Overview page.</p><a href="%s/docs/replay-proof">Replay Proof Guide</a><a href="%s/blog">Blog</a></article></body></html>`, serverURL, serverURL)
		case "/docs/replay-proof":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Proof Guide</title></head><body><article><h1>Replay Proof Guide</h1><p>Proof replay deterministic context for operators.</p><p>Inspect proof records and compare replay traces.</p></article></body></html>`)
		case "/blog":
			_, _ = fmt.Fprint(w, `<html><head><title>Blog</title></head><body><article><h1>Blog</h1><p>Company updates.</p></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		b.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := svc.Query(context.Background(), QueryRequest{
			Goal:          "proof replay deterministic",
			SeedURL:       server.URL,
			Profile:       core.ProfileTiny,
			DiscoveryMode: mode,
		})
		if err != nil {
			b.Fatalf("query failed: %v", err)
		}
	}
}

type testingReader interface {
	Helper()
	Fatalf(format string, args ...any)
}

func loadGoldenHTML(t testingReader, fixture string) string {
	t.Helper()

	path := filepath.Join("..", "..", "..", "testdata", "golden", fixture)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixture, err)
	}
	return string(data)
}

func containsChunkText(chunks []core.Chunk, needle string) bool {
	for _, chunk := range chunks {
		if strings.Contains(chunk.Text, needle) {
			return true
		}
	}
	return false
}

func containsLine(lines []string, needle string) bool {
	for _, line := range lines {
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}

func compressionRatio(rawHTML, packed string) float64 {
	packedTokens := tokenCount(packed)
	if packedTokens == 0 {
		return 0
	}
	return float64(tokenCount(rawHTML)) / float64(packedTokens)
}

func fidelityScore(chunks []core.Chunk, expected []string) float64 {
	if len(expected) == 0 {
		return 1
	}
	matched := 0
	for _, needle := range expected {
		if containsChunkText(chunks, needle) {
			matched++
		}
	}
	return float64(matched) / float64(len(expected))
}

func joinChunkText(chunks []core.Chunk) string {
	lines := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		lines = append(lines, chunk.Text)
	}
	return strings.Join(lines, "\n")
}

func naiveBaselineText(rawHTML string) string {
	root, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return ""
	}
	body := findHTMLNode(root, "body")
	if body == nil {
		body = root
	}
	var parts []string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			text := normalizeTestWhitespace(node.Data)
			if text != "" {
				parts = append(parts, text)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(body)
	return strings.Join(parts, " ")
}

func reducedBaselineText(rawHTML string) string {
	dom, err := pipeline.Reducer{}.ReduceProfile(pipeline.RawPage{
		URL:       "https://example.com/article",
		FinalURL:  "https://example.com/article",
		HTML:      rawHTML,
		FetchMode: core.FetchModeHTTP,
	}, "standard")
	if err != nil {
		return ""
	}
	segments := pipeline.Segmenter{MaxSegmentChars: 1200}.Segment(dom)
	if len(segments) == 0 {
		return ""
	}
	parts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if strings.TrimSpace(segment.Text) == "" {
			continue
		}
		parts = append(parts, segment.Text)
	}
	return strings.Join(parts, "\n")
}

func externalBaselineText(rawHTML string) (string, error) {
	command, ok := externalBaselineCommand()
	if !ok {
		return "", fmt.Errorf("external baseline command not configured")
	}

	cmd := exec.Command("bash", "-lc", command)
	cmd.Stdin = strings.NewReader(rawHTML)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func externalBaselineCommand() (string, bool) {
	command := strings.TrimSpace(os.Getenv("NEEDLEX_EXTERNAL_BASELINE_CMD"))
	if command == "" {
		return "", false
	}
	return command, true
}

func findHTMLNode(node *html.Node, name string) *html.Node {
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, name) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		found := findHTMLNode(child, name)
		if found != nil {
			return found
		}
	}
	return nil
}

func normalizeTestWhitespace(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func tokenCount(text string) int {
	return len(wordPattern.FindAllString(text, -1))
}

func signalDensity(text string, expected []string) float64 {
	tokens := tokenCount(text)
	if tokens == 0 {
		return 0
	}
	matches := 0
	lower := strings.ToLower(text)
	for _, needle := range expected {
		if strings.Contains(lower, strings.ToLower(needle)) {
			matches++
		}
	}
	return float64(matches) / float64(tokens)
}

func objectiveSignalDensity(text, objective string) float64 {
	tokens := tokenCount(text)
	if tokens == 0 {
		return 0
	}
	matches := 0
	lower := strings.ToLower(text)
	for _, token := range uniqueTokens(objective) {
		if strings.Contains(lower, token) {
			matches++
		}
	}
	return float64(matches) / float64(tokens)
}

func TestGoldenNFRMetricsShape(t *testing.T) {
	resp := runGoldenRead(t, "forum.html", ReadRequest{
		Profile:   core.ProfileTiny,
		Objective: "replay drift troubleshooting",
	})
	report := map[string]any{
		"compression_ratio":          compressionRatio(loadGoldenHTML(t, "forum.html"), joinChunkText(resp.ResultPack.Chunks)),
		"baseline_compression_ratio": compressionRatio(loadGoldenHTML(t, "forum.html"), naiveBaselineText(loadGoldenHTML(t, "forum.html"))),
		"deterministic":              resp.Replay.Deterministic,
		"fidelity_score": fidelityScore(resp.ResultPack.Chunks, []string{
			"Compare stage hashes first.",
			"inspect proof records by chunk id",
		}),
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal metrics report: %v", err)
	}
	if !strings.Contains(string(data), "compression_ratio") {
		t.Fatal("expected metrics report to include compression_ratio")
	}
}
