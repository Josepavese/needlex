package pipeline

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
