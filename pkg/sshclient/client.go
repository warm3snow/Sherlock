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
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

// ShellEscape escapes a string for safe use in shell commands.
// It wraps the string in single quotes and escapes any single quotes within.
func ShellEscape(s string) string {
	// Single quotes are the safest way to escape in shell
	// Replace single quotes with '\'' (end quote, escaped quote, start quote)
	escaped := strings.ReplaceAll(s, "'", "'\\''")
	return "'" + escaped + "'"
}

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

// Client represents an SSH client.
type Client struct {
	client      *ssh.Client
	hostInfo    *HostInfo
	sshConfig   *ssh.ClientConfig
	isConnected bool
	agentConn   net.Conn // Connection to SSH agent, if used
	cwd         string   // current working directory on remote host
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
	// StrictHostKeyChecking enables strict host key checking (like SSH).
	// When true, connections to unknown hosts are rejected.
	// When false (default), unknown hosts are accepted but changed keys are rejected.
	StrictHostKeyChecking bool
	// UseSSHConfig enables reading SSH config file (~/.ssh/config) for host settings.
	// Default is true.
	UseSSHConfig *bool
}

// NewClient creates a new SSH client with the given configuration.
// It follows SSH-like behavior:
// 1. Reads ~/.ssh/config for host-specific settings (user, port, identity files)
// 2. Tries SSH agent authentication first
// 3. Tries public key authentication with configured and default keys
// 4. Falls back to password authentication if provided
// 5. Uses ~/.ssh/known_hosts for host key verification
func NewClient(cfg *Config) (*Client, error) {
	if cfg.HostInfo == nil {
		return nil, errors.New("host info is required")
	}

	// Apply SSH config settings if enabled (default: true)
	useSSHConfig := cfg.UseSSHConfig == nil || *cfg.UseSSHConfig
	hostInfo := cfg.HostInfo
	var sshConfigIdentityFiles []string

	if useSSHConfig {
		sshConfig, err := ParseSSHConfig()
		if err == nil {
			hostInfo, sshConfigIdentityFiles = applySSHConfig(sshConfig, cfg.HostInfo)
		}
	}

	if hostInfo.User == "" {
		return nil, errors.New("user is required")
	}

	var authMethods []ssh.AuthMethod
	var agentConn net.Conn

	// Collect all signers into a single slice to avoid multiple auth attempts
	// This prevents exceeding MaxAuthTries on the server side
	var allSigners []ssh.Signer

	// Track already-tried key paths to avoid duplicates (O(1) lookup)
	triedPaths := make(map[string]bool)

	// Get signers from SSH agent first (highest priority)
	agentSigners, conn := getAgentSigners()
	if len(agentSigners) > 0 {
		allSigners = append(allSigners, agentSigners...)
		agentConn = conn
	}

	// Get signers from identity files in SSH config
	for _, keyPath := range sshConfigIdentityFiles {
		if triedPaths[keyPath] {
			continue
		}
		signer, err := loadPrivateKey(keyPath, "")
		if err == nil {
			allSigners = append(allSigners, signer)
			triedPaths[keyPath] = true
		}
	}

	// Get signer from specified key path
	if cfg.PrivateKeyPath != "" && !triedPaths[cfg.PrivateKeyPath] {
		signer, err := loadPrivateKey(cfg.PrivateKeyPath, cfg.PrivateKeyPassphrase)
		if err == nil {
			allSigners = append(allSigners, signer)
			triedPaths[cfg.PrivateKeyPath] = true
		}
	}

	// Get signers from default SSH key paths
	for _, keyPath := range GetDefaultKeyPaths() {
		if triedPaths[keyPath] {
			continue
		}
		signer, err := loadPrivateKey(keyPath, "")
		if err == nil {
			allSigners = append(allSigners, signer)
			triedPaths[keyPath] = true
		}
	}

	// Add a single public key auth method with all signers combined
	// This ensures all keys are tried within a single authentication attempt,
	// avoiding MaxAuthTries limits on the server
	if len(allSigners) > 0 {
		authMethods = append(authMethods, ssh.PublicKeys(allSigners...))
	}

	// Add password authentication if provided (as a separate method)
	if cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	}

	if len(authMethods) == 0 {
		// Close agent connection if we're not going to use it
		if agentConn != nil {
			agentConn.Close()
		}
		return nil, errors.New("at least one authentication method is required")
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Create host key callback using known_hosts file
	hostKeyCallback := CreateHostKeyCallback(cfg.StrictHostKeyChecking)

	sshConfig := &ssh.ClientConfig{
		User:            hostInfo.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}

	return &Client{
		hostInfo:  hostInfo,
		sshConfig: sshConfig,
		agentConn: agentConn,
	}, nil
}

