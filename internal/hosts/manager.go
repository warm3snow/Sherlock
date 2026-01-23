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

package hosts

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Manager manages hosts using SQLite3.
type Manager struct {
	dbPath string
	db     *sql.DB
}

// NewManager creates a new hosts manager.
func NewManager() (*Manager, error) {
	dbPath := GetDBPath()
	m := &Manager{
		dbPath: dbPath,
	}

	if err := m.initDB(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return m, nil
}

// GetDBPath returns the default database file path.
func GetDBPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "sherlock", "history.db")
}

// initDB initializes the SQLite database.
func (m *Manager) initDB() error {
	dir := filepath.Dir(m.dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	db, err := sql.Open("sqlite3", m.dbPath+"?_foreign_keys=on")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	m.db = db
	return nil
}

// Close closes the database connection.
func (m *Manager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// DB returns the underlying database connection.
func (m *Manager) DB() *sql.DB {
	return m.db
}

// AddHost adds a new host.
func (m *Manager) AddHost(ctx context.Context, host *Host) error {
	var alias, description sql.NullString
	if host.Alias != "" {
		alias = sql.NullString{String: host.Alias, Valid: true}
	}
	if host.Description != "" {
		description = sql.NullString{String: host.Description, Valid: true}
	}

	result, err := m.db.ExecContext(ctx, `
		INSERT INTO hosts (host, port, user, alias, description, timestamp, has_pub_key, login_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, host.Host, host.Port, host.User, alias, description, time.Now(), host.HasPubKey, 1)
	if err != nil {
		return fmt.Errorf("failed to add host: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	host.ID = id

	return nil
}

// UpdateHost updates an existing host.
func (m *Manager) UpdateHost(ctx context.Context, host *Host) error {
	var alias, description sql.NullString
	if host.Alias != "" {
		alias = sql.NullString{String: host.Alias, Valid: true}
	}
	if host.Description != "" {
		description = sql.NullString{String: host.Description, Valid: true}
	}

	_, err := m.db.ExecContext(ctx, `
		UPDATE hosts SET host = ?, port = ?, user = ?, alias = ?, description = ?, has_pub_key = ?
		WHERE id = ?
	`, host.Host, host.Port, host.User, alias, description, host.HasPubKey, host.ID)
	if err != nil {
		return fmt.Errorf("failed to update host: %w", err)
	}

	return nil
}

// DeleteHost deletes a host by ID.
func (m *Manager) DeleteHost(ctx context.Context, id int64) error {
	result, err := m.db.ExecContext(ctx, "DELETE FROM hosts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete host: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("host not found")
	}

	return nil
}

// GetHost returns a host by ID.
func (m *Manager) GetHost(ctx context.Context, id int64) (*Host, error) {
	row := m.db.QueryRowContext(ctx, `
		SELECT id, host, port, user, alias, description, timestamp, has_pub_key, login_count
		FROM hosts WHERE id = ?
	`, id)

	return m.scanHost(row)
}

// GetHostByAlias returns a host by alias.
func (m *Manager) GetHostByAlias(ctx context.Context, alias string) (*Host, error) {
	row := m.db.QueryRowContext(ctx, `
		SELECT id, host, port, user, alias, description, timestamp, has_pub_key, login_count
		FROM hosts WHERE alias = ?
	`, alias)

	return m.scanHost(row)
}

// GetHostByHostKey returns a host by host key (user@host:port).
func (m *Manager) GetHostByHostKey(ctx context.Context, hostAddr string, port int, user string) (*Host, error) {
	row := m.db.QueryRowContext(ctx, `
		SELECT id, host, port, user, alias, description, timestamp, has_pub_key, login_count
		FROM hosts WHERE host = ? AND port = ? AND user = ?
	`, hostAddr, port, user)

	return m.scanHost(row)
}

// ListHosts returns all hosts, optionally filtered.
func (m *Manager) ListHosts(ctx context.Context, filter *HostFilter) ([]Host, error) {
	query := `
		SELECT DISTINCT h.id, h.host, h.port, h.user, h.alias, h.description, h.timestamp, h.has_pub_key, h.login_count
		FROM hosts h
	`
	var args []interface{}
	var conditions []string

	if filter != nil {
		// Join with groups if filtering by group
		if len(filter.Groups) > 0 {
			query += " LEFT JOIN host_group_members hgm ON h.id = hgm.host_id"
			query += " LEFT JOIN host_groups hg ON hgm.group_id = hg.id"
			placeholders := make([]string, len(filter.Groups))
			for i, g := range filter.Groups {
				placeholders[i] = "?"
				args = append(args, g)
			}
			conditions = append(conditions, "hg.name IN ("+strings.Join(placeholders, ",")+")")
		}

		// Join with tags if filtering by tag
		if len(filter.Tags) > 0 {
			query += " LEFT JOIN host_tags ht ON h.id = ht.host_id"
			query += " LEFT JOIN tags t ON ht.tag_id = t.id"
			placeholders := make([]string, len(filter.Tags))
			for i, t := range filter.Tags {
				placeholders[i] = "?"
				args = append(args, t)
			}
			conditions = append(conditions, "t.name IN ("+strings.Join(placeholders, ",")+")")
		}

		// Free text search
		if filter.Query != "" {
			searchPattern := "%" + filter.Query + "%"
			conditions = append(conditions, "(h.host LIKE ? OR h.user LIKE ? OR h.alias LIKE ? OR h.description LIKE ?)")
			args = append(args, searchPattern, searchPattern, searchPattern, searchPattern)
		}

		// Filter by public key status
		if filter.HasPubKey != nil {
			conditions = append(conditions, "h.has_pub_key = ?")
			args = append(args, *filter.HasPubKey)
		}
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY h.timestamp DESC"

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}
	defer rows.Close()

	var hosts []Host
	for rows.Next() {
		host, err := m.scanHostFromRows(rows)
		if err != nil {
			continue
		}
		hosts = append(hosts, *host)
	}

	// Load groups and tags for each host
	for i := range hosts {
		hosts[i].Groups, _ = m.GetHostGroups(ctx, hosts[i].ID)
		hosts[i].Tags, _ = m.GetHostTags(ctx, hosts[i].ID)
	}

	return hosts, nil
}

// SearchHosts searches for hosts matching the query.
func (m *Manager) SearchHosts(ctx context.Context, query string) ([]Host, error) {
	return m.ListHosts(ctx, &HostFilter{Query: query})
}

// RecordLogin records a login to a host (used when connecting).
func (m *Manager) RecordLogin(ctx context.Context, hostAddr string, port int, user string, hasPubKey bool) (*Host, error) {
	// Try to update existing record first
	result, err := m.db.ExecContext(ctx, `
		UPDATE hosts SET
			timestamp = ?,
			login_count = login_count + 1,
			has_pub_key = CASE WHEN ? THEN 1 ELSE has_pub_key END
		WHERE host = ? AND port = ? AND user = ?
	`, time.Now(), hasPubKey, hostAddr, port, user)
	if err != nil {
		return nil, fmt.Errorf("failed to update login: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Insert new record
		_, err = m.db.ExecContext(ctx, `
			INSERT INTO hosts (host, port, user, timestamp, has_pub_key, login_count)
			VALUES (?, ?, ?, ?, ?, 1)
		`, hostAddr, port, user, time.Now(), hasPubKey)
		if err != nil {
			return nil, fmt.Errorf("failed to insert login: %w", err)
		}
	}

	return m.GetHostByHostKey(ctx, hostAddr, port, user)
}

// scanHost scans a single host from a row.
func (m *Manager) scanHost(row *sql.Row) (*Host, error) {
	var h Host
	var alias, description sql.NullString
	var timestamp string

	err := row.Scan(&h.ID, &h.Host, &h.Port, &h.User, &alias, &description, &timestamp, &h.HasPubKey, &h.LoginCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("host not found")
		}
		return nil, err
	}

	if alias.Valid {
		h.Alias = alias.String
	}
	if description.Valid {
		h.Description = description.String
	}
	h.Timestamp = parseTimestamp(timestamp)

	return &h, nil
}

// scanHostFromRows scans a single host from rows.
func (m *Manager) scanHostFromRows(rows *sql.Rows) (*Host, error) {
	var h Host
	var alias, description sql.NullString
	var timestamp string

	err := rows.Scan(&h.ID, &h.Host, &h.Port, &h.User, &alias, &description, &timestamp, &h.HasPubKey, &h.LoginCount)
	if err != nil {
		return nil, err
	}

	if alias.Valid {
		h.Alias = alias.String
	}
	if description.Valid {
		h.Description = description.String
	}
	h.Timestamp = parseTimestamp(timestamp)

	return &h, nil
}

// parseTimestamp attempts to parse a timestamp string in multiple formats.
func parseTimestamp(timestamp string) time.Time {
	formats := []string{
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
		"2006-01-02 15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timestamp); err == nil {
			return t
		}
	}
	return time.Time{}
}

// GetHostGroups returns all groups for a host.
func (m *Manager) GetHostGroups(ctx context.Context, hostID int64) ([]Group, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT g.id, g.name, g.description, g.created_at
		FROM host_groups g
		JOIN host_group_members hgm ON g.id = hgm.group_id
		WHERE hgm.host_id = ?
	`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		var description sql.NullString
		var createdAt string
		if err := rows.Scan(&g.ID, &g.Name, &description, &createdAt); err != nil {
			continue
		}
		if description.Valid {
			g.Description = description.String
		}
		g.CreatedAt = parseTimestamp(createdAt)
		groups = append(groups, g)
	}

	return groups, nil
}

// GetHostTags returns all tags for a host.
func (m *Manager) GetHostTags(ctx context.Context, hostID int64) ([]Tag, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT t.id, t.name, t.color
		FROM tags t
		JOIN host_tags ht ON t.id = ht.tag_id
		WHERE ht.host_id = ?
	`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color); err != nil {
			continue
		}
		tags = append(tags, t)
	}

	return tags, nil
}

// GetAllHosts returns all hosts without filtering.
func (m *Manager) GetAllHosts(ctx context.Context) ([]Host, error) {
	return m.ListHosts(ctx, nil)
}
