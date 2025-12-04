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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

// LocalClient represents a local command executor.
// It provides the same interface as Client but executes commands locally.
type LocalClient struct {
	hostname string
	username string
	cwd      string // current working directory
}

// NewLocalClient creates a new local client.
func NewLocalClient() *LocalClient {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}
	username := "unknown"
	if u, err := user.Current(); err == nil {
		username = u.Username
	}
	// Get the initial working directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "/"
	}
	return &LocalClient{
		hostname: hostname,
		username: username,
		cwd:      cwd,
	}
}

// Execute executes a command on the local host.
func (c *LocalClient) Execute(ctx context.Context, command string) *ExecuteResult {
	result := &ExecuteResult{}

	// Handle cd command specially to track directory changes
	command = strings.TrimSpace(command)
	if strings.HasPrefix(command, "cd ") || command == "cd" {
		return c.handleCd(command)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = c.cwd // Execute in the tracked working directory

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = err
		}
	}

	return result
}

// handleCd handles the cd command by changing the tracked working directory.
func (c *LocalClient) handleCd(command string) *ExecuteResult {
	result := &ExecuteResult{}

	// Parse the target directory
	target := strings.TrimSpace(strings.TrimPrefix(command, "cd"))
	if target == "" || target == "~" {
		// cd or cd ~ goes to home directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			result.Error = fmt.Errorf("cannot get home directory: %w", err)
			return result
		}
		target = homeDir
	} else if strings.HasPrefix(target, "~/") {
		// Expand ~ prefix
		homeDir, err := os.UserHomeDir()
		if err != nil {
			result.Error = fmt.Errorf("cannot get home directory: %w", err)
			return result
		}
		target = filepath.Join(homeDir, target[2:])
	} else if !filepath.IsAbs(target) {
		// Make relative path absolute based on current directory
		target = filepath.Join(c.cwd, target)
	}

	// Clean the path to handle . and ..
	target = filepath.Clean(target)

	// Verify the directory exists and is a directory
	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			result.Stderr = fmt.Sprintf("cd: %s: No such file or directory\n", target)
			result.ExitCode = 1
		} else {
			result.Error = err
		}
		return result
	}

	if !info.IsDir() {
		result.Stderr = fmt.Sprintf("cd: %s: Not a directory\n", target)
		result.ExitCode = 1
		return result
	}

	// Update the current working directory
	c.cwd = target
	return result
}

// GetCwd returns the current working directory.
func (c *LocalClient) GetCwd() string {
	return c.cwd
}

// ExecuteInteractive executes an interactive command (like top, htop) on the local host
// with PTY support. It connects the command's stdin/stdout/stderr to the current terminal.
func (c *LocalClient) ExecuteInteractive(ctx context.Context, command string) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = c.cwd // Execute in the tracked working directory

	// Connect to current terminal
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Put terminal into raw mode if it's a terminal
	fd := int(os.Stdin.Fd())
	var oldState *term.State
	var err error
	if term.IsTerminal(fd) {
		oldState, err = term.MakeRaw(fd)
		if err != nil {
			// Fallback to non-raw mode if we can't set raw mode
			return cmd.Run()
		}
		defer term.Restore(fd, oldState)
	}

	// Run the command
	err = cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Command exited with non-zero status, but that's not necessarily an error
			return nil
		}
		return err
	}

	return nil
}

// IsConnected always returns true for local client.
func (c *LocalClient) IsConnected() bool {
	return true
}

// Close is a no-op for local client.
func (c *LocalClient) Close() error {
	return nil
}

// HostInfoString returns a string representation of the local host.
func (c *LocalClient) HostInfoString() string {
	return c.username + "@" + c.hostname + ":local"
}
