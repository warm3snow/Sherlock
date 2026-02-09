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

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// SSHExecutor interface for SSH command execution.
type SSHExecutor interface {
	Execute(ctx context.Context, command string) *ExecuteResult
	IsConnected() bool
	HostInfoString() string
}

// ExecuteResult represents the result of SSH command execution.
type ExecuteResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Error    error
}

// ToolRegistry manages available tools for the AI agent.
type ToolRegistry struct {
	tools       map[string]tool.InvokableTool
	executor    SSHExecutor
	confirmFunc func(string) bool // Function to request user confirmation
}

// NewToolRegistry creates a new tool registry.
func NewToolRegistry(executor SSHExecutor) *ToolRegistry {
	registry := &ToolRegistry{
		tools:    make(map[string]tool.InvokableTool),
		executor: executor,
	}

	// Register default tools
	registry.registerDefaultTools()

	return registry
}

// SetConfirmFunc sets the confirmation function for dangerous operations.
func (r *ToolRegistry) SetConfirmFunc(fn func(string) bool) {
	r.confirmFunc = fn
}

// SetExecutor updates the SSH executor (e.g., when connecting to a new host).
func (r *ToolRegistry) SetExecutor(executor SSHExecutor) {
	r.executor = executor
}

// registerDefaultTools registers the default SSH tools.
func (r *ToolRegistry) registerDefaultTools() {
	r.tools["execute_command"] = &ExecuteCommandTool{registry: r}
	r.tools["read_file"] = &ReadFileTool{registry: r}
	r.tools["write_file"] = &WriteFileTool{registry: r}
	r.tools["check_service"] = &CheckServiceTool{registry: r}
	r.tools["list_directory"] = &ListDirectoryTool{registry: r}
	r.tools["find_files"] = &FindFilesTool{registry: r}
	r.tools["check_resource"] = &CheckResourceTool{registry: r}
	r.tools["manage_service"] = &ManageServiceTool{registry: r}
}

// GetTool returns a tool by name.
func (r *ToolRegistry) GetTool(name string) (tool.InvokableTool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// GetAllTools returns all registered tools.
func (r *ToolRegistry) GetAllTools() []tool.InvokableTool {
	tools := make([]tool.InvokableTool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// GetToolInfos returns ToolInfo for all registered tools.
func (r *ToolRegistry) GetToolInfos(ctx context.Context) ([]*schema.ToolInfo, error) {
	infos := make([]*schema.ToolInfo, 0, len(r.tools))
	for _, t := range r.tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// ExecuteTool executes a tool by name with JSON arguments.
func (r *ToolRegistry) ExecuteTool(ctx context.Context, name, argsJSON string) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return t.InvokableRun(ctx, argsJSON)
}

// ============================================================
// Tool Implementations
// ============================================================

// ExecuteCommandTool executes shell commands on the remote host.
type ExecuteCommandTool struct {
	registry *ToolRegistry
}

func (t *ExecuteCommandTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "execute_command",
		Desc: "Execute a shell command on the remote host. Use this for any command that needs to run on the target system.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command": {
				Type:     schema.String,
				Desc:     "The shell command to execute",
				Required: true,
			},
			"timeout": {
				Type:     schema.Integer,
				Desc:     "Timeout in seconds (default: 60)",
				Required: false,
			},
		}),
	}, nil
}

func (t *ExecuteCommandTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if t.registry.executor == nil || !t.registry.executor.IsConnected() {
		return "", fmt.Errorf("not connected to any host")
	}

	// Check for dangerous commands
	if isDangerousCommand(args.Command) {
		if t.registry.confirmFunc != nil && !t.registry.confirmFunc(args.Command) {
			return "Command execution cancelled by user", nil
		}
	}

	result := t.registry.executor.Execute(ctx, args.Command)

	output := map[string]interface{}{
		"stdout":    result.Stdout,
		"stderr":    result.Stderr,
		"exit_code": result.ExitCode,
		"success":   result.ExitCode == 0,
	}

	if result.Error != nil {
		output["error"] = result.Error.Error()
	}

	jsonOutput, _ := json.Marshal(output)
	return string(jsonOutput), nil
}

