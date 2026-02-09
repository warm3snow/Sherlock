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

// Package agent provides the AI agent for Sherlock that handles natural language
// processing for SSH operations.
package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
)

// ConversationMemory manages conversation history with sliding window and persistence.
type ConversationMemory struct {
	mu sync.RWMutex

	// SessionID is the unique identifier for the current session
	SessionID string

	// Messages contains the sliding window of conversation messages
	Messages []*schema.Message

	// MaxMessages is the maximum number of messages to keep in memory (default: 20, i.e., 10 rounds)
	MaxMessages int

	// SystemPrompt is the dynamic system prompt including machine context
	SystemPrompt string

	// db is the SQLite database connection for persistence
	db *sql.DB

	// HostContext stores the current connected host information
	HostContext string

	// CommandHistory stores recent executed commands for context
	CommandHistory []CommandRecord

	// MaxCommandHistory is the maximum number of commands to remember
	MaxCommandHistory int
}

// CommandRecord represents a recorded command execution.
type CommandRecord struct {
	Command   string    `json:"command"`
	Output    string    `json:"output"`
	ExitCode  int       `json:"exit_code"`
	Timestamp time.Time `json:"timestamp"`
}

// PersistedMessage represents a message stored in the database.
type PersistedMessage struct {
	ID        int64     `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// MemoryConfig holds configuration for conversation memory.
type MemoryConfig struct {
	MaxMessages       int  `json:"max_messages"`
	MaxCommandHistory int  `json:"max_command_history"`
	EnablePersistence bool `json:"enable_persistence"`
}

// DefaultMemoryConfig returns the default memory configuration.
func DefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		MaxMessages:       20, // 10 rounds of conversation
		MaxCommandHistory: 50,
		EnablePersistence: true,
	}
}

// NewConversationMemory creates a new conversation memory manager.
func NewConversationMemory(db *sql.DB, config *MemoryConfig) (*ConversationMemory, error) {
	if config == nil {
		config = DefaultMemoryConfig()
	}

	cm := &ConversationMemory{
		SessionID:         uuid.New().String(),
		Messages:          make([]*schema.Message, 0, config.MaxMessages),
		MaxMessages:       config.MaxMessages,
		db:                db,
		CommandHistory:    make([]CommandRecord, 0, config.MaxCommandHistory),
		MaxCommandHistory: config.MaxCommandHistory,
	}

	// Initialize database table if persistence is enabled
	if db != nil && config.EnablePersistence {
		if err := cm.initTable(); err != nil {
			return nil, fmt.Errorf("failed to initialize memory table: %w", err)
		}
	}

	return cm, nil
}

// initTable creates the conversation_memory table if it doesn't exist.
func (cm *ConversationMemory) initTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS conversation_memory (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		host_context TEXT,
		UNIQUE(session_id, timestamp)
	);
	CREATE INDEX IF NOT EXISTS idx_conversation_session ON conversation_memory(session_id);
	CREATE INDEX IF NOT EXISTS idx_conversation_timestamp ON conversation_memory(timestamp);
	`
	_, err := cm.db.Exec(query)
	return err
}

// AddMessage adds a message to the conversation memory with sliding window management.
func (cm *ConversationMemory) AddMessage(msg *schema.Message) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Add message to memory
	cm.Messages = append(cm.Messages, msg)

	// Apply sliding window - remove oldest messages if exceeds limit
	if len(cm.Messages) > cm.MaxMessages {
		// Keep the most recent messages
		cm.Messages = cm.Messages[len(cm.Messages)-cm.MaxMessages:]
	}

	// Persist to database if available
	if cm.db != nil {
		cm.persistMessage(msg)
	}
}

// persistMessage saves a message to the database.
func (cm *ConversationMemory) persistMessage(msg *schema.Message) {
	_, err := cm.db.Exec(`
		INSERT INTO conversation_memory (session_id, role, content, host_context)
		VALUES (?, ?, ?, ?)
	`, cm.SessionID, string(msg.Role), msg.Content, cm.HostContext)
	if err != nil {
		// Log error but don't fail - memory still works without persistence
		fmt.Printf("Warning: failed to persist message: %v\n", err)
	}
}

// AddUserMessage adds a user message to the conversation.
func (cm *ConversationMemory) AddUserMessage(content string) {
	cm.AddMessage(schema.UserMessage(content))
}

