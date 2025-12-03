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
	"os"
	"os/exec"
	"os/user"
)

// LocalClient represents a local command executor.
// It provides the same interface as Client but executes commands locally.
type LocalClient struct {
	hostname string
	username string
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
	return &LocalClient{
		hostname: hostname,
		username: username,
	}
}

// Execute executes a command on the local host.
func (c *LocalClient) Execute(ctx context.Context, command string) *ExecuteResult {
	result := &ExecuteResult{}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)

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