// ReadFileTool reads file contents from the remote host.
type ReadFileTool struct {
	registry *ToolRegistry
}

func (t *ReadFileTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_file",
		Desc: "Read the contents of a file from the remote host. Useful for viewing configuration files, logs, etc.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {
				Type:     schema.String,
				Desc:     "The file path to read",
				Required: true,
			},
			"lines": {
				Type:     schema.Integer,
				Desc:     "Number of lines to read (0 for all, negative for last N lines)",
				Required: false,
			},
		}),
	}, nil
}

func (t *ReadFileTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		Path  string `json:"path"`
		Lines int    `json:"lines"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if t.registry.executor == nil || !t.registry.executor.IsConnected() {
		return "", fmt.Errorf("not connected to any host")
	}

	var cmd string
	switch {
	case args.Lines > 0:
		cmd = fmt.Sprintf("head -n %d '%s'", args.Lines, args.Path)
	case args.Lines < 0:
		cmd = fmt.Sprintf("tail -n %d '%s'", -args.Lines, args.Path)
	default:
		cmd = fmt.Sprintf("cat '%s'", args.Path)
	}

	result := t.registry.executor.Execute(ctx, cmd)

	if result.ExitCode != 0 {
		return fmt.Sprintf("Error reading file: %s", result.Stderr), nil
	}

	return result.Stdout, nil
}

// WriteFileTool writes content to a file on the remote host.
type WriteFileTool struct {
	registry *ToolRegistry
}

func (t *WriteFileTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "write_file",
		Desc: "Write content to a file on the remote host. CAUTION: This will overwrite existing content unless append mode is used.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {
				Type:     schema.String,
				Desc:     "The file path to write to",
				Required: true,
			},
			"content": {
				Type:     schema.String,
				Desc:     "The content to write",
				Required: true,
			},
			"append": {
				Type:     schema.Boolean,
				Desc:     "If true, append to file instead of overwriting",
				Required: false,
			},
		}),
	}, nil
}

func (t *WriteFileTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Append  bool   `json:"append"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if t.registry.executor == nil || !t.registry.executor.IsConnected() {
		return "", fmt.Errorf("not connected to any host")
	}

	// Request confirmation for file writes
	if t.registry.confirmFunc != nil && !t.registry.confirmFunc(fmt.Sprintf("Write to file: %s", args.Path)) {
		return "File write cancelled by user", nil
	}

	operator := ">"
	if args.Append {
		operator = ">>"
	}

	// Escape content for shell
	escapedContent := strings.ReplaceAll(args.Content, "'", "'\\''")
	cmd := fmt.Sprintf("echo '%s' %s '%s'", escapedContent, operator, args.Path)

	result := t.registry.executor.Execute(ctx, cmd)

	if result.ExitCode != 0 {
		return fmt.Sprintf("Error writing file: %s", result.Stderr), nil
	}

	return fmt.Sprintf("Successfully wrote to %s", args.Path), nil
}

// CheckServiceTool checks the status of a system service.
type CheckServiceTool struct {
	registry *ToolRegistry
}

func (t *CheckServiceTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "check_service",
		Desc: "Check the status of a system service (systemd/init). Returns whether the service is running and its details.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"service_name": {
				Type:     schema.String,
				Desc:     "The name of the service to check (e.g., nginx, mysql, docker)",
				Required: true,
			},
		}),
	}, nil
}

