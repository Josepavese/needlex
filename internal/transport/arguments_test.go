package transport

import (
	"flag"
	"io"
	"reflect"
	"testing"
)

func TestInterspersedArgumentsUseTheRegisteredFlagSchema(t *testing.T) {
	for _, tc := range []struct {
		name        string
		args        []string
		path        string
		limit       int
		json        bool
		positionals []string
		fails       bool
	}{
		{"values_before_boolean", []string{"source", "--path", "a path", "--limit", "-3", "--json"}, "a path", -3, true, []string{"source"}, false},
		{"assigned_values", []string{"--path=a=b", "source", "--json=false", "--limit=0"}, "a=b", 0, false, []string{"source"}, false},
		{"terminator", []string{"source", "--json", "--", "--path", "literal", "-1"}, "", 5, true, []string{"source", "--path", "literal", "-1"}, false},
		{"single_dash_and_flag_like_value", []string{"-", "-path", "--json"}, "--json", 5, false, []string{"-"}, false},
		{"negative_value_before_boolean", []string{"--limit", "-1", "--json", "source"}, "", -1, true, []string{"source"}, false},
		{"boolean_keeps_positional", []string{"--json", "false"}, "", 5, true, []string{"false"}, false},
		{"missing_value", []string{"--path"}, "", 5, false, nil, true},
		{"unknown_flag", []string{"--unknown", "value"}, "", 5, false, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("fixture", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			path := fs.String("path", "", "")
			limit := fs.Int("limit", 5, "")
			json := fs.Bool("json", false, "")
			original := append([]string{}, tc.args...)
			err := fs.Parse(normalizeArgs(fs, tc.args))
			if !reflect.DeepEqual(tc.args, original) {
				t.Fatal("argument ownership violated")
			}
			if (err != nil) != tc.fails {
				t.Fatalf("unexpected parse result: %v", err)
			}
			if !tc.fails && (*path != tc.path || *limit != tc.limit || *json != tc.json || !reflect.DeepEqual(fs.Args(), tc.positionals)) {
				t.Fatalf("values changed: path=%q limit=%d json=%t positionals=%v", *path, *limit, *json, fs.Args())
			}
		})
	}
}
