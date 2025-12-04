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

// Package config provides configuration management for Sherlock.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SSHKeyPair represents a pair of SSH private and public key paths.
type SSHKeyPair struct {
	PrivateKeyPath string
	PublicKeyPath  string
}

// DetectSSHKeys auto-detects SSH keys from the ~/.ssh/ directory.
// It prioritizes id_ed25519 over id_rsa.
// Returns the detected key pair and a boolean indicating if keys were found.
func DetectSSHKeys() (*SSHKeyPair, bool) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, false
	}

	sshDir := filepath.Join(homeDir, ".ssh")

	// Prioritized list of key types to try
	keyTypes := []string{"id_ed25519", "id_rsa"}

	for _, keyType := range keyTypes {
		privateKeyPath := filepath.Join(sshDir, keyType)
		publicKeyPath := filepath.Join(sshDir, keyType+".pub")

		// Check if both private and public keys exist
		if _, err := os.Stat(privateKeyPath); err == nil {
			if _, err := os.Stat(publicKeyPath); err == nil {
				return &SSHKeyPair{
					PrivateKeyPath: privateKeyPath,
					PublicKeyPath:  publicKeyPath,
				}, true
			}
		}
	}

	return nil, false
}

// LLMProviderType defines the type of LLM provider.
type LLMProviderType string

const (
	// ProviderOllama represents a local Ollama instance.
	ProviderOllama LLMProviderType = "ollama"
	// ProviderOpenAI represents OpenAI API.
	ProviderOpenAI LLMProviderType = "openai"
	// ProviderDeepSeek represents DeepSeek API.
	ProviderDeepSeek LLMProviderType = "deepseek"
)

// LLMConfig holds LLM provider configuration.
type LLMConfig struct {
	// Provider specifies the LLM provider type.
	Provider LLMProviderType `json:"provider"`
	// APIKey is the API key for cloud providers (OpenAI, DeepSeek).
	APIKey string `json:"api_key,omitempty"`
	// BaseURL is the base URL for the LLM API.
	BaseURL string `json:"base_url,omitempty"`
	// Model specifies which model to use.
	Model string `json:"model"`
	// Temperature controls randomness in generation.
	Temperature float32 `json:"temperature,omitempty"`
}

// SSHKeyConfig holds SSH key configuration.
type SSHKeyConfig struct {
	// PrivateKeyPath is the path to the private key file.
	PrivateKeyPath string `json:"private_key_path"`
	// PublicKeyPath is the path to the public key file.
	PublicKeyPath string `json:"public_key_path"`
	// AutoAddToRemote indicates whether to automatically add the public key to remote authorized_keys.
	AutoAddToRemote bool `json:"auto_add_to_remote"`
}

// ShellCommandsConfig holds the shell commands whitelist configuration.
type ShellCommandsConfig struct {
	// Whitelist contains custom shell commands that can be executed directly without LLM translation.
	Whitelist []string `json:"whitelist,omitempty"`
}

// ThemeType defines the type of UI theme.
type ThemeType string

const (
	// ThemeDefault is the simple default theme.
	ThemeDefault ThemeType = "default"
	// ThemeDracula is the popular dark theme with purple/pink accents.
	ThemeDracula ThemeType = "dracula"
	// ThemeSolarized is the professional solarized color scheme.
	ThemeSolarized ThemeType = "solarized"
)

// UIConfig holds the UI configuration.
type UIConfig struct {
	// Theme specifies the UI color theme (default, dracula, solarized).
	Theme ThemeType `json:"theme,omitempty"`
}

// IsValidTheme checks if a theme name is valid.
func IsValidTheme(name ThemeType) bool {
	switch name {
	case ThemeDefault, ThemeDracula, ThemeSolarized:
		return true
	default:
		return false
	}
}

// Config represents the main application configuration.
type Config struct {
	// LLM holds the LLM provider configuration.
	LLM LLMConfig `json:"llm"`
	// SSHKey holds the SSH key configuration.
	SSHKey SSHKeyConfig `json:"ssh_key"`
	// ShellCommands holds the shell commands whitelist configuration.
	ShellCommands ShellCommandsConfig `json:"shell_commands,omitempty"`
	// UI holds the UI configuration.
	UI UIConfig `json:"ui,omitempty"`
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	cfg := &Config{
		LLM: LLMConfig{
			Provider:    ProviderOllama,
			BaseURL:     "http://localhost:11434",
			Model:       "qwen2.5:latest",
			Temperature: 0.7,
		},
		SSHKey: SSHKeyConfig{
			AutoAddToRemote: true,
		},
		UI: UIConfig{
			Theme: ThemeDefault,
    },
		ShellCommands: ShellCommandsConfig{
			Whitelist: []string{"kubectl", "helm"},
		},
	}

	// Auto-detect SSH keys from ~/.ssh/ directory
	if keyPair, found := DetectSSHKeys(); found {
		cfg.SSHKey.PrivateKeyPath = keyPair.PrivateKeyPath
		cfg.SSHKey.PublicKeyPath = keyPair.PublicKeyPath
	}

	return cfg
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.LLM.Provider == "" {
		return errors.New("LLM provider is required")
	}
	if c.LLM.Model == "" {
		return errors.New("LLM model is required")
	}
	switch c.LLM.Provider {
	case ProviderOpenAI, ProviderDeepSeek:
		if c.LLM.APIKey == "" {
			return fmt.Errorf("API key is required for provider %s", c.LLM.Provider)
		}
	case ProviderOllama:
		if c.LLM.BaseURL == "" {
			return errors.New("base URL is required for Ollama provider")
		}
	default:
		return fmt.Errorf("unsupported LLM provider: %s", c.LLM.Provider)
	}

	// Validate theme if specified
	if c.UI.Theme != "" && !IsValidTheme(c.UI.Theme) {
		return fmt.Errorf("unsupported UI theme: %s (valid: default, dracula, solarized)", c.UI.Theme)
	}

	return nil
}

// LoadConfig loads configuration from a file.
// If the config file doesn't exist, it creates one with default values.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Create default config and save it
			cfg := DefaultConfig()
			if saveErr := SaveConfig(path, cfg); saveErr != nil {
				// Log the save error but continue with the default config
				fmt.Fprintf(os.Stderr, "Warning: Failed to save default config to %s: %v\n", path, saveErr)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Auto-detect SSH keys if not specified in config file
	if cfg.SSHKey.PrivateKeyPath == "" || cfg.SSHKey.PublicKeyPath == "" {
		if keyPair, found := DetectSSHKeys(); found {
			cfg.SSHKey.PrivateKeyPath = keyPair.PrivateKeyPath
			cfg.SSHKey.PublicKeyPath = keyPair.PublicKeyPath
		}
	}

	return &cfg, nil
}

// SaveConfig saves configuration to a file.
func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetConfigPath returns the default configuration file path.
func GetConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "sherlock", "config.json")
}