func (t *CheckServiceTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		ServiceName string `json:"service_name"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if t.registry.executor == nil || !t.registry.executor.IsConnected() {
		return "", fmt.Errorf("not connected to any host")
	}

	cmd := fmt.Sprintf("systemctl status %s 2>&1 || service %s status 2>&1", args.ServiceName, args.ServiceName)
	result := t.registry.executor.Execute(ctx, cmd)

	status := "unknown"
	if strings.Contains(result.Stdout, "active (running)") || strings.Contains(result.Stdout, "is running") {
		status = "running"
	} else if strings.Contains(result.Stdout, "inactive") || strings.Contains(result.Stdout, "stopped") {
		status = "stopped"
	} else if strings.Contains(result.Stdout, "failed") {
		status = "failed"
	}

	output := map[string]interface{}{
		"service": args.ServiceName,
		"status":  status,
		"details": truncateForTool(result.Stdout, 500),
	}

	jsonOutput, _ := json.Marshal(output)
	return string(jsonOutput), nil
}

// ListDirectoryTool lists directory contents.
type ListDirectoryTool struct {
	registry *ToolRegistry
}

func (t *ListDirectoryTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "list_directory",
		Desc: "List the contents of a directory with file details (permissions, size, date).",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {
				Type:     schema.String,
				Desc:     "The directory path to list (default: current directory)",
				Required: false,
			},
			"all": {
				Type:     schema.Boolean,
				Desc:     "If true, show hidden files (starting with .)",
				Required: false,
			},
		}),
	}, nil
}

func (t *ListDirectoryTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		Path string `json:"path"`
		All  bool   `json:"all"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if t.registry.executor == nil || !t.registry.executor.IsConnected() {
		return "", fmt.Errorf("not connected to any host")
	}

	path := args.Path
	if path == "" {
		path = "."
	}

	flags := "-lh"
	if args.All {
		flags = "-lah"
	}

	cmd := fmt.Sprintf("ls %s '%s'", flags, path)
	result := t.registry.executor.Execute(ctx, cmd)

	if result.ExitCode != 0 {
		return fmt.Sprintf("Error listing directory: %s", result.Stderr), nil
	}

	return result.Stdout, nil
}

// FindFilesTool finds files matching criteria.
type FindFilesTool struct {
	registry *ToolRegistry
}

func (t *FindFilesTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "find_files",
		Desc: "Find files matching specific criteria (name pattern, size, type, modification time).",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {
				Type:     schema.String,
				Desc:     "The directory to search in (default: current directory)",
				Required: false,
			},
			"name": {
				Type:     schema.String,
				Desc:     "File name pattern (e.g., '*.log', 'config.*')",
				Required: false,
			},
			"type": {
				Type:     schema.String,
				Desc:     "File type: 'f' for files, 'd' for directories, 'l' for links",
				Required: false,
			},
			"size": {
				Type:     schema.String,
				Desc:     "Size filter (e.g., '+1G' for larger than 1GB, '-100M' for smaller than 100MB)",
				Required: false,
			},
			"mtime": {
				Type:     schema.String,
				Desc:     "Modification time (e.g., '-7' for last 7 days, '+30' for older than 30 days)",
				Required: false,
			},
			"maxdepth": {
				Type:     schema.Integer,
				Desc:     "Maximum directory depth to search",
				Required: false,
			},
		}),
	}, nil
}

