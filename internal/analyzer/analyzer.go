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

// Package analyzer provides AI-powered command output analysis and error diagnosis.
package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/warm3snow/sherlock/internal/ai"
)

// Analyzer provides AI-powered analysis capabilities.
type Analyzer struct {
	client ai.ModelClient
}

// NewAnalyzer creates a new analyzer with the given AI client.
func NewAnalyzer(client ai.ModelClient) *Analyzer {
	return &Analyzer{client: client}
}

// AnalysisResult represents the result of output analysis.
type AnalysisResult struct {
	Summary     string   `json:"summary"`      // Brief summary of the output
	Status      string   `json:"status"`       // ok, warning, error, info
	KeyFindings []string `json:"key_findings"` // Important findings
	Suggestions []string `json:"suggestions"`  // Suggestions for improvement
	Warnings    []string `json:"warnings"`     // Warning messages
}

// DiagnosisResult represents the result of error diagnosis.
type DiagnosisResult struct {
	ErrorType    string   `json:"error_type"`   // Type of error (permission, network, etc.)
	RootCause    string   `json:"root_cause"`   // Root cause analysis
	Explanation  string   `json:"explanation"`  // Detailed explanation
	FixCommands  []string `json:"fix_commands"` // Commands to fix the issue
	Verification string   `json:"verification"` // Command to verify the fix
	Confidence   string   `json:"confidence"`   // high, medium, low
}

// SessionSummary represents a summary of operations performed.
type SessionSummary struct {
	TotalCommands   int      `json:"total_commands"`
	SuccessfulCount int      `json:"successful_count"`
	FailedCount     int      `json:"failed_count"`
	HostsConnected  []string `json:"hosts_connected"`
	KeyOperations   []string `json:"key_operations"`
	IssuesResolved  []string `json:"issues_resolved"`
	Recommendations []string `json:"recommendations"`
}

const systemPromptAnalyze = `You are Sherlock, an AI assistant specializing in analyzing command output from Linux/Unix systems.
Your task is to analyze the output of shell commands and provide insights.

Analyze the command output and provide:
1. A brief summary of what the output shows
2. Status assessment (ok, warning, error, info)
3. Key findings (important metrics, issues, or information)
4. Suggestions for improvement or follow-up actions
5. Any warnings that need attention

Respond in JSON format only:
{
  "summary": "Brief summary of the output",
  "status": "ok|warning|error|info",
  "key_findings": ["finding1", "finding2"],
  "suggestions": ["suggestion1", "suggestion2"],
  "warnings": ["warning1", "warning2"]
}`

const systemPromptDiagnose = `You are Sherlock, an AI assistant specializing in diagnosing Linux/Unix system errors.
Your task is to analyze error messages and provide diagnosis and fix recommendations.

Given an error message or failed command output, provide:
1. The type of error (permission, network, disk, memory, process, syntax, etc.)
2. Root cause analysis
3. Detailed explanation of what went wrong
4. Specific commands to fix the issue
5. A command to verify the fix worked
6. Your confidence level in this diagnosis

Respond in JSON format only:
{
  "error_type": "permission|network|disk|memory|process|syntax|configuration|dependency|other",
  "root_cause": "Brief description of the root cause",
  "explanation": "Detailed explanation of the error",
  "fix_commands": ["command1", "command2"],
  "verification": "command to verify the fix",
  "confidence": "high|medium|low"
}`

const systemPromptSummarize = `You are Sherlock, an AI assistant for SSH remote operations.
Summarize the session operations performed by the user.

Given a list of commands executed during a session, provide:
1. Total count statistics
2. Key operations performed
3. Any issues that were resolved
4. Recommendations for future sessions

Respond in JSON format only:
{
  "total_commands": 10,
  "successful_count": 8,
  "failed_count": 2,
  "hosts_connected": ["host1", "host2"],
  "key_operations": ["Checked disk usage", "Restarted nginx"],
  "issues_resolved": ["Cleared disk space on /var"],
  "recommendations": ["Consider setting up log rotation", "Monitor memory usage"]
}`

