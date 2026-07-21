package rendering

import (
	"strings"
	"time"
)

func renderSnapshotBudget(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 5 * time.Second
	}
	if timeout < 10*time.Second {
		return maxDuration(time.Second, timeout/4)
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
