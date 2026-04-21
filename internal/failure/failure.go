package failure

import (
	"context"
	"errors"
	"strings"
)

type Class string

const (
	ClassRuntimeError           Class = "runtime_error"
	ClassProviderBlocked        Class = "provider_blocked"
	ClassUpstreamNotFound       Class = "upstream_not_found"
	ClassUpstreamTimeout        Class = "upstream_timeout"
	ClassUnsupportedContentType Class = "unsupported_content_type"
	ClassEmptyCandidates        Class = "empty_candidates"
	ClassUnavailableUpstream    Class = "unavailable_upstream"
)

func (c Class) String() string {
	if strings.TrimSpace(string(c)) == "" {
		return string(ClassRuntimeError)
	}
	return string(c)
}

func Classify(err error) Class {
	if err == nil {
		return ClassRuntimeError
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ClassUpstreamTimeout
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case containsAny(text, "unexpected status code 403", "unexpected status code 429", "anti-bot", "rate limit", "blocked", "bootstrap provider returned 403", "bootstrap provider returned 429"):
		return ClassProviderBlocked
	case containsAny(text, "unexpected status code 404", "bootstrap provider returned 404", " status code 404"):
		return ClassUpstreamNotFound
	case containsAny(text, "timeout", "deadline exceeded", "client.timeout exceeded", "i/o timeout"):
		return ClassUpstreamTimeout
	case containsAny(text, "unsupported content type"):
		return ClassUnsupportedContentType
	case containsAny(text, "empty candidates", "no candidates", "returned no candidates"):
		return ClassEmptyCandidates
	case containsAny(text, "provider unavailable", "not configured", "no such host", "connection refused", "connection reset"):
		return ClassUnavailableUpstream
	default:
		return ClassRuntimeError
	}
}

func IsProviderLevel(c Class) bool {
	switch c {
	case ClassProviderBlocked, ClassUpstreamTimeout, ClassUnavailableUpstream:
		return true
	default:
		return false
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