// applySSHConfig applies settings from SSH config file to the host info.
// It returns the updated host info and identity files to try.
func applySSHConfig(sshConfig *SSHConfig, hostInfo *HostInfo) (*HostInfo, []string) {
	configHost := sshConfig.GetHost(hostInfo.Host)
	if configHost == nil {
		return hostInfo, nil
	}

	// Create a copy of hostInfo to avoid modifying the original
	result := &HostInfo{
		Host: hostInfo.Host,
		Port: hostInfo.Port,
		User: hostInfo.User,
	}

	// Apply hostname from config if available (for aliases)
	if configHost.Hostname != "" {
		result.Host = configHost.Hostname
	}

	// Apply port from config if not already specified
	if hostInfo.Port == 22 && configHost.Port != 22 {
		result.Port = configHost.Port
	}

	// Apply user from config if not already specified
	if hostInfo.User == "" && configHost.User != "" {
		result.User = configHost.User
	}

	return result, configHost.IdentityFile
}

// getAgentSigners retrieves all signers from the SSH agent.
// It returns the signers and the connection to the agent (which should be closed when done).
func getAgentSigners() ([]ssh.Signer, net.Conn) {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil, nil
	}

	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, nil
	}

	agentClient := agent.NewClient(conn)
	signers, err := agentClient.Signers()
	if err != nil {
		conn.Close()
		return nil, nil
	}

	return signers, conn
}

