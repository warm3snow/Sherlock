// Copyright 2024 Sherlock Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package history

import (
	"database/sql"
	"fmt"
)

// Migration represents a database migration.
type Migration struct {
	Version     int
	Description string
	Up          string
}

// migrations defines all database migrations in order.
var migrations = []Migration{
	{
		Version:     1,
		Description: "Add alias and description to hosts, create groups and tags tables",
		Up: `
			-- Add new columns to hosts table
			ALTER TABLE hosts ADD COLUMN alias TEXT;
			ALTER TABLE hosts ADD COLUMN description TEXT;

			-- Create unique index for alias
			CREATE UNIQUE INDEX IF NOT EXISTS idx_hosts_alias ON hosts(alias) WHERE alias IS NOT NULL;

			-- Create host_groups table
			CREATE TABLE IF NOT EXISTS host_groups (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL UNIQUE,
				description TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);

			-- Create host_group_members table (many-to-many)
			CREATE TABLE IF NOT EXISTS host_group_members (
				host_id INTEGER NOT NULL,
				group_id INTEGER NOT NULL,
				PRIMARY KEY (host_id, group_id),
				FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE,
				FOREIGN KEY (group_id) REFERENCES host_groups(id) ON DELETE CASCADE
			);

			-- Create tags table
			CREATE TABLE IF NOT EXISTS tags (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL UNIQUE,
				color TEXT DEFAULT '#808080'
			);

			-- Create host_tags table (many-to-many)
			CREATE TABLE IF NOT EXISTS host_tags (
				host_id INTEGER NOT NULL,
				tag_id INTEGER NOT NULL,
				PRIMARY KEY (host_id, tag_id),
				FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE,
				FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
			);

			-- Create host_env_vars table
			CREATE TABLE IF NOT EXISTS host_env_vars (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				host_id INTEGER NOT NULL,
				name TEXT NOT NULL,
				value TEXT NOT NULL,
				UNIQUE(host_id, name),
				FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
			);

			-- Create command_history table
			CREATE TABLE IF NOT EXISTS command_history (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				host_id INTEGER NOT NULL,
				command TEXT NOT NULL,
				exit_code INTEGER,
				executed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
			);

			-- Create indexes
			CREATE INDEX IF NOT EXISTS idx_host_group_members_host ON host_group_members(host_id);
			CREATE INDEX IF NOT EXISTS idx_host_group_members_group ON host_group_members(group_id);
			CREATE INDEX IF NOT EXISTS idx_host_tags_host ON host_tags(host_id);
			CREATE INDEX IF NOT EXISTS idx_host_tags_tag ON host_tags(tag_id);
			CREATE INDEX IF NOT EXISTS idx_command_history_host ON command_history(host_id, executed_at DESC);
		`,
	},
	{
		Version:     2,
		Description: "Add database and cache connection tables",
		Up: `
			-- Database connections table
			CREATE TABLE IF NOT EXISTS database_connections (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				host TEXT NOT NULL,
				port INTEGER NOT NULL DEFAULT 3306,
				user TEXT NOT NULL,
				password TEXT,
				database_name TEXT,
				alias TEXT,
				description TEXT,
				timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
				login_count INTEGER DEFAULT 0,
				UNIQUE(host, port, user, database_name)
			);

			CREATE UNIQUE INDEX IF NOT EXISTS idx_db_alias ON database_connections(alias) WHERE alias IS NOT NULL AND alias != '';

			-- Database groups table
			CREATE TABLE IF NOT EXISTS db_groups (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL UNIQUE,
				description TEXT
			);

			-- Database group members (many-to-many)
			CREATE TABLE IF NOT EXISTS db_group_members (
				db_id INTEGER NOT NULL,
				group_id INTEGER NOT NULL,
				PRIMARY KEY (db_id, group_id),
				FOREIGN KEY (db_id) REFERENCES database_connections(id) ON DELETE CASCADE,
				FOREIGN KEY (group_id) REFERENCES db_groups(id) ON DELETE CASCADE
			);

			-- Database tags (many-to-many, reuse tags table)
			CREATE TABLE IF NOT EXISTS db_tags (
				db_id INTEGER NOT NULL,
				tag_id INTEGER NOT NULL,
				PRIMARY KEY (db_id, tag_id),
				FOREIGN KEY (db_id) REFERENCES database_connections(id) ON DELETE CASCADE,
				FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
			);

			-- Database query history
			CREATE TABLE IF NOT EXISTS db_query_history (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				db_id INTEGER NOT NULL,
				query TEXT NOT NULL,
				execution_time_ms INTEGER,
				rows_affected INTEGER,
				executed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (db_id) REFERENCES database_connections(id) ON DELETE CASCADE
			);

			-- Cache connections table
			CREATE TABLE IF NOT EXISTS cache_connections (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				host TEXT NOT NULL,
				port INTEGER NOT NULL DEFAULT 6379,
				password TEXT,
				database_index INTEGER DEFAULT 0,
				alias TEXT,
				description TEXT,
				timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
				login_count INTEGER DEFAULT 0,
				use_tls BOOLEAN DEFAULT FALSE,
				UNIQUE(host, port, database_index)
			);

			CREATE UNIQUE INDEX IF NOT EXISTS idx_cache_alias ON cache_connections(alias) WHERE alias IS NOT NULL AND alias != '';

			-- Cache groups table
			CREATE TABLE IF NOT EXISTS cache_groups (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL UNIQUE,
				description TEXT
			);

			-- Cache group members (many-to-many)
			CREATE TABLE IF NOT EXISTS cache_group_members (
				cache_id INTEGER NOT NULL,
				group_id INTEGER NOT NULL,
				PRIMARY KEY (cache_id, group_id),
				FOREIGN KEY (cache_id) REFERENCES cache_connections(id) ON DELETE CASCADE,
				FOREIGN KEY (group_id) REFERENCES cache_groups(id) ON DELETE CASCADE
			);

			-- Cache tags (many-to-many)
			CREATE TABLE IF NOT EXISTS cache_tags (
				cache_id INTEGER NOT NULL,
				tag_id INTEGER NOT NULL,
				PRIMARY KEY (cache_id, tag_id),
				FOREIGN KEY (cache_id) REFERENCES cache_connections(id) ON DELETE CASCADE,
				FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
			);

			-- Cache command history
			CREATE TABLE IF NOT EXISTS cache_command_history (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				cache_id INTEGER NOT NULL,
				command TEXT NOT NULL,
				executed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (cache_id) REFERENCES cache_connections(id) ON DELETE CASCADE
			);

			-- Create indexes
			CREATE INDEX IF NOT EXISTS idx_db_group_members_db ON db_group_members(db_id);
			CREATE INDEX IF NOT EXISTS idx_db_group_members_group ON db_group_members(group_id);
			CREATE INDEX IF NOT EXISTS idx_db_tags_db ON db_tags(db_id);
			CREATE INDEX IF NOT EXISTS idx_db_tags_tag ON db_tags(tag_id);
			CREATE INDEX IF NOT EXISTS idx_db_query_history_db ON db_query_history(db_id, executed_at DESC);
			CREATE INDEX IF NOT EXISTS idx_cache_group_members_cache ON cache_group_members(cache_id);
			CREATE INDEX IF NOT EXISTS idx_cache_group_members_group ON cache_group_members(group_id);
			CREATE INDEX IF NOT EXISTS idx_cache_tags_cache ON cache_tags(cache_id);
			CREATE INDEX IF NOT EXISTS idx_cache_tags_tag ON cache_tags(tag_id);
			CREATE INDEX IF NOT EXISTS idx_cache_command_history_cache ON cache_command_history(cache_id, executed_at DESC);
		`,
	},
}

