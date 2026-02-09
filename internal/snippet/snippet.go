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

// Package snippet provides command snippet/template management for Sherlock.
package snippet

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Snippet represents a saved command snippet/template.
type Snippet struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Command     string    `json:"command"`
	Description string    `json:"description,omitempty"`
	Category    string    `json:"category,omitempty"`
	UsageCount  int       `json:"usage_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Manager manages command snippets.
type Manager struct {
	db *sql.DB
}

// NewManager creates a new snippet manager.
func NewManager(db *sql.DB) (*Manager, error) {
	m := &Manager{db: db}
	if err := m.initTables(); err != nil {
		return nil, err
	}
	return m, nil
}

// initTables creates the necessary database tables.
func (m *Manager) initTables() error {
	createSQL := `
	CREATE TABLE IF NOT EXISTS snippets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		command TEXT NOT NULL,
		description TEXT,
		category TEXT DEFAULT 'default',
		usage_count INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_snippets_name ON snippets(name);
	CREATE INDEX IF NOT EXISTS idx_snippets_category ON snippets(category);
	`
	_, err := m.db.Exec(createSQL)
	return err
}

// Add adds a new snippet.
func (m *Manager) Add(snippet *Snippet) error {
	insertSQL := `
	INSERT INTO snippets (name, command, description, category)
	VALUES (?, ?, ?, ?)
	`
	result, err := m.db.Exec(insertSQL, snippet.Name, snippet.Command, snippet.Description, snippet.Category)
	if err != nil {
		return fmt.Errorf("failed to add snippet: %w", err)
	}
	snippet.ID, _ = result.LastInsertId()
	return nil
}

// Update updates an existing snippet.
func (m *Manager) Update(snippet *Snippet) error {
	updateSQL := `
	UPDATE snippets SET command = ?, description = ?, category = ?, updated_at = CURRENT_TIMESTAMP
	WHERE id = ?
	`
	_, err := m.db.Exec(updateSQL, snippet.Command, snippet.Description, snippet.Category, snippet.ID)
	return err
}

// Delete deletes a snippet by ID.
func (m *Manager) Delete(id int64) error {
	_, err := m.db.Exec("DELETE FROM snippets WHERE id = ?", id)
	return err
}

// DeleteByName deletes a snippet by name.
func (m *Manager) DeleteByName(name string) error {
	_, err := m.db.Exec("DELETE FROM snippets WHERE name = ?", name)
	return err
}

// Get retrieves a snippet by ID.
func (m *Manager) Get(id int64) (*Snippet, error) {
	row := m.db.QueryRow("SELECT id, name, command, description, category, usage_count, created_at, updated_at FROM snippets WHERE id = ?", id)
	return m.scanSnippet(row)
}

// GetByName retrieves a snippet by name.
func (m *Manager) GetByName(name string) (*Snippet, error) {
	row := m.db.QueryRow("SELECT id, name, command, description, category, usage_count, created_at, updated_at FROM snippets WHERE name = ?", name)
	return m.scanSnippet(row)
}

// List returns all snippets, optionally filtered by category.
func (m *Manager) List(category string) ([]*Snippet, error) {
	var query string
	var args []interface{}

	if category != "" {
		query = "SELECT id, name, command, description, category, usage_count, created_at, updated_at FROM snippets WHERE category = ? ORDER BY usage_count DESC, name"
		args = []interface{}{category}
	} else {
		query = "SELECT id, name, command, description, category, usage_count, created_at, updated_at FROM snippets ORDER BY usage_count DESC, name"
	}

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snippets []*Snippet
	for rows.Next() {
		s, err := m.scanSnippetRow(rows)
		if err != nil {
			continue
		}
		snippets = append(snippets, s)
	}
	return snippets, nil
}

// Search searches snippets by name or command content.
func (m *Manager) Search(query string) ([]*Snippet, error) {
	searchQuery := "%" + query + "%"
	rows, err := m.db.Query(`
		SELECT id, name, command, description, category, usage_count, created_at, updated_at 
		FROM snippets 
		WHERE name LIKE ? OR command LIKE ? OR description LIKE ?
		ORDER BY usage_count DESC, name
	`, searchQuery, searchQuery, searchQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snippets []*Snippet
	for rows.Next() {
		s, err := m.scanSnippetRow(rows)
		if err != nil {
			continue
		}
		snippets = append(snippets, s)
	}
	return snippets, nil
}

// IncrementUsage increments the usage count of a snippet.
func (m *Manager) IncrementUsage(id int64) error {
	_, err := m.db.Exec("UPDATE snippets SET usage_count = usage_count + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)
	return err
}

// ListCategories returns all unique categories.
func (m *Manager) ListCategories() ([]string, error) {
	rows, err := m.db.Query("SELECT DISTINCT category FROM snippets ORDER BY category")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categories []string
	for rows.Next() {
		var cat string
		if err := rows.Scan(&cat); err == nil {
			categories = append(categories, cat)
		}
	}
	return categories, nil
}

// GetNames returns all snippet names (for autocomplete).
func (m *Manager) GetNames() ([]string, error) {
	rows, err := m.db.Query("SELECT name FROM snippets ORDER BY usage_count DESC, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			names = append(names, name)
		}
	}
	return names, nil
}

// ExpandVariables expands variables in a snippet command.
// Variables are in the format ${VAR_NAME} or $VAR_NAME.
func ExpandVariables(command string, vars map[string]string) string {
	result := command
	for key, value := range vars {
		result = strings.ReplaceAll(result, "${"+key+"}", value)
		result = strings.ReplaceAll(result, "$"+key, value)
	}
	return result
}

// ParseVariables extracts variable names from a command template.
func ParseVariables(command string) []string {
	var vars []string
	seen := make(map[string]bool)

	// Match ${VAR_NAME} pattern
	for i := 0; i < len(command); i++ {
		if i+1 < len(command) && command[i] == '$' && command[i+1] == '{' {
			end := strings.Index(command[i:], "}")
			if end > 0 {
				varName := command[i+2 : i+end]
				if !seen[varName] {
					vars = append(vars, varName)
					seen[varName] = true
				}
			}
		}
	}
	return vars
}

func (m *Manager) scanSnippet(row *sql.Row) (*Snippet, error) {
	var s Snippet
	var createdAt, updatedAt string
	err := row.Scan(&s.ID, &s.Name, &s.Command, &s.Description, &s.Category, &s.UsageCount, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	s.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return &s, nil
}

func (m *Manager) scanSnippetRow(rows *sql.Rows) (*Snippet, error) {
	var s Snippet
	var createdAt, updatedAt string
	err := rows.Scan(&s.ID, &s.Name, &s.Command, &s.Description, &s.Category, &s.UsageCount, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	s.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	return &s, nil
}
