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

package batch

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Formatter formats batch results for display.
type Formatter struct {
	width int
}

// NewFormatter creates a new result formatter.
func NewFormatter() *Formatter {
	return &Formatter{width: 80}
}

// FormatTable formats the result as a table.
func (f *Formatter) FormatTable(result *BatchResult) string {
	var sb strings.Builder

	sb.WriteString("\nResults:\n")
	sb.WriteString(strings.Repeat("-", f.width) + "\n")

	for _, r := range result.Results {
		hostName := r.Host.DisplayName()
		if r.Host.Alias == "" {
			hostName = fmt.Sprintf("%s@%s:%d", r.Host.User, r.Host.Host, r.Host.Port)
		}

		status := "OK"
		if r.Error != nil || r.ExitCode != 0 {
			status = "FAIL"
		}

		// Truncate output if too long
		output := strings.TrimSpace(r.Stdout)
		if r.Error != nil {
			output = r.Error.Error()
		} else if r.Stderr != "" && output == "" {
			output = strings.TrimSpace(r.Stderr)
		}

		// Replace newlines with spaces for table format
		output = strings.ReplaceAll(output, "\n", " ")
		maxOutputLen := f.width - 40
		if len(output) > maxOutputLen {
			output = output[:maxOutputLen-3] + "..."
		}

		sb.WriteString(fmt.Sprintf("%-25s | %-4s | %s\n", truncateString(hostName, 25), status, output))
	}

	sb.WriteString(strings.Repeat("-", f.width) + "\n")
	sb.WriteString(fmt.Sprintf("Summary: %d succeeded, %d failed (%.2fs)\n",
		result.Successful, result.Failed, result.Duration().Seconds()))

	return sb.String()
}

// FormatDetailed formats the result with full output for each host.
func (f *Formatter) FormatDetailed(result *BatchResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\nBatch Execution Results (%d hosts)\n", result.TotalHosts))
	sb.WriteString(strings.Repeat("=", f.width) + "\n\n")

	for i, r := range result.Results {
		hostName := fmt.Sprintf("%s@%s:%d", r.Host.User, r.Host.Host, r.Host.Port)
		if r.Host.Alias != "" {
			hostName = fmt.Sprintf("%s (%s)", r.Host.Alias, hostName)
		}

		status := "SUCCESS"
		if r.Error != nil {
			status = "ERROR"
		} else if r.ExitCode != 0 {
			status = fmt.Sprintf("FAILED (exit code: %d)", r.ExitCode)
		}

		sb.WriteString(fmt.Sprintf("[%d] %s - %s (%.2fs)\n", i+1, hostName, status, r.Duration.Seconds()))
		sb.WriteString(strings.Repeat("-", f.width) + "\n")

		if r.Error != nil {
			sb.WriteString(fmt.Sprintf("Error: %s\n", r.Error.Error()))
		} else {
			if r.Stdout != "" {
				sb.WriteString(r.Stdout)
				if !strings.HasSuffix(r.Stdout, "\n") {
					sb.WriteString("\n")
				}
			}
			if r.Stderr != "" {
				sb.WriteString("STDERR:\n")
				sb.WriteString(r.Stderr)
				if !strings.HasSuffix(r.Stderr, "\n") {
					sb.WriteString("\n")
				}
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(strings.Repeat("=", f.width) + "\n")
	sb.WriteString(fmt.Sprintf("Summary: %d succeeded, %d failed | Total time: %.2fs\n",
		result.Successful, result.Failed, result.Duration().Seconds()))

	return sb.String()
}

// FormatJSON formats the result as JSON.
func (f *Formatter) FormatJSON(result *BatchResult) (string, error) {
	type jsonResult struct {
		Host     string  `json:"host"`
		Alias    string  `json:"alias,omitempty"`
		Status   string  `json:"status"`
		ExitCode int     `json:"exit_code"`
		Stdout   string  `json:"stdout,omitempty"`
		Stderr   string  `json:"stderr,omitempty"`
		Error    string  `json:"error,omitempty"`
		Duration float64 `json:"duration_seconds"`
	}

	type jsonOutput struct {
		TotalHosts int          `json:"total_hosts"`
		Successful int          `json:"successful"`
		Failed     int          `json:"failed"`
		Duration   float64      `json:"duration_seconds"`
		Results    []jsonResult `json:"results"`
	}

	output := jsonOutput{
		TotalHosts: result.TotalHosts,
		Successful: result.Successful,
		Failed:     result.Failed,
		Duration:   result.Duration().Seconds(),
		Results:    make([]jsonResult, 0, len(result.Results)),
	}

	for _, r := range result.Results {
		jr := jsonResult{
			Host:     fmt.Sprintf("%s@%s:%d", r.Host.User, r.Host.Host, r.Host.Port),
			Alias:    r.Host.Alias,
			ExitCode: r.ExitCode,
			Stdout:   r.Stdout,
			Stderr:   r.Stderr,
			Duration: r.Duration.Seconds(),
		}

		if r.Error != nil {
			jr.Status = "error"
			jr.Error = r.Error.Error()
		} else if r.ExitCode != 0 {
			jr.Status = "failed"
		} else {
			jr.Status = "success"
		}

		output.Results = append(output.Results, jr)
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// FormatSummary formats a brief summary of the result.
func (f *Formatter) FormatSummary(result *BatchResult) string {
	return fmt.Sprintf("Executed on %d hosts: %d succeeded, %d failed (%.2fs)",
		result.TotalHosts, result.Successful, result.Failed, result.Duration().Seconds())
}

// truncateString truncates a string to the specified length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
