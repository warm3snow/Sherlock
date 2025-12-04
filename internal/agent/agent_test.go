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
	"testing"
)

func TestIsShellCommand(t *testing.T) {
	// Create a test agent with no custom commands
	agent := NewAgent(nil)

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Basic commands
		{name: "ls command", input: "ls", want: true},
		{name: "ls with flags", input: "ls -la", want: true},
		{name: "ls with path", input: "ls /tmp", want: true},
		{name: "pwd command", input: "pwd", want: true},
		{name: "cd command", input: "cd /home", want: true},
		{name: "cat file", input: "cat /etc/passwd", want: true},
		{name: "grep pattern", input: "grep -r pattern .", want: true},

		// System commands
		{name: "df command", input: "df -h", want: true},
		{name: "ps command", input: "ps aux", want: true},
		{name: "top command", input: "top", want: true},
		{name: "free command", input: "free -m", want: true},
		{name: "uptime command", input: "uptime", want: true},

		// Network commands
		{name: "ping command", input: "ping google.com", want: true},
		{name: "curl command", input: "curl https://example.com", want: true},
		{name: "wget command", input: "wget https://example.com/file.tar.gz", want: true},
		{name: "netstat command", input: "netstat -tlnp", want: true},
		{name: "ss command", input: "ss -tlnp", want: true},

		// Package management
		{name: "apt command", input: "apt update", want: true},
		{name: "yum command", input: "yum install vim", want: true},
		{name: "pip command", input: "pip install requests", want: true},
		{name: "npm command", input: "npm install express", want: true},

		// Container commands
		{name: "docker command", input: "docker ps", want: true},
		{name: "kubectl command", input: "kubectl get pods", want: true},

		// Version control
		{name: "git command", input: "git status", want: true},
		{name: "git log", input: "git log --oneline", want: true},

		// Path-based commands
		{name: "absolute path script", input: "/usr/bin/ls", want: true},
		{name: "relative path script", input: "./script.sh", want: true},
		{name: "parent path script", input: "../script.sh", want: true},

		// Natural language (should not match)
		{name: "natural language disk usage", input: "show me disk usage", want: false},
		{name: "natural language list files", input: "list all files", want: false},
		{name: "chinese natural language", input: "查看磁盘使用情况", want: false},
		{name: "natural language process", input: "show running processes", want: false},
		{name: "help request", input: "help me check disk", want: false},
		{name: "question format", input: "how much disk space", want: false},

		// Edge cases
		{name: "empty string", input: "", want: false},
		{name: "whitespace only", input: "   ", want: false},
		{name: "unknown command", input: "nonexistent_cmd", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := agent.IsShellCommand(tt.input); got != tt.want {
				t.Errorf("IsShellCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsShellCommandWithCustomWhitelist(t *testing.T) {
	// Create a test agent with custom commands
	agent := NewAgent(nil)
	agent.SetCustomShellCommands([]string{"mycustomcmd", "another-cmd", "UPPERCASE"})

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Custom commands should match
		{name: "custom command", input: "mycustomcmd arg1 arg2", want: true},
		{name: "another custom command", input: "another-cmd --flag", want: true},
		{name: "uppercase custom command", input: "uppercase option", want: true},
		{name: "custom command case insensitive", input: "MYCUSTOMCMD", want: true},

		// Built-in commands should still work
		{name: "ls command", input: "ls -la", want: true},
		{name: "git command", input: "git status", want: true},

		// Unknown commands should not match
		{name: "unknown command", input: "unknowncmd", want: false},
		{name: "natural language", input: "run my custom command", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := agent.IsShellCommand(tt.input); got != tt.want {
				t.Errorf("IsShellCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseCommandDirect(t *testing.T) {
	// Create a test agent
	agent := NewAgent(nil)

	tests := []struct {
		name        string
		input       string
		wantNil     bool
		wantCommand string
	}{
		// Shell commands should be parsed directly
		{name: "ls command", input: "ls -la", wantNil: false, wantCommand: "ls -la"},
		{name: "df command", input: "df -h", wantNil: false, wantCommand: "df -h"},
		{name: "ps command", input: "ps aux", wantNil: false, wantCommand: "ps aux"},
		{name: "cat command", input: "cat /etc/hosts", wantNil: false, wantCommand: "cat /etc/hosts"},
		{name: "grep command", input: "grep -r error /var/log", wantNil: false, wantCommand: "grep -r error /var/log"},
		{name: "docker command", input: "docker ps -a", wantNil: false, wantCommand: "docker ps -a"},
		{name: "git command", input: "git status", wantNil: false, wantCommand: "git status"},
		{name: "absolute path", input: "/usr/bin/env", wantNil: false, wantCommand: "/usr/bin/env"},
		{name: "relative path", input: "./run.sh", wantNil: false, wantCommand: "./run.sh"},

		// Natural language should return nil
		{name: "natural language disk", input: "show me disk usage", wantNil: true},
		{name: "natural language files", input: "list all files in current directory", wantNil: true},
		{name: "chinese", input: "查看磁盘使用情况", wantNil: true},
		{name: "empty", input: "", wantNil: true},
		{name: "whitespace", input: "   ", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.parseCommandDirect(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Errorf("parseCommandDirect(%q) = %v, want nil", tt.input, result)
				}
				return
			}
			if result == nil {
				t.Errorf("parseCommandDirect(%q) = nil, want non-nil", tt.input)
				return
			}
			if len(result.Commands) != 1 || result.Commands[0] != tt.wantCommand {
				t.Errorf("parseCommandDirect(%q).Commands = %v, want [%q]", tt.input, result.Commands, tt.wantCommand)
			}
		})
	}
}

func TestIsDangerousCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Dangerous commands
		{name: "rm command", input: "rm file.txt", want: true},
		{name: "rm -rf command", input: "rm -rf /tmp/dir", want: true},
		{name: "rm with flag", input: "rm -r dir", want: true},
		{name: "chmod command", input: "chmod 755 file.sh", want: true},
		{name: "chown command", input: "chown root:root file", want: true},
		{name: "sudo command", input: "sudo apt update", want: true},
		{name: "systemctl command", input: "systemctl restart nginx", want: true},
		{name: "shutdown command", input: "shutdown -h now", want: true},
		{name: "reboot command", input: "reboot", want: true},
		{name: "apt command", input: "apt install vim", want: true},
		{name: "yum command", input: "yum remove httpd", want: true},
		{name: "useradd command", input: "useradd newuser", want: true},
		{name: "passwd command", input: "passwd root", want: true},
		{name: "fdisk command", input: "fdisk /dev/sda", want: true},
		{name: "dd command", input: "dd if=/dev/zero of=/dev/sda", want: true},

		// Safe commands
		{name: "ls command", input: "ls -la", want: false},
		{name: "cat command", input: "cat /etc/passwd", want: false},
		{name: "grep command", input: "grep pattern file", want: false},
		{name: "ps command", input: "ps aux", want: false},
		{name: "df command", input: "df -h", want: false},
		{name: "ping command", input: "ping google.com", want: false},
		{name: "curl command", input: "curl https://example.com", want: false},
		{name: "git command", input: "git status", want: false},
		{name: "docker ps", input: "docker ps", want: false},

		// Edge cases
		{name: "empty string", input: "", want: false},
		{name: "whitespace only", input: "   ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDangerousCommand(tt.input); got != tt.want {
				t.Errorf("isDangerousCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseCommandDirectNeedsConfirm(t *testing.T) {
	// Create a test agent
	agent := NewAgent(nil)

	tests := []struct {
		name             string
		input            string
		wantNeedsConfirm bool
	}{
		// Dangerous commands should require confirmation
		{name: "rm command", input: "rm file.txt", wantNeedsConfirm: true},
		{name: "rm -rf command", input: "rm -rf /tmp/dir", wantNeedsConfirm: true},
		{name: "sudo command", input: "sudo apt update", wantNeedsConfirm: true},
		{name: "chmod command", input: "chmod 755 file.sh", wantNeedsConfirm: true},
		{name: "shutdown command", input: "shutdown -h now", wantNeedsConfirm: true},

		// Safe commands should not require confirmation
		{name: "ls command", input: "ls -la", wantNeedsConfirm: false},
		{name: "cat command", input: "cat /etc/passwd", wantNeedsConfirm: false},
		{name: "df command", input: "df -h", wantNeedsConfirm: false},
		{name: "ps command", input: "ps aux", wantNeedsConfirm: false},
		{name: "git status", input: "git status", wantNeedsConfirm: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.parseCommandDirect(tt.input)
			if result == nil {
				t.Errorf("parseCommandDirect(%q) = nil, want non-nil", tt.input)
				return
			}
			if result.NeedsConfirm != tt.wantNeedsConfirm {
				t.Errorf("parseCommandDirect(%q).NeedsConfirm = %v, want %v", tt.input, result.NeedsConfirm, tt.wantNeedsConfirm)
			}
		})
	}
}

func TestSetCustomShellCommands(t *testing.T) {
	agent := NewAgent(nil)

	// Initially empty
	if agent.IsShellCommand("mycustomcmd") {
		t.Error("expected mycustomcmd to not be recognized initially")
	}

	// Set custom commands
	agent.SetCustomShellCommands([]string{"mycustomcmd", "another-cmd", "  spaces  ", ""})

	// Check that custom commands are recognized
	if !agent.IsShellCommand("mycustomcmd arg1") {
		t.Error("expected mycustomcmd to be recognized after setting")
	}
	if !agent.IsShellCommand("another-cmd") {
		t.Error("expected another-cmd to be recognized")
	}
	if !agent.IsShellCommand("spaces --flag") {
		t.Error("expected 'spaces' to be recognized (trimmed)")
	}

	// Case insensitive
	if !agent.IsShellCommand("MYCUSTOMCMD") {
		t.Error("expected MYCUSTOMCMD to be recognized (case insensitive)")
	}
}

func TestParseConnectionDirect(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHost string
		wantPort int
		wantUser string
		wantNil  bool
	}{
		{
			name:     "user@host:port pattern",
			input:    "ssh admin@example.com:2222",
			wantHost: "example.com",
			wantPort: 2222,
			wantUser: "admin",
		},
		{
			name:     "user@host pattern",
			input:    "root@192.168.1.100",
			wantHost: "192.168.1.100",
			wantPort: 22,
			wantUser: "root",
		},
		{
			name:     "simple IP address pattern",
			input:    "connect 192.168.40.22",
			wantHost: "192.168.40.22",
			wantPort: 22,
			wantUser: "root",
		},
		{
			name:     "Chinese connection with IP",
			input:    "连接192.168.40.22",
			wantHost: "192.168.40.22",
			wantPort: 22,
			wantUser: "root",
		},
		{
			name:     "IP address in sentence",
			input:    "please connect to 10.0.0.1 server",
			wantHost: "10.0.0.1",
			wantPort: 22,
			wantUser: "root",
		},
		{
			name:    "no connection info",
			input:   "show me disk usage",
			wantNil: true,
		},
		{
			name:    "invalid IP 999.999.999.999",
			input:   "connect 999.999.999.999",
			wantNil: true,
		},
		{
			name:    "invalid IP 256.1.1.1",
			input:   "connect 256.1.1.1",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseConnectionDirect(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Errorf("parseConnectionDirect() = %v, want nil", result)
				}
				return
			}
			if result == nil {
				t.Errorf("parseConnectionDirect() = nil, want non-nil")
				return
			}
			if result.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", result.Host, tt.wantHost)
			}
			if result.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", result.Port, tt.wantPort)
			}
			if result.User != tt.wantUser {
				t.Errorf("User = %q, want %q", result.User, tt.wantUser)
			}
		})
	}
}
