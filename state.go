package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// openState opens (or creates) the SQLite state database at path and runs the schema migration.
func openState(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open state database: %w", err)
	}
	if _, err = db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS deploys (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		target     TEXT     NOT NULL,
		commit_id  TEXT     NOT NULL,
		started_at DATETIME NOT NULL,
		succeeded  INTEGER  NOT NULL DEFAULT 0
	)`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create deploys table: %w", err)
	}
	return db, nil
}

// recordDeploy appends a deploy record for the given target.
func recordDeploy(db *sql.DB, target, commitID string, startedAt time.Time, succeeded bool) error {
	s := 0
	if succeeded {
		s = 1
	}
	_, err := db.Exec(
		`INSERT INTO deploys (target, commit_id, started_at, succeeded) VALUES (?, ?, ?, ?)`,
		target, commitID, startedAt.UTC().Format(time.RFC3339), s,
	)
	if err != nil {
		return fmt.Errorf("record deploy: %w", err)
	}
	return nil
}

// lastDeploy returns the most recent deploy record for target. ok is false if no record exists.
func lastDeploy(db *sql.DB, target string) (commitID string, succeeded bool, ok bool, err error) {
	row := db.QueryRow(
		`SELECT commit_id, succeeded FROM deploys WHERE target = ? ORDER BY id DESC LIMIT 1`,
		target,
	)
	var s int
	if err = row.Scan(&commitID, &s); err == sql.ErrNoRows {
		return "", false, false, nil
	} else if err != nil {
		return "", false, false, fmt.Errorf("query last deploy: %w", err)
	}
	return commitID, s != 0, true, nil
}
