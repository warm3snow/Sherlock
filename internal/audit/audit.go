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

// Package audit provides operation auditing and logging functionality.
package audit

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"
)

// OperationType defines the type of operation being audited.
type OperationType string

const (
	OpConnect   OperationType = "connect"
	OpExecute   OperationType = "execute"
	OpAnalyze   OperationType = "analyze"
	OpDiagnose  OperationType = "diagnose"
	OpUpload    OperationType = "upload"
	OpDownload  OperationType = "download"
	OpPlaybook  OperationType = "playbook"
	OpInspect   OperationType = "inspect"
	OpQuickfix  OperationType = "quickfix"
)

// Entry represents a single audit log entry.
type Entry struct {
	ID        int64         `json:"id"`
	Timestamp time.Time     `json:"timestamp"`
	Operation OperationType `json:"operation"`
	HostID    int64         `json:"host_id,omitempty"`
	HostName  string        `json:"host_name,omitempty"`
	Command   string        `json:"command,omitempty"`
	ExitCode  int           `json:"exit_code"`
	Duration  int64         `json:"duration_ms"` // milliseconds
	Success   bool          `json:"success"`
	Details   string        `json:"details,omitempty"`
	UserInput string        `json:"user_input,omitempty"` // Original user input (natural language)
}

// QueryFilter defines filters for querying audit logs.
type QueryFilter struct {
	StartTime  *time.Time
	EndTime    *time.Time
	Operation  OperationType
	HostID     int64
	HostName   string
	SuccessOnly bool
	FailedOnly  bool
	Limit      int
	Offset     int
}

// Statistics contains aggregated audit statistics.
type Statistics struct {
	TotalOperations   int            `json:"total_operations"`
	SuccessCount      int            `json:"success_count"`
	FailedCount       int            `json:"failed_count"`
	SuccessRate       float64        `json:"success_rate"`
	ByOperation       map[string]int `json:"by_operation"`
	ByHost            map[string]int `json:"by_host"`
	TotalDurationMs   int64          `json:"total_duration_ms"`
	AvgDurationMs     int64          `json:"avg_duration_ms"`
	TimeSavedEstimate string         `json:"time_saved_estimate"` // Estimated time saved by AI
}

// Manager handles audit logging operations.
type Manager struct {
	db      *sql.DB
	mu      sync.RWMutex
	enabled bool
	asyncCh chan *Entry // Async write channel
}

// NewManager creates a new audit manager.
func NewManager(db *sql.DB) (*Manager, error) {
	m := &Manager{
		db:      db,
		enabled: true,
		asyncCh: make(chan *Entry, 100),
	}

	if err := m.initTable(); err != nil {
		return nil, err
	}

	// Start async writer
	go m.asyncWriter()

	return m, nil
}

// initTable creates the audit table if it doesn't exist.
func (m *Manager) initTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		operation TEXT NOT NULL,
		host_id INTEGER,
		host_name TEXT,
		command TEXT,
		exit_code INTEGER DEFAULT 0,
		duration_ms INTEGER DEFAULT 0,
		success BOOLEAN DEFAULT 1,
		details TEXT,
		user_input TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_logs(timestamp);
	CREATE INDEX IF NOT EXISTS idx_audit_operation ON audit_logs(operation);
	CREATE INDEX IF NOT EXISTS idx_audit_host ON audit_logs(host_name);
	`
	_, err := m.db.Exec(query)
	return err
}

// asyncWriter processes audit entries asynchronously.
func (m *Manager) asyncWriter() {
	for entry := range m.asyncCh {
		m.writeEntry(entry)
	}
}

// writeEntry writes an entry to the database.
func (m *Manager) writeEntry(entry *Entry) error {
	query := `
	INSERT INTO audit_logs (timestamp, operation, host_id, host_name, command, exit_code, duration_ms, success, details, user_input)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := m.db.Exec(query,
		entry.Timestamp, entry.Operation, entry.HostID, entry.HostName,
		entry.Command, entry.ExitCode, entry.Duration, entry.Success,
		entry.Details, entry.UserInput)
	return err
}

// Log records an audit entry (async).
func (m *Manager) Log(entry *Entry) {
	if !m.enabled {
		return
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	
	select {
	case m.asyncCh <- entry:
	default:
		// Channel full, write synchronously
		m.writeEntry(entry)
	}
}

// LogOperation is a convenience method to log an operation.
func (m *Manager) LogOperation(op OperationType, hostName, command string, success bool, duration time.Duration, details string) {
	m.Log(&Entry{
		Timestamp: time.Now(),
		Operation: op,
		HostName:  hostName,
		Command:   command,
		Success:   success,
		Duration:  duration.Milliseconds(),
		Details:   details,
	})
}

// Query retrieves audit entries based on filters.
func (m *Manager) Query(filter QueryFilter) ([]Entry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var conditions []string
	var args []interface{}

	if filter.StartTime != nil {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, filter.StartTime)
	}
	if filter.EndTime != nil {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, filter.EndTime)
	}
	if filter.Operation != "" {
		conditions = append(conditions, "operation = ?")
		args = append(args, filter.Operation)
	}
	if filter.HostID > 0 {
		conditions = append(conditions, "host_id = ?")
		args = append(args, filter.HostID)
	}
	if filter.HostName != "" {
		conditions = append(conditions, "host_name LIKE ?")
		args = append(args, "%"+filter.HostName+"%")
	}
	if filter.SuccessOnly {
		conditions = append(conditions, "success = 1")
	}
	if filter.FailedOnly {
		conditions = append(conditions, "success = 0")
	}

	query := "SELECT id, timestamp, operation, host_id, host_name, command, exit_code, duration_ms, success, details, user_input FROM audit_logs"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var ts string
		var hostID, duration sql.NullInt64
		var hostName, command, details, userInput sql.NullString

		if err := rows.Scan(&e.ID, &ts, &e.Operation, &hostID, &hostName,
			&command, &e.ExitCode, &duration, &e.Success, &details, &userInput); err != nil {
			return nil, err
		}

		e.Timestamp, _ = time.Parse("2006-01-02 15:04:05", ts)
		if hostID.Valid {
			e.HostID = hostID.Int64
		}
		if hostName.Valid {
			e.HostName = hostName.String
		}
		if command.Valid {
			e.Command = command.String
		}
		if duration.Valid {
			e.Duration = duration.Int64
		}
		if details.Valid {
			e.Details = details.String
		}
		if userInput.Valid {
			e.UserInput = userInput.String
		}

		entries = append(entries, e)
	}

	return entries, nil
}

