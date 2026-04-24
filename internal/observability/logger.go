package observability

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/josepavese/needlex/internal/platform"
)

const (
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"

	defaultMaxBytes = 5 * 1024 * 1024
	defaultMaxFiles = 5
)

type Event struct {
	ID           string         `json:"id"`
	TimestampUTC string         `json:"ts_utc"`
	Level        string         `json:"level"`
	Component    string         `json:"component,omitempty"`
	Surface      string         `json:"surface,omitempty"`
	Operation    string         `json:"operation,omitempty"`
	Event        string         `json:"event"`
	Message      string         `json:"message"`
	Error        string         `json:"error,omitempty"`
	FailureClass string         `json:"failure_class,omitempty"`
	Fields       map[string]any `json:"fields,omitempty"`
}

type Logger struct {
	path     string
	maxBytes int64
	maxFiles int
}

type Options struct {
	Path     string
	MaxBytes int64
	MaxFiles int
}

type LogStats struct {
	Path       string    `json:"path"`
	Exists     bool      `json:"exists"`
	SizeBytes  int64     `json:"size_bytes"`
	LineCount  int       `json:"line_count"`
	Rotated    []string  `json:"rotated"`
	LastEvent  Event     `json:"last_event,omitempty"`
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`
}

func NewLogger(root string) Logger {
	layout := platform.NewStateLayout(root)
	return NewLoggerWithOptions(Options{Path: layout.RuntimeLog})
}

func NewLoggerWithOptions(opts Options) Logger {
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		path = platform.NewStateLayout("").RuntimeLog
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = defaultMaxFiles
	}
	return Logger{path: path, maxBytes: maxBytes, maxFiles: maxFiles}
}

func (l Logger) Path() string { return l.path }

func (l *Logger) Write(event Event) (Event, error) {
	if l == nil {
		return Event{}, errors.New("nil logger")
	}
	logMu.Lock()
	defer logMu.Unlock()

	normalized := normalizeEvent(event)
	data, err := json.Marshal(normalized)
	if err != nil {
		return Event{}, fmt.Errorf("marshal log event: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return Event{}, fmt.Errorf("create log dir: %w", err)
	}
	if err := l.rotateIfNeeded(int64(len(data))); err != nil {
		return Event{}, err
	}
	file, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return Event{}, fmt.Errorf("open runtime log: %w", err)
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Write(data); err != nil {
		return Event{}, fmt.Errorf("write runtime log: %w", err)
	}
	return normalized, nil
}

var logMu sync.Mutex

func (l Logger) Stats() (LogStats, error) {
	logMu.Lock()
	defer logMu.Unlock()

	stats := LogStats{Path: l.path}
	info, err := os.Stat(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			stats.Rotated = rotatedPaths(l.path, l.maxFiles)
			return stats, nil
		}
		return stats, err
	}
	stats.Exists = true
	stats.SizeBytes = info.Size()
	file, err := os.Open(l.path)
	if err != nil {
		return stats, err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		stats.LineCount++
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err == nil {
			stats.LastEvent = event
			if parsed, err := time.Parse(time.RFC3339Nano, event.TimestampUTC); err == nil {
				stats.LastSeenAt = parsed
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return stats, err
	}
	stats.Rotated = rotatedPaths(l.path, l.maxFiles)
	return stats, nil
}

func (l Logger) Tail(limit int) ([]Event, error) {
	logMu.Lock()
	defer logMu.Unlock()

	if limit <= 0 {
		limit = 50
	}
	file, err := os.Open(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()
	ring := make([]Event, 0, limit)
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if len(ring) == limit {
			copy(ring, ring[1:])
			ring[len(ring)-1] = event
			continue
		}
		ring = append(ring, event)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return ring, nil
}

func (l Logger) rotateIfNeeded(incoming int64) error {
	if l.maxBytes <= 0 {
		return nil
	}
	info, err := os.Stat(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat runtime log: %w", err)
	}
	if info.Size()+incoming <= l.maxBytes {
		return nil
	}
	for i := l.maxFiles - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", l.path, i)
		to := fmt.Sprintf("%s.%d", l.path, i+1)
		if _, err := os.Stat(from); err == nil {
			_ = os.Rename(from, to)
		}
	}
	if l.maxFiles > 0 {
		_ = os.Rename(l.path, l.path+".1")
		return nil
	}
	return os.Remove(l.path)
}

func normalizeEvent(event Event) Event {
	if strings.TrimSpace(event.ID) == "" {
		event.ID = newEventID()
	}
	if strings.TrimSpace(event.TimestampUTC) == "" {
		event.TimestampUTC = time.Now().UTC().Format(time.RFC3339Nano)
	}
	event.Level = firstNonEmpty(strings.ToLower(strings.TrimSpace(event.Level)), LevelInfo)
	event.Event = firstNonEmpty(event.Event, "runtime.event")
	event.Message = redactAndLimit(firstNonEmpty(event.Message, event.Event), 4096)
	event.Error = redactAndLimit(event.Error, 8192)
	event.FailureClass = redactAndLimit(event.FailureClass, 256)
	event.Component = redactAndLimit(event.Component, 128)
	event.Surface = redactAndLimit(event.Surface, 128)
	event.Operation = redactAndLimit(event.Operation, 128)
	if len(event.Fields) > 0 {
		out := make(map[string]any, len(event.Fields))
		for key, value := range event.Fields {
			out[redactAndLimit(key, 128)] = sanitizeValue(value)
		}
		event.Fields = out
	}
	return event
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return redactAndLimit(typed, 4096)
	case fmt.Stringer:
		return redactAndLimit(typed.String(), 4096)
	case error:
		return redactAndLimit(typed.Error(), 4096)
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactAndLimit(item, 1024))
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(typed))
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			out[redactAndLimit(key, 128)] = redactAndLimit(typed[key], 2048)
		}
		return out
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return redactAndLimit(fmt.Sprint(typed), 4096)
		}
		if len(data) > 4096 {
			return redactAndLimit(string(data[:4096]), 4096)
		}
		return json.RawMessage(data)
	}
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password)(\s*[:=]\s*)["']?[^\s,"'}]+`),
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(bearer\s+)?[^\s,"'}]+`),
	regexp.MustCompile(`(?i)([?&](?:api[_-]?key|token|secret|password)=)[^&#\s]+`),
}

func RedactString(value string) string {
	out := value
	for _, pattern := range secretPatterns {
		out = pattern.ReplaceAllString(out, `$1$2[REDACTED]`)
	}
	return out
}

func redactAndLimit(value string, max int) string {
	value = strings.TrimSpace(RedactString(value))
	if max > 0 && len(value) > max {
		return value[:max] + "..."
	}
	return value
}

func rotatedPaths(path string, maxFiles int) []string {
	out := []string{}
	for i := 1; i <= maxFiles; i++ {
		candidate := fmt.Sprintf("%s.%d", path, i)
		if _, err := os.Stat(candidate); err == nil {
			out = append(out, candidate)
		}
	}
	return out
}

func newEventID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return "log_" + hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("log_%d", time.Now().UTC().UnixNano())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