// AnalyzeOutput analyzes command output using AI.
func (a *Analyzer) AnalyzeOutput(ctx context.Context, command, output string, exitCode int) (*AnalysisResult, error) {
	if quickResult := a.quickAnalyze(command, output, exitCode); quickResult != nil {
		return quickResult, nil
	}

	prompt := fmt.Sprintf("Command: %s\nExit Code: %d\nOutput:\n%s", command, exitCode, truncateOutput(output, 2000))

	messages := []*schema.Message{
		schema.SystemMessage(systemPromptAnalyze),
		schema.UserMessage(prompt),
	}

	response, err := a.client.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("AI analysis failed: %w", err)
	}

	content := strings.TrimSpace(response.Content)
	content = extractJSON(content)

	var result AnalysisResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return &AnalysisResult{
			Summary: "Command completed",
			Status:  getStatusFromExitCode(exitCode),
		}, nil
	}

	return &result, nil
}

// DiagnoseError diagnoses an error and suggests fixes.
func (a *Analyzer) DiagnoseError(ctx context.Context, command, errorOutput string, exitCode int) (*DiagnosisResult, error) {
	if quickResult := a.quickDiagnose(command, errorOutput, exitCode); quickResult != nil {
		return quickResult, nil
	}

	prompt := fmt.Sprintf("Failed Command: %s\nExit Code: %d\nError Output:\n%s", command, exitCode, truncateOutput(errorOutput, 2000))

	messages := []*schema.Message{
		schema.SystemMessage(systemPromptDiagnose),
		schema.UserMessage(prompt),
	}

	response, err := a.client.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("AI diagnosis failed: %w", err)
	}

	content := strings.TrimSpace(response.Content)
	content = extractJSON(content)

	var result DiagnosisResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse diagnosis: %w", err)
	}

	return &result, nil
}

// SummarizeSession summarizes the operations performed in a session.
func (a *Analyzer) SummarizeSession(ctx context.Context, commands []string, hosts []string) (*SessionSummary, error) {
	prompt := fmt.Sprintf("Commands executed:\n%s\n\nHosts connected: %s",
		strings.Join(commands, "\n"), strings.Join(hosts, ", "))

	messages := []*schema.Message{
		schema.SystemMessage(systemPromptSummarize),
		schema.UserMessage(prompt),
	}

	response, err := a.client.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("AI summarization failed: %w", err)
	}

	content := strings.TrimSpace(response.Content)
	content = extractJSON(content)

	var result SessionSummary
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return &SessionSummary{
			TotalCommands:  len(commands),
			HostsConnected: hosts,
		}, nil
	}

	return &result, nil
}

func (a *Analyzer) quickAnalyze(command, output string, exitCode int) *AnalysisResult {
	// Always use AI analysis for the analyze command
	// Only use quick analysis for specific commands with well-known output formats
	lower := strings.ToLower(command)

	// df command has structured output we can parse directly
	if strings.HasPrefix(lower, "df ") || lower == "df" {
		if result := analyzeDiskUsage(output); result != nil && result.Status != "ok" {
			return result // Only return quick result if there's an issue to report
		}
	}

	// free command has structured output we can parse directly
	if strings.HasPrefix(lower, "free ") || lower == "free" {
		if result := analyzeMemoryUsage(output); result != nil && result.Status != "ok" {
			return result // Only return quick result if there's an issue to report
		}
	}

	// For all other cases, let AI do the analysis
	return nil
}

func (a *Analyzer) quickDiagnose(command, errorOutput string, exitCode int) *DiagnosisResult {
	lowerErr := strings.ToLower(errorOutput)

	if strings.Contains(lowerErr, "permission denied") {
		return &DiagnosisResult{
			ErrorType:    "permission",
			RootCause:    "Insufficient permissions to execute the operation",
			Explanation:  "The current user does not have the required permissions.",
			FixCommands:  []string{"sudo " + command},
			Verification: "echo $?",
			Confidence:   "high",
		}
	}

	if strings.Contains(lowerErr, "command not found") || strings.Contains(lowerErr, "not found") {
		cmdParts := strings.Fields(command)
		cmdName := cmdParts[0]
		return &DiagnosisResult{
			ErrorType:    "dependency",
			RootCause:    fmt.Sprintf("Command '%s' is not installed or not in PATH", cmdName),
			Explanation:  "The command you're trying to run is not available on this system.",
			FixCommands:  []string{fmt.Sprintf("which %s", cmdName), "apt install " + cmdName},
			Verification: fmt.Sprintf("which %s", cmdName),
			Confidence:   "high",
		}
	}

	if strings.Contains(lowerErr, "no space left") {
		return &DiagnosisResult{
			ErrorType:    "disk",
			RootCause:    "Disk is full - no space left on device",
			Explanation:  "The filesystem has run out of space.",
			FixCommands:  []string{"df -h", "du -sh /* | sort -hr | head -20", "sudo apt clean"},
			Verification: "df -h",
			Confidence:   "high",
		}
	}

	if strings.Contains(lowerErr, "connection refused") {
		return &DiagnosisResult{
			ErrorType:    "network",
			RootCause:    "Connection refused - service may not be running",
			Explanation:  "The target service is not accepting connections.",
			FixCommands:  []string{"sudo systemctl status <service>", "sudo netstat -tlnp | grep <port>"},
			Verification: "curl -v localhost:<port>",
			Confidence:   "medium",
		}
	}

	if strings.Contains(lowerErr, "out of memory") || strings.Contains(lowerErr, "cannot allocate memory") {
		return &DiagnosisResult{
			ErrorType:    "memory",
			RootCause:    "System is out of memory",
			Explanation:  "The system has run out of available memory.",
			FixCommands:  []string{"free -h", "ps aux --sort=-%mem | head -10"},
			Verification: "free -h",
			Confidence:   "high",
		}
	}

	return nil
}

