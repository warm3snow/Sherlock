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

package playbook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/warm3snow/sherlock/pkg/sshclient"
)

// StepResult represents the result of executing a single step.
type StepResult struct {
	StepName  string        `json:"step_name"`
	Command   string        `json:"command"`
	Stdout    string        `json:"stdout"`
	Stderr    string        `json:"stderr"`
	ExitCode  int           `json:"exit_code"`
	Success   bool          `json:"success"`
	Duration  time.Duration `json:"duration_ms"`
	Error     string        `json:"error,omitempty"`
	Skipped   bool          `json:"skipped"`
}

// ExecutionResult represents the result of executing a playbook.
type ExecutionResult struct {
	PlaybookName string        `json:"playbook_name"`
	HostName     string        `json:"host_name"`
	StartTime    time.Time     `json:"start_time"`
	EndTime      time.Time     `json:"end_time"`
	Duration     time.Duration `json:"duration_ms"`
	TotalSteps   int           `json:"total_steps"`
	Successful   int           `json:"successful"`
	Failed       int           `json:"failed"`
	Skipped      int           `json:"skipped"`
	Success      bool          `json:"success"`
	StepResults  []StepResult  `json:"step_results"`
}

// CommandExecutor is an interface for executing commands.
type CommandExecutor interface {
	Execute(ctx context.Context, command string) *sshclient.ExecuteResult
	HostInfoString() string
}

// ExecutorOptions configures playbook execution.
type ExecutorOptions struct {
	DryRun       bool              // Just show commands, don't execute
	StopOnError  bool              // Stop execution on first error
	Variables    map[string]string // Runtime variables
	Timeout      time.Duration     // Per-step timeout
	OnStepStart  func(step PlaybookStep, index int)
	OnStepEnd    func(result StepResult, index int)
}

// DefaultExecutorOptions returns default options.
func DefaultExecutorOptions() ExecutorOptions {
	return ExecutorOptions{
		DryRun:      false,
		StopOnError: false,
		Variables:   make(map[string]string),
		Timeout:     60 * time.Second,
	}
}

// Executor executes playbooks.
type Executor struct {
	manager *Manager
}

// NewExecutor creates a new playbook executor.
func NewExecutor(manager *Manager) *Executor {
	return &Executor{manager: manager}
}

