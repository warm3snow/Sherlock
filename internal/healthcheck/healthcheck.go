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

// Package healthcheck provides batch health inspection for hosts.
package healthcheck

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/warm3snow/sherlock/internal/hosts"
	"github.com/warm3snow/sherlock/internal/monitor"
)

// CheckLevel defines the depth of health check.
type CheckLevel string

const (
	LevelQuick    CheckLevel = "quick"    // TCP/SSH connectivity only
	LevelStandard CheckLevel = "standard" // + basic resources
	LevelDeep     CheckLevel = "deep"     // + service checks
)

// HostHealthReport represents the health report for a single host.
type HostHealthReport struct {
	HostID      int64                   `json:"host_id"`
	HostName    string                  `json:"host_name"`
	HostAlias   string                  `json:"host_alias,omitempty"`
	Status      HealthStatus            `json:"status"`
	Score       int                     `json:"score"` // 0-100
	Reachable   bool                    `json:"reachable"`
	SSHAvailable bool                   `json:"ssh_available"`
	Latency     time.Duration           `json:"latency_ms"`
	Metrics     *monitor.ResourceMetrics `json:"metrics,omitempty"`
	Issues      []string                `json:"issues,omitempty"`
	Suggestions []string                `json:"suggestions,omitempty"`
	CheckedAt   time.Time               `json:"checked_at"`
	Error       string                  `json:"error,omitempty"`
}

// HealthStatus represents the health status.
type HealthStatus string

const (
	StatusHealthy  HealthStatus = "healthy"
	StatusWarning  HealthStatus = "warning"
	StatusCritical HealthStatus = "critical"
	StatusUnknown  HealthStatus = "unknown"
)

// HealthReport represents the overall health report.
type HealthReport struct {
	Timestamp       time.Time          `json:"timestamp"`
	TotalHosts      int                `json:"total_hosts"`
	HealthyCount    int                `json:"healthy_count"`
	WarningCount    int                `json:"warning_count"`
	CriticalCount   int                `json:"critical_count"`
	UnreachableCount int               `json:"unreachable_count"`
	OverallScore    int                `json:"overall_score"` // 0-100
	OverallStatus   HealthStatus       `json:"overall_status"`
	HostReports     []HostHealthReport `json:"host_reports"`
	Recommendations []string           `json:"recommendations"`
	Duration        time.Duration      `json:"duration_ms"`
}

// Checker performs health checks on hosts.
type Checker struct {
	hostManager     *hosts.Manager
	resourceChecker *monitor.ResourceChecker
	connectChecker  *monitor.Checker
	privateKeyPath  string
	timeout         time.Duration
	concurrency     int
}

// NewChecker creates a new health checker.
func NewChecker(hostManager *hosts.Manager, privateKeyPath string) *Checker {
	return &Checker{
		hostManager:     hostManager,
		resourceChecker: monitor.NewResourceChecker(privateKeyPath),
		connectChecker:  monitor.NewChecker(hostManager, privateKeyPath),
		privateKeyPath:  privateKeyPath,
		timeout:         10 * time.Second,
		concurrency:     10,
	}
}

// SetTimeout sets the check timeout.
func (c *Checker) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
	c.connectChecker.SetTimeout(timeout)
}

// SetConcurrency sets the concurrency level.
func (c *Checker) SetConcurrency(n int) {
	if n > 0 {
		c.concurrency = n
	}
}

// CheckAll checks all hosts.
func (c *Checker) CheckAll(ctx context.Context, level CheckLevel) (*HealthReport, error) {
	allHosts, err := c.hostManager.GetAllHosts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get hosts: %w", err)
	}
	// Convert to pointers
	hostPtrs := make([]*hosts.Host, len(allHosts))
	for i := range allHosts {
		hostPtrs[i] = &allHosts[i]
	}
	return c.checkHosts(ctx, hostPtrs, level)
}

// CheckGroup checks hosts in a specific group.
func (c *Checker) CheckGroup(ctx context.Context, group string, level CheckLevel) (*HealthReport, error) {
	groupHosts, err := c.hostManager.GetGroupHostsByName(ctx, group)
	if err != nil {
		return nil, fmt.Errorf("failed to get group hosts: %w", err)
	}
	// Convert to pointers
	hostPtrs := make([]*hosts.Host, len(groupHosts))
	for i := range groupHosts {
		hostPtrs[i] = &groupHosts[i]
	}
	return c.checkHosts(ctx, hostPtrs, level)
}

