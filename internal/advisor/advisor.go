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

// Package advisor provides intelligent operations advice and optimization suggestions.
package advisor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/warm3snow/sherlock/internal/ai"
	"github.com/warm3snow/sherlock/internal/hosts"
	"github.com/warm3snow/sherlock/internal/monitor"
	"github.com/warm3snow/sherlock/pkg/sshclient"
)

// RiskLevel defines the severity of an identified risk.
type RiskLevel string

const (
	RiskCritical RiskLevel = "critical"
	RiskHigh     RiskLevel = "high"
	RiskMedium   RiskLevel = "medium"
	RiskLow      RiskLevel = "low"
	RiskInfo     RiskLevel = "info"
)

// Issue represents an identified issue or risk.
type Issue struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	RiskLevel   RiskLevel `json:"risk_level"`
	Category    string    `json:"category"` // cpu, memory, disk, network, security, service
	FixCommand  string    `json:"fix_command,omitempty"`
	AutoFixable bool      `json:"auto_fixable"`
}

// Advice represents optimization advice.
type Advice struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Impact      string    `json:"impact"` // What will improve
	Command     string    `json:"command,omitempty"`
	Priority    RiskLevel `json:"priority"`
}

// AdvisorResult contains the analysis result.
type AdvisorResult struct {
	HostName        string                   `json:"host_name"`
	AnalyzedAt      time.Time                `json:"analyzed_at"`
	OverallHealth   string                   `json:"overall_health"` // healthy, warning, critical
	Metrics         *monitor.ResourceMetrics `json:"metrics,omitempty"`
	Issues          []Issue                  `json:"issues"`
	Advice          []Advice                 `json:"advice"`
	QuickWins       []string                 `json:"quick_wins"` // One-liner fixes
	AIInsights      string                   `json:"ai_insights,omitempty"`
	TimeSavedMinutes int                     `json:"time_saved_minutes"`
}

// Advisor provides intelligent operations advice.
type Advisor struct {
	aiClient        ai.ModelClient
	resourceChecker *monitor.ResourceChecker
}

// NewAdvisor creates a new advisor.
func NewAdvisor(aiClient ai.ModelClient, privateKeyPath string) *Advisor {
	return &Advisor{
		aiClient:        aiClient,
		resourceChecker: monitor.NewResourceChecker(privateKeyPath),
	}
}

// Analyze analyzes a host and provides advice.
func (a *Advisor) Analyze(ctx context.Context, executor CommandExecutor, hostInfo *sshclient.HostInfo) (*AdvisorResult, error) {
	result := &AdvisorResult{
		HostName:   executor.HostInfoString(),
		AnalyzedAt: time.Now(),
		Issues:     make([]Issue, 0),
		Advice:     make([]Advice, 0),
		QuickWins:  make([]string, 0),
	}

	// Convert HostInfo to hosts.Host for resource checker
	host := &hosts.Host{
		Host: hostInfo.Host,
		Port: hostInfo.Port,
		User: hostInfo.User,
	}

	// Get resource metrics
	metrics, err := a.resourceChecker.GetMetrics(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}
	result.Metrics = metrics

	// Analyze metrics and identify issues
	a.analyzeMetrics(result, metrics)

	// Get additional system info for deeper analysis
	a.analyzeSystem(ctx, result, executor)

	// Determine overall health
	result.OverallHealth = a.calculateOverallHealth(result)

	// Get AI insights if available
	if a.aiClient != nil {
		a.getAIInsights(ctx, result)
	}

	// Estimate time saved
	result.TimeSavedMinutes = len(result.Issues)*5 + len(result.Advice)*3

	return result, nil
}

// CommandExecutor interface for executing commands.
type CommandExecutor interface {
	Execute(ctx context.Context, command string) *sshclient.ExecuteResult
	HostInfoString() string
}

