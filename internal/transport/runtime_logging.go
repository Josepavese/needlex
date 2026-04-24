package transport

import (
	"fmt"
	"io"
	"strings"

	"github.com/josepavese/needlex/internal/core/failure"
	"github.com/josepavese/needlex/internal/observability"
)

func (r Runner) runtimeLogger() observability.Logger {
	return observability.NewLogger(r.storeRoot)
}

func (r Runner) logRuntimeError(operation, eventName string, err error, fields map[string]any) observability.Event {
	logger := r.runtimeLogger()
	failureClass := failure.Classify(err).String()
	event, logErr := logger.Write(observability.Event{
		Level:        observability.LevelError,
		Component:    "transport",
		Surface:      "cli",
		Operation:    operation,
		Event:        firstNonEmptyRuntimeString(eventName, "runtime.error"),
		Message:      cleanLogMessage(operation, err),
		Error:        errorString(err),
		FailureClass: failureClass,
		Fields:       fields,
	})
	if logErr != nil {
		return observability.Event{
			ID:           "log_unavailable",
			Level:        observability.LevelError,
			Component:    "transport",
			Surface:      "cli",
			Operation:    operation,
			Event:        "runtime.log_unavailable",
			Message:      logErr.Error(),
			FailureClass: failureClass,
		}
	}
	return event
}

func (r Runner) logRuntimeWarning(operation, eventName, message string, fields map[string]any) observability.Event {
	logger := r.runtimeLogger()
	event, logErr := logger.Write(observability.Event{
		Level:     observability.LevelWarn,
		Component: "transport",
		Surface:   "runtime",
		Operation: operation,
		Event:     firstNonEmptyRuntimeString(eventName, "runtime.warning"),
		Message:   firstNonEmptyRuntimeString(message, operation+" warning"),
		Fields:    fields,
	})
	if logErr != nil {
		return observability.Event{ID: "log_unavailable", Event: "runtime.log_unavailable", Message: logErr.Error()}
	}
	return event
}

func (r Runner) logRuntimeInfo(operation, eventName, message string, fields map[string]any) observability.Event {
	logger := r.runtimeLogger()
	event, logErr := logger.Write(observability.Event{
		Level:     observability.LevelInfo,
		Component: "transport",
		Surface:   "runtime",
		Operation: operation,
		Event:     firstNonEmptyRuntimeString(eventName, "runtime.info"),
		Message:   firstNonEmptyRuntimeString(message, operation+" event"),
		Fields:    fields,
	})
	if logErr != nil {
		return observability.Event{ID: "log_unavailable", Event: "runtime.log_unavailable", Message: logErr.Error()}
	}
	return event
}

func (r Runner) logMCPError(operation, eventName string, err error, fields map[string]any) observability.Event {
	logger := r.runtimeLogger()
	event, logErr := logger.Write(observability.Event{
		Level:        observability.LevelError,
		Component:    "mcp",
		Surface:      "mcp",
		Operation:    operation,
		Event:        firstNonEmptyRuntimeString(eventName, "mcp.error"),
		Message:      cleanLogMessage(operation, err),
		Error:        errorString(err),
		FailureClass: failure.Classify(err).String(),
		Fields:       fields,
	})
	if logErr != nil {
		return observability.Event{ID: "log_unavailable", Event: "runtime.log_unavailable", Message: logErr.Error()}
	}
	return event
}

func (r Runner) reportCLIError(stderr io.Writer, operation string, err error, fields map[string]any) int {
	return r.reportCLIErrorCode(stderr, operation, err, fields, 1)
}

func (r Runner) reportCLIErrorCode(stderr io.Writer, operation string, err error, fields map[string]any, code int) int {
	event := r.logRuntimeError(operation, "runtime.error", err, fields)
	class := failure.Classify(err).String()
	logPath := r.runtimeLogger().Path()
	fmt.Fprintf(stderr, "%s failed. class=%s diagnostic_id=%s log=%s\n", operation, class, event.ID, logPath)
	return code
}

func cleanLogMessage(operation string, err error) string {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = "operation"
	}
	if err == nil {
		return operation + " failed"
	}
	return operation + " failed"
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmptyRuntimeString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