// MigrationManager handles database migrations.
type MigrationManager struct {
	db *sql.DB
}

// NewMigrationManager creates a new migration manager.
func NewMigrationManager(db *sql.DB) *MigrationManager {
	return &MigrationManager{db: db}
}

// Initialize creates the migrations table if it doesn't exist.
func (m *MigrationManager) Initialize() error {
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			description TEXT,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// GetCurrentVersion returns the current schema version.
func (m *MigrationManager) GetCurrentVersion() (int, error) {
	var version int
	err := m.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// Migrate runs all pending migrations.
func (m *MigrationManager) Migrate() error {
	if err := m.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize migrations: %w", err)
	}

	currentVersion, err := m.GetCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	for _, migration := range migrations {
		if migration.Version <= currentVersion {
			continue
		}

		if err := m.applyMigration(migration); err != nil {
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}
	}

	return nil
}

// applyMigration applies a single migration.
func (m *MigrationManager) applyMigration(migration Migration) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute migration SQL
	if _, err := tx.Exec(migration.Up); err != nil {
		return fmt.Errorf("migration SQL failed: %w", err)
	}

	// Record migration
	if _, err := tx.Exec(
		"INSERT INTO schema_migrations (version, description) VALUES (?, ?)",
		migration.Version, migration.Description,
	); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return tx.Commit()
}

// RunMigrations runs database migrations for the manager.
func (m *Manager) RunMigrations() error {
	migrator := NewMigrationManager(m.db)
	return migrator.Migrate()
}
