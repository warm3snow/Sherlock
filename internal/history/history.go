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

// Package history provides login history management for Sherlock using SQLite3.
package history

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Record represents a login history record.
type Record struct {
	// ID is the unique identifier.
	ID int64
	// Host is the hostname or IP address.
	Host string
	// Port is the SSH port.
	Port int
	// User is the SSH username.
	User string
	// Timestamp is when the connection was made.
	Timestamp time.Time
	// HasPubKey indicates if the public key was added to the remote host.
	HasPubKey bool
	// LoginCount is the number of times this host has been logged into.
	LoginCount int
}

// HostKey returns a unique key for the host (user@host:port).
func (r *Record) HostKey() string {
	return fmt.Sprintf("%s@%s:%d", r.User, r.Host, r.Port)
}

// Manager manages login history using SQLite3.
type Manager struct {
	dbPath string
	db     *sql.DB
}

// NewManager creates a new history manager.
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
		return fmt.Errorf("failed to create history directory: %w", err)
	}

	db, err := sql.Open("sqlite3", m.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Create table if not exists
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS hosts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		host TEXT NOT NULL,
		port INTEGER NOT NULL,
		user TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		has_pub_key BOOLEAN DEFAULT FALSE,
		login_count INTEGER DEFAULT 1,
		UNIQUE(host, port, user)
	);
	CREATE INDEX IF NOT EXISTS idx_hosts_timestamp ON hosts(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_hosts_host ON hosts(host);
	CREATE INDEX IF NOT EXISTS idx_hosts_user ON hosts(user);
	`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		db.Close()
		return fmt.Errorf("failed to create table: %w", err)
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

// AddRecord adds or updates a login record.
func (m *Manager) AddRecord(host string, port int, user string, hasPubKey bool) error {
	// Try to update existing record first
	updateSQL := `
	UPDATE hosts SET 
		timestamp = ?,
		login_count = login_count + 1,
		has_pub_key = CASE WHEN ? THEN TRUE ELSE has_pub_key END
	WHERE host = ? AND port = ? AND user = ?
	`

	result, err := m.db.Exec(updateSQL, time.Now(), hasPubKey, host, port, user)
	if err != nil {
		return fmt.Errorf("failed to update record: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	// If no rows were updated, insert a new record
	if rowsAffected == 0 {
		insertSQL := `
		INSERT INTO hosts (host, port, user, timestamp, has_pub_key, login_count)
		VALUES (?, ?, ?, ?, ?, 1)
		`
		_, err = m.db.Exec(insertSQL, host, port, user, time.Now(), hasPubKey)
		if err != nil {
			return fmt.Errorf("failed to insert record: %w", err)
		}
	}

	return nil
}

// MarkPubKeyAdded marks a host as having public key added.
func (m *Manager) MarkPubKeyAdded(host string, port int, user string) error {
	updateSQL := `UPDATE hosts SET has_pub_key = TRUE WHERE host = ? AND port = ? AND user = ?`
	_, err := m.db.Exec(updateSQL, host, port, user)
	return err
}

// HasPubKey checks if a host has public key added.
func (m *Manager) HasPubKey(host string, port int, user string) bool {
	var hasPubKey bool
	query := `SELECT has_pub_key FROM hosts WHERE host = ? AND port = ? AND user = ?`
	err := m.db.QueryRow(query, host, port, user).Scan(&hasPubKey)
	if err != nil {
		return false
	}
	return hasPubKey
}

// GetRecords returns all history records, sorted by timestamp (newest first).
func (m *Manager) GetRecords() []Record {
	query := `SELECT id, host, port, user, timestamp, has_pub_key, login_count FROM hosts ORDER BY timestamp DESC`
	return m.queryRecords(query)
}

// GetRecentRecords returns the most recent N history records.
func (m *Manager) GetRecentRecords(n int) []Record {
	query := `SELECT id, host, port, user, timestamp, has_pub_key, login_count FROM hosts ORDER BY timestamp DESC LIMIT ?`
	return m.queryRecordsWithArgs(query, n)
}

// SearchRecords searches for records matching the query.
// Query can be a host, user, or user@host pattern.
// Note: Uses LIKE with COLLATE NOCASE for case-insensitive search.
func (m *Manager) SearchRecords(query string) []Record {
	searchQuery := "%" + query + "%"
	sqlQuery := `
	SELECT id, host, port, user, timestamp, has_pub_key, login_count 
	FROM hosts 
	WHERE host LIKE ? COLLATE NOCASE OR user LIKE ? COLLATE NOCASE OR (host || ':' || port) LIKE ? COLLATE NOCASE
	ORDER BY timestamp DESC
	`
	return m.queryRecordsWithArgs(sqlQuery, searchQuery, searchQuery, searchQuery)
}

// GetRecordByID returns a record by its ID.
func (m *Manager) GetRecordByID(id int64) (*Record, error) {
	query := `SELECT id, host, port, user, timestamp, has_pub_key, login_count FROM hosts WHERE id = ?`
	row := m.db.QueryRow(query, id)

	var r Record
	var timestamp string
	err := row.Scan(&r.ID, &r.Host, &r.Port, &r.User, &timestamp, &r.HasPubKey, &r.LoginCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("record not found")
		}
		return nil, err
	}

	r.Timestamp = parseTimestamp(timestamp)
	return &r, nil
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

func (m *Manager) queryRecords(query string) []Record {
	rows, err := m.db.Query(query)
	if err != nil {
		// Return empty slice for query errors - this is expected for new databases
		return []Record{}
	}
	defer rows.Close()

	return m.scanRecords(rows)
}

func (m *Manager) queryRecordsWithArgs(query string, args ...interface{}) []Record {
	rows, err := m.db.Query(query, args...)
	if err != nil {
		// Return empty slice for query errors - this is expected for new databases
		return []Record{}
	}
	defer rows.Close()

	return m.scanRecords(rows)
}

func (m *Manager) scanRecords(rows *sql.Rows) []Record {
	var records []Record
	for rows.Next() {
		var r Record
		var timestamp string
		err := rows.Scan(&r.ID, &r.Host, &r.Port, &r.User, &timestamp, &r.HasPubKey, &r.LoginCount)
		if err != nil {
			// Skip rows that fail to scan - this could indicate schema changes
			continue
		}
		r.Timestamp = parseTimestamp(timestamp)
		records = append(records, r)
	}
	return records
}

// FormatRecords returns a formatted string of history records.
func FormatRecords(records []Record) string {
	if len(records) == 0 {
		return "No login history found."
	}

	var sb strings.Builder
	sb.WriteString("Login History:\n")
	sb.WriteString(strings.Repeat("-", 70) + "\n")
	sb.WriteString(fmt.Sprintf("%-4s %-30s %-6s %-20s\n", "ID", "Host", "Logins", "Last Login"))
	sb.WriteString(strings.Repeat("-", 70) + "\n")

	for _, r := range records {
		pubKeyStatus := ""
		if r.HasPubKey {
			pubKeyStatus = " [key]"
		}
		sb.WriteString(fmt.Sprintf("%-4d %-30s %-6d %s%s\n",
			r.ID,
			r.HostKey(),
			r.LoginCount,
			r.Timestamp.Format("2006-01-02 15:04:05"),
			pubKeyStatus))
	}

	return sb.String()
}

// FormatHostsSimple returns a simple formatted string of hosts for quick selection.
func FormatHostsSimple(records []Record) string {
	if len(records) == 0 {
		return "No saved hosts found."
	}

	var sb strings.Builder
	sb.WriteString("Saved Hosts:\n")
	sb.WriteString(strings.Repeat("-", 50) + "\n")

	for _, r := range records {
		pubKeyStatus := ""
		if r.HasPubKey {
			pubKeyStatus = " [key]"
		}
		sb.WriteString(fmt.Sprintf("[%d] %s%s\n", r.ID, r.HostKey(), pubKeyStatus))
	}

	sb.WriteString(strings.Repeat("-", 50) + "\n")
	sb.WriteString("Use 'connect <id>' to connect to a saved host.\n")

	return sb.String()
}
