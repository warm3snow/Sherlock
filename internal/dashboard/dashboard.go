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

// Package dashboard provides terminal-based visualization for system metrics.
package dashboard

import (
	"fmt"
	"strings"
	"time"
)

// Colors for terminal output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"
	ColorGray   = "\033[90m"
	ColorBold   = "\033[1m"
)

// HostMetrics represents metrics for a single host.
type HostMetrics struct {
	HostName    string
	HostAlias   string
	Status      HealthStatus
	CPUUsage    float64
	MemoryUsage float64
	DiskUsage   map[string]float64
	LoadAvg     [3]float64
	Uptime      string
	LastUpdated time.Time
	Error       error
}

// HealthStatus represents the health status of a host.
type HealthStatus string

const (
	StatusHealthy  HealthStatus = "healthy"
	StatusWarning  HealthStatus = "warning"
	StatusCritical HealthStatus = "critical"
	StatusUnknown  HealthStatus = "unknown"
)

// Dashboard manages the terminal dashboard display.
type Dashboard struct {
	metrics      map[string]*HostMetrics
	refreshRate  time.Duration
	showDetailed bool
}

// NewDashboard creates a new dashboard instance.
func NewDashboard() *Dashboard {
	return &Dashboard{
		metrics:     make(map[string]*HostMetrics),
		refreshRate: 5 * time.Second,
	}
}

// SetRefreshRate sets the dashboard refresh rate.
func (d *Dashboard) SetRefreshRate(rate time.Duration) {
	d.refreshRate = rate
}

// SetShowDetailed sets whether to show detailed metrics.
func (d *Dashboard) SetShowDetailed(show bool) {
	d.showDetailed = show
}

// ToggleDetailed toggles the detailed view.
func (d *Dashboard) ToggleDetailed() {
	d.showDetailed = !d.showDetailed
}

// UpdateMetrics updates metrics for a host.
func (d *Dashboard) UpdateMetrics(metrics *HostMetrics) {
	metrics.LastUpdated = time.Now()
	metrics.Status = calculateStatus(metrics)
	d.metrics[metrics.HostName] = metrics
}

// GetMetrics returns metrics for a specific host.
func (d *Dashboard) GetMetrics(hostname string) *HostMetrics {
	return d.metrics[hostname]
}

// GetAllMetrics returns all host metrics.
func (d *Dashboard) GetAllMetrics() map[string]*HostMetrics {
	return d.metrics
}

// ClearMetrics clears all metrics.
func (d *Dashboard) ClearMetrics() {
	d.metrics = make(map[string]*HostMetrics)
}

// calculateStatus determines the health status based on metrics.
func calculateStatus(m *HostMetrics) HealthStatus {
	if m.Error != nil {
		return StatusUnknown
	}

	// Critical thresholds
	if m.CPUUsage > 90 || m.MemoryUsage > 95 {
		return StatusCritical
	}

	// Check disk usage
	for _, usage := range m.DiskUsage {
		if usage > 95 {
			return StatusCritical
		}
	}

	// Warning thresholds
	if m.CPUUsage > 70 || m.MemoryUsage > 80 {
		return StatusWarning
	}

	for _, usage := range m.DiskUsage {
		if usage > 80 {
			return StatusWarning
		}
	}

	return StatusHealthy
}

// Render renders the dashboard to a string (with screen clear).
func (d *Dashboard) Render() string {
	var sb strings.Builder
	sb.WriteString("\033[H\033[2J") // Clear screen
	sb.WriteString(d.RenderOnce())
	sb.WriteString(d.renderFooter())
	return sb.String()
}

// RenderOnce renders the dashboard once without clearing screen.
func (d *Dashboard) RenderOnce() string {
	var sb strings.Builder

	sb.WriteString(d.renderHeader())
	sb.WriteString("\n")
	sb.WriteString(d.renderStatusMatrix())
	sb.WriteString("\n")

	if d.showDetailed {
		sb.WriteString(d.renderDetailedMetrics())
	}

	return sb.String()
}

// renderHeader renders the dashboard header.
func (d *Dashboard) renderHeader() string {
	var sb strings.Builder

	sb.WriteString(ColorBold + ColorCyan)
	sb.WriteString("╔══════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║          🖥️  Sherlock 可视化仪表盘 (Dashboard)                ║\n")
	sb.WriteString("╚══════════════════════════════════════════════════════════════╝\n")
	sb.WriteString(ColorReset)

	sb.WriteString(fmt.Sprintf("  %s更新时间: %s%s  |  ", ColorGray, time.Now().Format("15:04:05"), ColorReset))
	sb.WriteString(fmt.Sprintf("主机数: %d  |  ", len(d.metrics)))
	sb.WriteString(fmt.Sprintf("刷新间隔: %s\n", d.refreshRate))

	return sb.String()
}

