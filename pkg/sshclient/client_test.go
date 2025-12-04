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

func TestGetSSHKeyPaths(t *testing.T) {
	privateKeyPath, publicKeyPath := GetSSHKeyPaths()

	if privateKeyPath == "" {
		t.Error("Private key path should not be empty")
	}
	if publicKeyPath == "" {
		t.Error("Public key path should not be empty")
	}

	// Check that paths end with expected suffixes
	if !strings.HasSuffix(privateKeyPath, "id_rsa") {
		t.Errorf("Private key path should end with id_rsa, got %s", privateKeyPath)
	}
	if !strings.HasSuffix(publicKeyPath, "id_rsa.pub") {
		t.Errorf("Public key path should end with id_rsa.pub, got %s", publicKeyPath)
	}
}

func TestSshAgentAuth(t *testing.T) {
	// Test when SSH_AUTH_SOCK is not set
	originalSocket := os.Getenv("SSH_AUTH_SOCK")
	os.Unsetenv("SSH_AUTH_SOCK")
	defer func() {
		if originalSocket != "" {
			os.Setenv("SSH_AUTH_SOCK", originalSocket)
		}
	}()

	auth, conn := sshAgentAuth()
	if auth != nil {
		t.Error("sshAgentAuth should return nil auth when SSH_AUTH_SOCK is not set")
	}
	if conn != nil {
		t.Error("sshAgentAuth should return nil conn when SSH_AUTH_SOCK is not set")
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

func TestParseHostInfo(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *HostInfo
		wantErr bool
	}{
		{
			name:  "simple host",
			input: "192.168.1.100",
			want:  &HostInfo{Host: "192.168.1.100", Port: 22},
		},
		{
			name:  "host with port",
			input: "192.168.1.100:2222",
			want:  &HostInfo{Host: "192.168.1.100", Port: 2222},
		},
		{
			name:  "user@host",
			input: "root@192.168.1.100",
			want:  &HostInfo{Host: "192.168.1.100", Port: 22, User: "root"},
		},
		{
			name:  "user@host:port",
			input: "admin@192.168.1.100:2222",
			want:  &HostInfo{Host: "192.168.1.100", Port: 2222, User: "admin"},
		},
		{
			name:    "empty host",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHostInfo(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("ParseHostInfo should return error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseHostInfo returned unexpected error: %v", err)
			}
			if got.Host != tt.want.Host {
				t.Errorf("Host = %s, want %s", got.Host, tt.want.Host)
			}
			if got.Port != tt.want.Port {
				t.Errorf("Port = %d, want %d", got.Port, tt.want.Port)
			}
			if got.User != tt.want.User {
				t.Errorf("User = %s, want %s", got.User, tt.want.User)
			}
		})
	}
}
