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

const systemPromptSmartConnection = `You are Sherlock, an AI assistant for remote connection management.
Your task is to analyze user input and determine the connection type: SSH, MySQL database, or Redis cache.

Analyze the input and determine:
1. type: "ssh", "database", or "cache"
2. connection details based on the type

Key indicators for each type:
- SSH: keywords like "ssh", "connect", "server", "主机", common SSH ports (22, 2222)
- Database (MySQL): keywords like "mysql", "database", "db", "数据库", flags like "-u", "-p", "-P", common MySQL ports (3306, 30505, 33060)
- Cache (Redis): keywords like "redis", "cache", "缓存", common Redis ports (6379, 16379, 26379)

IMPORTANT port hints:
- Port 22, 2222: Usually SSH
- Port 3306, 33060, 30505: Usually MySQL database
- Port 6379, 16379, 26379: Usually Redis cache
- If the command looks like "mysql -h host -P port -u user -p password" format, it's a database connection

Respond in JSON format only:
{
  "type": "ssh|database|cache",
  "host": "hostname or IP",
  "port": number,
  "user": "username (for ssh/database)",
  "password": "password if provided",
  "database_name": "database name (for database type)"
}

Examples:
- "connect to 192.168.1.1" -> {"type": "ssh", "host": "192.168.1.1", "port": 22, "user": "root"}
- "ssh root@192.168.1.1:2222" -> {"type": "ssh", "host": "192.168.1.1", "port": 2222, "user": "root"}
- "mysql -h 192.168.1.1 -P3306 -uroot -ppassword" -> {"type": "database", "host": "192.168.1.1", "port": 3306, "user": "root", "password": "password"}
- "登录 mysql 数据库 192.168.1.1 -P30505 -uroot -proot123" -> {"type": "database", "host": "192.168.1.1", "port": 30505, "user": "root", "password": "root123"}
- "连接数据库 root@192.168.1.1:3306/testdb" -> {"type": "database", "host": "192.168.1.1", "port": 3306, "user": "root", "database_name": "testdb"}
- "redis-cli -h 192.168.1.1 -p 6379" -> {"type": "cache", "host": "192.168.1.1", "port": 6379}
- "连接 redis 192.168.1.1:6379" -> {"type": "cache", "host": "192.168.1.1", "port": 6379}
- "登录缓存 192.168.1.1" -> {"type": "cache", "host": "192.168.1.1", "port": 6379}`

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

// ConnectionType represents the type of connection (SSH, Database, Cache).
type ConnectionType string

const (
	ConnectionTypeSSH      ConnectionType = "ssh"
	ConnectionTypeDatabase ConnectionType = "database"
	ConnectionTypeCache    ConnectionType = "cache"
	ConnectionTypeUnknown  ConnectionType = "unknown"
)

// ConnectionInfo represents parsed connection information.
type ConnectionInfo struct {
	Host  string `json:"host"`
	Port  int    `json:"port"`
	User  string `json:"user"`
	Error string `json:"error,omitempty"`
}

