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

// Package theme provides theming support for Sherlock CLI.
package theme

import (
	"fmt"
	"strings"

	"github.com/warm3snow/sherlock/internal/config"
)

// ANSI color codes
const (
	Reset     = "\033[0m"
	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Italic    = "\033[3m"
	Underline = "\033[4m"

	// Standard colors
	Black   = "\033[30m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"

	// Bright colors
	BrightBlack   = "\033[90m"
	BrightRed     = "\033[91m"
	BrightGreen   = "\033[92m"
	BrightYellow  = "\033[93m"
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	BrightCyan    = "\033[96m"
	BrightWhite   = "\033[97m"
)

// Theme represents a color theme for the CLI.
type Theme struct {
	Name config.ThemeType

	// Banner colors
	BannerPrimary   string
	BannerSecondary string

	// Prompt colors
	PromptPrefix string
	PromptHost   string
	PromptSuffix string

	// Status and info colors
	Info    string
	Success string
	Warning string
	Error   string

	// Command colors
	CommandName string
	CommandDesc string

	// Command output colors
	Stdout string
	Stderr string

	// Table colors
	TableHeader  string
	TableBorder  string
	TableContent string

	// Reset
	Reset string
}

// DefaultTheme returns the default simple theme.
func DefaultTheme() *Theme {
	return &Theme{
		Name:            config.ThemeDefault,
		BannerPrimary:   "",
		BannerSecondary: "",
		PromptPrefix:    "",
		PromptHost:      "",
		PromptSuffix:    "",
		Info:            "",
		Success:         "",
		Warning:         "",
		Error:           "",
		CommandName:     "",
		CommandDesc:     "",
		Stdout:          "",
		Stderr:          "",
		TableHeader:     "",
		TableBorder:     "",
		TableContent:    "",
		Reset:           "",
	}
}

// DraculaTheme returns the Dracula dark theme.
func DraculaTheme() *Theme {
	return &Theme{
		Name:            config.ThemeDracula,
		BannerPrimary:   BrightMagenta + Bold,
		BannerSecondary: BrightCyan,
		PromptPrefix:    BrightMagenta + Bold,
		PromptHost:      BrightCyan,
		PromptSuffix:    BrightGreen,
		Info:            BrightCyan,
		Success:         BrightGreen,
		Warning:         BrightYellow,
		Error:           BrightRed,
		CommandName:     BrightMagenta,
		CommandDesc:     BrightWhite,
		Stdout:          BrightWhite,
		Stderr:          BrightRed,
		TableHeader:     BrightMagenta + Bold,
		TableBorder:     BrightBlack,
		TableContent:    BrightWhite,
		Reset:           Reset,
	}
}

// SolarizedTheme returns the Solarized color theme.
func SolarizedTheme() *Theme {
	return &Theme{
		Name:            config.ThemeSolarized,
		BannerPrimary:   Blue + Bold,
		BannerSecondary: Cyan,
		PromptPrefix:    Blue + Bold,
		PromptHost:      Cyan,
		PromptSuffix:    Green,
		Info:            Cyan,
		Success:         Green,
		Warning:         Yellow,
		Error:           Red,
		CommandName:     Blue,
		CommandDesc:     White,
		Stdout:          White,
		Stderr:          Red,
		TableHeader:     Blue + Bold,
		TableBorder:     BrightBlack,
		TableContent:    White,
		Reset:           Reset,
	}
}

// GetTheme returns a theme by name.
func GetTheme(name config.ThemeType) *Theme {
	switch name {
	case config.ThemeDracula:
		return DraculaTheme()
	case config.ThemeSolarized:
		return SolarizedTheme()
	default:
		return DefaultTheme()
	}
}

// FormatBanner formats the banner text with theme colors.
func (t *Theme) FormatBanner(banner string) string {
	if t.BannerPrimary == "" {
		return banner
	}
	return t.BannerPrimary + banner + t.Reset
}

// FormatBannerSubtitle formats the banner subtitle with theme colors.
func (t *Theme) FormatBannerSubtitle(subtitle string) string {
	if t.BannerSecondary == "" {
		return subtitle
	}
	return t.BannerSecondary + subtitle + t.Reset
}

// FormatPrompt formats the prompt with theme colors.
func (t *Theme) FormatPrompt(prefix, host, suffix string) string {
	if t.PromptPrefix == "" {
		return prefix + host + suffix
	}
	return t.PromptPrefix + prefix + t.Reset +
		t.PromptHost + host + t.Reset +
		t.PromptSuffix + suffix + t.Reset
}