func analyzeDiskUsage(output string) *AnalysisResult {
	result := &AnalysisResult{
		Summary: "Disk usage summary",
		Status:  "ok",
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			usePercent := strings.TrimSuffix(fields[4], "%")
			if percent := parsePercent(usePercent); percent >= 0 {
				mountPoint := fields[5]
				if len(fields) > 5 {
					mountPoint = fields[len(fields)-1]
				}

				if percent >= 90 {
					result.Status = "error"
					result.Warnings = append(result.Warnings, fmt.Sprintf("CRITICAL: %s is %d%% full", mountPoint, percent))
				} else if percent >= 80 {
					if result.Status != "error" {
						result.Status = "warning"
					}
					result.Warnings = append(result.Warnings, fmt.Sprintf("Warning: %s is %d%% full", mountPoint, percent))
				}

				result.KeyFindings = append(result.KeyFindings, fmt.Sprintf("%s: %d%% used", mountPoint, percent))
			}
		}
	}

	if len(result.Warnings) > 0 {
		result.Suggestions = append(result.Suggestions, "Consider cleaning up large files or expanding storage")
	}

	return result
}

func analyzeMemoryUsage(output string) *AnalysisResult {
	result := &AnalysisResult{
		Summary: "Memory usage summary",
		Status:  "ok",
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Mem:") {
			result.KeyFindings = append(result.KeyFindings, "Memory: "+line)
		} else if strings.HasPrefix(line, "Swap:") {
			result.KeyFindings = append(result.KeyFindings, "Swap: "+line)
		}
	}

	return result
}

func truncateOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + "\n... (truncated)"
}

func getStatusFromExitCode(exitCode int) string {
	if exitCode == 0 {
		return "ok"
	}
	return "error"
}

func parsePercent(s string) int {
	var percent int
	_, err := fmt.Sscanf(s, "%d", &percent)
	if err != nil {
		return -1
	}
	return percent
}

