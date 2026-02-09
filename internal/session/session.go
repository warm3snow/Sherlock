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

// Package session provides multi-session management for Sherlock.
package session

import (
	"fmt"
	"sync"
	"time"

	"github.com/warm3snow/sherlock/pkg/sshclient"
)

// Session represents an SSH session.
type Session struct {
	ID          int              `json:"id"`
	Name        string           `json:"name"`
	Host        string           `json:"host"`
	Port        int              `json:"port"`
	User        string           `json:"user"`
	Status      SessionStatus    `json:"status"`
	CreatedAt   time.Time        `json:"created_at"`
	LastActiveAt time.Time       `json:"last_active_at"`
	
	// Runtime
	Client      *sshclient.Client `json:"-"`
}

// SessionStatus represents the status of a session.
type SessionStatus string

const (
	StatusConnected    SessionStatus = "connected"
	StatusDisconnected SessionStatus = "disconnected"
	StatusConnecting   SessionStatus = "connecting"
	StatusError        SessionStatus = "error"
)

// Manager manages multiple SSH sessions.
type Manager struct {
	sessions      map[int]*Session
	currentID     int
	nextID        int
	mu            sync.RWMutex
	maxSessions   int
	lastSessionID int // For reconnect feature
}

// NewManager creates a new session manager.
func NewManager(maxSessions int) *Manager {
	if maxSessions <= 0 {
		maxSessions = 10
	}
	return &Manager{
		sessions:    make(map[int]*Session),
		nextID:      1,
		maxSessions: maxSessions,
	}
}

// Create creates a new session (without connecting).
func (m *Manager) Create(name, host string, port int, user string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.sessions) >= m.maxSessions {
		return nil, fmt.Errorf("maximum sessions (%d) reached, close some sessions first", m.maxSessions)
	}

	session := &Session{
		ID:          m.nextID,
		Name:        name,
		Host:        host,
		Port:        port,
		User:        user,
		Status:      StatusDisconnected,
		CreatedAt:   time.Now(),
		LastActiveAt: time.Now(),
	}

	if name == "" {
		session.Name = fmt.Sprintf("session-%d", m.nextID)
	}

	m.sessions[m.nextID] = session
	m.nextID++

	return session, nil
}

// Add adds an existing connected session.
func (m *Manager) Add(name string, client *sshclient.Client) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.sessions) >= m.maxSessions {
		return nil, fmt.Errorf("maximum sessions (%d) reached", m.maxSessions)
	}

	hostInfo := client.GetHostInfo()
	session := &Session{
		ID:           m.nextID,
		Name:         name,
		Host:         hostInfo.Host,
		Port:         hostInfo.Port,
		User:         hostInfo.User,
		Status:       StatusConnected,
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
		Client:       client,
	}

	if name == "" {
		session.Name = fmt.Sprintf("session-%d", m.nextID)
	}

	m.sessions[m.nextID] = session
	m.currentID = m.nextID
	m.lastSessionID = m.nextID
	m.nextID++

	return session, nil
}

// Switch switches to a different session.
func (m *Manager) Switch(id int) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %d not found", id)
	}

	m.currentID = id
	session.LastActiveAt = time.Now()
	return session, nil
}

// Current returns the current active session.
func (m *Manager) Current() *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[m.currentID]
}

// CurrentID returns the current session ID.
func (m *Manager) CurrentID() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentID
}

// Get returns a session by ID.
func (m *Manager) Get(id int) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

// GetByName returns a session by name.
func (m *Manager) GetByName(name string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		if s.Name == name {
			return s, true
		}
	}
	return nil, false
}

// List returns all sessions.
func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s)
	}
	return result
}

// Close closes a session by ID.
func (m *Manager) Close(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session %d not found", id)
	}

	if session.Client != nil && session.Client.IsConnected() {
		session.Client.Close()
	}
	session.Status = StatusDisconnected
	session.Client = nil

	delete(m.sessions, id)

	// If we closed the current session, switch to another one
	if m.currentID == id {
		m.currentID = 0
		for nextID := range m.sessions {
			if m.sessions[nextID].Status == StatusConnected {
				m.currentID = nextID
				break
			}
		}
	}

	return nil
}

// CloseAll closes all sessions.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, session := range m.sessions {
		if session.Client != nil && session.Client.IsConnected() {
			session.Client.Close()
		}
	}
	m.sessions = make(map[int]*Session)
	m.currentID = 0
}

// Rename renames a session.
func (m *Manager) Rename(id int, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session %d not found", id)
	}

	session.Name = name
	return nil
}

// Count returns the number of sessions.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// ConnectedCount returns the number of connected sessions.
func (m *Manager) ConnectedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, s := range m.sessions {
		if s.Status == StatusConnected && s.Client != nil && s.Client.IsConnected() {
			count++
		}
	}
	return count
}

// LastSession returns information about the last closed session for reconnect.
func (m *Manager) LastSession() (host string, port int, user string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if session, ok := m.sessions[m.lastSessionID]; ok {
		return session.Host, session.Port, session.User
	}
	return "", 0, ""
}

// SetLastSession records the last session info for reconnect feature.
func (m *Manager) SetLastSession(host string, port int, user string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Create a placeholder session to remember last connection
	for _, s := range m.sessions {
		if s.Host == host && s.Port == port && s.User == user {
			m.lastSessionID = s.ID
			return
		}
	}
}

// UpdateClient updates the client for a session.
func (m *Manager) UpdateClient(id int, client *sshclient.Client) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session %d not found", id)
	}

	if session.Client != nil && session.Client.IsConnected() {
		session.Client.Close()
	}

	session.Client = client
	if client != nil && client.IsConnected() {
		session.Status = StatusConnected
	} else {
		session.Status = StatusDisconnected
	}
	session.LastActiveAt = time.Now()

	return nil
}

// FormatSessionList formats the session list for display.
func FormatSessionList(sessions []*Session, currentID int) string {
	if len(sessions) == 0 {
		return "No sessions.\n"
	}

	result := "Sessions:\n"
	result += "--------------------------------------------------------------\n"
	result += fmt.Sprintf("%-3s | %-4s | %-12s | %-25s | %s\n", "", "ID", "Name", "Host", "Status")
	result += "--------------------------------------------------------------\n"

	for _, s := range sessions {
		marker := "  "
		if s.ID == currentID {
			marker = "→ "
		}

		hostStr := fmt.Sprintf("%s@%s:%d", s.User, s.Host, s.Port)
		status := string(s.Status)
		if s.Client != nil && s.Client.IsConnected() {
			status = "connected"
		} else {
			status = "disconnected"
		}

		result += fmt.Sprintf("%s | %-4d | %-12s | %-25s | %s\n",
			marker, s.ID, truncate(s.Name, 12), truncate(hostStr, 25), status)
	}

	result += "--------------------------------------------------------------\n"
	result += fmt.Sprintf("Total: %d sessions\n", len(sessions))

	return result
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