// Execute executes a playbook.
func (e *Executor) Execute(ctx context.Context, pb *Playbook, executor CommandExecutor, opts ExecutorOptions) *ExecutionResult {
	result := &ExecutionResult{
		PlaybookName: pb.Name,
		HostName:     executor.HostInfoString(),
		StartTime:    time.Now(),
		TotalSteps:   len(pb.Steps),
		StepResults:  make([]StepResult, 0, len(pb.Steps)),
	}

	// Merge variables (runtime overrides playbook defaults)
	variables := make(map[string]string)
	for k, v := range pb.Variables {
		variables[k] = v
	}
	for k, v := range opts.Variables {
		variables[k] = v
	}

	// Execute each step
	for i, step := range pb.Steps {
		if opts.OnStepStart != nil {
			opts.OnStepStart(step, i)
		}

		stepResult := e.executeStep(ctx, step, executor, variables, opts)
		result.StepResults = append(result.StepResults, stepResult)

		if opts.OnStepEnd != nil {
			opts.OnStepEnd(stepResult, i)
		}

		if stepResult.Skipped {
			result.Skipped++
		} else if stepResult.Success {
			result.Successful++
		} else {
			result.Failed++
			if opts.StopOnError && !step.ContinueOnError {
				break
			}
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Success = result.Failed == 0

	// Update usage count
	if e.manager != nil {
		e.manager.IncrementUsage(pb.Name)
	}

	return result
}

// executeStep executes a single step.
func (e *Executor) executeStep(ctx context.Context, step PlaybookStep, executor CommandExecutor, variables map[string]string, opts ExecutorOptions) StepResult {
	result := StepResult{
		StepName: step.Name,
	}

	// Expand variables in command
	command := ExpandVariables(step.Command, variables)
	result.Command = command

	// Dry run - just return
	if opts.DryRun {
		result.Success = true
		result.Stdout = "(dry run - command not executed)"
		return result
	}

	// Set timeout
	timeout := opts.Timeout
	if step.Timeout > 0 {
		timeout = time.Duration(step.Timeout) * time.Second
	}

	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	startTime := time.Now()

	// Execute with retry
	retryCount := step.RetryCount
	if retryCount < 0 {
		retryCount = 0
	}

	var execResult *sshclient.ExecuteResult
	for attempt := 0; attempt <= retryCount; attempt++ {
		execResult = executor.Execute(stepCtx, command)
		if execResult.Error == nil && execResult.ExitCode == step.ExpectedExitCode {
			break
		}
		if attempt < retryCount {
			time.Sleep(time.Second) // Wait before retry
		}
	}

	result.Duration = time.Since(startTime)
	result.Stdout = execResult.Stdout
	result.Stderr = execResult.Stderr
	result.ExitCode = execResult.ExitCode

	if execResult.Error != nil {
		result.Error = execResult.Error.Error()
		result.Success = step.ContinueOnError
	} else {
		result.Success = execResult.ExitCode == step.ExpectedExitCode
	}

	return result
}

// ExecuteByName executes a playbook by name.
func (e *Executor) ExecuteByName(ctx context.Context, name string, executor CommandExecutor, opts ExecutorOptions) (*ExecutionResult, error) {
	pb, err := e.manager.Get(name)
	if err != nil {
		return nil, fmt.Errorf("playbook not found: %s", name)
	}
	return e.Execute(ctx, pb, executor, opts), nil
}

// FormatResult formats an execution result for display.
func FormatResult(result *ExecutionResult) string {
	var sb strings.Builder

	// Header
	sb.WriteString("\n")
	sb.WriteString("╔══════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║            📜 Playbook 执行结果                               ║\n")
	sb.WriteString("╚══════════════════════════════════════════════════════════════╝\n")
	sb.WriteString("\n")

	// Summary
	statusIcon := "\033[32m✓\033[0m"
	if !result.Success {
		statusIcon = "\033[31m✗\033[0m"
	}

	sb.WriteString(fmt.Sprintf("  %s 剧本: %s\n", statusIcon, result.PlaybookName))
	sb.WriteString(fmt.Sprintf("  🖥️  主机: %s\n", result.HostName))
	sb.WriteString(fmt.Sprintf("  ⏱️  耗时: %v\n", result.Duration.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("  📊 步骤: %d 成功 / %d 失败 / %d 跳过 (共 %d 步)\n\n",
		result.Successful, result.Failed, result.Skipped, result.TotalSteps))

	// Step details
	sb.WriteString("  ─────────────────────────────────────────────────\n")

	for i, step := range result.StepResults {
		icon := "\033[32m✓\033[0m"
		if step.Skipped {
			icon = "\033[33m○\033[0m"
		} else if !step.Success {
			icon = "\033[31m✗\033[0m"
		}

		sb.WriteString(fmt.Sprintf("\n  [%d] %s %s (%v)\n", i+1, icon, step.StepName, step.Duration.Round(time.Millisecond)))
		sb.WriteString(fmt.Sprintf("      命令: %s\n", truncateStr(step.Command, 60)))

		if step.Stdout != "" {
			output := truncateStr(strings.TrimSpace(step.Stdout), 200)
			sb.WriteString(fmt.Sprintf("      输出: %s\n", output))
		}

		if step.Stderr != "" && !step.Success {
			sb.WriteString(fmt.Sprintf("      \033[31m错误: %s\033[0m\n", truncateStr(step.Stderr, 100)))
		}

		if step.Error != "" {
			sb.WriteString(fmt.Sprintf("      \033[31m异常: %s\033[0m\n", step.Error))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// FormatCompactResult formats a compact result.
func FormatCompactResult(result *ExecutionResult) string {
	statusIcon := "\033[32m✓\033[0m"
	if !result.Success {
		statusIcon = "\033[31m✗\033[0m"
	}

	return fmt.Sprintf("%s %s | %s | %d/%d 成功 | %v",
		statusIcon, result.PlaybookName, result.HostName,
		result.Successful, result.TotalSteps, result.Duration.Round(time.Millisecond))
}

func truncateStr(s string, maxLen int) string {
	// Replace newlines with spaces for single-line display
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
