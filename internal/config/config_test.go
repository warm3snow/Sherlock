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

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectSSHKeys_NoKeys(t *testing.T) {
	// Create a temporary directory to use as HOME
	tmpDir := t.TempDir()

	// Save original HOME and restore after test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create empty .ssh directory
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	keyPair, found := DetectSSHKeys()
	if found {
		t.Error("DetectSSHKeys should return false when no keys exist")
	}
	if keyPair != nil {
		t.Error("DetectSSHKeys should return nil when no keys exist")
	}
}

func TestDetectSSHKeys_OnlyEd25519(t *testing.T) {
	// Create a temporary directory to use as HOME
	tmpDir := t.TempDir()

	// Save original HOME and restore after test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create .ssh directory with ed25519 keys
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	// Create dummy ed25519 key files
	privateKeyPath := filepath.Join(sshDir, "id_ed25519")
	publicKeyPath := filepath.Join(sshDir, "id_ed25519.pub")
	if err := os.WriteFile(privateKeyPath, []byte("dummy private key"), 0600); err != nil {
		t.Fatalf("Failed to create private key: %v", err)
	}
	if err := os.WriteFile(publicKeyPath, []byte("dummy public key"), 0644); err != nil {
		t.Fatalf("Failed to create public key: %v", err)
	}

	keyPair, found := DetectSSHKeys()
	if !found {
		t.Error("DetectSSHKeys should return true when ed25519 keys exist")
	}
	if keyPair == nil {
		t.Fatal("DetectSSHKeys should return non-nil key pair")
	}
	if !strings.HasSuffix(keyPair.PrivateKeyPath, "id_ed25519") {
		t.Errorf("Expected private key path to end with id_ed25519, got %s", keyPair.PrivateKeyPath)
	}
	if !strings.HasSuffix(keyPair.PublicKeyPath, "id_ed25519.pub") {
		t.Errorf("Expected public key path to end with id_ed25519.pub, got %s", keyPair.PublicKeyPath)
	}
}

func TestDetectSSHKeys_OnlyRSA(t *testing.T) {
	// Create a temporary directory to use as HOME
	tmpDir := t.TempDir()

	// Save original HOME and restore after test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create .ssh directory with rsa keys only
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	// Create dummy rsa key files
	privateKeyPath := filepath.Join(sshDir, "id_rsa")
	publicKeyPath := filepath.Join(sshDir, "id_rsa.pub")
	if err := os.WriteFile(privateKeyPath, []byte("dummy private key"), 0600); err != nil {
		t.Fatalf("Failed to create private key: %v", err)
	}
	if err := os.WriteFile(publicKeyPath, []byte("dummy public key"), 0644); err != nil {
		t.Fatalf("Failed to create public key: %v", err)
	}

	keyPair, found := DetectSSHKeys()
	if !found {
		t.Error("DetectSSHKeys should return true when rsa keys exist")
	}
	if keyPair == nil {
		t.Fatal("DetectSSHKeys should return non-nil key pair")
	}
	if !strings.HasSuffix(keyPair.PrivateKeyPath, "id_rsa") {
		t.Errorf("Expected private key path to end with id_rsa, got %s", keyPair.PrivateKeyPath)
	}
	if !strings.HasSuffix(keyPair.PublicKeyPath, "id_rsa.pub") {
		t.Errorf("Expected public key path to end with id_rsa.pub, got %s", keyPair.PublicKeyPath)
	}
}