// CheckHost checks a single host.
func (c *Checker) CheckHost(ctx context.Context, host *hosts.Host, level CheckLevel) (*HostHealthReport, error) {
	report := &HostHealthReport{
		HostID:    host.ID,
		HostName:  fmt.Sprintf("%s@%s:%d", host.User, host.Host, host.Port),
		HostAlias: host.Alias,
		CheckedAt: time.Now(),
	}

	// Check connectivity first
	connStatus := c.connectChecker.CheckHost(ctx, host)
	report.Reachable = connStatus.Reachable
	report.SSHAvailable = connStatus.SSHAvailable
	report.Latency = connStatus.Latency

	if !connStatus.Reachable {
		report.Status = StatusCritical
		report.Score = 0
		report.Issues = append(report.Issues, "主机不可达")
		report.Suggestions = append(report.Suggestions, "检查网络连接和防火墙配置")
		if connStatus.Error != nil {
			report.Error = connStatus.Error.Error()
		}
		return report, nil
	}

	if !connStatus.SSHAvailable {
		report.Status = StatusCritical
		report.Score = 20
		report.Issues = append(report.Issues, "SSH服务不可用")
		report.Suggestions = append(report.Suggestions, "检查SSH服务状态和端口配置")
		return report, nil
	}

	// Quick check stops here
	if level == LevelQuick {
		report.Status = StatusHealthy
		report.Score = 100
		return report, nil
	}

	// Standard/Deep check - get resource metrics
	metrics, err := c.getResourceMetrics(ctx, host)
	if err != nil {
		report.Status = StatusWarning
		report.Score = 50
		report.Issues = append(report.Issues, "无法获取资源指标: "+err.Error())
		return report, nil
	}

	report.Metrics = metrics
	c.analyzeMetrics(report, metrics)

	return report, nil
}

// getResourceMetrics retrieves resource metrics from a host.
func (c *Checker) getResourceMetrics(ctx context.Context, host *hosts.Host) (*monitor.ResourceMetrics, error) {
	return c.resourceChecker.GetMetrics(ctx, host)
}

// analyzeMetrics analyzes metrics and updates the report.
func (c *Checker) analyzeMetrics(report *HostHealthReport, metrics *monitor.ResourceMetrics) {
	score := 100
	report.Status = StatusHealthy

	// CPU analysis
	if metrics.CPUUsage > 90 {
		score -= 30
		report.Issues = append(report.Issues, fmt.Sprintf("CPU使用率过高: %.1f%%", metrics.CPUUsage))
		report.Suggestions = append(report.Suggestions, "检查高CPU占用进程，考虑优化或扩容")
		report.Status = StatusCritical
	} else if metrics.CPUUsage > 70 {
		score -= 15
		report.Issues = append(report.Issues, fmt.Sprintf("CPU使用率较高: %.1f%%", metrics.CPUUsage))
		if report.Status != StatusCritical {
			report.Status = StatusWarning
		}
	}

	// Memory analysis
	if metrics.MemoryUsage > 95 {
		score -= 30
		report.Issues = append(report.Issues, fmt.Sprintf("内存使用率过高: %.1f%%", metrics.MemoryUsage))
		report.Suggestions = append(report.Suggestions, "清理内存或增加内存容量")
		report.Status = StatusCritical
	} else if metrics.MemoryUsage > 80 {
		score -= 15
		report.Issues = append(report.Issues, fmt.Sprintf("内存使用率较高: %.1f%%", metrics.MemoryUsage))
		if report.Status != StatusCritical {
			report.Status = StatusWarning
		}
	}

	// Disk analysis
	for mount, usage := range metrics.DiskUsage {
		if usage > 95 {
			score -= 25
			report.Issues = append(report.Issues, fmt.Sprintf("磁盘空间严重不足(%s): %.1f%%", mount, usage))
			report.Suggestions = append(report.Suggestions, fmt.Sprintf("清理磁盘 %s 或扩容", mount))
			report.Status = StatusCritical
		} else if usage > 85 {
			score -= 10
			report.Issues = append(report.Issues, fmt.Sprintf("磁盘空间较低(%s): %.1f%%", mount, usage))
			if report.Status != StatusCritical {
				report.Status = StatusWarning
			}
		}
	}

	// Load average analysis (comparing to number of CPUs - assuming 4 cores as baseline)
	if metrics.LoadAvg[0] > 8 {
		score -= 10
		report.Issues = append(report.Issues, fmt.Sprintf("系统负载过高: %.2f", metrics.LoadAvg[0]))
		if report.Status != StatusCritical {
			report.Status = StatusWarning
		}
	}

	if score < 0 {
		score = 0
	}
	report.Score = score
}