// analyzeMetrics analyzes resource metrics.
func (a *Advisor) analyzeMetrics(result *AdvisorResult, metrics *monitor.ResourceMetrics) {
	// CPU analysis
	if metrics.CPUUsage > 90 {
		result.Issues = append(result.Issues, Issue{
			ID:          "cpu-critical",
			Title:       "CPU使用率严重过高",
			Description: fmt.Sprintf("当前CPU使用率 %.1f%%，系统可能出现卡顿", metrics.CPUUsage),
			RiskLevel:   RiskCritical,
			Category:    "cpu",
			FixCommand:  "top -bn1 -o %CPU | head -20",
		})
		result.QuickWins = append(result.QuickWins, "查看高CPU进程: top -bn1 -o %CPU | head -10")
	} else if metrics.CPUUsage > 70 {
		result.Issues = append(result.Issues, Issue{
			ID:          "cpu-high",
			Title:       "CPU使用率较高",
			Description: fmt.Sprintf("当前CPU使用率 %.1f%%，建议关注", metrics.CPUUsage),
			RiskLevel:   RiskMedium,
			Category:    "cpu",
		})
	}

	// Memory analysis
	if metrics.MemoryUsage > 95 {
		result.Issues = append(result.Issues, Issue{
			ID:          "mem-critical",
			Title:       "内存即将耗尽",
			Description: fmt.Sprintf("当前内存使用率 %.1f%%，可能触发OOM", metrics.MemoryUsage),
			RiskLevel:   RiskCritical,
			Category:    "memory",
			FixCommand:  "ps aux --sort=-%mem | head -10",
			AutoFixable: false,
		})
		result.Advice = append(result.Advice, Advice{
			Title:       "清理内存缓存",
			Description: "可以尝试清理系统缓存释放内存",
			Impact:      "释放可用内存",
			Command:     "sync && echo 3 > /proc/sys/vm/drop_caches",
			Priority:    RiskHigh,
		})
	} else if metrics.MemoryUsage > 80 {
		result.Issues = append(result.Issues, Issue{
			ID:          "mem-high",
			Title:       "内存使用率较高",
			Description: fmt.Sprintf("当前内存使用率 %.1f%%", metrics.MemoryUsage),
			RiskLevel:   RiskMedium,
			Category:    "memory",
		})
	}

	// Disk analysis
	for mount, usage := range metrics.DiskUsage {
		if usage > 95 {
			result.Issues = append(result.Issues, Issue{
				ID:          fmt.Sprintf("disk-critical-%s", mount),
				Title:       fmt.Sprintf("磁盘空间严重不足: %s", mount),
				Description: fmt.Sprintf("磁盘 %s 使用率 %.1f%%，可能导致服务异常", mount, usage),
				RiskLevel:   RiskCritical,
				Category:    "disk",
				FixCommand:  fmt.Sprintf("du -sh %s/* 2>/dev/null | sort -hr | head -10", mount),
				AutoFixable: false,
			})
			result.Advice = append(result.Advice, Advice{
				Title:       fmt.Sprintf("清理磁盘 %s", mount),
				Description: "查找并清理大文件和旧日志",
				Impact:      "释放磁盘空间",
				Command:     "find /var/log -type f -name '*.gz' -mtime +7 -delete",
				Priority:    RiskCritical,
			})
		} else if usage > 85 {
			result.Issues = append(result.Issues, Issue{
				ID:          fmt.Sprintf("disk-warning-%s", mount),
				Title:       fmt.Sprintf("磁盘空间较低: %s", mount),
				Description: fmt.Sprintf("磁盘 %s 使用率 %.1f%%", mount, usage),
				RiskLevel:   RiskMedium,
				Category:    "disk",
			})
		}
	}

	// Load average analysis
	if metrics.LoadAvg[0] > 10 {
		result.Issues = append(result.Issues, Issue{
			ID:          "load-high",
			Title:       "系统负载过高",
			Description: fmt.Sprintf("1分钟负载 %.2f，系统响应可能变慢", metrics.LoadAvg[0]),
			RiskLevel:   RiskHigh,
			Category:    "cpu",
		})
	}
}

