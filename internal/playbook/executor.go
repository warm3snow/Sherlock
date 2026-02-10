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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/warm3snow/sherlock/internal/ai"
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
	sb.WriteString("╔══════════════════════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║                         📜 Playbook 执行结果                                  ║\n")
	sb.WriteString("╚══════════════════════════════════════════════════════════════════════════════╝\n")
	sb.WriteString("\n")

	// Summary with colors
	statusIcon := "\033[32m✓\033[0m"
	statusText := "\033[32m成功\033[0m"
	if !result.Success {
		statusIcon = "\033[31m✗\033[0m"
		statusText = "\033[31m失败\033[0m"
	}

	sb.WriteString(fmt.Sprintf("  %s 剧本: \033[1;36m%s\033[0m  [%s]\n", statusIcon, result.PlaybookName, statusText))
	sb.WriteString(fmt.Sprintf("  🖥️  主机: \033[1;33m%s\033[0m\n", result.HostName))
	sb.WriteString(fmt.Sprintf("  ⏱️  耗时: \033[1;35m%v\033[0m\n", result.Duration.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("  📊 步骤: \033[32m%d 成功\033[0m / \033[31m%d 失败\033[0m / \033[33m%d 跳过\033[0m (共 %d 步)\n",
		result.Successful, result.Failed, result.Skipped, result.TotalSteps))

	// Step details
	for i, step := range result.StepResults {
		sb.WriteString("\n")
		sb.WriteString("  ┌──────────────────────────────────────────────────────────────────────────────\n")

		icon := "\033[32m✓\033[0m"
		statusColor := "\033[32m"
		if step.Skipped {
			icon = "\033[33m○\033[0m"
			statusColor = "\033[33m"
		} else if !step.Success {
			icon = "\033[31m✗\033[0m"
			statusColor = "\033[31m"
		}

		sb.WriteString(fmt.Sprintf("  │ %s [%d/%d] \033[1m%s\033[0m %s(%v)\033[0m\n",
			icon, i+1, result.TotalSteps, step.StepName, statusColor, step.Duration.Round(time.Millisecond)))
		sb.WriteString("  ├──────────────────────────────────────────────────────────────────────────────\n")

		// Command (show full command, wrap if needed)
		sb.WriteString(fmt.Sprintf("  │ \033[36m命令:\033[0m %s\n", step.Command))

		// Output section
		if step.Stdout != "" {
			sb.WriteString("  │\n")
			sb.WriteString("  │ \033[36m输出:\033[0m\n")
			sb.WriteString(formatMultilineOutput(step.Stdout, "  │   ", 20))
		}

		// Error output (stderr)
		if step.Stderr != "" && !step.Success {
			sb.WriteString("  │\n")
			sb.WriteString("  │ \033[31m错误输出:\033[0m\n")
			sb.WriteString(formatMultilineOutput(step.Stderr, "  │   \033[31m", 10))
			sb.WriteString("\033[0m")
		}

		// Exception
		if step.Error != "" {
			sb.WriteString(fmt.Sprintf("  │ \033[31m异常: %s\033[0m\n", step.Error))
		}

		// Exit code for failed steps
		if !step.Success && !step.Skipped {
			sb.WriteString(fmt.Sprintf("  │ \033[31m退出码: %d\033[0m\n", step.ExitCode))
		}

		sb.WriteString("  └──────────────────────────────────────────────────────────────────────────────\n")
	}

	sb.WriteString("\n")
	return sb.String()
}