// checkHosts checks multiple hosts concurrently.
func (c *Checker) checkHosts(ctx context.Context, hostList []*hosts.Host, level CheckLevel) (*HealthReport, error) {
	startTime := time.Now()

	report := &HealthReport{
		Timestamp:  startTime,
		TotalHosts: len(hostList),
	}

	if len(hostList) == 0 {
		report.OverallScore = 100
		report.OverallStatus = StatusHealthy
		return report, nil
	}

	// Concurrent checking
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, c.concurrency)

	hostReports := make([]HostHealthReport, 0, len(hostList))

	for _, host := range hostList {
		wg.Add(1)
		sem <- struct{}{}

		go func(h *hosts.Host) {
			defer wg.Done()
			defer func() { <-sem }()

			hostReport, _ := c.CheckHost(ctx, h, level)

			mu.Lock()
			hostReports = append(hostReports, *hostReport)
			mu.Unlock()
		}(host)
	}

	wg.Wait()

	// Sort by status (critical first) then by name
	sort.Slice(hostReports, func(i, j int) bool {
		if hostReports[i].Status != hostReports[j].Status {
			return statusPriority(hostReports[i].Status) > statusPriority(hostReports[j].Status)
		}
		return hostReports[i].HostName < hostReports[j].HostName
	})

	report.HostReports = hostReports
	report.Duration = time.Since(startTime)

	// Calculate overall statistics
	totalScore := 0
	for _, hr := range hostReports {
		totalScore += hr.Score
		switch hr.Status {
		case StatusHealthy:
			report.HealthyCount++
		case StatusWarning:
			report.WarningCount++
		case StatusCritical:
			report.CriticalCount++
		case StatusUnknown:
			report.UnreachableCount++
		}
	}

	if len(hostReports) > 0 {
		report.OverallScore = totalScore / len(hostReports)
	}

	// Determine overall status
	if report.CriticalCount > 0 || report.UnreachableCount > 0 {
		report.OverallStatus = StatusCritical
	} else if report.WarningCount > 0 {
		report.OverallStatus = StatusWarning
	} else {
		report.OverallStatus = StatusHealthy
	}

	// Generate recommendations
	report.Recommendations = c.generateRecommendations(report)

	return report, nil
}

// statusPriority returns priority for sorting (higher = more severe).
func statusPriority(status HealthStatus) int {
	switch status {
	case StatusCritical:
		return 3
	case StatusWarning:
		return 2
	case StatusHealthy:
		return 1
	default:
		return 0
	}
}

// generateRecommendations generates overall recommendations based on the report.
func (c *Checker) generateRecommendations(report *HealthReport) []string {
	var recs []string

	if report.UnreachableCount > 0 {
		recs = append(recs, fmt.Sprintf("🔴 %d 台主机不可达，请检查网络连接", report.UnreachableCount))
	}

	if report.CriticalCount > 0 {
		recs = append(recs, fmt.Sprintf("🔴 %d 台主机存在严重问题，需要立即处理", report.CriticalCount))
	}

	if report.WarningCount > 0 {
		recs = append(recs, fmt.Sprintf("🟡 %d 台主机存在警告，建议尽快处理", report.WarningCount))
	}

	if report.OverallScore < 60 {
		recs = append(recs, "📊 整体健康评分较低，建议进行系统优化")
	}

	if len(recs) == 0 {
		recs = append(recs, "✅ 所有主机运行正常，无需特别处理")
	}

	return recs
}
