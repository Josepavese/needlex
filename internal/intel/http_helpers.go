package intel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultUpstreamTimeout    = 10 * time.Second
	maxModelResponseBytes     = 4 * 1024 * 1024
	maxEmbeddingResponseBytes = 16 * 1024 * 1024
)

func contextWithTimeoutMS(parent context.Context, timeoutMS int64) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if timeoutMS <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, time.Duration(timeoutMS)*time.Millisecond)
}

func httpClientOrDefault(client *http.Client, timeout time.Duration) *http.Client {
	if client != nil {
		return client
	}
	if timeout <= 0 {
		timeout = defaultUpstreamTimeout
	}
	return &http.Client{Timeout: timeout}
}

func decodeJSONLimited(body io.Reader, maxBytes int64, label string, target any) error {
	if maxBytes <= 0 {
		return fmt.Errorf("%s max response bytes must be > 0", label)
	}
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return fmt.Errorf("read %s response: %w", label, err)
	}
	if int64(len(data)) > maxBytes {
		return fmt.Errorf("%s response exceeds %d bytes", label, maxBytes)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return err
	}
	return nil
}
