package platform

import (
	"context"
	"database/sql"
	"fmt"
)

func ConfigureSQLite(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("sqlite db is nil")
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, stmt := range []string{
		`PRAGMA busy_timeout=5000`,
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA foreign_keys=ON`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("configure sqlite: %w", err)
		}
	}
	return nil
}
