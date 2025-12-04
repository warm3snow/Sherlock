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
	"testing"
)

func TestParseSSHConfigFile(t *testing.T) {
	// Create a temporary SSH config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config")

	configContent := `# SSH Config Test File
Host myserver
    Hostname 192.168.1.100
    Port 2222
    User admin
    IdentityFile ~/.ssh/id_rsa_myserver

Host dev
    Hostname dev.example.com
    User developer

Host *.prod.example.com
    User produser
    Port 22
    IdentityFile ~/.ssh/id_prod

Host *
    User defaultuser
`

	if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	config, err := ParseSSHConfigFile(configPath)
	if err != nil {
		t.Fatalf("ParseSSHConfigFile failed: %v", err)
	}

	// Test exact host match
	t.Run("exact_match_myserver", func(t *testing.T) {
		host := config.GetHost("myserver")
		if host == nil {
			t.Fatal("Expected to find host 'myserver'")
		}
		if host.Hostname != "192.168.1.100" {
			t.Errorf("Expected hostname '192.168.1.100', got '%s'", host.Hostname)
		}
		if host.Port != 2222 {
			t.Errorf("Expected port 2222, got %d", host.Port)
		}
		if host.User != "admin" {
			t.Errorf("Expected user 'admin', got '%s'", host.User)
		}
	})

	t.Run("exact_match_dev", func(t *testing.T) {
		host := config.GetHost("dev")
		if host == nil {
			t.Fatal("Expected to find host 'dev'")
		}
		if host.Hostname != "dev.example.com" {
			t.Errorf("Expected hostname 'dev.example.com', got '%s'", host.Hostname)
		}
		if host.User != "developer" {
			t.Errorf("Expected user 'developer', got '%s'", host.User)
		}
	})

	t.Run("wildcard_match", func(t *testing.T) {
		host := config.GetHost("server1.prod.example.com")
		if host == nil {
			t.Fatal("Expected to find host matching '*.prod.example.com'")
		}
		if host.User != "produser" {
			t.Errorf("Expected user 'produser', got '%s'", host.User)
		}
	})

	t.Run("default_wildcard", func(t *testing.T) {
		host := config.GetHost("unknown-host")
		if host == nil {
			t.Fatal("Expected to find default host '*'")
		}
		if host.User != "defaultuser" {
			t.Errorf("Expected user 'defaultuser', got '%s'", host.User)
		}
	})
}

func TestParseSSHConfigFile_NotExists(t *testing.T) {
	config, err := ParseSSHConfigFile("/nonexistent/path/config")
	if err != nil {
		t.Fatalf("Expected no error for non-existent file, got: %v", err)
	}
	if config == nil {
		t.Fatal("Expected non-nil config")
	}
	// Should return empty config
	host := config.GetHost("anyhost")
	if host != nil {
		t.Error("Expected nil host for empty config")
	}
}

func TestMatchHostPattern(t *testing.T) {
	tests := []struct {
		pattern string
		host    string
		want    bool
	}{
		{"*", "anything", true},
		{"*.example.com", "server.example.com", true},
		{"*.example.com", "server.other.com", false},
		{"server*", "server1", true},
		{"server*", "myserver", false},
		{"exact", "exact", true},
		{"exact", "different", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.host, func(t *testing.T) {
			got := matchHostPattern(tt.pattern, tt.host)
			if got != tt.want {
				t.Errorf("matchHostPattern(%q, %q) = %v, want %v", tt.pattern, tt.host, got, tt.want)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	homeDir, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"~/.ssh/id_rsa", filepath.Join(homeDir, ".ssh", "id_rsa")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandPath(tt.input)
			if got != tt.expected {
				t.Errorf("expandPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGetKnownHostsPath(t *testing.T) {
	path := GetKnownHostsPath()
	if path == "" {
		t.Error("GetKnownHostsPath should return a non-empty path")
	}
	// Should end with known_hosts
	if filepath.Base(path) != "known_hosts" {
		t.Errorf("Expected path to end with 'known_hosts', got %s", path)
	}
}

func TestApplySSHConfig(t *testing.T) {
	// Create a mock SSH config
	config := &SSHConfig{
		hosts: map[string]*SSHConfigHost{
			"myalias": {
				Host:         "myalias",
				Hostname:     "actual.host.com",
				Port:         2222,
				User:         "configuser",
				IdentityFile: []string{"/path/to/key"},
			},
		},
	}

	t.Run("apply_alias_settings", func(t *testing.T) {
		hostInfo := &HostInfo{
			Host: "myalias",
			Port: 22,
			User: "",
		}
		result, identityFiles := applySSHConfig(config, hostInfo)

		if result.Host != "actual.host.com" {
			t.Errorf("Expected host 'actual.host.com', got '%s'", result.Host)
		}
		if result.Port != 2222 {
			t.Errorf("Expected port 2222, got %d", result.Port)
		}
		if result.User != "configuser" {
			t.Errorf("Expected user 'configuser', got '%s'", result.User)
		}
		if len(identityFiles) != 1 || identityFiles[0] != "/path/to/key" {
			t.Errorf("Expected identity file '/path/to/key', got %v", identityFiles)
		}
	})

	t.Run("preserve_explicit_settings", func(t *testing.T) {
		hostInfo := &HostInfo{
			Host: "myalias",
			Port: 3333, // Explicitly set
			User: "explicituser",
		}
		result, _ := applySSHConfig(config, hostInfo)

		if result.Host != "actual.host.com" {
			t.Errorf("Expected host 'actual.host.com', got '%s'", result.Host)
		}
		// Port should be preserved since it was explicitly set (not default 22)
		if result.Port != 3333 {
			t.Errorf("Expected port 3333 to be preserved, got %d", result.Port)
		}
		// User should be preserved since it was explicitly set
		if result.User != "explicituser" {
			t.Errorf("Expected user 'explicituser' to be preserved, got '%s'", result.User)
		}
	})

	t.Run("unknown_host", func(t *testing.T) {
		hostInfo := &HostInfo{
			Host: "unknown",
			Port: 22,
			User: "myuser",
		}
		result, identityFiles := applySSHConfig(config, hostInfo)

		// Should return unchanged host info
		if result.Host != "unknown" {
			t.Errorf("Expected host 'unknown', got '%s'", result.Host)
		}
		if result.Port != 22 {
			t.Errorf("Expected port 22, got %d", result.Port)
		}
		if result.User != "myuser" {
			t.Errorf("Expected user 'myuser', got '%s'", result.User)
		}
		if len(identityFiles) != 0 {
			t.Errorf("Expected no identity files, got %v", identityFiles)
		}
	})
}
