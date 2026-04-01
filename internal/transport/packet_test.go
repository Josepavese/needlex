package transport

import (
	"strings"
	"testing"

	coreservice "github.com/josepavese/needlex/internal/core/service"
)

func TestCompactReadResponseLimitsChunksAndCleansDisplay(t *testing.T) {
	resp := fakeResponse()
	resp.Document.Title = " \u200bNeedle Runtime "
	resp.ResultPack.Outline = []string{"\u200b Usage \u200b>\u200b FAQ ", " Docs "}
	resp.AgentContext.Title = resp.Document.Title
	resp.AgentContext.Chunks = []coreservice.AgentChunk{
		{Text: "one", HeadingPath: []string{"\u200b Usage", " Intro "}, Confidence: 0.95},
		{Text: "two", HeadingPath: []string{"Two"}, Confidence: 0.94},
		{Text: "three", HeadingPath: []string{"Three"}, Confidence: 0.93},
		{Text: "four", HeadingPath: []string{"Four"}, Confidence: 0.92},
		{Text: "five", HeadingPath: []string{"Five"}, Confidence: 0.91},
		{Text: "six", HeadingPath: []string{"Six"}, Confidence: 0.90},
	}

	compact := compactReadResponse(resp)
	if compact.Title != "Needle Runtime" {
		t.Fatalf("unexpected cleaned title %q", compact.Title)
	}
	if len(compact.Chunks) != 5 {
		t.Fatalf("expected compact chunk limit 5, got %d", len(compact.Chunks))
	}
	if got := compact.Chunks[0].HeadingPath[0]; got != "Usage" {
		t.Fatalf("unexpected cleaned heading %q", got)
	}
	if strings.Contains(compact.Outline[0], "\u200b") {
		t.Fatalf("expected outline to be cleaned, got %q", compact.Outline[0])
	}
}

func TestCompactChunkSelectionPrefersDiverseChunks(t *testing.T) {
	chunks := []coreservice.AgentChunk{
		{Text: "Alpha product overview with core capabilities and rollout details.", HeadingPath: []string{"Overview"}},
		{Text: "Alpha product overview with core capabilities and rollout details. Extended duplicate tail.", HeadingPath: []string{"Overview"}},
		{Text: "Deployment options for on-prem and cloud environments.", HeadingPath: []string{"Deployment"}},
	}

	selected := compactChunkSelection(chunks)
	if len(selected) != 2 {
		t.Fatalf("expected redundant compact chunk to be dropped, got %d", len(selected))
	}
	if cleanDisplayString(selected[0].Text) != cleanDisplayString(chunks[0].Text) {
		t.Fatalf("expected first chunk to be retained")
	}
	if cleanDisplayString(selected[1].Text) != cleanDisplayString(chunks[2].Text) {
		t.Fatalf("expected diverse chunk to be promoted ahead of duplicate, got %q", selected[1].Text)
	}
}

func TestCompactChunkSelectionDropsLowUtilityTailAfterStrongAnchor(t *testing.T) {
	chunks := []coreservice.AgentChunk{
		{
			Text:        "SQLite is a database engine used across many devices and applications. It is small, fast, self-contained, and reliable for embedded and local workloads.",
			HeadingPath: []string{"SQLite Home Page"},
		},
		{
			Text:        "SQLite is made possible in part by sponsors and consortium members. This page was last updated on 2026-03-16 20:07:10Z.",
			HeadingPath: []string{"Latest Release", "Common Links", "Sponsors"},
		},
		{
			Text:        "Features When to use SQLite Getting Started Try it live SQL Syntax Pragmas SQL functions Date and time functions Aggregate functions Window functions JSON functions",
			HeadingPath: []string{"Latest Release", "Common Links"},
		},
	}

	selected := compactChunkSelection(chunks)
	if len(selected) != 2 {
		t.Fatalf("expected low-utility tail chunk to be dropped, got %d", len(selected))
	}
	if cleanDisplayString(selected[1].Text) != cleanDisplayString(chunks[2].Text) {
		t.Fatalf("expected utility-preserving chunk to remain, got %q", selected[1].Text)
	}
}

func TestCompactChunkSelectionDropsNavLikeTokenLineAfterStrongAnchor(t *testing.T) {
	chunks := []coreservice.AgentChunk{
		{
			Text:        "SQLite is a database engine used across many devices and applications. It is small, fast, self-contained, and reliable for embedded and local workloads.",
			HeadingPath: []string{"SQLite Home Page"},
		},
		{
			Text:        "Home Menu About Documentation Download License Support Purchase Search About Documentation Download Support Purchase",
			HeadingPath: []string{"SQLite Home Page"},
		},
		{
			Text:        "Features When to use SQLite Getting Started Try it live SQL Syntax Pragmas SQL functions Date and time functions Aggregate functions Window functions JSON functions",
			HeadingPath: []string{"Latest Release", "Common Links"},
		},
	}

	selected := compactChunkSelection(chunks)
	if len(selected) != 2 {
		t.Fatalf("expected nav-like token line to be dropped, got %d", len(selected))
	}
	if cleanDisplayString(selected[1].Text) != cleanDisplayString(chunks[2].Text) {
		t.Fatalf("expected docs-index chunk to remain, got %q", selected[1].Text)
	}
}

func TestDeriveSummaryPrefixesInformativeTitleForShortHead(t *testing.T) {
	chunks := []coreservice.AgentChunk{
		{Text: "367 releases over 25.6 years. This page was last updated on 2026-03-14 10:12:28Z"},
	}

	got := deriveSummary("History Of SQLite Releases", chunks)
	if !strings.HasPrefix(got, "History Of SQLite Releases.") {
		t.Fatalf("expected informative title prefix, got %q", got)
	}
}

func TestCleanDisplayStringTightensPunctuation(t *testing.T) {
	got := cleanDisplayString("small , fast , self-contained .")
	if got != "small, fast, self-contained." {
		t.Fatalf("unexpected cleaned string %q", got)
	}
}
