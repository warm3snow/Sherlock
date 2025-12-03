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

// Package sshclient provides SSH connection and command execution capabilities.
package sshclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// AuthMethod represents the SSH authentication method.
type AuthMethod string

const (
	// AuthPassword uses password authentication.
	AuthPassword AuthMethod = "password"
	// AuthPublicKey uses public key authentication.
	AuthPublicKey AuthMethod = "publickey"
)

// HostInfo contains information about a remote host.
type HostInfo struct {
	// Host is the hostname or IP address.
	Host string
	// Port is the SSH port (default 22).
	Port int
	// User is the SSH username.
	User string
}

// ParseHostInfo parses a host string in the format [user@]host[:port].
func ParseHostInfo(hostStr string) (*HostInfo, error) {
	info := &HostInfo{
		Port: 22,
	}

	// Parse user@host:port format
	if strings.Contains(hostStr, "@") {
		parts := strings.SplitN(hostStr, "@", 2)
		info.User = parts[0]
		hostStr = parts[1]
	}

	// Parse host:port format
	host, port, err := net.SplitHostPort(hostStr)
	if err != nil {
		// No port specified
		info.Host = hostStr
	} else {
		info.Host = host
		fmt.Sscanf(port, "%d", &info.Port)
	}

	if info.Host == "" {
		return nil, errors.New("host is required")
	}

	return info, nil
}

// Client represents an SSH client.
type Client struct {
	client      *ssh.Client
	hostInfo    *HostInfo
	sshConfig   *ssh.ClientConfig
	isConnected bool
}

// Config holds the configuration for creating a new SSH client.
type Config struct {
	// HostInfo contains the remote host information.
	HostInfo *HostInfo
	// Password is used for password authentication.
	Password string
	// PrivateKeyPath is the path to the private key file for public key authentication.
	PrivateKeyPath string
	// PrivateKeyPassphrase is the passphrase for the private key.
	PrivateKeyPassphrase string
	// Timeout is the connection timeout.
	Timeout time.Duration
}

// NewClient creates a new SSH client with the given configuration.
func NewClient(cfg *Config) (*Client, error) {
	if cfg.HostInfo == nil {
		return nil, errors.New("host info is required")
	}
	if cfg.HostInfo.User == "" {
		return nil, errors.New("user is required")
	}

	var authMethods []ssh.AuthMethod

	// Try public key authentication first
	if cfg.PrivateKeyPath != "" {
		keyAuth, err := publicKeyAuth(cfg.PrivateKeyPath, cfg.PrivateKeyPassphrase)
		if err == nil {
			authMethods = append(authMethods, keyAuth)
		}
	}

	// Add password authentication if provided
	if cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	}

	if len(authMethods) == 0 {
		return nil, errors.New("at least one authentication method is required")
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.HostInfo.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Implement proper host key verification
		Timeout:         timeout,
	}

	return &Client{
		hostInfo:  cfg.HostInfo,
		sshConfig: sshConfig,
	}, nil
}

// publicKeyAuth creates an ssh.AuthMethod from a private key file.
func publicKeyAuth(keyPath, passphrase string) (ssh.AuthMethod, error) {
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	var signer ssh.Signer
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey(keyData)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return ssh.PublicKeys(signer), nil
}

// Connect establishes the SSH connection.
func (c *Client) Connect(_ context.Context) error {
	if c.isConnected {
		return nil
	}

	addr := fmt.Sprintf("%s:%d", c.hostInfo.Host, c.hostInfo.Port)
	client, err := ssh.Dial("tcp", addr, c.sshConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	c.client = client
	c.isConnected = true
	return nil
}

// Close closes the SSH connection.
func (c *Client) Close() error {
	if !c.isConnected || c.client == nil {
		return nil
	}
	c.isConnected = false
	return c.client.Close()
}

// IsConnected returns true if the client is connected.
func (c *Client) IsConnected() bool {
	return c.isConnected && c.client != nil
}

// ExecuteResult represents the result of a command execution.
type ExecuteResult struct {
	// Stdout contains the standard output of the command.
	Stdout string
	// Stderr contains the standard error of the command.
	Stderr string
	// ExitCode is the exit code of the command.
	ExitCode int
	// Error contains any error that occurred during execution.
	Error error
}

// Executor is the interface for command execution on local or remote hosts.
type Executor interface {
	// Execute runs a command and returns the result.
	Execute(ctx context.Context, command string) *ExecuteResult
	// IsConnected returns true if the executor is ready.
	IsConnected() bool
	// Close closes the executor.
	Close() error
	// HostInfoString returns a string representation of the host.
	HostInfoString() string
}

// Execute executes a command on the remote host.
func (c *Client) Execute(_ context.Context, command string) *ExecuteResult {
	result := &ExecuteResult{}

	if !c.isConnected {
		result.Error = errors.New("not connected")
		return result
	}

	session, err := c.client.NewSession()
	if err != nil {
		result.Error = fmt.Errorf("failed to create session: %w", err)
		return result
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(command)
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	if err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitStatus()
		} else {
			result.Error = err
		}
	}

	return result
}

// AddPublicKeyToAuthorizedKeys adds the local public key to the remote host's authorized_keys.
func (c *Client) AddPublicKeyToAuthorizedKeys(ctx context.Context, publicKeyPath string) error {
	if !c.isConnected {
		return errors.New("not connected")
	}

	// Read the public key
	pubKeyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	pubKey := strings.TrimSpace(string(pubKeyData))

	// Check if the key already exists
	checkCmd := fmt.Sprintf("grep -qF '%s' ~/.ssh/authorized_keys 2>/dev/null && echo 'exists' || echo 'notfound'", pubKey)
	result := c.Execute(ctx, checkCmd)
	if result.Error != nil {
		return fmt.Errorf("failed to check authorized_keys: %w", result.Error)
	}

	if strings.TrimSpace(result.Stdout) == "exists" {
		return nil // Key already exists
	}

	// Create .ssh directory if it doesn't exist
	mkdirResult := c.Execute(ctx, "mkdir -p ~/.ssh && chmod 700 ~/.ssh")
	if mkdirResult.Error != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", mkdirResult.Error)
	}

	// Add the public key to authorized_keys
	addCmd := fmt.Sprintf("echo '%s' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys", pubKey)
	addResult := c.Execute(ctx, addCmd)
	if addResult.Error != nil {
		return fmt.Errorf("failed to add public key: %w", addResult.Error)
	}

	return nil
}

// HostInfoString returns a string representation of the host info.
func (c *Client) HostInfoString() string {
	if c.hostInfo == nil {
		return ""
	}
	return fmt.Sprintf("%s@%s:%d", c.hostInfo.User, c.hostInfo.Host, c.hostInfo.Port)
}

// GetSSHKeyPaths returns the default SSH key paths.
func GetSSHKeyPaths() (privateKeyPath, publicKeyPath string) {
	homeDir, _ := os.UserHomeDir()
	privateKeyPath = filepath.Join(homeDir, ".ssh", "id_rsa")
	publicKeyPath = filepath.Join(homeDir, ".ssh", "id_rsa.pub")
	return
}
