// Package localapi provides a local database-backed store that mirrors the
// lovyou.ai REST API, enabling fully offline operation of the hive pipeline.
package localapi

import "database/sql"

// Migrate creates (or verifies) the local tables needed by the store.
// Safe to call on every startup — all statements use IF NOT EXISTS.
func Migrate(db *sql.DB) error {
	return MigrateIfLocal(db, "local_nodes")
}

// MigrateIfLocal creates the auxiliary tables (agents, diagnostics) and,
// when tableName is "local_nodes", also creates the local_nodes table.
// When tableName is "nodes" (site DB) the nodes table already exists so
// only the auxiliary tables are created.
func MigrateIfLocal(db *sql.DB, tableName string) error {
	var stmts []string

	if tableName == "local_nodes" {
		stmts = append(stmts,
			`CREATE TABLE IF NOT EXISTS local_nodes (
				id            TEXT PRIMARY KEY,
				space_id      TEXT NOT NULL DEFAULT 'hive',
				parent_id     TEXT,
				kind          TEXT NOT NULL DEFAULT 'task',
				title         TEXT NOT NULL DEFAULT '',
				body          TEXT NOT NULL DEFAULT '',
				state         TEXT NOT NULL DEFAULT 'open',
				priority      TEXT NOT NULL DEFAULT 'medium',
				assignee      TEXT,
				assignee_id   TEXT,
				author        TEXT,
				author_id     TEXT,
				author_kind   TEXT,
				due_date      TEXT,
				pinned        BOOLEAN NOT NULL DEFAULT false,
				created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
				updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_local_nodes_space_kind ON local_nodes (space_id, kind)`,
			`CREATE INDEX IF NOT EXISTS idx_local_nodes_state ON local_nodes (state)`,
			`CREATE INDEX IF NOT EXISTS idx_local_nodes_parent ON local_nodes (parent_id)`,
		)
	}

	stmts = append(stmts,
		`CREATE TABLE IF NOT EXISTS local_agents (
			name        TEXT PRIMARY KEY,
			display     TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			category    TEXT NOT NULL DEFAULT '',
			prompt      TEXT NOT NULL DEFAULT '',
			model       TEXT NOT NULL DEFAULT 'sonnet',
			active      BOOLEAN NOT NULL DEFAULT true,
			session_id  TEXT NOT NULL DEFAULT ''
		)`,

		`CREATE TABLE IF NOT EXISTS local_diagnostics (
			id         SERIAL PRIMARY KEY,
			payload    JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	)

	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