// GetStatistics returns aggregated statistics.
func (m *Manager) GetStatistics(filter QueryFilter) (*Statistics, error) {
	entries, err := m.Query(filter)
	if err != nil {
		return nil, err
	}

	stats := &Statistics{
		ByOperation: make(map[string]int),
		ByHost:      make(map[string]int),
	}

	for _, e := range entries {
		stats.TotalOperations++
		if e.Success {
			stats.SuccessCount++
		} else {
			stats.FailedCount++
		}
		stats.TotalDurationMs += e.Duration
		stats.ByOperation[string(e.Operation)]++
		if e.HostName != "" {
			stats.ByHost[e.HostName]++
		}
	}

	if stats.TotalOperations > 0 {
		stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.TotalOperations) * 100
		stats.AvgDurationMs = stats.TotalDurationMs / int64(stats.TotalOperations)
	}

	// Estimate time saved (each AI-analyzed command saves ~30 seconds manual work)
	aiOps := stats.ByOperation["analyze"] + stats.ByOperation["diagnose"]
	timeSavedSec := aiOps * 30
	if timeSavedSec > 3600 {
		stats.TimeSavedEstimate = fmt.Sprintf("%.1f hours", float64(timeSavedSec)/3600)
	} else if timeSavedSec > 60 {
		stats.TimeSavedEstimate = fmt.Sprintf("%d minutes", timeSavedSec/60)
	} else {
		stats.TimeSavedEstimate = fmt.Sprintf("%d seconds", timeSavedSec)
	}

	return stats, nil
}

// GetRecentEntries returns the most recent audit entries.
func (m *Manager) GetRecentEntries(limit int) ([]Entry, error) {
	return m.Query(QueryFilter{Limit: limit})
}

// GetTodayStatistics returns statistics for today.
func (m *Manager) GetTodayStatistics() (*Statistics, error) {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return m.GetStatistics(QueryFilter{StartTime: &startOfDay})
}

// Clear removes all audit entries (use with caution).
func (m *Manager) Clear() error {
	_, err := m.db.Exec("DELETE FROM audit_logs")
	return err
}

// SetEnabled enables or disables audit logging.
func (m *Manager) SetEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
}

// Close closes the audit manager.
func (m *Manager) Close() error {
	close(m.asyncCh)
	return nil
}

// FormatEntry formats a single entry for display.
func FormatEntry(e *Entry) string {
	status := "✓"
	if !e.Success {
		status = "✗"
	}
	return fmt.Sprintf("[%s] %s %s %s (%dms) - %s",
		e.Timestamp.Format("15:04:05"),
		status,
		e.Operation,
		e.HostName,
		e.Duration,
		truncateString(e.Command, 50))
}

// FormatStatistics formats statistics for display.
func FormatStatistics(stats *Statistics) string {
	var sb strings.Builder

	sb.WriteString("📊 审计统计报告\n")
	sb.WriteString("═══════════════════════════════════════\n\n")

	sb.WriteString(fmt.Sprintf("📈 总操作数: %d\n", stats.TotalOperations))
	sb.WriteString(fmt.Sprintf("   ✓ 成功: %d (%.1f%%)\n", stats.SuccessCount, stats.SuccessRate))
	sb.WriteString(fmt.Sprintf("   ✗ 失败: %d\n", stats.FailedCount))
	sb.WriteString(fmt.Sprintf("⏱️  平均耗时: %dms\n", stats.AvgDurationMs))
	sb.WriteString(fmt.Sprintf("🚀 AI节省时间: %s\n\n", stats.TimeSavedEstimate))

	if len(stats.ByOperation) > 0 {
		sb.WriteString("📋 按操作类型:\n")
		for op, count := range stats.ByOperation {
			sb.WriteString(fmt.Sprintf("   %s: %d\n", op, count))
		}
		sb.WriteString("\n")
	}

	if len(stats.ByHost) > 0 {
		sb.WriteString("🖥️  按主机:\n")
		for host, count := range stats.ByHost {
			sb.WriteString(fmt.Sprintf("   %s: %d\n", host, count))
		}
	}

	return sb.String()
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
