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

// Package batch provides batch command execution across multiple hosts.
package batch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/warm3snow/sherlock/internal/hosts"
	"github.com/warm3snow/sherlock/pkg/sshclient"
)

// ExecutionResult represents a single host's execution result.
type ExecutionResult struct {
	Host     *hosts.Host
	Stdout   string
	Stderr   string
	ExitCode int
	Error    error
	Duration time.Duration
}

// BatchResult represents aggregated batch execution results.
type BatchResult struct {
	TotalHosts int
	Successful int
	Failed     int
	Results    []ExecutionResult
	StartTime  time.Time
	EndTime    time.Time
}

// Duration returns the total execution duration.
func (r *BatchResult) Duration() time.Duration {
	return r.EndTime.Sub(r.StartTime)
}

// Options configures batch execution.
type Options struct {
	Concurrency     int           // Max parallel connections (default: 10)
	Timeout         time.Duration // Per-host timeout (default: 60s)
	ContinueOnError bool          // Continue if a host fails
	PrivateKeyPath  string        // SSH private key path
	OnProgress      func(completed, total int) // Progress callback
}

// DefaultOptions returns default batch options.
func DefaultOptions() *Options {
	return &Options{
		Concurrency:     10,
		Timeout:         60 * time.Second,
		ContinueOnError: true,
	}
}

// Executor executes commands on multiple hosts.
type Executor struct {
	hostManager *hosts.Manager
	sshConfig   *sshclient.Config
}

// NewExecutor creates a new batch executor.
func NewExecutor(hostManager *hosts.Manager, privateKeyPath string) *Executor {
	return &Executor{
		hostManager: hostManager,
		sshConfig: &sshclient.Config{
			PrivateKeyPath: privateKeyPath,
		},
	}
}

// Execute runs a command on multiple hosts.
func (e *Executor) Execute(ctx context.Context, hostList []hosts.Host, command string, opts *Options) *BatchResult {
	if opts == nil {
		opts = DefaultOptions()
	}

	result := &BatchResult{
		TotalHosts: len(hostList),
		StartTime:  time.Now(),
		Results:    make([]ExecutionResult, 0, len(hostList)),
	}

	if len(hostList) == 0 {
		result.EndTime = time.Now()
		return result
	}

	// Create semaphore for concurrency control
	semaphore := make(chan struct{}, opts.Concurrency)
	resultChan := make(chan ExecutionResult, len(hostList))

	var wg sync.WaitGroup
	completed := 0
	var mu sync.Mutex

	for _, host := range hostList {
		wg.Add(1)
		go func(h hosts.Host) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Execute command on host
			execResult := e.executeOnHost(ctx, &h, command, opts)
			resultChan <- execResult

			// Update progress
			if opts.OnProgress != nil {
				mu.Lock()
				completed++
				opts.OnProgress(completed, len(hostList))
				mu.Unlock()
			}
		}(host)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	for r := range resultChan {
		result.Results = append(result.Results, r)
		if r.Error == nil && r.ExitCode == 0 {
			result.Successful++
		} else {
			result.Failed++
		}
	}

	result.EndTime = time.Now()
	return result
}

// ExecuteOnGroup runs a command on all hosts in a group.
func (e *Executor) ExecuteOnGroup(ctx context.Context, groupName string, command string, opts *Options) (*BatchResult, error) {
	hostList, err := e.hostManager.GetGroupHostsByName(ctx, groupName)
	if err != nil {
		return nil, fmt.Errorf("failed to get hosts in group %s: %w", groupName, err)
	}

	if len(hostList) == 0 {
		return nil, fmt.Errorf("no hosts found in group %s", groupName)
	}

	return e.Execute(ctx, hostList, command, opts), nil
}

// ExecuteOnTag runs a command on all hosts with a specific tag.
func (e *Executor) ExecuteOnTag(ctx context.Context, tagName string, command string, opts *Options) (*BatchResult, error) {
	hostList, err := e.hostManager.GetHostsByTagName(ctx, tagName)
	if err != nil {
		return nil, fmt.Errorf("failed to get hosts with tag %s: %w", tagName, err)
	}

	if len(hostList) == 0 {
		return nil, fmt.Errorf("no hosts found with tag %s", tagName)
	}

	return e.Execute(ctx, hostList, command, opts), nil
}

// ExecuteOnAll runs a command on all hosts.
func (e *Executor) ExecuteOnAll(ctx context.Context, command string, opts *Options) (*BatchResult, error) {
	hostList, err := e.hostManager.GetAllHosts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all hosts: %w", err)
	}

	if len(hostList) == 0 {
		return nil, fmt.Errorf("no hosts found")
	}

	return e.Execute(ctx, hostList, command, opts), nil
}

// executeOnHost executes a command on a single host.
func (e *Executor) executeOnHost(ctx context.Context, host *hosts.Host, command string, opts *Options) ExecutionResult {
	result := ExecutionResult{
		Host: host,
	}

	startTime := time.Now()

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	// Create SSH client config
	cfg := &sshclient.Config{
		HostInfo: &sshclient.HostInfo{
			Host: host.Host,
			Port: host.Port,
			User: host.User,
		},
		PrivateKeyPath: opts.PrivateKeyPath,
		Timeout:        opts.Timeout,
	}

	// Create SSH client
	client, err := sshclient.NewClient(cfg)
	if err != nil {
		result.Error = fmt.Errorf("failed to create SSH client: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}
	defer client.Close()

	// Connect
	if err := client.Connect(execCtx); err != nil {
		result.Error = fmt.Errorf("failed to connect: %w", err)
		result.Duration = time.Since(startTime)
		return result
	}

	// Execute command
	execResult := client.Execute(execCtx, command)
	result.Stdout = execResult.Stdout
	result.Stderr = execResult.Stderr
	result.ExitCode = execResult.ExitCode
	result.Error = execResult.Error
	result.Duration = time.Since(startTime)

	return result
}
