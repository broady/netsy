// Netsy <https://netsy.dev>
// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package localdb

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (db *database) Connect() error {
	if db.file == "" {
		return errors.New("db file path not configured")
	}

	// check directory exists
	dbDir := filepath.Dir(db.file)
	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		if err := os.Mkdir(dbDir, 0750); err != nil {
			return fmt.Errorf("error creating database directory %s: %w", dbDir, err)
		}
	}

	// connect
	conn, err := sql.Open("sqlite3", db.file)
	if err != nil {
		return err
	}
	db.conn = conn

	// Set busy timeout so concurrent writers retry instead of failing immediately
	// with SQLITE_BUSY. 5 seconds is enough for short write transactions.
	if _, err := conn.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return fmt.Errorf("failed to set busy_timeout: %w", err)
	}

	// Bound the connection pool. WAL mode allows concurrent readers but
	// only one writer; limiting connections prevents resource exhaustion.
	conn.SetMaxOpenConns(4)
	conn.SetMaxIdleConns(4)

	// Enable WAL mode for better concurrency (allows reads during writes)
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Ensure every commit is fsynced to disk (critical for Receipt durability)
	if _, err := conn.Exec("PRAGMA synchronous=FULL"); err != nil {
		return fmt.Errorf("failed to set synchronous=FULL: %w", err)
	}

	// Ensure schema is correctly applied
	if err := db.checkSchemaVersion(); err != nil {
		return err
	}
	if err := db.applyMigrations(); err != nil {
		return err
	}

	return nil
}
