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
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SSHConfigHost represents a host entry from SSH config file.
type SSHConfigHost struct {
	Host         string   // Host pattern (alias)
	Hostname     string   // Actual hostname or IP
	Port         int      // SSH port
	User         string   // Username
	IdentityFile []string // Paths to identity files (private keys)
}

// SSHConfig represents the parsed SSH config file.
type SSHConfig struct {
	hosts map[string]*SSHConfigHost
}

// ParseSSHConfig parses the SSH config file (~/.ssh/config).
func ParseSSHConfig() (*SSHConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return &SSHConfig{hosts: make(map[string]*SSHConfigHost)}, nil
	}

	configPath := filepath.Join(homeDir, ".ssh", "config")
	return ParseSSHConfigFile(configPath)
}

// ParseSSHConfigFile parses an SSH config file at the given path.
func ParseSSHConfigFile(configPath string) (*SSHConfig, error) {
	config := &SSHConfig{hosts: make(map[string]*SSHConfigHost)}

	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil // Return empty config if file doesn't exist
		}
		return nil, err
	}
	defer file.Close()

	var currentHost *SSHConfigHost
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split line into key and value
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		key := strings.ToLower(parts[0])
		value := strings.Join(parts[1:], " ")

		switch key {
		case "host":
			// Start a new host entry
			currentHost = &SSHConfigHost{
				Host: value,
				Port: 22, // Default port
			}
			config.hosts[value] = currentHost
		case "hostname":
			if currentHost != nil {
				currentHost.Hostname = value
			}
		case "port":
			if currentHost != nil {
				if port, err := strconv.Atoi(value); err == nil {
					currentHost.Port = port
				}
			}
		case "user":
			if currentHost != nil {
				currentHost.User = value
			}
		case "identityfile":
			if currentHost != nil {
				// Expand ~ to home directory
				expandedPath := expandPath(value)
				currentHost.IdentityFile = append(currentHost.IdentityFile, expandedPath)
			}
		}
	}

	return config, scanner.Err()
}

// GetHost returns the SSH config for a given host alias or hostname.
// It first looks for an exact match, then tries pattern matching with wildcards.
// More specific patterns take priority over less specific ones.
func (c *SSHConfig) GetHost(host string) *SSHConfigHost {
	// Try exact match first
	if h, ok := c.hosts[host]; ok {
		return h
	}

	// Try wildcard matching, prioritizing more specific patterns
	var bestMatch *SSHConfigHost
	bestSpecificity := -1

	for pattern, h := range c.hosts {
		if matchHostPattern(pattern, host) {
			specificity := patternSpecificity(pattern)
			if specificity > bestSpecificity {
				bestMatch = h
				bestSpecificity = specificity
			}
		}
	}

	return bestMatch
}

// patternSpecificity returns a score indicating how specific a pattern is.
// Higher scores mean more specific patterns.
func patternSpecificity(pattern string) int {
	// "*" is the least specific
	if pattern == "*" {
		return 0
	}
	// Count non-wildcard characters for specificity
	return len(pattern) - strings.Count(pattern, "*")
}

// matchHostPattern checks if a hostname matches a pattern (with * wildcard support).
func matchHostPattern(pattern, host string) bool {
	if pattern == "*" {
		return true
	}

	// Handle patterns like "*.example.com"
	if strings.HasPrefix(pattern, "*") {
		suffix := pattern[1:]
		return strings.HasSuffix(host, suffix)
	}

	// Handle patterns like "server*"
	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(host, prefix)
	}

	return pattern == host
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if homeDir, err := os.UserHomeDir(); err == nil {
			return filepath.Join(homeDir, path[2:])
		}
	}
	return path
}

// GetKnownHostsPath returns the path to the known_hosts file.
func GetKnownHostsPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".ssh", "known_hosts")
}

// CreateHostKeyCallback creates a host key callback that uses known_hosts file.
// If the known_hosts file doesn't exist or can't be read, it falls back to
// prompting the user or accepting the key (depending on strictHostKeyChecking).
func CreateHostKeyCallback(strictHostKeyChecking bool) ssh.HostKeyCallback {
	knownHostsPath := GetKnownHostsPath()

	// Try to create a callback from known_hosts file
	callback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		// known_hosts file doesn't exist or can't be read
		if strictHostKeyChecking {
			// Return a callback that rejects all unknown hosts
			return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				return fmt.Errorf("host key verification failed: strict host key checking is enabled and host '%s' is not in known_hosts file (%s)", hostname, knownHostsPath)
			}
		}
		// Return a callback that accepts all hosts (like ssh with StrictHostKeyChecking=no)
		return ssh.InsecureIgnoreHostKey()
	}

	if strictHostKeyChecking {
		return callback
	}

	// Return a callback that accepts unknown hosts but validates known ones
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := callback(hostname, remote, key)
		if err != nil {
			// Check if it's a key error (unknown host vs. changed key)
			var keyErr *knownhosts.KeyError
			if errors.As(err, &keyErr) {
				// If it's just an unknown host, accept it
				if len(keyErr.Want) == 0 {
					return nil
				}
				// If it's a changed key, reject it
				return err
			}
		}
		return err
	}
}

// AddKnownHost adds a host key to the known_hosts file.
func AddKnownHost(hostname string, remote net.Addr, key ssh.PublicKey) error {
	knownHostsPath := GetKnownHostsPath()

	// Ensure .ssh directory exists
	sshDir := filepath.Dir(knownHostsPath)
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return err
	}

	// Open file in append mode
	file, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	// Format the host key line
	// Format: hostname,ip key-type base64-key
	line := knownhosts.Line([]string{hostname}, key)
	_, err = file.WriteString(line + "\n")
	return err
}
