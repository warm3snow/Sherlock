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
	"time"
)

// SetEnvVar sets an environment variable for a host.
func (m *Manager) SetEnvVar(ctx context.Context, hostID int64, name, value string) error {
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO host_env_vars (host_id, name, value)
		VALUES (?, ?, ?)
		ON CONFLICT(host_id, name) DO UPDATE SET value = excluded.value
	`, hostID, name, value)
	if err != nil {
		return fmt.Errorf("failed to set env var: %w", err)
	}
	return nil
}

// GetEnvVars returns all environment variables for a host.
func (m *Manager) GetEnvVars(ctx context.Context, hostID int64) (map[string]string, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT name, value FROM host_env_vars WHERE host_id = ?
	`, hostID)
	if err != nil {
		return nil, fmt.Errorf("failed to get env vars: %w", err)
	}
	defer rows.Close()

	envVars := make(map[string]string)
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			continue
		}
		envVars[name] = value
	}

	return envVars, nil
}

// DeleteEnvVar deletes an environment variable for a host.
func (m *Manager) DeleteEnvVar(ctx context.Context, hostID int64, name string) error {
	result, err := m.db.ExecContext(ctx, `
		DELETE FROM host_env_vars WHERE host_id = ? AND name = ?
	`, hostID, name)
	if err != nil {
		return fmt.Errorf("failed to delete env var: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("env var not found")
	}

	return nil
}

// AddCommandHistory adds a command to the history for a host.
func (m *Manager) AddCommandHistory(ctx context.Context, hostID int64, command string, exitCode *int) error {
	_, err := m.db.ExecContext(ctx, `
		INSERT INTO command_history (host_id, command, exit_code, executed_at)
		VALUES (?, ?, ?, ?)
	`, hostID, command, exitCode, time.Now())
	if err != nil {
		return fmt.Errorf("failed to add command history: %w", err)
	}
	return nil
}

// GetCommandHistory returns command history for a host.
func (m *Manager) GetCommandHistory(ctx context.Context, hostID int64, limit int) ([]CommandHistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := m.db.QueryContext(ctx, `
		SELECT id, host_id, command, exit_code, executed_at
		FROM command_history
		WHERE host_id = ?
		ORDER BY executed_at DESC
		LIMIT ?
	`, hostID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get command history: %w", err)
	}
	defer rows.Close()

	var entries []CommandHistoryEntry
	for rows.Next() {
		var e CommandHistoryEntry
		var exitCode sql.NullInt64
		var executedAt string

		if err := rows.Scan(&e.ID, &e.HostID, &e.Command, &exitCode, &executedAt); err != nil {
			continue
		}

		if exitCode.Valid {
			code := int(exitCode.Int64)
			e.ExitCode = &code
		}
		e.ExecutedAt = parseTimestamp(executedAt)
		entries = append(entries, e)
	}

	return entries, nil
}

// SearchCommandHistory searches command history for a host.
func (m *Manager) SearchCommandHistory(ctx context.Context, hostID int64, query string, limit int) ([]CommandHistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	searchPattern := "%" + query + "%"
	rows, err := m.db.QueryContext(ctx, `
		SELECT id, host_id, command, exit_code, executed_at
		FROM command_history
		WHERE host_id = ? AND command LIKE ?
		ORDER BY executed_at DESC
		LIMIT ?
	`, hostID, searchPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search command history: %w", err)
	}
	defer rows.Close()

	var entries []CommandHistoryEntry
	for rows.Next() {
		var e CommandHistoryEntry
		var exitCode sql.NullInt64
		var executedAt string

		if err := rows.Scan(&e.ID, &e.HostID, &e.Command, &exitCode, &executedAt); err != nil {
			continue
		}

		if exitCode.Valid {
			code := int(exitCode.Int64)
			e.ExitCode = &code
		}
		e.ExecutedAt = parseTimestamp(executedAt)
		entries = append(entries, e)
	}

	return entries, nil
}

// ClearCommandHistory clears command history for a host.
func (m *Manager) ClearCommandHistory(ctx context.Context, hostID int64) error {
	_, err := m.db.ExecContext(ctx, `
		DELETE FROM command_history WHERE host_id = ?
	`, hostID)
	if err != nil {
		return fmt.Errorf("failed to clear command history: %w", err)
	}
	return nil
}

// GetAllCommandHistory returns command history across all hosts.
func (m *Manager) GetAllCommandHistory(ctx context.Context, limit int) ([]CommandHistoryEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := m.db.QueryContext(ctx, `
		SELECT id, host_id, command, exit_code, executed_at
		FROM command_history
		ORDER BY executed_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get all command history: %w", err)
	}
	defer rows.Close()

	var entries []CommandHistoryEntry
	for rows.Next() {
		var e CommandHistoryEntry
		var exitCode sql.NullInt64
		var executedAt string

		if err := rows.Scan(&e.ID, &e.HostID, &e.Command, &exitCode, &executedAt); err != nil {
			continue
		}

		if exitCode.Valid {
			code := int(exitCode.Int64)
			e.ExitCode = &code
		}
		e.ExecutedAt = parseTimestamp(executedAt)
		entries = append(entries, e)
	}

	return entries, nil
}