func TestDetectSSHKeys_BothExist_Ed25519Prioritized(t *testing.T) {
	// Create a temporary directory to use as HOME
	tmpDir := t.TempDir()

	// Save original HOME and restore after test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create .ssh directory with both ed25519 and rsa keys
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	// Create dummy ed25519 key files
	ed25519PrivatePath := filepath.Join(sshDir, "id_ed25519")
	ed25519PublicPath := filepath.Join(sshDir, "id_ed25519.pub")
	if err := os.WriteFile(ed25519PrivatePath, []byte("dummy ed25519 private key"), 0600); err != nil {
		t.Fatalf("Failed to create ed25519 private key: %v", err)
	}
	if err := os.WriteFile(ed25519PublicPath, []byte("dummy ed25519 public key"), 0644); err != nil {
		t.Fatalf("Failed to create ed25519 public key: %v", err)
	}

	// Create dummy rsa key files
	rsaPrivatePath := filepath.Join(sshDir, "id_rsa")
	rsaPublicPath := filepath.Join(sshDir, "id_rsa.pub")
	if err := os.WriteFile(rsaPrivatePath, []byte("dummy rsa private key"), 0600); err != nil {
		t.Fatalf("Failed to create rsa private key: %v", err)
	}
	if err := os.WriteFile(rsaPublicPath, []byte("dummy rsa public key"), 0644); err != nil {
		t.Fatalf("Failed to create rsa public key: %v", err)
	}

	keyPair, found := DetectSSHKeys()
	if !found {
		t.Error("DetectSSHKeys should return true when keys exist")
	}
	if keyPair == nil {
		t.Fatal("DetectSSHKeys should return non-nil key pair")
	}
	// ed25519 should be prioritized over rsa
	if !strings.HasSuffix(keyPair.PrivateKeyPath, "id_ed25519") {
		t.Errorf("Expected ed25519 to be prioritized over rsa, got %s", keyPair.PrivateKeyPath)
	}
	if !strings.HasSuffix(keyPair.PublicKeyPath, "id_ed25519.pub") {
		t.Errorf("Expected ed25519.pub to be prioritized over rsa.pub, got %s", keyPair.PublicKeyPath)
	}
}

func TestDetectSSHKeys_MissingPublicKey(t *testing.T) {
	// Create a temporary directory to use as HOME
	tmpDir := t.TempDir()

	// Save original HOME and restore after test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create .ssh directory with only private key (no public key)
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	// Create only private key
	privateKeyPath := filepath.Join(sshDir, "id_ed25519")
	if err := os.WriteFile(privateKeyPath, []byte("dummy private key"), 0600); err != nil {
		t.Fatalf("Failed to create private key: %v", err)
	}

	keyPair, found := DetectSSHKeys()
	if found {
		t.Error("DetectSSHKeys should return false when public key is missing")
	}
	if keyPair != nil {
		t.Error("DetectSSHKeys should return nil when public key is missing")
	}
}

func TestDefaultConfig_AutoDetectsKeys(t *testing.T) {
	// Create a temporary directory to use as HOME
	tmpDir := t.TempDir()

	// Save original HOME and restore after test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create .ssh directory with ed25519 keys
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	privateKeyPath := filepath.Join(sshDir, "id_ed25519")
	publicKeyPath := filepath.Join(sshDir, "id_ed25519.pub")
	if err := os.WriteFile(privateKeyPath, []byte("dummy private key"), 0600); err != nil {
		t.Fatalf("Failed to create private key: %v", err)
	}
	if err := os.WriteFile(publicKeyPath, []byte("dummy public key"), 0644); err != nil {
		t.Fatalf("Failed to create public key: %v", err)
	}

	cfg := DefaultConfig()

	if !strings.HasSuffix(cfg.SSHKey.PrivateKeyPath, "id_ed25519") {
		t.Errorf("Expected DefaultConfig to auto-detect ed25519 private key, got %s", cfg.SSHKey.PrivateKeyPath)
	}
	if !strings.HasSuffix(cfg.SSHKey.PublicKeyPath, "id_ed25519.pub") {
		t.Errorf("Expected DefaultConfig to auto-detect ed25519 public key, got %s", cfg.SSHKey.PublicKeyPath)
	}
}

