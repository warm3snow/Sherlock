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

// Package tunnel provides SSH tunnel/port forwarding functionality for Sherlock.
package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// TunnelType represents the type of SSH tunnel.
type TunnelType string

const (
	// LocalForward forwards a local port to a remote address (ssh -L).
	LocalForward TunnelType = "local"
	// RemoteForward forwards a remote port to a local address (ssh -R).
	RemoteForward TunnelType = "remote"
	// DynamicForward creates a SOCKS proxy (ssh -D).
	DynamicForward TunnelType = "dynamic"
)

// Tunnel represents an SSH tunnel configuration.
type Tunnel struct {
	ID          string     `json:"id"`
	Type        TunnelType `json:"type"`
	LocalAddr   string     `json:"local_addr"`   // Local bind address (host:port)
	RemoteAddr  string     `json:"remote_addr"`  // Remote target address (host:port)
	Description string     `json:"description,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`

	// Runtime state
	listener net.Listener
	sshConn  *ssh.Client
	ctx      context.Context
	cancel   context.CancelFunc
	active   bool
	mu       sync.RWMutex
}

// Manager manages SSH tunnels.
type Manager struct {
	tunnels map[string]*Tunnel
	mu      sync.RWMutex
}

// NewManager creates a new tunnel manager.
func NewManager() *Manager {
	return &Manager{
		tunnels: make(map[string]*Tunnel),
	}
}

// CreateLocalForward creates a local port forwarding tunnel.
// This is equivalent to: ssh -L localAddr:remoteAddr user@host
func (m *Manager) CreateLocalForward(sshClient *ssh.Client, localAddr, remoteAddr, description string) (*Tunnel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate tunnel ID
	id := fmt.Sprintf("L-%s-%s", localAddr, remoteAddr)

	if existing, ok := m.tunnels[id]; ok && existing.IsActive() {
		return nil, fmt.Errorf("tunnel already exists and is active: %s", id)
	}

	ctx, cancel := context.WithCancel(context.Background())

	tunnel := &Tunnel{
		ID:          id,
		Type:        LocalForward,
		LocalAddr:   localAddr,
		RemoteAddr:  remoteAddr,
		Description: description,
		CreatedAt:   time.Now(),
		sshConn:     sshClient,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start local listener
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to listen on %s: %w", localAddr, err)
	}
	tunnel.listener = listener
	tunnel.active = true

	// Start accepting connections
	go tunnel.acceptLocalForward()

	m.tunnels[id] = tunnel
	return tunnel, nil
}

// CreateRemoteForward creates a remote port forwarding tunnel.
// This is equivalent to: ssh -R remoteAddr:localAddr user@host
func (m *Manager) CreateRemoteForward(sshClient *ssh.Client, remoteAddr, localAddr, description string) (*Tunnel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("R-%s-%s", remoteAddr, localAddr)

	if existing, ok := m.tunnels[id]; ok && existing.IsActive() {
		return nil, fmt.Errorf("tunnel already exists and is active: %s", id)
	}

	ctx, cancel := context.WithCancel(context.Background())

	tunnel := &Tunnel{
		ID:          id,
		Type:        RemoteForward,
		LocalAddr:   localAddr,
		RemoteAddr:  remoteAddr,
		Description: description,
		CreatedAt:   time.Now(),
		sshConn:     sshClient,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start remote listener on SSH server
	listener, err := sshClient.Listen("tcp", remoteAddr)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to listen on remote %s: %w", remoteAddr, err)
	}
	tunnel.listener = listener
	tunnel.active = true

	// Start accepting connections
	go tunnel.acceptRemoteForward()

	m.tunnels[id] = tunnel
	return tunnel, nil
}

// Close closes a tunnel by ID.
func (m *Manager) Close(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tunnel, ok := m.tunnels[id]
	if !ok {
		return fmt.Errorf("tunnel not found: %s", id)
	}

	tunnel.Close()
	delete(m.tunnels, id)
	return nil
}

// CloseAll closes all tunnels.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, tunnel := range m.tunnels {
		tunnel.Close()
	}
	m.tunnels = make(map[string]*Tunnel)
}

// List returns all tunnels.
func (m *Manager) List() []*Tunnel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Tunnel, 0, len(m.tunnels))
	for _, t := range m.tunnels {
		result = append(result, t)
	}
	return result
}

// Get returns a tunnel by ID.
func (m *Manager) Get(id string) (*Tunnel, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tunnels[id]
	return t, ok
}

// IsActive returns whether the tunnel is active.
func (t *Tunnel) IsActive() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.active
}

// Close closes the tunnel.
func (t *Tunnel) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.active {
		return
	}

	t.active = false
	if t.cancel != nil {
		t.cancel()
	}
	if t.listener != nil {
		t.listener.Close()
	}
}

// acceptLocalForward handles incoming connections for local forwarding.
func (t *Tunnel) acceptLocalForward() {
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		conn, err := t.listener.Accept()
		if err != nil {
			if t.IsActive() {
				continue
			}
			return
		}

		go t.handleLocalForwardConnection(conn)
	}
}

// handleLocalForwardConnection handles a single local forward connection.
func (t *Tunnel) handleLocalForwardConnection(localConn net.Conn) {
	defer localConn.Close()

	// Connect to the remote target through SSH
	remoteConn, err := t.sshConn.Dial("tcp", t.RemoteAddr)
	if err != nil {
		return
	}
	defer remoteConn.Close()

	// Bidirectional copy
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(remoteConn, localConn)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(localConn, remoteConn)
		done <- struct{}{}
	}()

	// Wait for one direction to finish
	select {
	case <-done:
	case <-t.ctx.Done():
	}
}

// acceptRemoteForward handles incoming connections for remote forwarding.
func (t *Tunnel) acceptRemoteForward() {
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		conn, err := t.listener.Accept()
		if err != nil {
			if t.IsActive() {
				continue
			}
			return
		}

		go t.handleRemoteForwardConnection(conn)
	}
}

// handleRemoteForwardConnection handles a single remote forward connection.
func (t *Tunnel) handleRemoteForwardConnection(remoteConn net.Conn) {
	defer remoteConn.Close()

	// Connect to the local target
	localConn, err := net.Dial("tcp", t.LocalAddr)
	if err != nil {
		return
	}
	defer localConn.Close()

	// Bidirectional copy
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(localConn, remoteConn)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(remoteConn, localConn)
		done <- struct{}{}
	}()

	// Wait for one direction to finish
	select {
	case <-done:
	case <-t.ctx.Done():
	}
}

// FormatTunnelList formats a list of tunnels for display.
func FormatTunnelList(tunnels []*Tunnel) string {
	if len(tunnels) == 0 {
		return "No active tunnels.\n"
	}

	result := "Active SSH Tunnels:\n"
	result += "-----------------------------------------------------------\n"
	result += fmt.Sprintf("%-20s | %-8s | %-20s | %-20s\n", "ID", "Type", "Local", "Remote")
	result += "-----------------------------------------------------------\n"

	for _, t := range tunnels {
		typeStr := "LOCAL"
		if t.Type == RemoteForward {
			typeStr = "REMOTE"
		} else if t.Type == DynamicForward {
			typeStr = "DYNAMIC"
		}

		status := ""
		if t.IsActive() {
			status = " [active]"
		}

		result += fmt.Sprintf("%-20s | %-8s | %-20s | %-20s%s\n",
			truncate(t.ID, 20), typeStr, t.LocalAddr, t.RemoteAddr, status)
	}

	return result
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
