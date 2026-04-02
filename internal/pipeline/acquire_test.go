package pipeline

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcquireFollowsRedirectAndCapturesHTML(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, "<html><body><article><p>Hello</p></article></body></html>")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	page, err := Acquirer{}.Acquire(context.Background(), AcquireInput{
		URL:      server.URL + "/start",
		Timeout:  2 * time.Second,
		MaxBytes: 4096,
	})
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	if page.FinalURL != server.URL+"/final" {
		t.Fatalf("expected final url to be redirected endpoint, got %q", page.FinalURL)
	}
	if !strings.Contains(page.HTML, "<article>") {
		t.Fatalf("expected html body to be captured, got %q", page.HTML)
	}
}

func TestAcquireRejectsOversizedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, strings.Repeat("a", 64))
	}))
	defer server.Close()

	_, err := Acquirer{}.Acquire(context.Background(), AcquireInput{
		URL:      server.URL,
		Timeout:  2 * time.Second,
		MaxBytes: 8,
	})
	if err == nil {
		t.Fatal("expected oversized body to fail")
	}
}

func TestAcquireRetriesOnceOnTimeout(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		if call == 1 {
			time.Sleep(900 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, "<html><body><p>retry success</p></body></html>")
	}))
	defer server.Close()

	page, err := Acquirer{}.Acquire(context.Background(), AcquireInput{
		URL:      server.URL,
		Timeout:  600 * time.Millisecond,
		MaxBytes: 4096,
	})
	if err != nil {
		t.Fatalf("expected retry to recover timeout, got %v", err)
	}
	if !strings.Contains(page.HTML, "retry success") {
		t.Fatalf("expected html payload after retry, got %q", page.HTML)
	}
	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Fatalf("expected retry call, got %d requests", got)
	}
}

func TestAcquireUsesBrowserLikeProfileByDefault(t *testing.T) {
	var seenUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, "<html><body><p>ok</p></body></html>")
	}))
	defer server.Close()

	page, err := Acquirer{}.Acquire(context.Background(), AcquireInput{
		URL:      server.URL,
		Timeout:  2 * time.Second,
		MaxBytes: 4096,
	})
	if err != nil {
		t.Fatalf("acquire failed: %v", err)
	}
	if page.FetchProfile != "browser_like" {
		t.Fatalf("expected browser_like fetch profile, got %q", page.FetchProfile)
	}
	if !strings.Contains(seenUserAgent, "Mozilla/5.0") {
		t.Fatalf("expected browser-like user agent, got %q", seenUserAgent)
	}
}

func TestAcquireRetriesWithRetryProfileOnBlockedStatus(t *testing.T) {
	var calls int32
	var seenUserAgents []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUserAgents = append(seenUserAgents, r.Header.Get("User-Agent"))
		call := atomic.AddInt32(&calls, 1)
		if call == 1 {
			http.Error(w, "blocked", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, "<html><body><p>ok</p></body></html>")
	}))
	defer server.Close()

	page, err := Acquirer{}.Acquire(context.Background(), AcquireInput{
		URL:          server.URL,
		Timeout:      2 * time.Second,
		MaxBytes:     4096,
		Profile:      "standard",
		RetryProfile: "browser_like",
	})
	if err != nil {
		t.Fatalf("expected blocked retry to recover, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", got)
	}
	if page.FetchProfile != "browser_like" {
		t.Fatalf("expected retry profile to win, got %q", page.FetchProfile)
	}
	if len(seenUserAgents) != 2 {
		t.Fatalf("expected 2 user agents, got %d", len(seenUserAgents))
	}
	if seenUserAgents[0] != defaultUserAgent {
		t.Fatalf("expected standard user agent first, got %q", seenUserAgents[0])
	}
	if !strings.Contains(seenUserAgents[1], "Mozilla/5.0") {
		t.Fatalf("expected browser-like user agent on retry, got %q", seenUserAgents[1])
	}
}

func TestShouldFallbackToHTTP(t *testing.T) {
	if !shouldFallbackToHTTP(errors.New(`fetch page: Get "https://sqlite.org/about.html": http2: unexpected ALPN protocol ""; want "h2"`)) {
		t.Fatal("expected ALPN mismatch to trigger HTTP fallback")
	}
	if shouldFallbackToHTTP(errors.New("fetch page: context deadline exceeded")) {
		t.Fatal("did not expect timeout to trigger HTTP fallback")
	}
}