// formatMultilineOutput formats multi-line output with prefix and line limit.
func formatMultilineOutput(output string, prefix string, maxLines int) string {
	var sb strings.Builder
	lines := strings.Split(strings.TrimSpace(output), "\n")

	displayLines := lines
	truncated := false
	if len(lines) > maxLines {
		displayLines = lines[:maxLines]
		truncated = true
	}

	for _, line := range displayLines {
		// Truncate very long lines
		if len(line) > 120 {
			line = line[:117] + "..."
		}
		sb.WriteString(fmt.Sprintf("%s%s\n", prefix, line))
	}

	if truncated {
		sb.WriteString(fmt.Sprintf("%s\033[2m... 省略 %d 行 ...\033[0m\n", prefix, len(lines)-maxLines))
	}

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

// PlaybookAnalysis represents the AI analysis result of playbook execution.
type PlaybookAnalysis struct {
	Summary        string   `json:"summary"`         // Overall summary of execution
	Status         string   `json:"status"`          // healthy, warning, critical
	Issues         []string `json:"issues"`          // Issues found during execution
	Recommendations []string `json:"recommendations"` // Recommended actions
	RiskLevel      string   `json:"risk_level"`      // low, medium, high
	NextSteps      []string `json:"next_steps"`      // Suggested next steps
}

const playbookAnalysisPrompt = `You are Sherlock, an AI assistant specializing in analyzing Linux/Unix system operations.
Your task is to analyze the execution results of an operations playbook and provide decision-making insights.

Analyze the playbook execution results and provide:
1. Overall summary of what was executed and the results
2. Status assessment (healthy, warning, critical)
3. Any issues found during execution (errors, warnings, anomalies)
4. Recommendations for system improvement or follow-up actions
5. Risk level assessment (low, medium, high)
6. Suggested next steps for the operator

Focus on:
- Failed steps and their impact
- Abnormal output patterns (high resource usage, errors in logs, etc.)
- Security concerns if any
- Performance optimization opportunities
- Preventive maintenance suggestions

Respond in JSON format only:
{
  "summary": "Overall summary of playbook execution and findings",
  "status": "healthy|warning|critical",
  "issues": ["issue1", "issue2"],
  "recommendations": ["recommendation1", "recommendation2"],
  "risk_level": "low|medium|high",
  "next_steps": ["step1", "step2"]
}

Use Chinese for the response content.`

// AnalyzeResult analyzes playbook execution result using AI.
func AnalyzeResult(ctx context.Context, aiClient ai.ModelClient, result *ExecutionResult) (*PlaybookAnalysis, error) {
	if aiClient == nil {
		return nil, fmt.Errorf("AI client not available")
	}

	// Build analysis context from execution result
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Playbook: %s\n", result.PlaybookName))
	sb.WriteString(fmt.Sprintf("Host: %s\n", result.HostName))
	sb.WriteString(fmt.Sprintf("Duration: %v\n", result.Duration))
	sb.WriteString(fmt.Sprintf("Overall Success: %v\n", result.Success))
	sb.WriteString(fmt.Sprintf("Steps: %d total, %d successful, %d failed, %d skipped\n\n",
		result.TotalSteps, result.Successful, result.Failed, result.Skipped))

	sb.WriteString("Step Details:\n")
	sb.WriteString("─────────────────────────────────────────\n")

	for i, step := range result.StepResults {
		status := "✓ SUCCESS"
		if step.Skipped {
			status = "○ SKIPPED"
		} else if !step.Success {
			status = "✗ FAILED"
		}

		sb.WriteString(fmt.Sprintf("\n[%d] %s - %s\n", i+1, step.StepName, status))
		sb.WriteString(fmt.Sprintf("Command: %s\n", step.Command))
		sb.WriteString(fmt.Sprintf("Exit Code: %d\n", step.ExitCode))

		// Include output (truncated for large outputs)
		if step.Stdout != "" {
			output := step.Stdout
			if len(output) > 1500 {
				output = output[:1500] + "\n... (truncated)"
			}
			sb.WriteString(fmt.Sprintf("Output:\n%s\n", output))
		}

		if step.Stderr != "" && !step.Success {
			stderr := step.Stderr
			if len(stderr) > 500 {
				stderr = stderr[:500] + "\n... (truncated)"
			}
			sb.WriteString(fmt.Sprintf("Stderr:\n%s\n", stderr))
		}

		if step.Error != "" {
			sb.WriteString(fmt.Sprintf("Error: %s\n", step.Error))
		}
	}

	// Truncate total content if too large
	content := sb.String()
	if len(content) > 8000 {
		content = content[:8000] + "\n... (content truncated for analysis)"
	}

	messages := []*schema.Message{
		schema.SystemMessage(playbookAnalysisPrompt),
		schema.UserMessage(content),
	}

	response, err := aiClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("AI analysis failed: %w", err)
	}

	// Extract JSON from response
	responseContent := strings.TrimSpace(response.Content)
	jsonContent := extractJSONFromResponse(responseContent)

	var analysis PlaybookAnalysis
	if err := json.Unmarshal([]byte(jsonContent), &analysis); err != nil {
		// If JSON parsing fails, create a basic analysis
		return &PlaybookAnalysis{
			Summary:        responseContent,
			Status:         assessStatus(result),
			Issues:         []string{},
			Recommendations: []string{},
			RiskLevel:      assessRiskLevel(result),
			NextSteps:      []string{},
		}, nil
	}

	return &analysis, nil
}

