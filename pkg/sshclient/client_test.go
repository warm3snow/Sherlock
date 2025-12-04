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
	"context"
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

func TestLocalClientCd(t *testing.T) {
	client := NewLocalClient()
	ctx := context.Background()

	// Get initial working directory
	initialCwd := client.GetCwd()
	if initialCwd == "" {
		t.Error("Initial cwd should not be empty")
	}

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Test cd to absolute path
	result := client.Execute(ctx, "cd "+tmpDir)
	if result.Error != nil {
		t.Errorf("cd to temp directory failed: %v", result.Error)
	}
	if result.ExitCode != 0 {
		t.Errorf("cd should have exit code 0, got %d, stderr: %s", result.ExitCode, result.Stderr)
	}
	if client.GetCwd() != tmpDir {
		t.Errorf("cwd should be %s, got %s", tmpDir, client.GetCwd())
	}

	// Test that subsequent commands run in the new directory
	result = client.Execute(ctx, "pwd")
	if result.Error != nil {
		t.Errorf("pwd failed: %v", result.Error)
	}
	pwdOutput := strings.TrimSpace(result.Stdout)
	if pwdOutput != tmpDir {
		t.Errorf("pwd should return %s, got %s", tmpDir, pwdOutput)
	}

	// Test cd with relative path
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}
	result = client.Execute(ctx, "cd "+tmpDir)
	if result.Error != nil {
		t.Errorf("cd failed: %v", result.Error)
	}
	result = client.Execute(ctx, "cd subdir")
	if result.Error != nil {
		t.Errorf("cd to subdir failed: %v", result.Error)
	}
	if client.GetCwd() != subDir {
		t.Errorf("cwd should be %s, got %s", subDir, client.GetCwd())
	}

	// Test cd ..
	result = client.Execute(ctx, "cd ..")
	if result.Error != nil {
		t.Errorf("cd .. failed: %v", result.Error)
	}
	if client.GetCwd() != tmpDir {
		t.Errorf("cwd should be %s after cd .., got %s", tmpDir, client.GetCwd())
	}

	// Test cd to non-existent directory
	result = client.Execute(ctx, "cd /nonexistent/directory/path")
	if result.ExitCode != 1 {
		t.Errorf("cd to non-existent directory should have exit code 1, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Stderr, "No such file or directory") {
		t.Errorf("cd to non-existent directory should have appropriate error message, got: %s", result.Stderr)
	}
	// cwd should remain unchanged
	if client.GetCwd() != tmpDir {
		t.Errorf("cwd should still be %s after failed cd, got %s", tmpDir, client.GetCwd())
	}

	// Test cd to home directory (bare cd)
	result = client.Execute(ctx, "cd")
	if result.Error != nil {
		t.Errorf("cd to home failed: %v", result.Error)
	}
	homeDir, _ := os.UserHomeDir()
	if client.GetCwd() != homeDir {
		t.Errorf("cwd should be home directory %s, got %s", homeDir, client.GetCwd())
	}
}

func TestLocalClientCdTilde(t *testing.T) {
	client := NewLocalClient()
	ctx := context.Background()

	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Could not get home directory")
	}

	// Test cd ~
	tmpDir := t.TempDir()
	client.Execute(ctx, "cd "+tmpDir)
	result := client.Execute(ctx, "cd ~")
	if result.Error != nil {
		t.Errorf("cd ~ failed: %v", result.Error)
	}
	if client.GetCwd() != homeDir {
		t.Errorf("cd ~ should go to %s, got %s", homeDir, client.GetCwd())
	}

	// Test cd ~/subpath (if it exists)
	// First, go back to temp dir
	client.Execute(ctx, "cd "+tmpDir)

	// Check if ~/.ssh exists (a common directory)
	sshDir := filepath.Join(homeDir, ".ssh")
	if _, err := os.Stat(sshDir); err == nil {
		result = client.Execute(ctx, "cd ~/.ssh")
		if result.Error != nil {
			t.Errorf("cd ~/.ssh failed: %v", result.Error)
		}
		if client.GetCwd() != sshDir {
			t.Errorf("cd ~/.ssh should go to %s, got %s", sshDir, client.GetCwd())
		}
	}
}