// AddAssistantMessage adds an assistant message to the conversation.
func (cm *ConversationMemory) AddAssistantMessage(content string) {
	cm.AddMessage(schema.AssistantMessage(content, nil))
}

// AddSystemMessage sets or updates the system message.
func (cm *ConversationMemory) SetSystemPrompt(content string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.SystemPrompt = content
}

// RecordCommand records a command execution in history.
func (cm *ConversationMemory) RecordCommand(command, output string, exitCode int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	record := CommandRecord{
		Command:   command,
		Output:    truncateOutput(output, 500), // Limit output size
		ExitCode:  exitCode,
		Timestamp: time.Now(),
	}

	cm.CommandHistory = append(cm.CommandHistory, record)

	// Apply sliding window for command history
	if len(cm.CommandHistory) > cm.MaxCommandHistory {
		cm.CommandHistory = cm.CommandHistory[len(cm.CommandHistory)-cm.MaxCommandHistory:]
	}
}

// truncateOutput truncates output to a maximum length.
func truncateOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + "... (truncated)"
}

// GetMessages returns all messages including the system prompt for AI context.
func (cm *ConversationMemory) GetMessages() []*schema.Message {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	messages := make([]*schema.Message, 0, len(cm.Messages)+1)

	// Add system prompt if set
	if cm.SystemPrompt != "" {
		messages = append(messages, schema.SystemMessage(cm.SystemPrompt))
	}

	// Add conversation messages
	messages = append(messages, cm.Messages...)

	return messages
}

// GetMessagesWithContext returns messages with additional context about recent commands.
func (cm *ConversationMemory) GetMessagesWithContext(systemPrompt string) []*schema.Message {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	messages := make([]*schema.Message, 0, len(cm.Messages)+1)

	// Build enhanced system prompt with command history context
	enhancedPrompt := systemPrompt
	if len(cm.CommandHistory) > 0 {
		enhancedPrompt += "\n\n## Recent Command History (for context):\n"
		// Include last 5 commands for context
		start := 0
		if len(cm.CommandHistory) > 5 {
			start = len(cm.CommandHistory) - 5
		}
		for _, cmd := range cm.CommandHistory[start:] {
			status := "✓"
			if cmd.ExitCode != 0 {
				status = fmt.Sprintf("✗ (exit %d)", cmd.ExitCode)
			}
			enhancedPrompt += fmt.Sprintf("- %s: %s\n", status, cmd.Command)
		}
	}

	messages = append(messages, schema.SystemMessage(enhancedPrompt))

	// Add conversation messages
	messages = append(messages, cm.Messages...)

	return messages
}

// SetHostContext updates the current host context.
func (cm *ConversationMemory) SetHostContext(hostInfo string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.HostContext = hostInfo
}

// GetHostContext returns the current host context.
func (cm *ConversationMemory) GetHostContext() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.HostContext
}

// Clear clears the current conversation but optionally keeps command history.
func (cm *ConversationMemory) Clear(keepCommandHistory bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.Messages = make([]*schema.Message, 0, cm.MaxMessages)

	if !keepCommandHistory {
		cm.CommandHistory = make([]CommandRecord, 0, cm.MaxCommandHistory)
	}
}

// NewSession starts a new session, optionally preserving context.
func (cm *ConversationMemory) NewSession(preserveContext bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.SessionID = uuid.New().String()
	cm.Messages = make([]*schema.Message, 0, cm.MaxMessages)

	if !preserveContext {
		cm.CommandHistory = make([]CommandRecord, 0, cm.MaxCommandHistory)
		cm.HostContext = ""
	}
}

// GetSessionID returns the current session ID.
func (cm *ConversationMemory) GetSessionID() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.SessionID
}

// LoadSession loads a previous session from the database.
func (cm *ConversationMemory) LoadSession(ctx context.Context, sessionID string) error {
	if cm.db == nil {
		return fmt.Errorf("database not available")
	}

	rows, err := cm.db.QueryContext(ctx, `
		SELECT role, content, timestamp, host_context
		FROM conversation_memory
		WHERE session_id = ?
		ORDER BY timestamp ASC
		LIMIT ?
	`, sessionID, cm.MaxMessages)
	if err != nil {
		return fmt.Errorf("failed to load session: %w", err)
	}
	defer rows.Close()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.SessionID = sessionID
	cm.Messages = make([]*schema.Message, 0)

	for rows.Next() {
		var role, content, hostContext string
		var timestamp time.Time
		if err := rows.Scan(&role, &content, &timestamp, &hostContext); err != nil {
			continue
		}

		msg := &schema.Message{
			Role:    schema.RoleType(role),
			Content: content,
		}
		cm.Messages = append(cm.Messages, msg)

		if hostContext != "" {
			cm.HostContext = hostContext
		}
	}

	return rows.Err()
}

