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

package dashboard

import (
	"fmt"
	"strings"
)

// ChartType defines the type of chart to render.
type ChartType string

const (
	ChartBar       ChartType = "bar"
	ChartProgress  ChartType = "progress"
	ChartSparkline ChartType = "sparkline"
)

// BarChartData represents data for a bar chart.
type BarChartData struct {
	Label string
	Value float64
	Max   float64
}

// RenderProgressBar renders a colored progress bar.
func RenderProgressBar(percentage float64, width int) string {
	if width < 5 {
		width = 5
	}

	filled := int(percentage / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	color := ColorGreen
	if percentage > 90 {
		color = ColorRed
	} else if percentage > 70 {
		color = ColorYellow
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("%s[%s]%s", color, bar, ColorReset)
}

// RenderBarChart renders a horizontal bar chart.
func RenderBarChart(data []BarChartData, width int) string {
	if width < 20 {
		width = 20
	}

	var sb strings.Builder
	labelWidth := 0
	for _, d := range data {
		if len(d.Label) > labelWidth {
			labelWidth = len(d.Label)
		}
	}

	barWidth := width - labelWidth - 15

	for _, d := range data {
		percentage := d.Value / d.Max * 100
		if percentage > 100 {
			percentage = 100
		}

		filled := int(percentage / 100.0 * float64(barWidth))
		if filled < 0 {
			filled = 0
		}

		color := ColorGreen
		if percentage > 90 {
			color = ColorRed
		} else if percentage > 70 {
			color = ColorYellow
		}

		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		sb.WriteString(fmt.Sprintf("  %-*s %s%s%s %6.1f%%\n",
			labelWidth, d.Label, color, bar, ColorReset, percentage))
	}

	return sb.String()
}

// RenderSparkline renders a sparkline from values.
func RenderSparkline(values []float64, width int) string {
	if len(values) == 0 {
		return strings.Repeat("▁", width)
	}

	// Scale values to 0-7 for sparkline characters
	chars := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	// Find min and max
	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	// Normalize and render
	var sb strings.Builder
	span := max - min
	if span == 0 {
		span = 1
	}

	// Sample or repeat values to match width
	step := float64(len(values)) / float64(width)
	for i := 0; i < width; i++ {
		idx := int(float64(i) * step)
		if idx >= len(values) {
			idx = len(values) - 1
		}
		normalized := (values[idx] - min) / span
		charIdx := int(normalized * 7)
		if charIdx > 7 {
			charIdx = 7
		}
		if charIdx < 0 {
			charIdx = 0
		}
		sb.WriteRune(chars[charIdx])
	}

	return sb.String()
}

// RenderStatusMatrix renders a matrix of status indicators.
func RenderStatusMatrix(hosts []string, statuses []HealthStatus, cols int) string {
	var sb strings.Builder

	if cols < 1 {
		cols = 5
	}

	for i, host := range hosts {
		if i > 0 && i%cols == 0 {
			sb.WriteString("\n")
		}

		var icon string
		switch statuses[i] {
		case StatusHealthy:
			icon = ColorGreen + "●" + ColorReset
		case StatusWarning:
			icon = ColorYellow + "●" + ColorReset
		case StatusCritical:
			icon = ColorRed + "●" + ColorReset
		default:
			icon = ColorGray + "○" + ColorReset
		}

		// Truncate long hostnames
		displayHost := host
		if len(displayHost) > 12 {
			displayHost = displayHost[:10] + ".."
		}

		sb.WriteString(fmt.Sprintf("%s %-12s  ", icon, displayHost))
	}

	sb.WriteString("\n")
	return sb.String()
}

// RenderGauge renders a semicircular gauge (ASCII art).
func RenderGauge(label string, value float64, max float64) string {
	percentage := value / max * 100
	if percentage > 100 {
		percentage = 100
	}

	color := ColorGreen
	if percentage > 90 {
		color = ColorRed
	} else if percentage > 70 {
		color = ColorYellow
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("    %s\n", label))
	sb.WriteString(fmt.Sprintf("   ╭────────────╮\n"))
	sb.WriteString(fmt.Sprintf("   │ %s%6.1f%%%s     │\n", color, percentage, ColorReset))
	sb.WriteString(fmt.Sprintf("   │ %s │\n", RenderProgressBar(percentage, 10)))
	sb.WriteString(fmt.Sprintf("   ╰────────────╯\n"))

	return sb.String()
}

// RenderResourcePanel renders a resource panel with multiple metrics.
func RenderResourcePanel(cpu, mem float64, diskUsage map[string]float64) string {
	var sb strings.Builder

	sb.WriteString("  ┌─────────────────────────────────────────────────┐\n")
	sb.WriteString("  │               📊 资源使用情况                      │\n")
	sb.WriteString("  ├─────────────────────────────────────────────────┤\n")

	// CPU
	sb.WriteString(fmt.Sprintf("  │  CPU    %s %5.1f%% │\n", RenderProgressBar(cpu, 25), cpu))

	// Memory
	sb.WriteString(fmt.Sprintf("  │  内存   %s %5.1f%% │\n", RenderProgressBar(mem, 25), mem))

	// Disks
	for mount, usage := range diskUsage {
		displayMount := mount
		if len(displayMount) > 8 {
			displayMount = displayMount[:8]
		}
		sb.WriteString(fmt.Sprintf("  │  %-6s %s %5.1f%% │\n", displayMount, RenderProgressBar(usage, 25), usage))
	}

	sb.WriteString("  └─────────────────────────────────────────────────┘\n")

	return sb.String()
}

// RenderCompactStatus renders a compact one-line status.
func RenderCompactStatus(hostname string, status HealthStatus, cpu, mem float64) string {
	var statusIcon string
	switch status {
	case StatusHealthy:
		statusIcon = ColorGreen + "✓" + ColorReset
	case StatusWarning:
		statusIcon = ColorYellow + "!" + ColorReset
	case StatusCritical:
		statusIcon = ColorRed + "✗" + ColorReset
	default:
		statusIcon = ColorGray + "?" + ColorReset
	}

	cpuColor := ColorGreen
	if cpu > 80 {
		cpuColor = ColorRed
	} else if cpu > 60 {
		cpuColor = ColorYellow
	}

	memColor := ColorGreen
	if mem > 80 {
		memColor = ColorRed
	} else if mem > 60 {
		memColor = ColorYellow
	}

	return fmt.Sprintf("%s %-15s  CPU:%s%5.1f%%%s  MEM:%s%5.1f%%%s",
		statusIcon, hostname,
		cpuColor, cpu, ColorReset,
		memColor, mem, ColorReset)
}