// SmartConnectionInfo represents parsed connection information with type detection.
type SmartConnectionInfo struct {
	Type         ConnectionType `json:"type"`
	Host         string         `json:"host"`
	Port         int            `json:"port"`
	User         string         `json:"user,omitempty"`
	Password     string         `json:"password,omitempty"`
	DatabaseName string         `json:"database_name,omitempty"`
	Error        string         `json:"error,omitempty"`
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

// parseSmartConnectionDirect tries to parse connection patterns directly without LLM.
func parseSmartConnectionDirect(request string) *SmartConnectionInfo {
	lower := strings.ToLower(request)

	// Detect connection type from keywords
	isDatabase := strings.Contains(lower, "mysql") || strings.Contains(lower, "数据库") ||
		strings.Contains(lower, "database") || strings.Contains(lower, "db ")
	isCache := strings.Contains(lower, "redis") || strings.Contains(lower, "缓存") ||
		strings.Contains(lower, "cache")

	// Parse MySQL command line format: mysql -h host -P port -u user -p password
	if isDatabase || strings.Contains(request, "-P") {
		info := parseMySQLFormat(request)
		if info != nil {
			return info
		}
	}

	// Parse Redis command line format: redis-cli -h host -p port
	if isCache {
		info := parseRedisFormat(request)
		if info != nil {
			return info
		}
	}

	// Parse database connection string: user@host:port/dbname
	if isDatabase {
		info := parseDatabaseConnString(request)
		if info != nil {
			return info
		}
	}

	// Parse cache connection string: host:port
	if isCache {
		info := parseCacheConnString(request)
		if info != nil {
			return info
		}
	}

	return nil
}

// parseMySQLFormat parses MySQL command line format.
func parseMySQLFormat(request string) *SmartConnectionInfo {
	info := &SmartConnectionInfo{
		Type: ConnectionTypeDatabase,
		Port: 3306,
		User: "root",
	}

	// Extract host: -h host or -hhost
	hostRe := regexp.MustCompile(`-h\s*([a-zA-Z0-9.-]+)`)
	if matches := hostRe.FindStringSubmatch(request); len(matches) == 2 {
		info.Host = matches[1]
	}

	// Extract port: -P port or -Pport
	portRe := regexp.MustCompile(`-P\s*(\d+)`)
	if matches := portRe.FindStringSubmatch(request); len(matches) == 2 {
		if port, err := strconv.Atoi(matches[1]); err == nil {
			info.Port = port
		}
	}

	// Extract user: -u user or -uuser
	userRe := regexp.MustCompile(`-u\s*([a-zA-Z0-9_-]+)`)
	if matches := userRe.FindStringSubmatch(request); len(matches) == 2 {
		info.User = matches[1]
	}

	// Extract password: -p password or -ppassword (but not just -p)
	passRe := regexp.MustCompile(`-p([a-zA-Z0-9_!@#$%^&*()-]+)`)
	if matches := passRe.FindStringSubmatch(request); len(matches) == 2 {
		info.Password = matches[1]
	}

	// If no host found via -h, try to find IP address directly
	if info.Host == "" {
		ipPattern := regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
		if matches := ipPattern.FindStringSubmatch(request); len(matches) == 2 {
			if net.ParseIP(matches[1]) != nil {
				info.Host = matches[1]
			}
		}
	}

	if info.Host == "" {
		return nil
	}

	return info
}

// parseRedisFormat parses Redis command line format.
func parseRedisFormat(request string) *SmartConnectionInfo {
	info := &SmartConnectionInfo{
		Type: ConnectionTypeCache,
		Port: 6379,
	}

	// Extract host: -h host
	hostRe := regexp.MustCompile(`-h\s*([a-zA-Z0-9.-]+)`)
	if matches := hostRe.FindStringSubmatch(request); len(matches) == 2 {
		info.Host = matches[1]
	}

	// Extract port: -p port (lowercase p for redis)
	portRe := regexp.MustCompile(`-p\s*(\d+)`)
	if matches := portRe.FindStringSubmatch(request); len(matches) == 2 {
		if port, err := strconv.Atoi(matches[1]); err == nil {
			info.Port = port
		}
	}

	// Extract password: -a password
	passRe := regexp.MustCompile(`-a\s*([a-zA-Z0-9_!@#$%^&*()-]+)`)
	if matches := passRe.FindStringSubmatch(request); len(matches) == 2 {
		info.Password = matches[1]
	}

	// If no host found, try IP address
	if info.Host == "" {
		ipPattern := regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
		if matches := ipPattern.FindStringSubmatch(request); len(matches) == 2 {
			if net.ParseIP(matches[1]) != nil {
				info.Host = matches[1]
			}
		}
	}

	if info.Host == "" {
		return nil
	}

	return info
}

// parseDatabaseConnString parses database connection string format: user@host:port/dbname
func parseDatabaseConnString(request string) *SmartConnectionInfo {
	// Pattern: user@host:port/dbname or user@host/dbname or host:port/dbname
	re := regexp.MustCompile(`([a-zA-Z0-9_-]+)?@?(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}|[a-zA-Z0-9.-]+):?(\d+)?/?([a-zA-Z0-9_-]+)?`)
	matches := re.FindStringSubmatch(request)
	if len(matches) < 3 {
		return nil
	}

	info := &SmartConnectionInfo{
		Type: ConnectionTypeDatabase,
		Port: 3306,
		User: "root",
	}

	if matches[1] != "" {
		info.User = matches[1]
	}
	info.Host = matches[2]
	if matches[3] != "" {
		if port, err := strconv.Atoi(matches[3]); err == nil {
			info.Port = port
		}
	}
	if len(matches) > 4 && matches[4] != "" {
		info.DatabaseName = matches[4]
	}

	// Validate host is an IP or hostname
	if net.ParseIP(info.Host) == nil && !strings.Contains(info.Host, ".") {
		return nil
	}

	return info
}

// parseCacheConnString parses cache connection string format: host:port
func parseCacheConnString(request string) *SmartConnectionInfo {
	// Pattern: host:port
	re := regexp.MustCompile(`(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}|[a-zA-Z0-9.-]+):(\d+)`)
	matches := re.FindStringSubmatch(request)

	info := &SmartConnectionInfo{
		Type: ConnectionTypeCache,
		Port: 6379,
	}

	if len(matches) == 3 {
		info.Host = matches[1]
		if port, err := strconv.Atoi(matches[2]); err == nil {
			info.Port = port
		}
	} else {
		// Try just IP address
		ipPattern := regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
		if ipMatches := ipPattern.FindStringSubmatch(request); len(ipMatches) == 2 {
			if net.ParseIP(ipMatches[1]) != nil {
				info.Host = ipMatches[1]
			}
		}
	}

	if info.Host == "" {
		return nil
	}

	return info
}

// ParseSmartConnectionRequest analyzes user input and determines the connection type.
func (a *Agent) ParseSmartConnectionRequest(ctx context.Context, request string) (*SmartConnectionInfo, error) {
	// First try to parse common patterns directly
	if info := parseSmartConnectionDirect(request); info != nil {
		return info, nil
	}

	// Fall back to AI parsing
	messages := []*schema.Message{
		schema.SystemMessage(systemPromptSmartConnection),
		schema.UserMessage(request),
	}

	response, err := a.aiClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to generate response: %w", err)
	}

	content := strings.TrimSpace(response.Content)
	content = extractJSON(content)

	var info SmartConnectionInfo
	if err := json.Unmarshal([]byte(content), &info); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if info.Error != "" {
		return nil, fmt.Errorf("connection parse error: %s", info.Error)
	}

	// Set default ports based on type
	if info.Port == 0 {
		switch info.Type {
		case ConnectionTypeSSH:
			info.Port = 22
		case ConnectionTypeDatabase:
			info.Port = 3306
		case ConnectionTypeCache:
			info.Port = 6379
		}
	}

	// Set default user for SSH
	if info.Type == ConnectionTypeSSH && info.User == "" {
		info.User = "root"
	}

	return &info, nil
}

