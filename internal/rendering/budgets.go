package rendering

import (
	"strings"
	"time"
)

const (
	browserContextCleanupTimeout       = time.Second
	dumpDOMVirtualTime                 = 3 * time.Second
	cdpEnvelopeHeadroomBytes     int64 = 8_000_000
)

func renderSnapshotBudget(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 5 * time.Second
	}
	if timeout < 10*time.Second {
		return max(time.Second, timeout/4)
	}
	return 5 * time.Second
}

func firstNonEmptyRenderString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
