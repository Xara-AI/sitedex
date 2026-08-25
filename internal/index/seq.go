package index

import (
	"database/sql"
	"fmt"
)

// nextSeq hands out the next value from seq_counter, incrementing it in
// the same transaction as the row write it stamps — so a page or product
// upsert and its seq assignment commit or roll back together. Safe without
// extra locking because the DB is opened with SetMaxOpenConns(1): this
// process never has two writers racing on the same index.db.
func nextSeq(tx *sql.Tx) (int64, error) {
	if _, err := tx.Exec(`UPDATE seq_counter SET value = value + 1 WHERE id = 1`); err != nil {
		return 0, fmt.Errorf("bump seq_counter: %w", err)
	}
	var v int64
	if err := tx.QueryRow(`SELECT value FROM seq_counter WHERE id = 1`).Scan(&v); err != nil {
		return 0, fmt.Errorf("read seq_counter: %w", err)
	}
	return v, nil
}