// SherlockCommandInfo represents parsed sherlock internal command.
type SherlockCommandInfo struct {
	Command string   `json:"command"` // The sherlock command: host, db, cache, connect, etc.
	Action  string   `json:"action"`  // The action: list, add, delete, show, connect, etc.
	Args    []string `json:"args"`    // Additional arguments
	Error   string   `json:"error,omitempty"`
}

const systemPromptSherlockCommand = `You are Sherlock, an AI assistant for remote connection management.
Your task is to parse natural language requests into Sherlock internal commands.

Available Sherlock commands:
1. host - Manage SSH hosts (list, add, delete, show, edit, group, tag)
2. db - Manage MySQL database connections (list, add, delete, show, connect, exec)
3. cache - Manage Redis cache connections (list, add, delete, show, connect)
4. connect <id> - Connect to a saved host by ID
5. batch - Execute commands on multiple hosts
6. check - Check host connectivity
7. upload/download - File transfer

Parse the user's natural language input and determine:
1. command: The main command (host, db, cache, connect, batch, check, upload, download)
2. action: The specific action (list, add, delete, show, connect, exec, group, tag)
3. args: Additional arguments as an array

Respond in JSON format only:
{
  "command": "host",
  "action": "list",
  "args": []
}

Examples:
- "show all hosts" -> {"command": "host", "action": "list", "args": []}
- "list database connections" -> {"command": "db", "action": "list", "args": []}
- "add host root@192.168.1.1:22" -> {"command": "host", "action": "add", "args": ["root@192.168.1.1:22"]}
- "delete host 1" -> {"command": "host", "action": "delete", "args": ["1"]}
- "connect to database 2" -> {"command": "db", "action": "connect", "args": ["2"]}
- "show redis connections" -> {"command": "cache", "action": "list", "args": []}
- "查看主机列表" -> {"command": "host", "action": "list", "args": []}
- "显示数据库连接" -> {"command": "db", "action": "list", "args": []}
- "添加主机 admin@10.0.0.1" -> {"command": "host", "action": "add", "args": ["admin@10.0.0.1"]}
- "删除数据库连接 3" -> {"command": "db", "action": "delete", "args": ["3"]}
- "连接数据库 1" -> {"command": "db", "action": "connect", "args": ["1"]}
- "查看缓存连接" -> {"command": "cache", "action": "list", "args": []}
- "检查主机连通性" -> {"command": "check", "action": "", "args": []}

If the input doesn't match any sherlock command, respond with:
{"error": "not a sherlock command"}`

