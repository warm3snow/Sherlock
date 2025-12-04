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

// Package agent provides the AI agent for Sherlock that handles natural language
// processing for SSH operations.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/warm3snow/sherlock/internal/ai"
	"github.com/warm3snow/sherlock/pkg/sshclient"
)

// Agent handles natural language processing for SSH operations.
type Agent struct {
	aiClient            ai.ModelClient
	customShellCommands map[string]bool
}

// NewAgent creates a new Agent with the given AI client.
func NewAgent(aiClient ai.ModelClient) *Agent {
	return &Agent{
		aiClient:            aiClient,
		customShellCommands: make(map[string]bool),
	}
}

// SetCustomShellCommands sets the custom shell commands whitelist.
// These commands will be executed directly without LLM translation.
func (a *Agent) SetCustomShellCommands(commands []string) {
	a.customShellCommands = make(map[string]bool, len(commands))
	for _, cmd := range commands {
		cmd = strings.TrimSpace(strings.ToLower(cmd))
		if cmd != "" {
			a.customShellCommands[cmd] = true
		}
	}
}

const systemPromptConnection = `You are Sherlock, an AI assistant for SSH remote operations.
Your task is to parse natural language requests to connect to remote hosts.
You must support both English and Chinese inputs.

When the user provides connection information, extract:
1. Host: The hostname or IP address
2. Port: The SSH port (default 22 if not specified)
3. User: The username (default "root" if not specified)

Respond in JSON format only:
{
  "host": "hostname or IP",
  "port": 22,
  "user": "username"
}

If you cannot determine the required information, respond with an error:
{
  "error": "description of what's missing"
}

Examples:
- "connect to 192.168.1.100 as root" -> {"host": "192.168.1.100", "port": 22, "user": "root"}
- "ssh user@example.com:2222" -> {"host": "example.com", "port": 2222, "user": "user"}
- "login to server 10.0.0.1 port 2222 as admin" -> {"host": "10.0.0.1", "port": 2222, "user": "admin"}
- "连接192.168.1.100" -> {"host": "192.168.1.100", "port": 22, "user": "root"}
- "连接到192.168.1.100用户admin" -> {"host": "192.168.1.100", "port": 22, "user": "admin"}
- "登录服务器10.0.0.1端口2222用户admin" -> {"host": "10.0.0.1", "port": 2222, "user": "admin"}`

const systemPromptCommand = `You are Sherlock, an AI assistant for SSH remote operations.
Your task is to translate natural language requests into shell commands.

When the user describes what they want to do, generate the appropriate shell command(s).

Respond in JSON format only:
{
  "commands": ["command1", "command2"],
  "description": "brief description of what these commands do",
  "needs_confirm": false
}

Set "needs_confirm" to true for potentially dangerous operations like:
- Deleting files or directories
- Modifying system configuration
- Stopping/restarting services
- Any command that could cause data loss

Examples:
- "show me disk usage" -> {"commands": ["df -h"], "description": "Display disk space usage in human-readable format", "needs_confirm": false}
- "list files in current directory" -> {"commands": ["ls -la"], "description": "List all files including hidden ones with details", "needs_confirm": false}
- "remove the tmp folder" -> {"commands": ["rm -rf tmp"], "description": "Recursively remove the tmp directory and its contents", "needs_confirm": true}
- "restart nginx service" -> {"commands": ["sudo systemctl restart nginx"], "description": "Restart the nginx service", "needs_confirm": true}`

// ConnectionInfo represents parsed connection information.
type ConnectionInfo struct {
	Host  string `json:"host"`
	Port  int    `json:"port"`
	User  string `json:"user"`
	Error string `json:"error,omitempty"`
}

// CommandInfo represents parsed command information.
type CommandInfo struct {
	Commands     []string `json:"commands"`
	Description  string   `json:"description"`
	NeedsConfirm bool     `json:"needs_confirm"`
	Error        string   `json:"error,omitempty"`
}

// ParseConnectionRequest parses a natural language connection request.
func (a *Agent) ParseConnectionRequest(ctx context.Context, request string) (*ConnectionInfo, error) {
	// First try to parse common patterns directly
	if info := parseConnectionDirect(request); info != nil {
		return info, nil
	}

	// Fall back to AI parsing
	messages := []*schema.Message{
		schema.SystemMessage(systemPromptConnection),
		schema.UserMessage(request),
	}

	response, err := a.aiClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to generate response: %w", err)
	}

	content := strings.TrimSpace(response.Content)
	content = extractJSON(content)

	var info ConnectionInfo
	if err := json.Unmarshal([]byte(content), &info); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if info.Error != "" {
		return nil, fmt.Errorf("connection parse error: %s", info.Error)
	}

	if info.Port == 0 {
		info.Port = 22
	}

	return &info, nil
}