// analyzeSystem performs additional system analysis.
func (a *Advisor) analyzeSystem(ctx context.Context, result *AdvisorResult, executor CommandExecutor) {
	// Check for zombie processes
	zombieResult := executor.Execute(ctx, "ps aux | grep -c 'Z' 2>/dev/null || echo 0")
	if zombieResult.Error == nil {
		count := strings.TrimSpace(zombieResult.Stdout)
		if count != "0" && count != "1" { // 1 is the grep itself
			result.Issues = append(result.Issues, Issue{
				ID:          "zombie-process",
				Title:       "存在僵尸进程",
				Description: fmt.Sprintf("检测到 %s 个僵尸进程", count),
				RiskLevel:   RiskLow,
				Category:    "process",
				FixCommand:  "ps aux | grep 'Z'",
			})
		}
	}

	// Check failed systemd services
	failedResult := executor.Execute(ctx, "systemctl --failed --no-pager 2>/dev/null | grep -c 'failed' || echo 0")
	if failedResult.Error == nil {
		count := strings.TrimSpace(failedResult.Stdout)
		if count != "0" {
			result.Issues = append(result.Issues, Issue{
				ID:          "failed-services",
				Title:       "存在失败的服务",
				Description: fmt.Sprintf("检测到 %s 个失败的systemd服务", count),
				RiskLevel:   RiskMedium,
				Category:    "service",
				FixCommand:  "systemctl --failed --no-pager",
			})
		}
	}

	// Check for available updates (quick check)
	updateResult := executor.Execute(ctx, "apt list --upgradable 2>/dev/null | wc -l || yum check-update 2>/dev/null | wc -l || echo 0")
	if updateResult.Error == nil {
		count := strings.TrimSpace(updateResult.Stdout)
		if count != "0" && count != "1" {
			result.Advice = append(result.Advice, Advice{
				Title:       "系统更新可用",
				Description: fmt.Sprintf("有 %s 个软件包可更新", count),
				Impact:      "提升安全性和稳定性",
				Priority:    RiskInfo,
			})
		}
	}

	// Check uptime
	uptimeResult := executor.Execute(ctx, "uptime -s 2>/dev/null || echo ''")
	if uptimeResult.Error == nil && uptimeResult.Stdout != "" {
		// If uptime > 365 days, suggest reboot
		result.Advice = append(result.Advice, Advice{
			Title:       "定期重启建议",
			Description: "长时间运行的系统建议定期重启以应用内核更新",
			Impact:      "应用安全补丁",
			Priority:    RiskInfo,
		})
	}
}

// calculateOverallHealth determines overall health status.
func (a *Advisor) calculateOverallHealth(result *AdvisorResult) string {
	hasCritical := false
	hasWarning := false

	for _, issue := range result.Issues {
		if issue.RiskLevel == RiskCritical {
			hasCritical = true
		}
		if issue.RiskLevel == RiskHigh || issue.RiskLevel == RiskMedium {
			hasWarning = true
		}
	}

	if hasCritical {
		return "critical"
	}
	if hasWarning {
		return "warning"
	}
	return "healthy"
}

// getAIInsights gets AI-powered insights.
func (a *Advisor) getAIInsights(ctx context.Context, result *AdvisorResult) {
	if a.aiClient == nil || len(result.Issues) == 0 {
		return
	}

	// Build context for AI
	var issueDesc strings.Builder
	for _, issue := range result.Issues {
		issueDesc.WriteString(fmt.Sprintf("- %s: %s\n", issue.Title, issue.Description))
	}

	prompt := fmt.Sprintf(`作为运维专家，请分析以下服务器问题并给出简洁建议（不超过100字）：

服务器状态：
- CPU: %.1f%%
- 内存: %.1f%%
- 负载: %.2f

发现的问题：
%s

请给出最重要的1-2条建议。`,
		result.Metrics.CPUUsage,
		result.Metrics.MemoryUsage,
		result.Metrics.LoadAvg[0],
		issueDesc.String())

	// Use AI to get insights (with timeout)
	aiCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Build message for AI
	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: prompt,
		},
	}

	response, err := a.aiClient.Generate(aiCtx, messages)
	if err == nil && response != nil && response.Content != "" {
		result.AIInsights = response.Content
	}
}

