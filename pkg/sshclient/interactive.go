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
	"strings"
)

// interactiveCommandsMap contains commands that require PTY support
// for proper display due to continuous output or interactive features.
var interactiveCommandsMap = func() map[string]bool {
	commands := []string{
		// System monitoring tools with continuous/interactive output
		"top",
		"htop",
		"btop",
		"atop",
		"iotop",
		"iftop",
		"nload",
		"bmon",
		"glances",
		"nmon",
		"nethogs",
		"powertop",
		// Text editors
		"vi",
		"vim",
		"nvim",
		"nano",
		"emacs",
		"pico",
		"joe",
		"mcedit",
		// File managers
		"mc",
		"ranger",
		"nnn",
		"lf",
		"vifm",
		// Terminal multiplexers
		"tmux",
		"screen",
		"byobu",
		// Interactive shells and REPLs
		"bash",
		"zsh",
		"fish",
		"sh",
		"python",
		"python3",
		"ipython",
		"node",
		"irb",
		"ghci",
		"lua",
		"php",
		"mysql",
		"psql",
		"sqlite3",
		"mongo",
		"redis-cli",
		// Other interactive tools
		"less",
		"more",
		"watch",
		"cfdisk",
		"parted",
		"cgdisk",
	}
	m := make(map[string]bool, len(commands))
	for _, cmd := range commands {
		m[cmd] = true
	}
	return m
}()

// IsInteractiveCommand checks if the given command is an interactive command
// that requires PTY support for proper display.
func IsInteractiveCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}

	// Get the first word (command name)
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return false
	}
	cmdName := strings.ToLower(parts[0])

	// Handle commands with path prefix (e.g., /usr/bin/top)
	if strings.Contains(cmdName, "/") {
		// Extract the base name
		lastSlash := strings.LastIndex(cmdName, "/")
		if lastSlash >= 0 && lastSlash < len(cmdName)-1 {
			cmdName = cmdName[lastSlash+1:]
		}
	}

	// Check if it's in the interactive commands map
	if interactiveCommandsMap[cmdName] {
		return true
	}

	// Check for tail -f pattern (tail with follow option)
	if cmdName == "tail" {
		for _, arg := range parts[1:] {
			if arg == "-f" || arg == "-F" || arg == "--follow" || strings.HasPrefix(arg, "-f") {
				return true
			}
		}
	}

	// Check for journalctl -f pattern
	if cmdName == "journalctl" {
		for _, arg := range parts[1:] {
			if arg == "-f" || arg == "--follow" {
				return true
			}
		}
	}

	// Check for dmesg -w pattern (watch for new messages)
	if cmdName == "dmesg" {
		for _, arg := range parts[1:] {
			if arg == "-w" || arg == "--follow" {
				return true
			}
		}
	}

	return false
}
