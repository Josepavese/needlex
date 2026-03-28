package pipeline

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"strings"
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
