package service

import (
	"reflect"
	"testing"
)

func TestNormalizeRewriteQueriesKeepsFallbackAndDeduplicates(t *testing.T) {
	got := normalizeRewriteQueries(
		[]string{
			"dance school Alessandria",
			"ASD Charly Brown scuola di danza",
			`"ASD Charly Brown" Cassine`,
		},
		"ASD Charly Brown",
		"ASD Charly Brown dance school Alessandria",
	)
	want := []string{
		"ASD Charly Brown dance school Alessandria",
		"dance school Alessandria",
		"ASD Charly Brown scuola di danza",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized queries\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestNormalizeRewriteQueriesFallsBackWhenQueriesEmpty(t *testing.T) {
	got := normalizeRewriteQueries(
		nil,
		"ASD Charly Brown",
		"ASD Charly Brown dance school Alessandria",
	)
	want := []string{
		"ASD Charly Brown dance school Alessandria",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized queries\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestNormalizeRewriteQueriesAllowsSemanticVariantsWithoutCanonicalEntity(t *testing.T) {
	got := normalizeRewriteQueries(
		[]string{
			"official site comitato olimpico nazionale italiano",
			"sito ufficiale coni",
		},
		"",
		"official site for Comitato Olimpico Nazionale Italiano",
	)
	want := []string{
		"official site for Comitato Olimpico Nazionale Italiano",
		"official site comitato olimpico nazionale italiano",
		"sito ufficiale coni",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized queries\nwant: %#v\ngot:  %#v", want, got)
	}
}
