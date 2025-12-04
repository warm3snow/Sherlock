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

package sshclient

import (
	"testing"
)

func TestIsInteractiveCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		// Interactive commands
		{name: "top", command: "top", want: true},
		{name: "htop", command: "htop", want: true},
		{name: "btop", command: "btop", want: true},
		{name: "vim", command: "vim", want: true},
		{name: "vim with file", command: "vim /etc/hosts", want: true},
		{name: "nano", command: "nano", want: true},
		{name: "less", command: "less file.txt", want: true},
		{name: "watch", command: "watch -n 1 date", want: true},
		{name: "tmux", command: "tmux", want: true},
		{name: "screen", command: "screen", want: true},
		{name: "python", command: "python", want: true},
		{name: "python3", command: "python3", want: true},
		{name: "mysql", command: "mysql -u root", want: true},
		{name: "psql", command: "psql", want: true},
		// Interactive with path prefix
		{name: "top with path", command: "/usr/bin/top", want: true},
		{name: "vim with path", command: "/usr/bin/vim file.txt", want: true},
		// tail -f variants
		{name: "tail -f", command: "tail -f /var/log/syslog", want: true},
		{name: "tail -F", command: "tail -F /var/log/syslog", want: true},
		{name: "tail --follow", command: "tail --follow /var/log/syslog", want: true},
		// journalctl -f
		{name: "journalctl -f", command: "journalctl -f", want: true},
		{name: "journalctl --follow", command: "journalctl --follow", want: true},
		// dmesg -w
		{name: "dmesg -w", command: "dmesg -w", want: true},
		{name: "dmesg --follow", command: "dmesg --follow", want: true},
		// Non-interactive commands
		{name: "ls", command: "ls -la", want: false},
		{name: "cat", command: "cat file.txt", want: false},
		{name: "grep", command: "grep pattern file.txt", want: false},
		{name: "ps", command: "ps aux", want: false},
		{name: "df", command: "df -h", want: false},
		{name: "du", command: "du -sh *", want: false},
		{name: "tail without -f", command: "tail -n 10 /var/log/syslog", want: false},
		{name: "journalctl without -f", command: "journalctl -n 100", want: false},
		{name: "dmesg without -w", command: "dmesg | tail", want: false},
		{name: "empty command", command: "", want: false},
		{name: "whitespace only", command: "   ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsInteractiveCommand(tt.command); got != tt.want {
				t.Errorf("IsInteractiveCommand(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}