// FormatInfo formats informational text.
func (t *Theme) FormatInfo(text string) string {
	if t.Info == "" {
		return text
	}
	return t.Info + text + t.Reset
}

// FormatSuccess formats success text.
func (t *Theme) FormatSuccess(text string) string {
	if t.Success == "" {
		return text
	}
	return t.Success + text + t.Reset
}

// FormatWarning formats warning text.
func (t *Theme) FormatWarning(text string) string {
	if t.Warning == "" {
		return text
	}
	return t.Warning + text + t.Reset
}

// FormatError formats error text.
func (t *Theme) FormatError(text string) string {
	if t.Error == "" {
		return text
	}
	return t.Error + text + t.Reset
}

// FormatCommand formats a command name.
func (t *Theme) FormatCommand(name string) string {
	if t.CommandName == "" {
		return name
	}
	return t.CommandName + name + t.Reset
}

// FormatDescription formats a command description.
func (t *Theme) FormatDescription(desc string) string {
	if t.CommandDesc == "" {
		return desc
	}
	return t.CommandDesc + desc + t.Reset
}

// FormatStdout formats command standard output.
func (t *Theme) FormatStdout(output string) string {
	if t.Stdout == "" {
		return output
	}
	return t.Stdout + output + t.Reset
}

// FormatStderr formats command standard error output.
func (t *Theme) FormatStderr(output string) string {
	if t.Stderr == "" {
		return output
	}
	return t.Stderr + output + t.Reset
}

// FormatTableHeader formats table header text.
func (t *Theme) FormatTableHeader(header string) string {
	if t.TableHeader == "" {
		return header
	}
	return t.TableHeader + header + t.Reset
}

// FormatTableBorder formats table border text.
func (t *Theme) FormatTableBorder(border string) string {
	if t.TableBorder == "" {
		return border
	}
	return t.TableBorder + border + t.Reset
}

// FormatTableContent formats table content text.
func (t *Theme) FormatTableContent(content string) string {
	if t.TableContent == "" {
		return content
	}
	return t.TableContent + content + t.Reset
}

// FormatHistoryRecords formats history records with theme colors.
func (t *Theme) FormatHistoryRecords(records []HistoryRecord) string {
	if len(records) == 0 {
		return t.FormatInfo("No login history found.\n")
	}

	var sb strings.Builder
	sb.WriteString(t.FormatTableHeader("Login History:\n"))
	sb.WriteString(t.FormatTableBorder(strings.Repeat("-", 70) + "\n"))
	sb.WriteString(t.FormatTableHeader(fmt.Sprintf("%-4s %-30s %-6s %-20s\n", "ID", "Host", "Logins", "Last Login")))
	sb.WriteString(t.FormatTableBorder(strings.Repeat("-", 70) + "\n"))

	for _, r := range records {
		pubKeyStatus := ""
		if r.HasPubKey {
			pubKeyStatus = t.FormatSuccess(" [key]")
		}
		line := fmt.Sprintf("%-4d %-30s %-6d %s",
			r.ID,
			r.HostKey,
			r.LoginCount,
			r.Timestamp)
		sb.WriteString(t.FormatTableContent(line) + pubKeyStatus + "\n")
	}

	return sb.String()
}

// FormatHostsSimple formats hosts list with theme colors.
func (t *Theme) FormatHostsSimple(records []HistoryRecord) string {
	if len(records) == 0 {
		return t.FormatInfo("No saved hosts found.\n")
	}

	var sb strings.Builder
	sb.WriteString(t.FormatTableHeader("Saved Hosts:\n"))
	sb.WriteString(t.FormatTableBorder(strings.Repeat("-", 50) + "\n"))

	for _, r := range records {
		pubKeyStatus := ""
		if r.HasPubKey {
			pubKeyStatus = t.FormatSuccess(" [key]")
		}
		id := t.FormatInfo(fmt.Sprintf("[%d]", r.ID))
		host := t.FormatTableContent(r.HostKey)
		sb.WriteString(fmt.Sprintf("%s %s%s\n", id, host, pubKeyStatus))
	}

	sb.WriteString(t.FormatTableBorder(strings.Repeat("-", 50) + "\n"))
	sb.WriteString(t.FormatInfo("Use 'connect <id>' to connect to a saved host.\n"))

	return sb.String()
}

// HistoryRecord is a simplified history record for theme formatting.
type HistoryRecord struct {
	ID         int64
	HostKey    string
	LoginCount int
	Timestamp  string
	HasPubKey  bool
}
