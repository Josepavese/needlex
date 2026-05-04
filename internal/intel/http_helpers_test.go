package intel

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDecodeJSONLimitedRejectsOversizedResponse(t *testing.T) {
	var payload map[string]any
	err := decodeJSONLimited(strings.NewReader(strings.Repeat("x", int(maxModelResponseBytes)+1)), maxModelResponseBytes, "model backend", &payload)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversized response error, got %v", err)
	}
}

func TestOpenAICompatibleRuntimeEnforcesRequestTimeoutWithCustomClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"confidence\":0.5}"}}]}`))
	}))
	defer server.Close()

	runtime := OpenAICompatibleRuntime{
		BaseURL: server.URL,
		Client:  server.Client(),
		Models:  RuntimeModels{MicroSolver: "micro"},
	}
	_, err := runtime.Run(context.Background(), ModelRequest{
		Task:            TaskQueryRewrite,
		ModelClass:      ModelClassMicroSolver,
		MaxInputTokens:  100,
		MaxOutputTokens: 100,
		TimeoutMS:       20,
		SchemaName:      "query_rewrite",
		Input:           map[string]any{"goal": "docs"},
	})
	if err == nil {
		t.Fatal("expected request timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "context deadline exceeded") && !errors.Is(err, io.EOF) {
		t.Fatalf("expected context deadline failure, got %v", err)
	}
}