func TestDefaultConfig_NoKeysFound(t *testing.T) {
	// Create a temporary directory to use as HOME
	tmpDir := t.TempDir()

	// Save original HOME and restore after test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create empty .ssh directory
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	cfg := DefaultConfig()

	// When no keys are found, paths should be empty
	if cfg.SSHKey.PrivateKeyPath != "" {
		t.Errorf("Expected empty private key path when no keys found, got %s", cfg.SSHKey.PrivateKeyPath)
	}
	if cfg.SSHKey.PublicKeyPath != "" {
		t.Errorf("Expected empty public key path when no keys found, got %s", cfg.SSHKey.PublicKeyPath)
	}
	// AutoAddToRemote should still be true
	if !cfg.SSHKey.AutoAddToRemote {
		t.Error("Expected AutoAddToRemote to be true by default")
	}
}

func TestLoadConfig_AutoDetectsKeys(t *testing.T) {
	// Create a temporary directory to use as HOME
	tmpDir := t.TempDir()

	// Save original HOME and restore after test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create .ssh directory with ed25519 keys
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	privateKeyPath := filepath.Join(sshDir, "id_ed25519")
	publicKeyPath := filepath.Join(sshDir, "id_ed25519.pub")
	if err := os.WriteFile(privateKeyPath, []byte("dummy private key"), 0600); err != nil {
		t.Fatalf("Failed to create private key: %v", err)
	}
	if err := os.WriteFile(publicKeyPath, []byte("dummy public key"), 0644); err != nil {
		t.Fatalf("Failed to create public key: %v", err)
	}

	// Create a config file without SSH key paths
	configDir := filepath.Join(tmpDir, ".config", "sherlock")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	configContent := `{
		"llm": {
			"provider": "ollama",
			"base_url": "http://localhost:11434",
			"model": "test-model"
		}
	}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// LoadConfig should have auto-detected the SSH keys
	if !strings.HasSuffix(cfg.SSHKey.PrivateKeyPath, "id_ed25519") {
		t.Errorf("Expected LoadConfig to auto-detect ed25519 private key, got %s", cfg.SSHKey.PrivateKeyPath)
	}
	if !strings.HasSuffix(cfg.SSHKey.PublicKeyPath, "id_ed25519.pub") {
		t.Errorf("Expected LoadConfig to auto-detect ed25519 public key, got %s", cfg.SSHKey.PublicKeyPath)
	}
}

func TestLoadConfig_DoesNotOverrideConfiguredKeys(t *testing.T) {
	// Create a temporary directory to use as HOME
	tmpDir := t.TempDir()

	// Save original HOME and restore after test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create .ssh directory with ed25519 keys
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	privateKeyPath := filepath.Join(sshDir, "id_ed25519")
	publicKeyPath := filepath.Join(sshDir, "id_ed25519.pub")
	if err := os.WriteFile(privateKeyPath, []byte("dummy private key"), 0600); err != nil {
		t.Fatalf("Failed to create private key: %v", err)
	}
	if err := os.WriteFile(publicKeyPath, []byte("dummy public key"), 0644); err != nil {
		t.Fatalf("Failed to create public key: %v", err)
	}

	// Create a config file with explicitly configured SSH key paths
	configDir := filepath.Join(tmpDir, ".config", "sherlock")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	configContent := `{
		"llm": {
			"provider": "ollama",
			"base_url": "http://localhost:11434",
			"model": "test-model"
		},
		"ssh_key": {
			"private_key_path": "/custom/path/id_rsa",
			"public_key_path": "/custom/path/id_rsa.pub"
		}
	}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// LoadConfig should NOT override configured SSH keys
	if cfg.SSHKey.PrivateKeyPath != "/custom/path/id_rsa" {
		t.Errorf("Expected LoadConfig to preserve configured private key path, got %s", cfg.SSHKey.PrivateKeyPath)
	}
	if cfg.SSHKey.PublicKeyPath != "/custom/path/id_rsa.pub" {
		t.Errorf("Expected LoadConfig to preserve configured public key path, got %s", cfg.SSHKey.PublicKeyPath)
	}
}

