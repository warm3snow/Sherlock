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

package sshclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetDefaultKeyPaths(t *testing.T) {
	paths := GetDefaultKeyPaths()

	if len(paths) == 0 {
		t.Error("GetDefaultKeyPaths should return at least one path")
	}

	// Check that all paths end with expected key names
	expectedNames := []string{"id_ed25519", "id_ecdsa", "id_rsa", "id_dsa"}
	for i, path := range paths {
		if i >= len(expectedNames) {
			break
		}
		if !strings.HasSuffix(path, expectedNames[i]) {
			t.Errorf("Expected path %d to end with %s, got %s", i, expectedNames[i], path)
		}
	}
}

func TestGetAgentSigners(t *testing.T) {
	// Test when SSH_AUTH_SOCK is not set
	originalSocket := os.Getenv("SSH_AUTH_SOCK")
	os.Unsetenv("SSH_AUTH_SOCK")
	defer func() {
		if originalSocket != "" {
			os.Setenv("SSH_AUTH_SOCK", originalSocket)
		}
	}()

	signers, conn := getAgentSigners()
	if len(signers) != 0 {
		t.Error("getAgentSigners should return empty slice when SSH_AUTH_SOCK is not set")
	}
	if conn != nil {
		t.Error("getAgentSigners should return nil conn when SSH_AUTH_SOCK is not set")
	}
}

func TestLoadPrivateKey(t *testing.T) {
	// Test with non-existent file
	_, err := loadPrivateKey("/nonexistent/path/to/key", "")
	if err == nil {
		t.Error("loadPrivateKey should return error for non-existent file")
	}
	if !strings.Contains(err.Error(), "failed to read private key") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestNewClientWithoutAuthMethods(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Save original home and restore after test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create .ssh directory
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	// Unset SSH_AUTH_SOCK to ensure agent auth fails
	originalSocket := os.Getenv("SSH_AUTH_SOCK")
	os.Unsetenv("SSH_AUTH_SOCK")
	defer func() {
		if originalSocket != "" {
			os.Setenv("SSH_AUTH_SOCK", originalSocket)
		}
	}()

	cfg := &Config{
		HostInfo: &HostInfo{
			Host: "example.com",
			Port: 22,
			User: "testuser",
		},
		// No password, no key path - should fail
	}

	_, err := NewClient(cfg)
	if err == nil {
		t.Error("NewClient should return error when no auth methods are available")
	}
	// Check that error message contains expected text
	if err != nil && !strings.Contains(err.Error(), "authentication method") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestNewClientWithPassword(t *testing.T) {
	cfg := &Config{
		HostInfo: &HostInfo{
			Host: "example.com",
			Port: 22,
			User: "testuser",
		},
		Password: "testpassword",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient should succeed with password: %v", err)
	}
	if client == nil {
		t.Error("NewClient should return a non-nil client")
	}
	if client.IsConnected() {
		t.Error("Client should not be connected immediately after creation")
	}
}

func TestIsValidTermType(t *testing.T) {
	tests := []struct {
		name     string
		term     string
		expected bool
	}{
		{"valid xterm", "xterm", true},
		{"valid xterm-256color", "xterm-256color", true},
		{"valid screen", "screen", true},
		{"valid linux", "linux", true},
		{"valid vt100", "vt100", true},
		{"valid with underscore", "xterm_256color", true},
		{"empty string", "", false},
		{"contains semicolon", "xterm;ls", false},
		{"contains backtick", "xterm`ls`", false},
		{"contains dollar", "xterm$HOME", false},
		{"contains space", "xterm 256color", false},
		{"contains single quote", "xterm'ls'", false},
		{"contains double quote", "xterm\"ls\"", false},
		{"contains pipe", "xterm|ls", false},
		{"contains ampersand", "xterm&ls", false},
		{"contains newline", "xterm\nls", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidTermType(tt.term); got != tt.expected {
				t.Errorf("isValidTermType(%q) = %v, want %v", tt.term, got, tt.expected)
			}
		})
	}
}
