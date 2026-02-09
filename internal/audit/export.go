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

package audit

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// ExportFormat defines the export format type.
type ExportFormat string

const (
	FormatJSON ExportFormat = "json"
	FormatCSV  ExportFormat = "csv"
	FormatText ExportFormat = "text"
)

// ExportOptions configures export behavior.
type ExportOptions struct {
	Format      ExportFormat
	Filter      QueryFilter
	IncludeStats bool
	Pretty       bool
}

// ExportReport contains the exported data.
type ExportReport struct {
	GeneratedAt time.Time    `json:"generated_at"`
	Filter      QueryFilter  `json:"filter,omitempty"`
	Statistics  *Statistics  `json:"statistics,omitempty"`
	Entries     []Entry      `json:"entries"`
}

// Export exports audit data to the specified format.
func (m *Manager) Export(opts ExportOptions) (string, error) {
	entries, err := m.Query(opts.Filter)
	if err != nil {
		return "", err
	}

	report := ExportReport{
		GeneratedAt: time.Now(),
		Filter:      opts.Filter,
		Entries:     entries,
	}

	if opts.IncludeStats {
		stats, _ := m.GetStatistics(opts.Filter)
		report.Statistics = stats
	}

	switch opts.Format {
	case FormatJSON:
		return exportJSON(report, opts.Pretty)
	case FormatCSV:
		return exportCSV(entries)
	case FormatText:
		return exportText(report)
	default:
		return exportJSON(report, opts.Pretty)
	}
}

// ExportToFile exports audit data to a file.
func (m *Manager) ExportToFile(filename string, opts ExportOptions) error {
	content, err := m.Export(opts)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, []byte(content), 0644)
}

func exportJSON(report ExportReport, pretty bool) (string, error) {
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

func exportCSV(entries []Entry) (string, error) {
	var sb strings.Builder
	writer := csv.NewWriter(&sb)

	// Write header
	header := []string{"ID", "Timestamp", "Operation", "Host", "Command", "Exit Code", "Duration (ms)", "Success", "Details"}
	if err := writer.Write(header); err != nil {
		return "", err
	}

	// Write entries
	for _, e := range entries {
		success := "Yes"
		if !e.Success {
			success = "No"
		}
		row := []string{
			fmt.Sprintf("%d", e.ID),
			e.Timestamp.Format("2006-01-02 15:04:05"),
			string(e.Operation),
			e.HostName,
			e.Command,
			fmt.Sprintf("%d", e.ExitCode),
			fmt.Sprintf("%d", e.Duration),
			success,
			e.Details,
		}
		if err := writer.Write(row); err != nil {
			return "", err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}

	return sb.String(), nil
}

func exportText(report ExportReport) (string, error) {
	var sb strings.Builder

	sb.WriteString("═══════════════════════════════════════════════════════════════\n")
	sb.WriteString("                    Sherlock 审计日志报告\n")
	sb.WriteString("═══════════════════════════════════════════════════════════════\n\n")

	sb.WriteString(fmt.Sprintf("生成时间: %s\n", report.GeneratedAt.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("记录总数: %d\n\n", len(report.Entries)))

	if report.Statistics != nil {
		sb.WriteString("━━━━━━━━━━━━━━━━━ 统计摘要 ━━━━━━━━━━━━━━━━━\n\n")
		sb.WriteString(FormatStatistics(report.Statistics))
		sb.WriteString("\n")
	}

	sb.WriteString("━━━━━━━━━━━━━━━━━ 操作详情 ━━━━━━━━━━━━━━━━━\n\n")

	for i, e := range report.Entries {
		status := "✓ 成功"
		if !e.Success {
			status = "✗ 失败"
		}
		
		sb.WriteString(fmt.Sprintf("[%d] %s - %s\n", i+1, e.Timestamp.Format("2006-01-02 15:04:05"), status))
		sb.WriteString(fmt.Sprintf("    操作类型: %s\n", e.Operation))
		if e.HostName != "" {
			sb.WriteString(fmt.Sprintf("    目标主机: %s\n", e.HostName))
		}
		if e.Command != "" {
			sb.WriteString(fmt.Sprintf("    执行命令: %s\n", e.Command))
		}
		sb.WriteString(fmt.Sprintf("    执行耗时: %dms\n", e.Duration))
		if e.Details != "" {
			sb.WriteString(fmt.Sprintf("    详细信息: %s\n", e.Details))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("═══════════════════════════════════════════════════════════════\n")
	sb.WriteString("                       报告结束\n")
	sb.WriteString("═══════════════════════════════════════════════════════════════\n")

	return sb.String(), nil
}

// GenerateComplianceReport generates a compliance-ready report.
func (m *Manager) GenerateComplianceReport(startTime, endTime time.Time) (string, error) {
	opts := ExportOptions{
		Format: FormatText,
		Filter: QueryFilter{
			StartTime: &startTime,
			EndTime:   &endTime,
		},
		IncludeStats: true,
		Pretty:       true,
	}

	return m.Export(opts)
}

// GenerateDailyReport generates a daily summary report.
func (m *Manager) GenerateDailyReport() (string, error) {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endOfDay := startOfDay.Add(24 * time.Hour).Add(-time.Second)

	return m.GenerateComplianceReport(startOfDay, endOfDay)
}

// GenerateWeeklyReport generates a weekly summary report.
func (m *Manager) GenerateWeeklyReport() (string, error) {
	now := time.Now()
	startOfWeek := now.AddDate(0, 0, -int(now.Weekday()))
	startOfWeek = time.Date(startOfWeek.Year(), startOfWeek.Month(), startOfWeek.Day(), 0, 0, 0, 0, now.Location())

	return m.GenerateComplianceReport(startOfWeek, now)
}