// parseConnectionDirect tries to parse common connection patterns directly.
func parseConnectionDirect(request string) *ConnectionInfo {
	// Pattern: user@host:port
	userHostPortRe := regexp.MustCompile(`([a-zA-Z0-9_-]+)@([a-zA-Z0-9.-]+):(\d+)`)
	if matches := userHostPortRe.FindStringSubmatch(request); len(matches) == 4 {
		port, _ := strconv.Atoi(matches[3])
		return &ConnectionInfo{
			User: matches[1],
			Host: matches[2],
			Port: port,
		}
	}

	// Pattern: user@host
	userHostRe := regexp.MustCompile(`([a-zA-Z0-9_-]+)@([a-zA-Z0-9.-]+)`)
	if matches := userHostRe.FindStringSubmatch(request); len(matches) == 3 {
		return &ConnectionInfo{
			User: matches[1],
			Host: matches[2],
			Port: 22,
		}
	}

	// Pattern: just an IP address (e.g., "connect 192.168.40.22" or "连接192.168.40.22")
	// Default user is "root"
	ipPattern := regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
	if matches := ipPattern.FindStringSubmatch(request); len(matches) == 2 {
		// Validate that the IP is actually valid
		if net.ParseIP(matches[1]) != nil {
			return &ConnectionInfo{
				Host: matches[1],
				Port: 22,
				User: "root",
			}
		}
	}

	return nil
}

// commonShellCommandsMap is a map for O(1) lookup of common shell commands.
var commonShellCommandsMap = func() map[string]bool {
	commands := []string{
		// File and directory operations
		"ls", "cd", "pwd", "mkdir", "rmdir", "rm", "cp", "mv",
		"touch", "cat", "head", "tail", "less", "more", "find", "locate", "tree",
		"ln", "file", "stat", "du", "df", "mount", "umount",
		// Text processing
		"grep", "awk", "sed", "cut", "sort", "uniq", "wc", "tr", "diff", "comm",
		"xargs", "tee",
		// System information
		"uname", "hostname", "uptime", "date", "cal", "who", "w", "id", "whoami",
		"last", "lastlog", "free", "top", "htop", "vmstat", "iostat", "sar",
		"lscpu", "lsmem", "lsblk", "lspci", "lsusb", "dmesg", "journalctl",
		// Process management
		"ps", "kill", "killall", "pkill", "pgrep", "nice", "renice", "nohup",
		"jobs", "bg", "fg", "disown",
		// Network
		"ping", "traceroute", "tracepath", "netstat", "ss", "ip", "ifconfig",
		"route", "arp", "dig", "nslookup", "host", "wget", "curl", "nc", "telnet",
		"ssh", "scp", "rsync", "ftp", "sftp", "iptables", "nft", "firewall-cmd",
		// Package management
		"apt", "apt-get", "dpkg", "yum", "dnf", "rpm", "pacman", "zypper", "brew",
		"pip", "pip3", "npm", "yarn", "gem", "cargo", "go",
		// Service management
		"systemctl", "service", "chkconfig", "update-rc.d",
		// User and permission management
		"useradd", "userdel", "usermod", "groupadd", "groupdel", "groupmod",
		"passwd", "chown", "chmod", "chgrp", "sudo", "su",
		// Archive and compression
		"tar", "gzip", "gunzip", "zip", "unzip", "bzip2", "xz", "7z",
		// Disk and filesystem
		"fdisk", "parted", "mkfs", "fsck", "dd", "sync",
		// Environment
		"env", "export", "set", "unset", "source", "alias", "unalias", "echo",
		"printf", "read", "test",
		// Editors and viewers
		"vi", "vim", "nano", "emacs", "ed",
		// Other utilities
		"man", "info", "which", "whereis", "type", "clear",
		"reset", "shutdown", "reboot", "halt", "poweroff",
		"sleep", "watch", "timeout", "time", "seq", "yes", "true", "false",
		// Docker and container
		"docker", "docker-compose", "podman", "kubectl", "crictl",
		// Version control
		"git", "svn", "hg",
	}
	m := make(map[string]bool, len(commands))
	for _, cmd := range commands {
		m[cmd] = true
	}
	return m
}()

