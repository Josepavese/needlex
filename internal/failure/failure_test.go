package failure

import (
	"context"
	"errors"
	"testing"
)

func TestClassifyFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want Class
	}{
		{"nil", nil, ClassRuntimeError},
		{"deadline", context.DeadlineExceeded, ClassUpstreamTimeout},
		{"blocked", errors.New("bootstrap provider returned 429"), ClassProviderBlocked},
		{"not found", errors.New("unexpected status code 404"), ClassUpstreamNotFound},
		{"unsupported", errors.New("unsupported content type application/pdf"), ClassUnsupportedContentType},
		{"empty", errors.New("discover web returned no candidates"), ClassEmptyCandidates},
		{"unavailable", errors.New("brave api key not configured"), ClassUnavailableUpstream},
		{"other", errors.New("decode payload"), ClassRuntimeError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.err); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestIsProviderLevel(t *testing.T) {
	for _, class := range []Class{ClassProviderBlocked, ClassUpstreamTimeout, ClassUnavailableUpstream} {
		if !IsProviderLevel(class) {
			t.Fatalf("expected %q to be provider-level", class)
		}
	}
	if IsProviderLevel(ClassEmptyCandidates) {
		t.Fatal("empty candidates is not a provider-level transport failure")
	}
}
