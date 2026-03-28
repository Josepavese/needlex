package pipeline

import (
	"strings"
	"testing"
)

const sampleHTML = `
<html>
  <head>
    <title>Needle X Spec</title>
  </head>
  <body>
    <header><p>brand chrome</p></header>
    <div class="cookie-banner">Accept cookies</div>
    <article>
      <h1>Main Title</h1>
      <p>First paragraph.</p>
      <p>Second paragraph.</p>
      <h2>Details</h2>
      <ul>
        <li>Point A</li>
        <li>Point B</li>
      </ul>
      <pre><code>fmt.Println("ok")</code></pre>
    </article>
    <footer><p>footer text</p></footer>
  </body>
</html>
`

func TestReduceRemovesNoiseAndKeepsMainContent(t *testing.T) {
	dom, err := Reducer{}.Reduce(RawPage{
		URL:       "https://example.com/spec",
		FinalURL:  "https://example.com/spec",
		HTML:      sampleHTML,
		FetchMode: "http",
	})
	if err != nil {
		t.Fatalf("reduce failed: %v", err)
	}
	if dom.Title != "Needle X Spec" {
		t.Fatalf("expected title, got %q", dom.Title)
	}
	if len(dom.Nodes) == 0 {
		t.Fatal("expected simplified nodes")
	}

	allText := ""
	for _, node := range dom.Nodes {
		allText += node.Text + "\n"
	}
	if strings.Contains(allText, "cookies") || strings.Contains(allText, "footer text") || strings.Contains(allText, "brand chrome") {
		t.Fatalf("expected noise to be removed, got %q", allText)
	}
	if !strings.Contains(allText, "First paragraph.") || !strings.Contains(allText, "Point A") {
		t.Fatalf("expected core content to remain, got %q", allText)
	}
}

func TestSegmentBuildsHeadingAwareSegments(t *testing.T) {
	dom, err := Reducer{}.Reduce(RawPage{
		URL:       "https://example.com/spec",
		FinalURL:  "https://example.com/spec",
		HTML:      sampleHTML,
		FetchMode: "http",
	})
	if err != nil {
		t.Fatalf("reduce failed: %v", err)
	}

	segments := Segmenter{MaxSegmentChars: 80}.Segment(dom)
	if len(segments) < 3 {
		t.Fatalf("expected multiple segments, got %d", len(segments))
	}
	if len(segments[0].HeadingPath) == 0 || segments[0].HeadingPath[0] != "Main Title" {
		t.Fatalf("expected first segment to inherit h1 path, got %#v", segments[0].HeadingPath)
	}
	last := segments[len(segments)-1]
	if last.Kind != "code" {
		t.Fatalf("expected final code segment, got %q", last.Kind)
	}
	if !strings.Contains(last.Text, `fmt.Println("ok")`) {
		t.Fatalf("expected code block content, got %q", last.Text)
	}
}

func TestReduceProfileAggressiveRemovesAdditionalNoiseHints(t *testing.T) {
	dom, err := Reducer{}.ReduceProfile(RawPage{
		URL:       "https://example.com/spec",
		FinalURL:  "https://example.com/spec",
		HTML:      `<html><body><div class="hero-banner">Hero chrome</div><article><h1>Main</h1><p>Useful core text.</p></article></body></html>`,
		FetchMode: "http",
	}, "aggressive")
	if err != nil {
		t.Fatalf("reduce failed: %v", err)
	}

	allText := ""
	for _, node := range dom.Nodes {
		allText += node.Text + "\n"
	}
	if strings.Contains(allText, "Hero chrome") {
		t.Fatalf("expected aggressive profile to remove hero content, got %q", allText)
	}
	if !strings.Contains(allText, "Useful core text.") {
		t.Fatalf("expected core content to remain, got %q", allText)
	}
}

func TestReduceExtractsEmbeddedPayloadWhenSemanticDOMIsSparse(t *testing.T) {
	dom, err := Reducer{}.ReduceProfile(RawPage{
		URL:      "https://example.com/app",
		FinalURL: "https://example.com/app",
		HTML: `<html><head><title>App Shell</title></head><body><app-root></app-root><script>window._a2s={"configuration":{"blog":[{"title":"Needle Runtime Update","description":"<p>Needle-X compiles noisy pages into compact proof-carrying context for agents.</p>"}]}}</script></body></html>`,
	}, "standard")
	if err != nil {
		t.Fatalf("reduce failed: %v", err)
	}

	if len(dom.Nodes) == 0 {
		t.Fatal("expected embedded payload nodes to be extracted")
	}
	found := false
	for _, node := range dom.Nodes {
		if strings.Contains(node.Text, "Needle-X compiles noisy pages into compact proof-carrying context for agents.") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected embedded payload text in reduced nodes, got %#v", dom.Nodes)
	}
}
