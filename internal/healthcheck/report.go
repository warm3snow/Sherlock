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

package healthcheck

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// FormatReport formats a health report for terminal display.
func FormatReport(report *HealthReport) string {
	var sb strings.Builder

	// Header
	sb.WriteString("\n")
	sb.WriteString("╔══════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║            🏥 Sherlock 健康巡检报告                            ║\n")
	sb.WriteString("╚══════════════════════════════════════════════════════════════╝\n")
	sb.WriteString("\n")

	// Summary
	sb.WriteString(fmt.Sprintf("  📅 检查时间: %s\n", report.Timestamp.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("  ⏱️  耗时: %v\n", report.Duration.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("  🖥️  主机总数: %d\n\n", report.TotalHosts))

	// Status bar
	sb.WriteString("  ┌─────────────────────────────────────────────────┐\n")
	sb.WriteString("  │                   状态分布                        │\n")
	sb.WriteString("  ├─────────────────────────────────────────────────┤\n")
	sb.WriteString(fmt.Sprintf("  │  \033[32m● 健康: %-3d\033[0m  \033[33m● 警告: %-3d\033[0m  \033[31m● 严重: %-3d\033[0m  ○ 未知: %-3d │\n",
		report.HealthyCount, report.WarningCount, report.CriticalCount, report.UnreachableCount))
	sb.WriteString("  └─────────────────────────────────────────────────┘\n\n")

	// Overall score
	scoreColor := "\033[32m" // Green
	if report.OverallScore < 60 {
		scoreColor = "\033[31m" // Red
	} else if report.OverallScore < 80 {
		scoreColor = "\033[33m" // Yellow
	}

	sb.WriteString("  ┌─────────────────────────────────────────────────┐\n")
	sb.WriteString(fmt.Sprintf("  │             综合健康评分: %s%3d/100\033[0m               │\n", scoreColor, report.OverallScore))
	sb.WriteString(fmt.Sprintf("  │             %s                              │\n", renderScoreBar(report.OverallScore)))
	sb.WriteString("  └─────────────────────────────────────────────────┘\n\n")

	// Host details
	sb.WriteString("  📋 主机详情\n")
	sb.WriteString("  ─────────────────────────────────────────────────\n")

	for i, hr := range report.HostReports {
		statusIcon := getStatusIcon(hr.Status)
		displayName := hr.HostName
		if hr.HostAlias != "" {
			displayName = fmt.Sprintf("%s (%s)", hr.HostAlias, hr.HostName)
		}

		sb.WriteString(fmt.Sprintf("\n  %d. %s %s [评分: %d]\n", i+1, statusIcon, displayName, hr.Score))

		if hr.Error != "" {
			sb.WriteString(fmt.Sprintf("     \033[31m错误: %s\033[0m\n", hr.Error))
			continue
		}

		if hr.Metrics != nil {
			sb.WriteString(fmt.Sprintf("     CPU: %5.1f%%  |  内存: %5.1f%%  |  延迟: %v\n",
				hr.Metrics.CPUUsage, hr.Metrics.MemoryUsage, hr.Latency.Round(time.Millisecond)))
		}

		if len(hr.Issues) > 0 {
			sb.WriteString("     ⚠️  问题:\n")
			for _, issue := range hr.Issues {
				sb.WriteString(fmt.Sprintf("        - %s\n", issue))
			}
		}

		if len(hr.Suggestions) > 0 {
			sb.WriteString("     💡 建议:\n")
			for _, sug := range hr.Suggestions {
				sb.WriteString(fmt.Sprintf("        - %s\n", sug))
			}
		}
	}

	// Recommendations
	if len(report.Recommendations) > 0 {
		sb.WriteString("\n  📌 总体建议\n")
		sb.WriteString("  ─────────────────────────────────────────────────\n")
		for _, rec := range report.Recommendations {
			sb.WriteString(fmt.Sprintf("  %s\n", rec))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// FormatCompactReport formats a compact one-line-per-host report.
func FormatCompactReport(report *HealthReport) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\n🏥 健康巡检 | 评分: %d/100 | 主机: %d | 耗时: %v\n\n",
		report.OverallScore, report.TotalHosts, report.Duration.Round(time.Millisecond)))

	sb.WriteString("  主机                          状态   评分   CPU    内存\n")
	sb.WriteString("  ─────────────────────────────────────────────────────────\n")

	for _, hr := range report.HostReports {
		displayName := hr.HostName
		if hr.HostAlias != "" {
			displayName = hr.HostAlias
		}
		if len(displayName) > 28 {
			displayName = displayName[:25] + "..."
		}

		statusIcon := getStatusIcon(hr.Status)

		cpu := "-"
		mem := "-"
		if hr.Metrics != nil {
			cpu = fmt.Sprintf("%5.1f%%", hr.Metrics.CPUUsage)
			mem = fmt.Sprintf("%5.1f%%", hr.Metrics.MemoryUsage)
		}

		sb.WriteString(fmt.Sprintf("  %-28s  %s   %3d    %s  %s\n",
			displayName, statusIcon, hr.Score, cpu, mem))
	}

	sb.WriteString("\n")
	return sb.String()
}

// FormatJSON formats the report as JSON.
func FormatJSON(report *HealthReport, pretty bool) (string, error) {
	var data []byte
	var err error

	if pretty {
		data, err = json.MarshalIndent(report, "", "  ")
	} else {
		data, err = json.Marshal(report)
	}

	if err != nil {
		return "", err
	}
	return string(data), nil
}

// getStatusIcon returns the icon for a health status.
func getStatusIcon(status HealthStatus) string {
	switch status {
	case StatusHealthy:
		return "\033[32m●\033[0m" // Green dot
	case StatusWarning:
		return "\033[33m●\033[0m" // Yellow dot
	case StatusCritical:
		return "\033[31m●\033[0m" // Red dot
	default:
		return "\033[90m○\033[0m" // Gray circle
	}
}

// renderScoreBar renders a score bar.
func renderScoreBar(score int) string {
	width := 30
	filled := score * width / 100
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	color := "\033[32m" // Green
	if score < 60 {
		color = "\033[31m" // Red
	} else if score < 80 {
		color = "\033[33m" // Yellow
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("%s%s\033[0m", color, bar)
}

// GenerateMarkdownReport generates a markdown report.
func GenerateMarkdownReport(report *HealthReport) string {
	var sb strings.Builder

	sb.WriteString("# Sherlock 健康巡检报告\n\n")
	sb.WriteString(fmt.Sprintf("**检查时间**: %s\n\n", report.Timestamp.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("**检查耗时**: %v\n\n", report.Duration.Round(time.Millisecond)))

	// Summary table
	sb.WriteString("## 概览\n\n")
	sb.WriteString("| 指标 | 数值 |\n")
	sb.WriteString("|------|------|\n")
	sb.WriteString(fmt.Sprintf("| 主机总数 | %d |\n", report.TotalHosts))
	sb.WriteString(fmt.Sprintf("| 健康 | %d |\n", report.HealthyCount))
	sb.WriteString(fmt.Sprintf("| 警告 | %d |\n", report.WarningCount))
	sb.WriteString(fmt.Sprintf("| 严重 | %d |\n", report.CriticalCount))
	sb.WriteString(fmt.Sprintf("| 不可达 | %d |\n", report.UnreachableCount))
	sb.WriteString(fmt.Sprintf("| **综合评分** | **%d/100** |\n\n", report.OverallScore))

	// Host details
	sb.WriteString("## 主机详情\n\n")
	sb.WriteString("| 主机 | 状态 | 评分 | CPU | 内存 | 问题 |\n")
	sb.WriteString("|------|------|------|-----|------|------|\n")

	for _, hr := range report.HostReports {
		displayName := hr.HostName
		if hr.HostAlias != "" {
			displayName = hr.HostAlias
		}

		status := string(hr.Status)
		cpu := "-"
		mem := "-"
		if hr.Metrics != nil {
			cpu = fmt.Sprintf("%.1f%%", hr.Metrics.CPUUsage)
			mem = fmt.Sprintf("%.1f%%", hr.Metrics.MemoryUsage)
		}

		issues := "-"
		if len(hr.Issues) > 0 {
			issues = strings.Join(hr.Issues, "; ")
		}

		sb.WriteString(fmt.Sprintf("| %s | %s | %d | %s | %s | %s |\n",
			displayName, status, hr.Score, cpu, mem, issues))
	}

	// Recommendations
	if len(report.Recommendations) > 0 {
		sb.WriteString("\n## 建议\n\n")
		for _, rec := range report.Recommendations {
			sb.WriteString(fmt.Sprintf("- %s\n", rec))
		}
	}

	return sb.String()
}