// ParseSherlockCommandDirect tries to parse common sherlock command patterns directly without LLM.
func ParseSherlockCommandDirect(request string) *SherlockCommandInfo {
	lower := strings.ToLower(request)

	// Host commands
	hostKeywords := []string{"host", "hosts", "主机", "服务器"}
	dbKeywords := []string{"db", "database", "mysql", "数据库"}
	cacheKeywords := []string{"cache", "redis", "缓存"}
	listKeywords := []string{"list", "show", "显示", "查看", "列表", "all"}
	addKeywords := []string{"add", "添加", "新增"}
	deleteKeywords := []string{"delete", "remove", "删除", "移除"}
	connectKeywords := []string{"connect", "连接"}
	checkKeywords := []string{"check", "检查", "测试"}

	containsAny := func(s string, keywords []string) bool {
		for _, kw := range keywords {
			if strings.Contains(s, kw) {
				return true
			}
		}
		return false
	}

	// Determine command type
	var command string
	if containsAny(lower, hostKeywords) {
		command = "host"
	} else if containsAny(lower, dbKeywords) {
		command = "db"
	} else if containsAny(lower, cacheKeywords) {
		command = "cache"
	} else if containsAny(lower, checkKeywords) {
		return &SherlockCommandInfo{Command: "check", Action: "", Args: []string{}}
	} else {
		return nil
	}

	// Determine action
	var action string
	if containsAny(lower, listKeywords) {
		action = "list"
	} else if containsAny(lower, addKeywords) {
		action = "add"
	} else if containsAny(lower, deleteKeywords) {
		action = "delete"
	} else if containsAny(lower, connectKeywords) {
		action = "connect"
	} else {
		action = "list" // default to list
	}

	// Extract ID or arguments (numbers)
	var args []string
	idRe := regexp.MustCompile(`\b(\d+)\b`)
	if matches := idRe.FindAllStringSubmatch(request, -1); len(matches) > 0 {
		for _, m := range matches {
			args = append(args, m[1])
		}
	}

	// Extract connection string (user@host:port format)
	connRe := regexp.MustCompile(`([a-zA-Z0-9_-]+@)?(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}|[a-zA-Z0-9.-]+)(:\d+)?(/[a-zA-Z0-9_-]+)?`)
	if matches := connRe.FindStringSubmatch(request); len(matches) > 0 && matches[0] != "" {
		// Only add if it looks like a connection string (has @ or has host:port)
		if strings.Contains(matches[0], "@") || strings.Contains(matches[0], ":") {
			args = append(args, matches[0])
		}
	}

	return &SherlockCommandInfo{
		Command: command,
		Action:  action,
		Args:    args,
	}
}

// ParseSherlockCommand parses natural language input into sherlock internal commands.
func (a *Agent) ParseSherlockCommand(ctx context.Context, request string) (*SherlockCommandInfo, error) {
	// First try direct parsing
	if info := ParseSherlockCommandDirect(request); info != nil {
		return info, nil
	}

	// Fall back to AI parsing
	messages := []*schema.Message{
		schema.SystemMessage(systemPromptSherlockCommand),
		schema.UserMessage(request),
	}

	response, err := a.aiClient.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to generate response: %w", err)
	}

	content := strings.TrimSpace(response.Content)
	content = extractJSON(content)

	var info SherlockCommandInfo
	if err := json.Unmarshal([]byte(content), &info); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if info.Error != "" {
		return nil, fmt.Errorf("not a sherlock command")
	}

	return &info, nil
}

