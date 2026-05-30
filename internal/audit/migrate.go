// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"database/sql"
	"fmt"
)

type migration func(tx *sql.Tx) error

var migrations = []migration{
	migrateV1Baseline,
}

func migrateV1Baseline(tx *sql.Tx) error {
	_, err := tx.Exec(SchemaSQL)
	return err
}

func migrateDB(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("creating schema_version table: %w", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_version`).Scan(&count); err != nil {
		return fmt.Errorf("reading schema_version count: %w", err)
	}
	if count == 0 {
		if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (0)`); err != nil {
			return fmt.Errorf("initializing schema_version: %w", err)
		}
	}

	var current int
	if err := db.QueryRow(`SELECT version FROM schema_version`).Scan(&current); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	for i := current; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("beginning migration %d: %w", i+1, err)
		}

		if err := migrations[i](tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}

		if _, err := tx.Exec(`UPDATE schema_version SET version = ?`, i+1); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("updating schema_version to %d: %w", i+1, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", i+1, err)
		}
	}

	return nil
}
