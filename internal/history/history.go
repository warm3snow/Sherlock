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

// Package history provides login history management for Sherlock.
package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Record represents a login history record.
type Record struct {
	// Host is the hostname or IP address.
	Host string `json:"host"`
	// Port is the SSH port.
	Port int `json:"port"`
	// User is the SSH username.
	User string `json:"user"`
	// Timestamp is when the connection was made.
	Timestamp time.Time `json:"timestamp"`
	// HasPubKey indicates if the public key was added to the remote host.
	HasPubKey bool `json:"has_pub_key"`
}

// HostKey returns a unique key for the host (user@host:port).
func (r *Record) HostKey() string {
	return fmt.Sprintf("%s@%s:%d", r.User, r.Host, r.Port)
}

// Manager manages login history.
type Manager struct {
	historyPath string
	records     []Record
}

// NewManager creates a new history manager.
func NewManager() (*Manager, error) {
	historyPath := GetHistoryPath()
	m := &Manager{
		historyPath: historyPath,
		records:     make([]Record, 0),
	}

	if err := m.load(); err != nil {
		// If file doesn't exist, that's ok
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load history: %w", err)
		}
	}

	return m, nil
}

// GetHistoryPath returns the default history file path.
func GetHistoryPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "sherlock", "history.json")
}

// load loads history from file.
func (m *Manager) load() error {
	data, err := os.ReadFile(m.historyPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &m.records)
}

// save saves history to file.
func (m *Manager) save() error {
	dir := filepath.Dir(m.historyPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create history directory: %w", err)
	}

	data, err := json.MarshalIndent(m.records, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal history: %w", err)
	}

	return os.WriteFile(m.historyPath, data, 0600)
}

// AddRecord adds a new login record.
func (m *Manager) AddRecord(host string, port int, user string, hasPubKey bool) error {
	record := Record{
		Host:      host,
		Port:      port,
		User:      user,
		Timestamp: time.Now(),
		HasPubKey: hasPubKey,
	}

	// Check if this host already exists and update it
	hostKey := record.HostKey()
	for i, r := range m.records {
		if r.HostKey() == hostKey {
			// Update existing record
			m.records[i].Timestamp = record.Timestamp
			if hasPubKey {
				m.records[i].HasPubKey = true
			}
			return m.save()
		}
	}

	// Add new record
	m.records = append(m.records, record)
	return m.save()
}

// MarkPubKeyAdded marks a host as having public key added.
func (m *Manager) MarkPubKeyAdded(host string, port int, user string) error {
	hostKey := fmt.Sprintf("%s@%s:%d", user, host, port)
	for i, r := range m.records {
		if r.HostKey() == hostKey {
			m.records[i].HasPubKey = true
			return m.save()
		}
	}
	return nil
}

// HasPubKey checks if a host has public key added.
func (m *Manager) HasPubKey(host string, port int, user string) bool {
	hostKey := fmt.Sprintf("%s@%s:%d", user, host, port)
	for _, r := range m.records {
		if r.HostKey() == hostKey {
			return r.HasPubKey
		}
	}
	return false
}

// GetRecords returns all history records, sorted by timestamp (newest first).
func (m *Manager) GetRecords() []Record {
	records := make([]Record, len(m.records))
	copy(records, m.records)

	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp.After(records[j].Timestamp)
	})

	return records
}

// GetRecentRecords returns the most recent N history records.
func (m *Manager) GetRecentRecords(n int) []Record {
	records := m.GetRecords()
	if n > len(records) {
		n = len(records)
	}
	return records[:n]
}

// SearchRecords searches for records matching the query.
// Query can be a host, user, or user@host pattern.
func (m *Manager) SearchRecords(query string) []Record {
	query = strings.ToLower(query)
	var results []Record

	for _, r := range m.records {
		// Match against host, user, or full hostKey
		if strings.Contains(strings.ToLower(r.Host), query) ||
			strings.Contains(strings.ToLower(r.User), query) ||
			strings.Contains(strings.ToLower(r.HostKey()), query) {
			results = append(results, r)
		}
	}

	// Sort by timestamp (newest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})

	return results
}

// FormatRecords returns a formatted string of history records.
func FormatRecords(records []Record) string {
	if len(records) == 0 {
		return "No login history found."
	}

	var sb strings.Builder
	sb.WriteString("Login History:\n")
	sb.WriteString(strings.Repeat("-", 60) + "\n")

	for i, r := range records {
		pubKeyStatus := ""
		if r.HasPubKey {
			pubKeyStatus = " [key]"
		}
		sb.WriteString(fmt.Sprintf("%2d. %s%s\n    Last login: %s\n",
			i+1,
			r.HostKey(),
			pubKeyStatus,
			r.Timestamp.Format("2006-01-02 15:04:05")))
	}

	return sb.String()
}