func TestLoadConfig_CreatesConfigFileIfNotExist(t *testing.T) {
	// Create a temporary directory to use as HOME
	tmpDir := t.TempDir()

	// Save original HOME and restore after test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create .ssh directory with ed25519 keys for auto-detection
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	privateKeyPath := filepath.Join(sshDir, "id_ed25519")
	publicKeyPath := filepath.Join(sshDir, "id_ed25519.pub")
	if err := os.WriteFile(privateKeyPath, []byte("dummy private key"), 0600); err != nil {
		t.Fatalf("Failed to create private key: %v", err)
	}
	if err := os.WriteFile(publicKeyPath, []byte("dummy public key"), 0644); err != nil {
		t.Fatalf("Failed to create public key: %v", err)
	}

	// Define config path that doesn't exist
	configDir := filepath.Join(tmpDir, ".config", "sherlock")
	configPath := filepath.Join(configDir, "config.json")

	// Verify config file doesn't exist initially
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatal("Config file should not exist before LoadConfig")
	}

	// Load config - this should create the config file
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify config file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file should have been created by LoadConfig")
	}

	// Verify the content of the created config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read created config file: %v", err)
	}

	var savedCfg Config
	if err := json.Unmarshal(data, &savedCfg); err != nil {
		t.Fatalf("Failed to parse created config file: %v", err)
	}

	// Verify LLM config
	if savedCfg.LLM.Provider != ProviderOllama {
		t.Errorf("Expected LLM provider to be ollama, got %s", savedCfg.LLM.Provider)
	}
	if savedCfg.LLM.BaseURL != "http://localhost:11434" {
		t.Errorf("Expected LLM base_url to be http://localhost:11434, got %s", savedCfg.LLM.BaseURL)
	}
	if savedCfg.LLM.Model != "qwen2.5:latest" {
		t.Errorf("Expected LLM model to be qwen2.5:latest, got %s", savedCfg.LLM.Model)
	}
	if savedCfg.LLM.Temperature != 0.7 {
		t.Errorf("Expected LLM temperature to be 0.7, got %f", savedCfg.LLM.Temperature)
	}

	// Verify shell_commands whitelist
	expectedWhitelist := []string{"kubectl", "helm"}
	if len(savedCfg.ShellCommands.Whitelist) != len(expectedWhitelist) {
		t.Errorf("Expected shell_commands whitelist length %d, got %d", len(expectedWhitelist), len(savedCfg.ShellCommands.Whitelist))
	} else {
		for i, cmd := range expectedWhitelist {
			if savedCfg.ShellCommands.Whitelist[i] != cmd {
				t.Errorf("Expected whitelist[%d] to be %s, got %s", i, cmd, savedCfg.ShellCommands.Whitelist[i])
			}
		}
	}

	// Verify the returned config has the same values
	if cfg.LLM.Model != "qwen2.5:latest" {
		t.Errorf("Expected returned config model to be qwen2.5:latest, got %s", cfg.LLM.Model)
	}
	if len(cfg.ShellCommands.Whitelist) != 2 {
		t.Errorf("Expected returned config whitelist length 2, got %d", len(cfg.ShellCommands.Whitelist))
	}
}

func TestDefaultConfig_HasShellCommandsWhitelist(t *testing.T) {
	cfg := DefaultConfig()

	expectedWhitelist := []string{"kubectl", "helm"}
	if len(cfg.ShellCommands.Whitelist) != len(expectedWhitelist) {
		t.Fatalf("Expected shell_commands whitelist length %d, got %d", len(expectedWhitelist), len(cfg.ShellCommands.Whitelist))
	}
	for i, cmd := range expectedWhitelist {
		if cfg.ShellCommands.Whitelist[i] != cmd {
			t.Errorf("Expected whitelist[%d] to be %s, got %s", i, cmd, cfg.ShellCommands.Whitelist[i])
		}
	}
}

func TestDefaultConfig_HasCorrectModel(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.LLM.Model != "qwen2.5:latest" {
		t.Errorf("Expected default model to be qwen2.5:latest, got %s", cfg.LLM.Model)
	}
}