// extractJSONFromResponse extracts JSON content from AI response.
func extractJSONFromResponse(content string) string {
	// Try to find JSON block in markdown
	if start := strings.Index(content, "```json"); start != -1 {
		content = content[start+7:]
		if end := strings.Index(content, "```"); end != -1 {
			content = content[:end]
		}
	} else if start := strings.Index(content, "```"); start != -1 {
		content = content[start+3:]
		if end := strings.Index(content, "```"); end != -1 {
			content = content[:end]
		}
	}

	// Find JSON object boundaries
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start != -1 && end != -1 && end > start {
		content = content[start : end+1]
	}

	return strings.TrimSpace(content)
}

// assessStatus determines execution status.
func assessStatus(result *ExecutionResult) string {
	if result.Failed == 0 {
		return "healthy"
	}
	if float64(result.Failed)/float64(result.TotalSteps) > 0.5 {
		return "critical"
	}
	return "warning"
}

// assessRiskLevel determines risk level.
func assessRiskLevel(result *ExecutionResult) string {
	if result.Failed == 0 {
		return "low"
	}
	if result.Failed > 2 || float64(result.Failed)/float64(result.TotalSteps) > 0.3 {
		return "high"
	}
	return "medium"
}

// FormatAnalysis formats playbook analysis for display.
func FormatAnalysis(analysis *PlaybookAnalysis) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("╔══════════════════════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║                         🤖 AI 分析报告                                       ║\n")
	sb.WriteString("╚══════════════════════════════════════════════════════════════════════════════╝\n")
	sb.WriteString("\n")

	// Status with color
	statusIcon := "\033[32m●\033[0m"
	statusColor := "\033[32m"
	switch analysis.Status {
	case "warning":
		statusIcon = "\033[33m●\033[0m"
		statusColor = "\033[33m"
	case "critical":
		statusIcon = "\033[31m●\033[0m"
		statusColor = "\033[31m"
	}

	sb.WriteString(fmt.Sprintf("  %s 状态: %s%s\033[0m", statusIcon, statusColor, analysis.Status))

	// Risk level
	riskColor := "\033[32m"
	switch analysis.RiskLevel {
	case "medium":
		riskColor = "\033[33m"
	case "high":
		riskColor = "\033[31m"
	}
	sb.WriteString(fmt.Sprintf("    风险等级: %s%s\033[0m\n", riskColor, analysis.RiskLevel))
	sb.WriteString("\n")

	// Summary
	sb.WriteString("  📋 \033[1m总结:\033[0m\n")
	sb.WriteString(wrapText(analysis.Summary, "     ", 76))
	sb.WriteString("\n")

	// Issues
	if len(analysis.Issues) > 0 {
		sb.WriteString("  ⚠️  \033[1m发现的问题:\033[0m\n")
		for _, issue := range analysis.Issues {
			sb.WriteString(fmt.Sprintf("     \033[33m•\033[0m %s\n", issue))
		}
		sb.WriteString("\n")
	}

	// Recommendations
	if len(analysis.Recommendations) > 0 {
		sb.WriteString("  💡 \033[1m建议:\033[0m\n")
		for _, rec := range analysis.Recommendations {
			sb.WriteString(fmt.Sprintf("     \033[36m•\033[0m %s\n", rec))
		}
		sb.WriteString("\n")
	}

	// Next steps
	if len(analysis.NextSteps) > 0 {
		sb.WriteString("  📌 \033[1m后续步骤:\033[0m\n")
		for i, step := range analysis.NextSteps {
			sb.WriteString(fmt.Sprintf("     %d. %s\n", i+1, step))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("──────────────────────────────────────────────────────────────────────────────\n")

	return sb.String()
}

// wrapText wraps text with prefix for multi-line display.
func wrapText(text string, prefix string, maxWidth int) string {
	var sb strings.Builder
	words := strings.Fields(text)
	lineLen := 0

	sb.WriteString(prefix)
	for i, word := range words {
		wordLen := len(word)
		if lineLen+wordLen+1 > maxWidth && lineLen > 0 {
			sb.WriteString("\n" + prefix)
			lineLen = 0
		}
		if i > 0 && lineLen > 0 {
			sb.WriteString(" ")
			lineLen++
		}
		sb.WriteString(word)
		lineLen += wordLen
	}
	sb.WriteString("\n")
	return sb.String()
}