// loadPrivateKey loads a private key from a file and returns an ssh.Signer.
func loadPrivateKey(keyPath, passphrase string) (ssh.Signer, error) {
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

	return signer, nil
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
	// Close agent connection if present
	if c.agentConn != nil {
		c.agentConn.Close()
		c.agentConn = nil
	}

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
	// ExecuteInteractive runs an interactive command with PTY support.
	ExecuteInteractive(ctx context.Context, command string) error
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

	// Handle cd command specially to track directory changes
	command = strings.TrimSpace(command)
	isCdCommand := strings.HasPrefix(command, "cd ") || command == "cd"

	session, err := c.client.NewSession()
	if err != nil {
		result.Error = fmt.Errorf("failed to create session: %w", err)
		return result
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// Prepend cd to the command if we have a tracked working directory
	fullCommand := command
	if c.cwd != "" && !isCdCommand {
		// For regular commands, cd to the tracked directory first
		fullCommand = fmt.Sprintf("cd %s && %s", ShellEscape(c.cwd), command)
	} else if isCdCommand {
		// For cd commands, we need to track the new directory
		// First, resolve the target directory on the remote host
		target := strings.TrimSpace(strings.TrimPrefix(command, "cd"))
		if target == "" {
			target = "~"
		}
		
		var resolveCmd string
		if c.cwd != "" && target != "" && !strings.HasPrefix(target, "/") && !strings.HasPrefix(target, "~") {
			// Relative path - resolve from current directory
			resolveCmd = fmt.Sprintf("cd %s && cd %s && pwd", ShellEscape(c.cwd), ShellEscape(target))
		} else {
			resolveCmd = fmt.Sprintf("cd %s && pwd", ShellEscape(target))
		}
		
		err = session.Run(resolveCmd)
		if err != nil {
			result.Stdout = stdout.String()
			result.Stderr = stderr.String()
			var exitErr *ssh.ExitError
			if errors.As(err, &exitErr) {
				result.ExitCode = exitErr.ExitStatus()
			} else {
				result.Error = err
			}
			return result
		}
		
		// Update the tracked working directory
		newCwd := strings.TrimSpace(stdout.String())
		if newCwd != "" {
			c.cwd = newCwd
		}
		return result
	}

	// Set TERM environment variable to ensure commands like 'clear' work properly.
	// We prepend the export command to handle cases where the server doesn't accept
	// environment variables via Setenv (depends on AcceptEnv in sshd_config).
	termType := os.Getenv("TERM")
	if termType == "" || !isValidTermType(termType) {
		termType = "xterm-256color"
	}
	// Use single quotes for additional shell safety, even though isValidTermType
	// already ensures the value contains only safe characters.
	fullCommand = fmt.Sprintf("export TERM='%s'; %s", termType, fullCommand)

	err = session.Run(fullCommand)
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

// GetCwd returns the current working directory on the remote host.
func (c *Client) GetCwd() string {
	return c.cwd
}

// ExecuteInteractive executes an interactive command (like top, htop) on the remote host
// with PTY support. It connects the command's stdin/stdout/stderr to the current terminal.
func (c *Client) ExecuteInteractive(_ context.Context, command string) error {
	if !c.isConnected {
		return errors.New("not connected")
	}

	session, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Get terminal size
	fd := int(os.Stdin.Fd())
	width, height := 80, 24
	if term.IsTerminal(fd) {
		w, h, err := term.GetSize(fd)
		if err == nil {
			width, height = w, h
		}
	}

	// Request a PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	termType := os.Getenv("TERM")
	if termType == "" {
		termType = "xterm-256color"
	}

	if err := session.RequestPty(termType, height, width, modes); err != nil {
		return fmt.Errorf("failed to request PTY: %w", err)
	}

	// Set up stdin/stdout/stderr
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	// Put terminal into raw mode if it's a terminal
	var oldState *term.State
	if term.IsTerminal(fd) {
		oldState, err = term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("failed to set raw terminal: %w", err)
		}
		defer term.Restore(fd, oldState)
	}

	// Handle window resize
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	defer signal.Stop(sigwinch)

	go func() {
		for range sigwinch {
			if term.IsTerminal(fd) {
				w, h, err := term.GetSize(fd)
				if err == nil {
					_ = session.WindowChange(h, w)
				}
			}
		}
	}()

	// Prepend cd to the command if we have a tracked working directory
	fullCommand := command
	if c.cwd != "" {
		fullCommand = fmt.Sprintf("cd %s && %s", ShellEscape(c.cwd), command)
	}

	// Start the command
	if err := session.Start(fullCommand); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Copy stdin to remote
	go func() {
		_, _ = io.Copy(stdin, os.Stdin)
		stdin.Close()
	}()

	// Wait for the command to complete
	err = session.Wait()
	if err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			// Command exited with non-zero status, but that's not necessarily an error
			return nil
		}
		return err
	}

	return nil
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

// GetDefaultKeyPaths returns all default SSH private key paths to try.
func GetDefaultKeyPaths() []string {
	homeDir, _ := os.UserHomeDir()
	sshDir := filepath.Join(homeDir, ".ssh")
	return []string{
		filepath.Join(sshDir, "id_ed25519"),
		filepath.Join(sshDir, "id_ecdsa"),
		filepath.Join(sshDir, "id_rsa"),
		filepath.Join(sshDir, "id_dsa"),
	}
}

// isValidTermType validates that a TERM value contains only safe characters.
// Valid TERM values should only contain alphanumeric characters, hyphens, and underscores.
// This prevents potential command injection through malicious TERM values.
func isValidTermType(term string) bool {
	if term == "" {
		return false
	}
	for _, c := range term {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
