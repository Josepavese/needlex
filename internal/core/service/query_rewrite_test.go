package service

import (
	"reflect"
	"testing"
)

func TestNormalizeRewriteQueriesPreservesCanonicalEntity(t *testing.T) {
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
		"ASD Charly Brown scuola di danza",
		`"ASD Charly Brown" Cassine`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized queries\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestNormalizeRewriteQueriesDropsGenericQueries(t *testing.T) {
	got := normalizeRewriteQueries(
		[]string{
			"dance school Alessandria",
			"dance classes Alessandria",
		},
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