func (t *FindFilesTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		Path     string `json:"path"`
		Name     string `json:"name"`
		Type     string `json:"type"`
		Size     string `json:"size"`
		Mtime    string `json:"mtime"`
		MaxDepth int    `json:"maxdepth"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if t.registry.executor == nil || !t.registry.executor.IsConnected() {
		return "", fmt.Errorf("not connected to any host")
	}

	path := args.Path
	if path == "" {
		path = "."
	}

	var cmdParts []string
	cmdParts = append(cmdParts, "find", fmt.Sprintf("'%s'", path))

	if args.MaxDepth > 0 {
		cmdParts = append(cmdParts, "-maxdepth", fmt.Sprintf("%d", args.MaxDepth))
	}

	if args.Type != "" {
		cmdParts = append(cmdParts, "-type", args.Type)
	}

	if args.Name != "" {
		cmdParts = append(cmdParts, "-name", fmt.Sprintf("'%s'", args.Name))
	}

	if args.Size != "" {
		cmdParts = append(cmdParts, "-size", args.Size)
	}

	if args.Mtime != "" {
		cmdParts = append(cmdParts, "-mtime", args.Mtime)
	}

	cmdParts = append(cmdParts, "2>/dev/null")

	cmd := strings.Join(cmdParts, " ")
	result := t.registry.executor.Execute(ctx, cmd)

	if result.Stdout == "" && result.ExitCode == 0 {
		return "No files found matching the criteria", nil
	}

	return result.Stdout, nil
}

// CheckResourceTool checks system resource usage.
type CheckResourceTool struct {
	registry *ToolRegistry
}

func (t *CheckResourceTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "check_resource",
		Desc: "Check system resource usage (CPU, memory, disk, network).",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"resource": {
				Type:     schema.String,
				Desc:     "Resource type: 'cpu', 'memory', 'disk', 'network', 'all'",
				Required: true,
				Enum:     []string{"cpu", "memory", "disk", "network", "all"},
			},
		}),
	}, nil
}

func (t *CheckResourceTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		Resource string `json:"resource"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if t.registry.executor == nil || !t.registry.executor.IsConnected() {
		return "", fmt.Errorf("not connected to any host")
	}

	var cmd string
	switch args.Resource {
	case "cpu":
		cmd = "top -bn1 | head -5; echo '---'; mpstat 2>&1 || cat /proc/loadavg"
	case "memory":
		cmd = "free -h"
	case "disk":
		cmd = "df -h"
	case "network":
		cmd = "netstat -tuln 2>/dev/null || ss -tuln"
	case "all":
		cmd = "echo '=== CPU ===' && uptime && echo '' && echo '=== MEMORY ===' && free -h && echo '' && echo '=== DISK ===' && df -h"
	default:
		return "", fmt.Errorf("unknown resource type: %s", args.Resource)
	}

	result := t.registry.executor.Execute(ctx, cmd)
	return result.Stdout, nil
}

// ManageServiceTool manages system services.
type ManageServiceTool struct {
	registry *ToolRegistry
}

func (t *ManageServiceTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "manage_service",
		Desc: "Manage system services (start, stop, restart, enable, disable). CAUTION: These operations affect system services.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"service_name": {
				Type:     schema.String,
				Desc:     "The name of the service",
				Required: true,
			},
			"action": {
				Type:     schema.String,
				Desc:     "Action to perform: 'start', 'stop', 'restart', 'enable', 'disable'",
				Required: true,
				Enum:     []string{"start", "stop", "restart", "enable", "disable"},
			},
		}),
	}, nil
}

func (t *ManageServiceTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		ServiceName string `json:"service_name"`
		Action      string `json:"action"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if t.registry.executor == nil || !t.registry.executor.IsConnected() {
		return "", fmt.Errorf("not connected to any host")
	}

	// Always require confirmation for service management
	confirmMsg := fmt.Sprintf("%s service: %s", args.Action, args.ServiceName)
	if t.registry.confirmFunc != nil && !t.registry.confirmFunc(confirmMsg) {
		return "Service operation cancelled by user", nil
	}

	cmd := fmt.Sprintf("sudo systemctl %s %s 2>&1 || sudo service %s %s 2>&1",
		args.Action, args.ServiceName, args.ServiceName, args.Action)

	result := t.registry.executor.Execute(ctx, cmd)

	if result.ExitCode != 0 {
		return fmt.Sprintf("Failed to %s service %s: %s", args.Action, args.ServiceName, result.Stderr), nil
	}

	return fmt.Sprintf("Successfully %sed service: %s", args.Action, args.ServiceName), nil
}

// truncateForTool truncates output for tool responses.
func truncateForTool(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}

// Verify interface compliance
var (
	_ tool.InvokableTool = (*ExecuteCommandTool)(nil)
	_ tool.InvokableTool = (*ReadFileTool)(nil)
	_ tool.InvokableTool = (*WriteFileTool)(nil)
	_ tool.InvokableTool = (*CheckServiceTool)(nil)
	_ tool.InvokableTool = (*ListDirectoryTool)(nil)
	_ tool.InvokableTool = (*FindFilesTool)(nil)
	_ tool.InvokableTool = (*CheckResourceTool)(nil)
	_ tool.InvokableTool = (*ManageServiceTool)(nil)
)
