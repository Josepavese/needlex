package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/platform"
)

func TestSQLiteStoreOpenAppliesOperationalPragmas(t *testing.T) {
	store := NewSQLiteStore(t.TempDir(), "discovery/discovery.db")
	conn, err := store.open(context.Background())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer platform.Close(conn)

	var busyTimeout int
	if err := conn.QueryRowContext(context.Background(), `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if busyTimeout < 5000 {
		t.Fatalf("expected busy_timeout >= 5000, got %d", busyTimeout)
	}

	var journalMode string
	if err := conn.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("expected WAL journal mode, got %q", journalMode)
	}
}