// ListSessions returns a list of recent sessions.
func (cm *ConversationMemory) ListSessions(ctx context.Context, limit int) ([]SessionInfo, error) {
	if cm.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	rows, err := cm.db.QueryContext(ctx, `
		SELECT session_id, MIN(timestamp) as started, MAX(timestamp) as last_activity, 
		       COUNT(*) as message_count, MAX(host_context) as host_context
		FROM conversation_memory
		GROUP BY session_id
		ORDER BY last_activity DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []SessionInfo
	for rows.Next() {
		var info SessionInfo
		var hostContext sql.NullString
		if err := rows.Scan(&info.SessionID, &info.StartedAt, &info.LastActivity,
			&info.MessageCount, &hostContext); err != nil {
			continue
		}
		if hostContext.Valid {
			info.HostContext = hostContext.String
		}
		sessions = append(sessions, info)
	}

	return sessions, rows.Err()
}

// SessionInfo contains information about a session.
type SessionInfo struct {
	SessionID    string    `json:"session_id"`
	StartedAt    time.Time `json:"started_at"`
	LastActivity time.Time `json:"last_activity"`
	MessageCount int       `json:"message_count"`
	HostContext  string    `json:"host_context,omitempty"`
}

// GetConversationSummary generates a summary of the current conversation for context.
func (cm *ConversationMemory) GetConversationSummary() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.Messages) == 0 {
		return ""
	}

	summary := fmt.Sprintf("Session %s: %d messages", cm.SessionID[:8], len(cm.Messages))
	if cm.HostContext != "" {
		summary += fmt.Sprintf(", connected to %s", cm.HostContext)
	}
	if len(cm.CommandHistory) > 0 {
		summary += fmt.Sprintf(", %d commands executed", len(cm.CommandHistory))
	}

	return summary
}

// ExportSession exports the current session as JSON.
func (cm *ConversationMemory) ExportSession() ([]byte, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	export := struct {
		SessionID      string          `json:"session_id"`
		Messages       []*schema.Message `json:"messages"`
		CommandHistory []CommandRecord `json:"command_history"`
		HostContext    string          `json:"host_context"`
		ExportedAt     time.Time       `json:"exported_at"`
	}{
		SessionID:      cm.SessionID,
		Messages:       cm.Messages,
		CommandHistory: cm.CommandHistory,
		HostContext:    cm.HostContext,
		ExportedAt:     time.Now(),
	}

	return json.Marshal(export)
}

// CleanupOldSessions removes sessions older than the specified duration.
func (cm *ConversationMemory) CleanupOldSessions(ctx context.Context, maxAge time.Duration) (int64, error) {
	if cm.db == nil {
		return 0, fmt.Errorf("database not available")
	}

	cutoff := time.Now().Add(-maxAge)
	result, err := cm.db.ExecContext(ctx, `
		DELETE FROM conversation_memory
		WHERE timestamp < ?
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup sessions: %w", err)
	}

	return result.RowsAffected()
}

// MessageCount returns the current number of messages in memory.
func (cm *ConversationMemory) MessageCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.Messages)
}

// CommandCount returns the current number of commands in history.
func (cm *ConversationMemory) CommandCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.CommandHistory)
}

// GetLastCommand returns the last executed command, if any.
func (cm *ConversationMemory) GetLastCommand() *CommandRecord {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.CommandHistory) == 0 {
		return nil
	}
	return &cm.CommandHistory[len(cm.CommandHistory)-1]
}

// GetRecentCommands returns the N most recent commands.
func (cm *ConversationMemory) GetRecentCommands(n int) []CommandRecord {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if len(cm.CommandHistory) == 0 {
		return nil
	}

	start := 0
	if len(cm.CommandHistory) > n {
		start = len(cm.CommandHistory) - n
	}

	result := make([]CommandRecord, len(cm.CommandHistory)-start)
	copy(result, cm.CommandHistory[start:])
	return result
}