// renderStatusMatrix renders the host status matrix.
func (d *Dashboard) renderStatusMatrix() string {
	var sb strings.Builder

	sb.WriteString(ColorBold + "\n  📊 主机状态概览\n" + ColorReset)
	sb.WriteString("  ────────────────────────────────────────────────\n")

	if len(d.metrics) == 0 {
		sb.WriteString("  (暂无主机数据)\n")
		return sb.String()
	}

	// Status summary
	healthy, warning, critical, unknown := 0, 0, 0, 0
	for _, m := range d.metrics {
		switch m.Status {
		case StatusHealthy:
			healthy++
		case StatusWarning:
			warning++
		case StatusCritical:
			critical++
		default:
			unknown++
		}
	}

	sb.WriteString(fmt.Sprintf("  %s● 健康: %d%s  ", ColorGreen, healthy, ColorReset))
	sb.WriteString(fmt.Sprintf("%s● 警告: %d%s  ", ColorYellow, warning, ColorReset))
	sb.WriteString(fmt.Sprintf("%s● 严重: %d%s  ", ColorRed, critical, ColorReset))
	if unknown > 0 {
		sb.WriteString(fmt.Sprintf("%s● 未知: %d%s", ColorGray, unknown, ColorReset))
	}
	sb.WriteString("\n\n")

	// Host list with status indicators
	for name, m := range d.metrics {
		statusIcon := d.getStatusIcon(m.Status)
		displayName := name
		if m.HostAlias != "" {
			displayName = m.HostAlias
		}

		sb.WriteString(fmt.Sprintf("  %s %-20s ", statusIcon, displayName))
		sb.WriteString(fmt.Sprintf("CPU: %s  ", d.renderMiniBar(m.CPUUsage)))
		sb.WriteString(fmt.Sprintf("MEM: %s  ", d.renderMiniBar(m.MemoryUsage)))

		// Show max disk usage
		maxDisk := 0.0
		for _, usage := range m.DiskUsage {
			if usage > maxDisk {
				maxDisk = usage
			}
		}
		sb.WriteString(fmt.Sprintf("DISK: %s", d.renderMiniBar(maxDisk)))
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderDetailedMetrics renders detailed metrics for each host.
func (d *Dashboard) renderDetailedMetrics() string {
	var sb strings.Builder

	sb.WriteString(ColorBold + "\n  📈 详细资源指标\n" + ColorReset)
	sb.WriteString("  ────────────────────────────────────────────────\n")

	for name, m := range d.metrics {
		displayName := name
		if m.HostAlias != "" {
			displayName = fmt.Sprintf("%s (%s)", m.HostAlias, name)
		}

		sb.WriteString(fmt.Sprintf("\n  %s%s %s%s\n", ColorBold, d.getStatusIcon(m.Status), displayName, ColorReset))

		if m.Error != nil {
			sb.WriteString(fmt.Sprintf("    %s错误: %v%s\n", ColorRed, m.Error, ColorReset))
			continue
		}

		// CPU
		sb.WriteString(fmt.Sprintf("    CPU使用率: %s %.1f%%\n", RenderProgressBar(m.CPUUsage, 20), m.CPUUsage))

		// Memory
		sb.WriteString(fmt.Sprintf("    内存使用率: %s %.1f%%\n", RenderProgressBar(m.MemoryUsage, 20), m.MemoryUsage))

		// Disk
		for mount, usage := range m.DiskUsage {
			sb.WriteString(fmt.Sprintf("    磁盘(%s): %s %.1f%%\n", mount, RenderProgressBar(usage, 20), usage))
		}

		// Load average
		sb.WriteString(fmt.Sprintf("    负载均衡: %.2f / %.2f / %.2f\n", m.LoadAvg[0], m.LoadAvg[1], m.LoadAvg[2]))

		// Uptime
		if m.Uptime != "" {
			sb.WriteString(fmt.Sprintf("    运行时间: %s\n", m.Uptime))
		}
	}

	return sb.String()
}

// renderFooter renders the dashboard footer.
func (d *Dashboard) renderFooter() string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(ColorGray + "  ────────────────────────────────────────────────\n")
	sb.WriteString("  按 'q' 退出  |  按 'd' 切换详细视图  |  按 'r' 刷新\n")
	sb.WriteString(ColorReset)

	return sb.String()
}

// getStatusIcon returns the appropriate icon for a status.
func (d *Dashboard) getStatusIcon(status HealthStatus) string {
	switch status {
	case StatusHealthy:
		return ColorGreen + "●" + ColorReset
	case StatusWarning:
		return ColorYellow + "●" + ColorReset
	case StatusCritical:
		return ColorRed + "●" + ColorReset
	default:
		return ColorGray + "○" + ColorReset
	}
}

// renderMiniBar renders a minimal progress bar.
func (d *Dashboard) renderMiniBar(percentage float64) string {
	width := 10
	filled := int(percentage / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	color := ColorGreen
	if percentage > 80 {
		color = ColorRed
	} else if percentage > 60 {
		color = ColorYellow
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("%s%s%s %3.0f%%", color, bar, ColorReset, percentage)
}

// GetStatusSummary returns a summary of host statuses.
func (d *Dashboard) GetStatusSummary() (healthy, warning, critical, unknown int) {
	for _, m := range d.metrics {
		switch m.Status {
		case StatusHealthy:
			healthy++
		case StatusWarning:
			warning++
		case StatusCritical:
			critical++
		default:
			unknown++
		}
	}
	return
}
