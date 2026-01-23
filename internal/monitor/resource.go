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

package monitor

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/warm3snow/sherlock/internal/hosts"
	"github.com/warm3snow/sherlock/pkg/sshclient"
)

// ResourceMetrics represents system resource metrics.
type ResourceMetrics struct {
	CPUUsage    float64            // Percentage (0-100)
	MemoryUsed  int64              // Bytes
	MemoryTotal int64              // Bytes
	MemoryUsage float64            // Percentage (0-100)
	DiskUsage   map[string]float64 // Mount point -> percentage
	LoadAvg     [3]float64         // 1, 5, 15 minute load averages
	Uptime      string             // Uptime string
}

// ResourceChecker gets resource metrics from hosts.
type ResourceChecker struct {
	privateKeyPath string
}

// NewResourceChecker creates a new resource checker.
func NewResourceChecker(privateKeyPath string) *ResourceChecker {
	return &ResourceChecker{
		privateKeyPath: privateKeyPath,
	}
}

// GetMetrics gets resource metrics from a host.
func (r *ResourceChecker) GetMetrics(ctx context.Context, host *hosts.Host) (*ResourceMetrics, error) {
	cfg := &sshclient.Config{
		HostInfo: &sshclient.HostInfo{
			Host: host.Host,
			Port: host.Port,
			User: host.User,
		},
		PrivateKeyPath: r.privateKeyPath,
	}

	client, err := sshclient.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH client: %w", err)
	}
	defer client.Close()

	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	metrics := &ResourceMetrics{
		DiskUsage: make(map[string]float64),
	}

	// Get CPU usage
	cpuResult := client.Execute(ctx, "top -bn1 | grep 'Cpu(s)' | awk '{print $2}' | cut -d'%' -f1")
	if cpuResult.Error == nil && cpuResult.Stdout != "" {
		if cpu, err := strconv.ParseFloat(strings.TrimSpace(cpuResult.Stdout), 64); err == nil {
			metrics.CPUUsage = cpu
		}
	}

	// Alternative CPU command for some systems
	if metrics.CPUUsage == 0 {
		cpuResult = client.Execute(ctx, "vmstat 1 2 | tail -1 | awk '{print 100-$15}'")
		if cpuResult.Error == nil && cpuResult.Stdout != "" {
			if cpu, err := strconv.ParseFloat(strings.TrimSpace(cpuResult.Stdout), 64); err == nil {
				metrics.CPUUsage = cpu
			}
		}
	}

	// Get memory usage
	memResult := client.Execute(ctx, "free -b | grep Mem")
	if memResult.Error == nil && memResult.Stdout != "" {
		fields := strings.Fields(memResult.Stdout)
		if len(fields) >= 3 {
			if total, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				metrics.MemoryTotal = total
			}
			if used, err := strconv.ParseInt(fields[2], 10, 64); err == nil {
				metrics.MemoryUsed = used
			}
			if metrics.MemoryTotal > 0 {
				metrics.MemoryUsage = float64(metrics.MemoryUsed) / float64(metrics.MemoryTotal) * 100
			}
		}
	}

	// Get disk usage
	diskResult := client.Execute(ctx, "df -h | grep '^/' | awk '{print $6, $5}'")
	if diskResult.Error == nil && diskResult.Stdout != "" {
		lines := strings.Split(strings.TrimSpace(diskResult.Stdout), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				mountPoint := fields[0]
				usageStr := strings.TrimSuffix(fields[1], "%")
				if usage, err := strconv.ParseFloat(usageStr, 64); err == nil {
					metrics.DiskUsage[mountPoint] = usage
				}
			}
		}
	}

	// Get load average
	loadResult := client.Execute(ctx, "cat /proc/loadavg | awk '{print $1, $2, $3}'")
	if loadResult.Error == nil && loadResult.Stdout != "" {
		fields := strings.Fields(loadResult.Stdout)
		if len(fields) >= 3 {
			for i := 0; i < 3; i++ {
				if load, err := strconv.ParseFloat(fields[i], 64); err == nil {
					metrics.LoadAvg[i] = load
				}
			}
		}
	}

	// Get uptime
	uptimeResult := client.Execute(ctx, "uptime -p 2>/dev/null || uptime | awk -F'up' '{print $2}' | awk -F',' '{print $1}'")
	if uptimeResult.Error == nil {
		metrics.Uptime = strings.TrimSpace(uptimeResult.Stdout)
	}

	return metrics, nil
}

// FormatMetrics formats resource metrics for display.
func FormatMetrics(host *hosts.Host, metrics *ResourceMetrics) string {
	var sb strings.Builder

	hostName := host.DisplayName()
	sb.WriteString(fmt.Sprintf("\nResource Metrics (%s):\n", hostName))
	sb.WriteString(strings.Repeat("-", 50) + "\n")

	// CPU
	sb.WriteString(fmt.Sprintf("  CPU:     %.1f%%\n", metrics.CPUUsage))

	// Memory
	memUsed := formatBytesHuman(metrics.MemoryUsed)
	memTotal := formatBytesHuman(metrics.MemoryTotal)
	sb.WriteString(fmt.Sprintf("  Memory:  %s / %s (%.1f%%)\n", memUsed, memTotal, metrics.MemoryUsage))

	// Disk
	sb.WriteString("  Disk:\n")
	for mount, usage := range metrics.DiskUsage {
		sb.WriteString(fmt.Sprintf("    %-15s - %.1f%%\n", mount, usage))
	}

	// Load
	sb.WriteString(fmt.Sprintf("  Load:    %.2f, %.2f, %.2f\n", metrics.LoadAvg[0], metrics.LoadAvg[1], metrics.LoadAvg[2]))

	// Uptime
	if metrics.Uptime != "" {
		sb.WriteString(fmt.Sprintf("  Uptime:  %s\n", metrics.Uptime))
	}

	sb.WriteString(strings.Repeat("-", 50) + "\n")

	return sb.String()
}

// formatBytesHuman formats bytes to human readable string.
func formatBytesHuman(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1fTB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// ParseResourceOutput parses common resource command outputs.
// This is useful for parsing output from batch commands.
func ParseResourceOutput(output string) *ResourceMetrics {
	metrics := &ResourceMetrics{
		DiskUsage: make(map[string]float64),
	}

	// Try to parse CPU from top output
	cpuRegex := regexp.MustCompile(`(?i)cpu.*?(\d+\.?\d*)%?\s*(?:us|user)`)
	if match := cpuRegex.FindStringSubmatch(output); len(match) > 1 {
		if cpu, err := strconv.ParseFloat(match[1], 64); err == nil {
			metrics.CPUUsage = cpu
		}
	}

	// Try to parse memory
	memRegex := regexp.MustCompile(`(?i)mem.*?(\d+)\s+(\d+)`)
	if match := memRegex.FindStringSubmatch(output); len(match) > 2 {
		if total, err := strconv.ParseInt(match[1], 10, 64); err == nil {
			metrics.MemoryTotal = total * 1024 // Assume KB
		}
		if used, err := strconv.ParseInt(match[2], 10, 64); err == nil {
			metrics.MemoryUsed = used * 1024 // Assume KB
		}
	}

	return metrics
}
