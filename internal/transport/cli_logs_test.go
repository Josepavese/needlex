package transport

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
)

func TestRunnerWritesCleanRuntimeLogOnCLIError(t *testing.T) {
	root := t.TempDir()
	runner := NewRunner()
	runner.storeRoot = root
	runner.read = func(context.Context, config.Config, coreservice.ReadRequest) (coreservice.ReadResponse, error) {
		return coreservice.ReadResponse{}, errors.New("unexpected status code 403 TOKEN=super-secret")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"read", "https://example.com", "--json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should stay clean, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "diagnostic_id=") || strings.Contains(stderr.String(), "super-secret") || strings.Contains(stderr.String(), "unexpected status") {
		t.Fatalf("stderr should be concise and redacted, got %q", stderr.String())
	}

	events, err := runner.runtimeLogger().Tail(1)
	if err != nil {
		t.Fatalf("tail logs: %v", err)
	}
	if len(events) != 1 || events[0].Operation != "read" || events[0].FailureClass != "provider_blocked" {
		t.Fatalf("unexpected log events: %+v", events)
	}
	if strings.Contains(events[0].Error, "super-secret") || !strings.Contains(events[0].Error, "[REDACTED]") {
		t.Fatalf("expected redacted log error, got %q", events[0].Error)
	}
}

func TestRunnerLogsCommandReportsStatsAndTail(t *testing.T) {
	root := t.TempDir()
	runner := NewRunner()
	runner.storeRoot = root
	_ = runner.logRuntimeError("query", "runtime.error", errors.New("no candidates"), map[string]any{"seed_url": "https://example.com"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"logs", "stats"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("logs stats exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Runtime Log:") || !strings.Contains(stdout.String(), "Events: 1") {
		t.Fatalf("unexpected logs stats: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = runner.Run([]string{"logs", "tail", "--limit", "1", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("logs tail exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"operation": "query"`) {
		t.Fatalf("unexpected logs tail json: %q", stdout.String())
	}
}

func TestRunnerPanicRecoveryWritesRuntimeLog(t *testing.T) {
	root := t.TempDir()
	runner := NewRunner()
	runner.storeRoot = root
	runner.read = func(context.Context, config.Config, coreservice.ReadRequest) (coreservice.ReadResponse, error) {
		panic("TOKEN=crash-secret")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runner.Run([]string{"read", "https://example.com"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should stay clean, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "diagnostic_id=") || strings.Contains(stderr.String(), "crash-secret") {
		t.Fatalf("stderr should contain only diagnostic pointer, got %q", stderr.String())
	}

	events, err := runner.runtimeLogger().Tail(1)
	if err != nil {
		t.Fatalf("tail logs: %v", err)
	}
	if len(events) != 1 || events[0].Event != "runtime.panic" || !strings.Contains(events[0].Error, "[REDACTED]") {
		t.Fatalf("unexpected panic log event: %+v", events)
	}
}
