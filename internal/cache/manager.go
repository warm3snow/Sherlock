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

package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/warm3snow/sherlock/internal/crypto"
	"gopkg.in/yaml.v3"
)

// Manager manages cache connections.
type Manager struct {
	db        *sql.DB
	encryptor *crypto.Encryptor
}

// NewManager creates a new cache connection manager.
func NewManager(db *sql.DB) (*Manager, error) {
	encryptor, err := crypto.DefaultEncryptor()
	if err != nil {
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}

	return &Manager{
		db:        db,
		encryptor: encryptor,
	}, nil
}

// AddConnection adds a new cache connection.
func (m *Manager) AddConnection(ctx context.Context, conn *Connection) error {
	// Encrypt password if provided
	encryptedPwd := ""
	if conn.Password != "" {
		var err error
		encryptedPwd, err = m.encryptor.Encrypt(conn.Password)
		if err != nil {
			return fmt.Errorf("failed to encrypt password: %w", err)
		}
	}

	result, err := m.db.ExecContext(ctx, `
		INSERT INTO cache_connections (host, port, password, database_index, alias, description, timestamp, login_count, use_tls)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?)
	`, conn.Host, conn.Port, encryptedPwd, conn.DatabaseIndex, conn.Alias, conn.Description, time.Now(), conn.UseTLS)
	if err != nil {
		return fmt.Errorf("failed to add connection: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	conn.ID = id

	return nil
}

// UpdateConnection updates an existing cache connection.
func (m *Manager) UpdateConnection(ctx context.Context, conn *Connection) error {
	// Encrypt password if provided
	encryptedPwd := ""
	if conn.Password != "" {
		var err error
		encryptedPwd, err = m.encryptor.Encrypt(conn.Password)
		if err != nil {
			return fmt.Errorf("failed to encrypt password: %w", err)
		}
	}

	_, err := m.db.ExecContext(ctx, `
		UPDATE cache_connections
		SET host = ?, port = ?, password = ?, database_index = ?, alias = ?, description = ?, use_tls = ?
		WHERE id = ?
	`, conn.Host, conn.Port, encryptedPwd, conn.DatabaseIndex, conn.Alias, conn.Description, conn.UseTLS, conn.ID)
	if err != nil {
		return fmt.Errorf("failed to update connection: %w", err)
	}

	return nil
}

// DeleteConnection deletes a cache connection.
func (m *Manager) DeleteConnection(ctx context.Context, id int64) error {
	result, err := m.db.ExecContext(ctx, `DELETE FROM cache_connections WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete connection: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("connection not found")
	}

	return nil
}

// GetConnection returns a connection by ID.
func (m *Manager) GetConnection(ctx context.Context, id int64) (*Connection, error) {
	conn := &Connection{}
	var timestamp string
	var password sql.NullString

	err := m.db.QueryRowContext(ctx, `
		SELECT id, host, port, password, database_index, alias, description, timestamp, login_count, use_tls
		FROM cache_connections WHERE id = ?
	`, id).Scan(&conn.ID, &conn.Host, &conn.Port, &password, &conn.DatabaseIndex,
		&conn.Alias, &conn.Description, &timestamp, &conn.LoginCount, &conn.UseTLS)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("connection not found")
		}
		return nil, err
	}

	conn.Timestamp = parseTimestamp(timestamp)
	if password.Valid {
		conn.Password = password.String
	}

	// Load groups
	conn.Groups, _ = m.GetConnectionGroups(ctx, id)

	// Load tags
	conn.Tags, _ = m.GetConnectionTags(ctx, id)

	return conn, nil
}

// GetConnectionByAlias returns a connection by alias.
func (m *Manager) GetConnectionByAlias(ctx context.Context, alias string) (*Connection, error) {
	var id int64
	err := m.db.QueryRowContext(ctx, `SELECT id FROM cache_connections WHERE alias = ?`, alias).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("connection not found")
		}
		return nil, err
	}
	return m.GetConnection(ctx, id)
}

// GetConnectionByKey returns a connection by host, port, and database index.
func (m *Manager) GetConnectionByKey(ctx context.Context, host string, port, dbIndex int) (*Connection, error) {
	var id int64
	err := m.db.QueryRowContext(ctx, `
		SELECT id FROM cache_connections
		WHERE host = ? AND port = ? AND database_index = ?
	`, host, port, dbIndex).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("connection not found")
		}
		return nil, err
	}
	return m.GetConnection(ctx, id)
}

// RecordLogin records a login to a cache connection (create or update).
func (m *Manager) RecordLogin(ctx context.Context, conn *Connection) (*Connection, error) {
	// Try to update existing record
	result, err := m.db.ExecContext(ctx, `
		UPDATE cache_connections SET
			timestamp = ?,
			login_count = login_count + 1,
			password = CASE WHEN ? != '' THEN ? ELSE password END
		WHERE host = ? AND port = ? AND database_index = ?
	`, time.Now(), conn.Password, conn.Password, conn.Host, conn.Port, conn.DatabaseIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to update login: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Insert new record
		if err := m.AddConnection(ctx, conn); err != nil {
			return nil, fmt.Errorf("failed to add connection: %w", err)
		}
	}

	return m.GetConnectionByKey(ctx, conn.Host, conn.Port, conn.DatabaseIndex)
}

// ListConnections returns all connections with optional filters.
func (m *Manager) ListConnections(ctx context.Context, filter *ListFilter) ([]Connection, error) {
	query := `
		SELECT DISTINCT c.id, c.host, c.port, c.password, c.database_index,
		       c.alias, c.description, c.timestamp, c.login_count, c.use_tls
		FROM cache_connections c
	`
	var args []interface{}
	var conditions []string

	if filter != nil {
		if filter.GroupID > 0 {
			query += ` JOIN cache_group_members gm ON c.id = gm.cache_id`
			conditions = append(conditions, "gm.group_id = ?")
			args = append(args, filter.GroupID)
		} else if filter.GroupName != "" {
			query += ` JOIN cache_group_members gm ON c.id = gm.cache_id JOIN cache_groups g ON gm.group_id = g.id`
			conditions = append(conditions, "g.name = ?")
			args = append(args, filter.GroupName)
		}

		if filter.TagID > 0 {
			query += ` JOIN cache_tags ct ON c.id = ct.cache_id`
			conditions = append(conditions, "ct.tag_id = ?")
			args = append(args, filter.TagID)
		} else if filter.TagName != "" {
			query += ` JOIN cache_tags ct ON c.id = ct.cache_id JOIN tags t ON ct.tag_id = t.id`
			conditions = append(conditions, "t.name = ?")
			args = append(args, filter.TagName)
		}

		if filter.Search != "" {
			conditions = append(conditions, "(c.host LIKE ? OR c.alias LIKE ?)")
			searchPattern := "%" + filter.Search + "%"
			args = append(args, searchPattern, searchPattern)
		}
	}

	if len(conditions) > 0 {
		query += " WHERE "
		for i, cond := range conditions {
			if i > 0 {
				query += " AND "
			}
			query += cond
		}
	}

	query += " ORDER BY c.timestamp DESC"

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list connections: %w", err)
	}
	defer rows.Close()

	var connections []Connection
	for rows.Next() {
		var c Connection
		var timestamp string
		var password sql.NullString

		if err := rows.Scan(&c.ID, &c.Host, &c.Port, &password, &c.DatabaseIndex,
			&c.Alias, &c.Description, &timestamp, &c.LoginCount, &c.UseTLS); err != nil {
			continue
		}

		c.Timestamp = parseTimestamp(timestamp)
		if password.Valid {
			c.Password = password.String
		}

		c.Groups, _ = m.GetConnectionGroups(ctx, c.ID)
		c.Tags, _ = m.GetConnectionTags(ctx, c.ID)

		connections = append(connections, c)
	}

	return connections, nil
}

// IncrementLoginCount increments the login count for a connection.
func (m *Manager) IncrementLoginCount(ctx context.Context, id int64) error {
	_, err := m.db.ExecContext(ctx, `
		UPDATE cache_connections
		SET login_count = login_count + 1, timestamp = ?
		WHERE id = ?
	`, time.Now(), id)
	return err
}

// DecryptPassword decrypts the password for a connection.
func (m *Manager) DecryptPassword(encryptedPassword string) (string, error) {
	if encryptedPassword == "" {
		return "", nil
	}
	return m.encryptor.Decrypt(encryptedPassword)
}

// AddCommandHistory adds a command to the history.
func (m *Manager) AddCommandHistory(ctx context.Context, cacheID int64, command string) error {
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO cache_command_history (cache_id, command, executed_at)
		VALUES (?, ?, ?)
	`, cacheID, command, time.Now())
	return err
}

// GetCommandHistory returns command history for a connection.
func (m *Manager) GetCommandHistory(ctx context.Context, cacheID int64, limit int) ([]CommandHistory, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := m.db.QueryContext(ctx, `
		SELECT id, cache_id, command, executed_at
		FROM cache_command_history
		WHERE cache_id = ?
		ORDER BY executed_at DESC
		LIMIT ?
	`, cacheID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []CommandHistory
	for rows.Next() {
		var h CommandHistory
		var executedAt string

		if err := rows.Scan(&h.ID, &h.CacheID, &h.Command, &executedAt); err != nil {
			continue
		}

		h.ExecutedAt = parseTimestamp(executedAt)
		history = append(history, h)
	}

	return history, nil
}

// Export exports connections to the specified format.
func (m *Manager) Export(ctx context.Context, format ExportFormat) ([]byte, error) {
	connections, err := m.ListConnections(ctx, nil)
	if err != nil {
		return nil, err
	}

	groups, err := m.ListGroups(ctx)
	if err != nil {
		return nil, err
	}

	tags, err := m.ListTags(ctx)
	if err != nil {
		return nil, err
	}

	data := ExportData{
		Version:     "1.0",
		Connections: make([]ExportConnection, 0, len(connections)),
		Groups:      make([]ExportGroup, 0, len(groups)),
		Tags:        make([]ExportTag, 0, len(tags)),
	}

	for _, c := range connections {
		ec := ExportConnection{
			Host:          c.Host,
			Port:          c.Port,
			DatabaseIndex: c.DatabaseIndex,
			UseTLS:        c.UseTLS,
			Alias:         c.Alias,
			Description:   c.Description,
		}
		for _, g := range c.Groups {
			ec.Groups = append(ec.Groups, g.Name)
		}
		for _, t := range c.Tags {
			ec.Tags = append(ec.Tags, t.Name)
		}
		data.Connections = append(data.Connections, ec)
	}

	for _, g := range groups {
		data.Groups = append(data.Groups, ExportGroup{
			Name:        g.Name,
			Description: g.Description,
		})
	}

	for _, t := range tags {
		data.Tags = append(data.Tags, ExportTag{
			Name:  t.Name,
			Color: t.Color,
		})
	}

	switch format {
	case ExportJSON:
		return json.MarshalIndent(data, "", "  ")
	case ExportYAML:
		return yaml.Marshal(data)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// Import imports connections from the specified format.
func (m *Manager) Import(ctx context.Context, rawData []byte, format ExportFormat) error {
	var data ExportData

	switch format {
	case ExportJSON:
		if err := json.Unmarshal(rawData, &data); err != nil {
			return fmt.Errorf("failed to parse JSON: %w", err)
		}
	case ExportYAML:
		if err := yaml.Unmarshal(rawData, &data); err != nil {
			return fmt.Errorf("failed to parse YAML: %w", err)
		}
	default:
		return fmt.Errorf("unsupported import format: %s", format)
	}

	// Import groups first
	for _, g := range data.Groups {
		existing, _ := m.GetGroupByName(ctx, g.Name)
		if existing != nil {
			continue
		}
		group := &Group{Name: g.Name, Description: g.Description}
		_ = m.CreateGroup(ctx, group)
	}

	// Import tags
	for _, t := range data.Tags {
		existing, _ := m.GetTagByName(ctx, t.Name)
		if existing != nil {
			continue
		}
		tag := &Tag{Name: t.Name, Color: t.Color}
		_ = m.CreateTag(ctx, tag)
	}

	// Import connections
	for _, c := range data.Connections {
		existing, _ := m.GetConnectionByKey(ctx, c.Host, c.Port, c.DatabaseIndex)
		var connID int64

		if existing != nil {
			connID = existing.ID
			if c.Alias != "" && existing.Alias == "" {
				existing.Alias = c.Alias
			}
			if c.Description != "" && existing.Description == "" {
				existing.Description = c.Description
			}
			_ = m.UpdateConnection(ctx, existing)
		} else {
			conn := &Connection{
				Host:          c.Host,
				Port:          c.Port,
				DatabaseIndex: c.DatabaseIndex,
				UseTLS:        c.UseTLS,
				Alias:         c.Alias,
				Description:   c.Description,
			}
			if err := m.AddConnection(ctx, conn); err != nil {
				continue
			}
			connID = conn.ID
		}

		// Add to groups
		for _, groupName := range c.Groups {
			_ = m.AddConnectionToGroupByName(ctx, connID, groupName)
		}

		// Add tags
		for _, tagName := range c.Tags {
			_ = m.AddTagToConnectionByName(ctx, connID, tagName)
		}
	}

	return nil
}

// parseTimestamp parses a timestamp string.
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