// FormatResult formats the advisor result for display.
func FormatResult(result *AdvisorResult) string {
	var sb strings.Builder

	// Header
	sb.WriteString("\n")
	sb.WriteString("╔══════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║            🤖 智能运维助手分析报告                             ║\n")
	sb.WriteString("╚══════════════════════════════════════════════════════════════╝\n")
	sb.WriteString("\n")

	// Health status
	healthIcon := "\033[32m●\033[0m 健康"
	if result.OverallHealth == "critical" {
		healthIcon = "\033[31m●\033[0m 严重"
	} else if result.OverallHealth == "warning" {
		healthIcon = "\033[33m●\033[0m 警告"
	}

	sb.WriteString(fmt.Sprintf("  🖥️  主机: %s\n", result.HostName))
	sb.WriteString(fmt.Sprintf("  📊 状态: %s\n", healthIcon))
	sb.WriteString(fmt.Sprintf("  ⏱️  分析时间: %s\n", result.AnalyzedAt.Format("15:04:05")))
	sb.WriteString(fmt.Sprintf("  🚀 预计节省时间: %d 分钟\n\n", result.TimeSavedMinutes))

	// Resource overview
	if result.Metrics != nil {
		sb.WriteString("  📈 资源概览\n")
		sb.WriteString("  ─────────────────────────────────────────────────\n")
		sb.WriteString(fmt.Sprintf("  CPU: %.1f%%  |  内存: %.1f%%  |  负载: %.2f\n\n",
			result.Metrics.CPUUsage, result.Metrics.MemoryUsage, result.Metrics.LoadAvg[0]))
	}

	// Issues
	if len(result.Issues) > 0 {
		sb.WriteString("  ⚠️  发现的问题\n")
		sb.WriteString("  ─────────────────────────────────────────────────\n")
		for i, issue := range result.Issues {
			levelIcon := getRiskIcon(issue.RiskLevel)
			sb.WriteString(fmt.Sprintf("  %d. %s %s\n", i+1, levelIcon, issue.Title))
			sb.WriteString(fmt.Sprintf("     %s\n", issue.Description))
			if issue.FixCommand != "" {
				sb.WriteString(fmt.Sprintf("     \033[36m💡 诊断命令: %s\033[0m\n", issue.FixCommand))
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("  ✅ 未发现明显问题\n\n")
	}

	// Advice
	if len(result.Advice) > 0 {
		sb.WriteString("  💡 优化建议\n")
		sb.WriteString("  ─────────────────────────────────────────────────\n")
		for i, advice := range result.Advice {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, advice.Title))
			sb.WriteString(fmt.Sprintf("     %s\n", advice.Description))
			if advice.Command != "" {
				sb.WriteString(fmt.Sprintf("     \033[32m执行: %s\033[0m\n", advice.Command))
			}
			sb.WriteString("\n")
		}
	}

	// Quick wins
	if len(result.QuickWins) > 0 {
		sb.WriteString("  ⚡ 快速诊断命令\n")
		sb.WriteString("  ─────────────────────────────────────────────────\n")
		for _, qw := range result.QuickWins {
			sb.WriteString(fmt.Sprintf("  • %s\n", qw))
		}
		sb.WriteString("\n")
	}

	// AI Insights
	if result.AIInsights != "" {
		sb.WriteString("  🧠 AI 洞察\n")
		sb.WriteString("  ─────────────────────────────────────────────────\n")
		sb.WriteString(fmt.Sprintf("  %s\n\n", result.AIInsights))
	}

	return sb.String()
}

// getRiskIcon returns the icon for a risk level.
func getRiskIcon(level RiskLevel) string {
	switch level {
	case RiskCritical:
		return "\033[31m🔴\033[0m"
	case RiskHigh:
		return "\033[31m🟠\033[0m"
	case RiskMedium:
		return "\033[33m🟡\033[0m"
	case RiskLow:
		return "\033[34m🔵\033[0m"
	default:
		return "\033[37m⚪\033[0m"
	}
}