func extractJSON(content string) string {
	jsonBlockRe := regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)```")
	if matches := jsonBlockRe.FindStringSubmatch(content); len(matches) == 2 {
		return strings.TrimSpace(matches[1])
	}

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		return content[start : end+1]
	}

	return content
}

// FormatAnalysisResult formats the analysis result for display.
func FormatAnalysisResult(result *AnalysisResult) string {
	var sb strings.Builder

	statusIcon := "✓"
	switch result.Status {
	case "warning":
		statusIcon = "⚠"
	case "error":
		statusIcon = "✗"
	case "info":
		statusIcon = "ℹ"
	}

	sb.WriteString(fmt.Sprintf("\n%s Analysis: %s\n", statusIcon, result.Summary))

	if len(result.KeyFindings) > 0 {
		sb.WriteString("\nKey Findings:\n")
		for _, f := range result.KeyFindings {
			sb.WriteString(fmt.Sprintf("  • %s\n", f))
		}
	}

	if len(result.Warnings) > 0 {
		sb.WriteString("\n⚠ Warnings:\n")
		for _, w := range result.Warnings {
			sb.WriteString(fmt.Sprintf("  • %s\n", w))
		}
	}

	if len(result.Suggestions) > 0 {
		sb.WriteString("\n💡 Suggestions:\n")
		for _, s := range result.Suggestions {
			sb.WriteString(fmt.Sprintf("  • %s\n", s))
		}
	}

	return sb.String()
}

// FormatDiagnosisResult formats the diagnosis result for display.
func FormatDiagnosisResult(result *DiagnosisResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n🔍 Error Diagnosis (Confidence: %s)\n", result.Confidence))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("Type: %s\n", result.ErrorType))
	sb.WriteString(fmt.Sprintf("Root Cause: %s\n", result.RootCause))
	sb.WriteString(fmt.Sprintf("\nExplanation:\n  %s\n", result.Explanation))

	if len(result.FixCommands) > 0 {
		sb.WriteString("\n🔧 Suggested Fix:\n")
		for i, cmd := range result.FixCommands {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, cmd))
		}
	}

	if result.Verification != "" {
		sb.WriteString(fmt.Sprintf("\n✓ Verify with: %s\n", result.Verification))
	}

	return sb.String()
}

// FormatSessionSummary formats the session summary for display.
func FormatSessionSummary(summary *SessionSummary) string {
	var sb strings.Builder

	sb.WriteString("\n📊 Session Summary\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("Commands Executed: %d (Success: %d, Failed: %d)\n",
		summary.TotalCommands, summary.SuccessfulCount, summary.FailedCount))

	if len(summary.HostsConnected) > 0 {
		sb.WriteString(fmt.Sprintf("Hosts Connected: %s\n", strings.Join(summary.HostsConnected, ", ")))
	}

	if len(summary.KeyOperations) > 0 {
		sb.WriteString("\nKey Operations:\n")
		for _, op := range summary.KeyOperations {
			sb.WriteString(fmt.Sprintf("  • %s\n", op))
		}
	}

	if len(summary.IssuesResolved) > 0 {
		sb.WriteString("\n✓ Issues Resolved:\n")
		for _, issue := range summary.IssuesResolved {
			sb.WriteString(fmt.Sprintf("  • %s\n", issue))
		}
	}

	if len(summary.Recommendations) > 0 {
		sb.WriteString("\n💡 Recommendations:\n")
		for _, rec := range summary.Recommendations {
			sb.WriteString(fmt.Sprintf("  • %s\n", rec))
		}
	}

	return sb.String()
}

// ProactiveAnalyzer provides intelligent proactive analysis after command execution.
type ProactiveAnalyzer struct {
	analyzer *Analyzer
	enabled  bool

	// Configuration options
	analyzeOnError      bool // Always analyze when command fails
	analyzeOnWarning    bool // Analyze when output contains warning keywords
	analyzeOnLargeOutput bool // Analyze when output is large
	minOutputForAnalysis int  // Minimum output length to trigger analysis
}

// ProactiveAnalyzerConfig holds configuration for proactive analysis.
type ProactiveAnalyzerConfig struct {
	Enabled              bool `json:"enabled"`
	AnalyzeOnError       bool `json:"analyze_on_error"`
	AnalyzeOnWarning     bool `json:"analyze_on_warning"`
	AnalyzeOnLargeOutput bool `json:"analyze_on_large_output"`
	MinOutputForAnalysis int  `json:"min_output_for_analysis"`
}

// DefaultProactiveAnalyzerConfig returns the default configuration.
func DefaultProactiveAnalyzerConfig() *ProactiveAnalyzerConfig {
	return &ProactiveAnalyzerConfig{
		Enabled:              true,
		AnalyzeOnError:       true,
		AnalyzeOnWarning:     false, // Disabled by default to avoid false positives
		AnalyzeOnLargeOutput: false,
		MinOutputForAnalysis: 1000,
	}
}

// NewProactiveAnalyzer creates a new proactive analyzer.
func NewProactiveAnalyzer(client ai.ModelClient, config *ProactiveAnalyzerConfig) *ProactiveAnalyzer {
	if config == nil {
		config = DefaultProactiveAnalyzerConfig()
	}

	return &ProactiveAnalyzer{
		analyzer:             NewAnalyzer(client),
		enabled:              config.Enabled,
		analyzeOnError:       config.AnalyzeOnError,
		analyzeOnWarning:     config.AnalyzeOnWarning,
		analyzeOnLargeOutput: config.AnalyzeOnLargeOutput,
		minOutputForAnalysis: config.MinOutputForAnalysis,
	}
}

// ProactiveResult represents the result of proactive analysis.
type ProactiveResult struct {
	ShouldShow    bool             `json:"should_show"`    // Whether to show this result to user
	Trigger       string           `json:"trigger"`        // What triggered the analysis (error, warning, large_output)
	Analysis      *AnalysisResult  `json:"analysis,omitempty"`
	Diagnosis     *DiagnosisResult `json:"diagnosis,omitempty"`
	QuickSuggestion string         `json:"quick_suggestion,omitempty"` // One-liner suggestion
}

// AnalyzeCommandOutput performs proactive analysis on command output.
// It returns nil if no analysis is needed.
func (pa *ProactiveAnalyzer) AnalyzeCommandOutput(ctx context.Context, command, stdout, stderr string, exitCode int) *ProactiveResult {
	if !pa.enabled {
		return nil
	}

	// Combine output for analysis
	output := stdout
	if stderr != "" {
		output = stdout + "\n" + stderr
	}

	// Check if analysis is needed
	trigger := pa.shouldAnalyze(command, output, exitCode)
	if trigger == "" {
		return nil
	}

	result := &ProactiveResult{
		ShouldShow: true,
		Trigger:    trigger,
	}

	// For errors, provide diagnosis
	if trigger == "error" && exitCode != 0 {
		diagnosis, err := pa.analyzer.DiagnoseError(ctx, command, stderr, exitCode)
		if err == nil && diagnosis != nil {
			result.Diagnosis = diagnosis
			result.QuickSuggestion = pa.buildQuickSuggestion(diagnosis)
		}
		return result
	}

	// For warnings and large output, provide analysis
	analysis, err := pa.analyzer.AnalyzeOutput(ctx, command, output, exitCode)
	if err == nil && analysis != nil {
		result.Analysis = analysis
		if len(analysis.Suggestions) > 0 {
			result.QuickSuggestion = analysis.Suggestions[0]
		}
	}

	return result
}

// shouldAnalyze determines if analysis should be performed.
func (pa *ProactiveAnalyzer) shouldAnalyze(command, output string, exitCode int) string {
	// Always analyze on error
	if pa.analyzeOnError && exitCode != 0 {
		return "error"
	}

	// Check for warning keywords
	if pa.analyzeOnWarning && containsWarningKeywords(output) {
		return "warning"
	}

	// Check for large output
	if pa.analyzeOnLargeOutput && len(output) >= pa.minOutputForAnalysis {
		return "large_output"
	}

	return ""
}

// containsWarningKeywords checks if output contains warning-related keywords.
func containsWarningKeywords(output string) bool {
	lower := strings.ToLower(output)
	warningKeywords := []string{
		"warning", "warn", "deprecated", "error", "failed", "failure",
		"critical", "危险", "警告", "错误", "失败",
		"disk full", "no space", "out of memory", "permission denied",
		"connection refused", "timeout", "timed out",
	}

	for _, keyword := range warningKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}

	return false
}

// buildQuickSuggestion creates a one-liner suggestion from diagnosis.
func (pa *ProactiveAnalyzer) buildQuickSuggestion(diagnosis *DiagnosisResult) string {
	if diagnosis == nil {
		return ""
	}

	if len(diagnosis.FixCommands) > 0 {
		return fmt.Sprintf("Try: %s", diagnosis.FixCommands[0])
	}

	return diagnosis.RootCause
}

// SetEnabled enables or disables proactive analysis.
func (pa *ProactiveAnalyzer) SetEnabled(enabled bool) {
	pa.enabled = enabled
}

// IsEnabled returns whether proactive analysis is enabled.
func (pa *ProactiveAnalyzer) IsEnabled() bool {
	return pa.enabled
}

// GetAnalyzer returns the underlying analyzer.
func (pa *ProactiveAnalyzer) GetAnalyzer() *Analyzer {
	return pa.analyzer
}

// FormatProactiveResult formats the proactive analysis result for display.
func FormatProactiveResult(result *ProactiveResult) string {
	if result == nil || !result.ShouldShow {
		return ""
	}

	var sb strings.Builder

	switch result.Trigger {
	case "error":
		sb.WriteString("\n🔴 ")
		sb.WriteString("AI 检测到错误，正在分析...\n")
		if result.Diagnosis != nil {
			sb.WriteString(FormatDiagnosisResult(result.Diagnosis))
		}
	case "warning":
		sb.WriteString("\n🟡 ")
		sb.WriteString("AI 检测到警告信息\n")
		if result.Analysis != nil {
			sb.WriteString(FormatAnalysisResult(result.Analysis))
		}
	case "large_output":
		sb.WriteString("\n📊 ")
		sb.WriteString("AI 输出分析\n")
		if result.Analysis != nil {
			sb.WriteString(FormatAnalysisResult(result.Analysis))
		}
	}

	if result.QuickSuggestion != "" {
		sb.WriteString(fmt.Sprintf("\n💡 快速建议: %s\n", result.QuickSuggestion))
	}

	return sb.String()
}
