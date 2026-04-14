package platform

import (
	"database/sql"
	"errors"
	"io"
)

func Close(closer io.Closer) {
	if closer == nil {
		return
	}
	_ = closer.Close()
}

func Rollback(tx interface{ Rollback() error }) {
	if tx == nil {
		return
	}
	err := tx.Rollback()
	if errors.Is(err, sql.ErrTxDone) {
		return
	}
}

func Flush(flusher interface{ Flush() error }) {
	if flusher == nil {
		return
	}
	_ = flusher.Flush()
}