// dangerousCommandsMap is a map for O(1) lookup of dangerous commands.
var dangerousCommandsMap = func() map[string]bool {
	commands := []string{
		// File operations that may cause data loss
		"rm", "rmdir", "mv", "dd",
		// Permission changes
		"chmod", "chown", "chgrp",
		// System operations
		"shutdown", "reboot", "halt", "poweroff",
		"systemctl", "service",
		// Elevated privileges
		"sudo", "su",
		// Disk operations
		"fdisk", "parted", "mkfs", "fsck",
		// Package installation/removal (may modify system)
		"apt", "apt-get", "dpkg", "yum", "dnf", "rpm", "pacman", "zypper",
		// Network configuration
		"iptables", "nft", "firewall-cmd",
		// User management
		"useradd", "userdel", "usermod", "groupadd", "groupdel", "groupmod", "passwd",
	}
	m := make(map[string]bool, len(commands))
	for _, cmd := range commands {
		m[cmd] = true
	}
	return m
}()

// isDangerousCommand checks if the command is potentially dangerous
// and should require user confirmation.
func isDangerousCommand(input string) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return false
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false
	}
	cmdName := strings.ToLower(parts[0])

	// O(1) lookup using map
	return dangerousCommandsMap[cmdName]
}

// IsShellCommand checks if the input looks like a common shell command.
// It returns true if the input starts with a known command prefix or is in the custom whitelist.
func (a *Agent) IsShellCommand(input string) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return false
	}

	// Get the first word (command name)
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false
	}
	cmdName := strings.ToLower(parts[0])

	// Check custom whitelist first (O(1) lookup)
	if a.customShellCommands[cmdName] {
		return true
	}

	// O(1) lookup using built-in map
	if commonShellCommandsMap[cmdName] {
		return true
	}

	// Check for commands with path prefix (e.g., /usr/bin/ls, ./script.sh)
	// This allows users to run local scripts directly without LLM translation
	if strings.HasPrefix(input, "/") || strings.HasPrefix(input, "./") || strings.HasPrefix(input, "../") {
		return true
	}

	return false
}

// parseCommandDirect handles commands that can be executed directly without LLM.
func (a *Agent) parseCommandDirect(request string) *CommandInfo {
	cmd := strings.TrimSpace(request)
	if cmd == "" {
		return nil
	}

	// Check if it's a shell command
	if a.IsShellCommand(cmd) {
		// Generate a more descriptive message based on the command
		parts := strings.Fields(cmd)
		cmdName := parts[0]
		description := fmt.Sprintf("Execute: %s", cmdName)

		return &CommandInfo{
			Commands:     []string{cmd},
			Description:  description,
			NeedsConfirm: isDangerousCommand(cmd),
		}
	}

	return nil
}

// ParseCommandRequest parses a natural language command request.
func (a *Agent) ParseCommandRequest(ctx context.Context, request string) (*CommandInfo, error) {
	// Check for direct command execution with $ prefix
	if strings.HasPrefix(strings.TrimSpace(request), "$") {
		cmd := strings.TrimPrefix(strings.TrimSpace(request), "$")
		cmd = strings.TrimSpace(cmd)
		return &CommandInfo{
			Commands:     []string{cmd},
			Description:  "Direct command execution",
			NeedsConfirm: false,
		}, nil
	}

	// Check if it's a common shell command that can be executed directly
	if info := a.parseCommandDirect(request); info != nil {
		return info, nil
	}

	// Fall back to AI parsing for natural language requests
	messages := []*schema.Message{
		schema.SystemMessage(systemPromptCommand),
		schema.UserMessage(request),
	}

	response, err := a.aiClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to generate response: %w", err)
	}

	content := strings.TrimSpace(response.Content)
	content = extractJSON(content)

	var info CommandInfo
	if err := json.Unmarshal([]byte(content), &info); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if info.Error != "" {
		return nil, fmt.Errorf("command parse error: %s", info.Error)
	}

	return &info, nil
}

// extractJSON extracts JSON from a response that may contain markdown code blocks.
func extractJSON(content string) string {
	// Try to extract JSON from markdown code blocks
	jsonBlockRe := regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)```")
	if matches := jsonBlockRe.FindStringSubmatch(content); len(matches) == 2 {
		return strings.TrimSpace(matches[1])
	}

	// Try to find JSON object directly
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		return content[start : end+1]
	}

	return content
}

// ToHostInfo converts ConnectionInfo to sshclient.HostInfo.
func (c *ConnectionInfo) ToHostInfo() *sshclient.HostInfo {
	return &sshclient.HostInfo{
		Host: c.Host,
		Port: c.Port,
		User: c.User,
	}
}
