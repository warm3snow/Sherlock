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

package theme

import (
	"strings"
	"testing"

	"github.com/warm3snow/sherlock/internal/config"
)

func TestGetTheme(t *testing.T) {
	tests := []struct {
		name      string
		themeType config.ThemeType
		wantName  config.ThemeType
	}{
		{
			name:      "default theme",
			themeType: config.ThemeDefault,
			wantName:  config.ThemeDefault,
		},
		{
			name:      "dracula theme",
			themeType: config.ThemeDracula,
			wantName:  config.ThemeDracula,
		},
		{
			name:      "solarized theme",
			themeType: config.ThemeSolarized,
			wantName:  config.ThemeSolarized,
		},
		{
			name:      "unknown theme falls back to default",
			themeType: config.ThemeType("unknown"),
			wantName:  config.ThemeDefault,
		},
		{
			name:      "empty theme falls back to default",
			themeType: config.ThemeType(""),
			wantName:  config.ThemeDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			theme := GetTheme(tt.themeType)
			if theme.Name != tt.wantName {
				t.Errorf("GetTheme(%q) = %q, want %q", tt.themeType, theme.Name, tt.wantName)
			}
		})
	}
}

func TestDefaultTheme(t *testing.T) {
	theme := DefaultTheme()

	if theme.Name != config.ThemeDefault {
		t.Errorf("DefaultTheme().Name = %q, want %q", theme.Name, config.ThemeDefault)
	}

	// Default theme should have no ANSI codes
	if theme.BannerPrimary != "" {
		t.Errorf("DefaultTheme().BannerPrimary should be empty")
	}
	if theme.Reset != "" {
		t.Errorf("DefaultTheme().Reset should be empty")
	}
}

func TestDraculaTheme(t *testing.T) {
	theme := DraculaTheme()

	if theme.Name != config.ThemeDracula {
		t.Errorf("DraculaTheme().Name = %q, want %q", theme.Name, config.ThemeDracula)
	}

	// Dracula theme should have ANSI codes
	if theme.BannerPrimary == "" {
		t.Errorf("DraculaTheme().BannerPrimary should not be empty")
	}
	if theme.Reset != Reset {
		t.Errorf("DraculaTheme().Reset = %q, want %q", theme.Reset, Reset)
	}
}

func TestSolarizedTheme(t *testing.T) {
	theme := SolarizedTheme()

	if theme.Name != config.ThemeSolarized {
		t.Errorf("SolarizedTheme().Name = %q, want %q", theme.Name, config.ThemeSolarized)
	}

	// Solarized theme should have ANSI codes
	if theme.BannerPrimary == "" {
		t.Errorf("SolarizedTheme().BannerPrimary should not be empty")
	}
	if theme.Reset != Reset {
		t.Errorf("SolarizedTheme().Reset = %q, want %q", theme.Reset, Reset)
	}
}

func TestThemeFormatMethods(t *testing.T) {
	// Test with default theme (no colors)
	defaultTheme := DefaultTheme()
	if got := defaultTheme.FormatBanner("test"); got != "test" {
		t.Errorf("DefaultTheme.FormatBanner() = %q, want %q", got, "test")
	}
	if got := defaultTheme.FormatInfo("info"); got != "info" {
		t.Errorf("DefaultTheme.FormatInfo() = %q, want %q", got, "info")
	}

	// Test with Dracula theme (with colors)
	draculaTheme := DraculaTheme()
	bannerResult := draculaTheme.FormatBanner("test")
	if !strings.HasPrefix(bannerResult, draculaTheme.BannerPrimary) {
		t.Errorf("DraculaTheme.FormatBanner() should start with BannerPrimary color code")
	}
	if !strings.HasSuffix(bannerResult, draculaTheme.Reset) {
		t.Errorf("DraculaTheme.FormatBanner() should end with Reset code")
	}
}

func TestThemeFormatPrompt(t *testing.T) {
	// Test with default theme (no colors)
	defaultTheme := DefaultTheme()
	prompt := defaultTheme.FormatPrompt("sherlock[", "localhost", "]> ")
	expected := "sherlock[localhost]> "
	if prompt != expected {
		t.Errorf("DefaultTheme.FormatPrompt() = %q, want %q", prompt, expected)
	}

	// Test with Dracula theme
	draculaTheme := DraculaTheme()
	coloredPrompt := draculaTheme.FormatPrompt("sherlock[", "localhost", "]> ")
	if coloredPrompt == expected {
		t.Errorf("DraculaTheme.FormatPrompt() should contain color codes")
	}
	// Should contain the actual text
	if !strings.Contains(coloredPrompt, "sherlock[") ||
		!strings.Contains(coloredPrompt, "localhost") ||
		!strings.Contains(coloredPrompt, "]> ") {
		t.Errorf("DraculaTheme.FormatPrompt() should contain the original text")
	}
}

func TestThemeFormatStdout(t *testing.T) {
	// Test with default theme (no colors)
	defaultTheme := DefaultTheme()
	if got := defaultTheme.FormatStdout("output"); got != "output" {
		t.Errorf("DefaultTheme.FormatStdout() = %q, want %q", got, "output")
	}

	// Test with Dracula theme (with colors)
	draculaTheme := DraculaTheme()
	stdoutResult := draculaTheme.FormatStdout("output")
	if !strings.HasPrefix(stdoutResult, draculaTheme.Stdout) {
		t.Errorf("DraculaTheme.FormatStdout() should start with Stdout color code")
	}
	if !strings.HasSuffix(stdoutResult, draculaTheme.Reset) {
		t.Errorf("DraculaTheme.FormatStdout() should end with Reset code")
	}
	if !strings.Contains(stdoutResult, "output") {
		t.Errorf("DraculaTheme.FormatStdout() should contain the original text")
	}

	// Test with Solarized theme
	solarizedTheme := SolarizedTheme()
	solarizedResult := solarizedTheme.FormatStdout("test output")
	if !strings.HasPrefix(solarizedResult, solarizedTheme.Stdout) {
		t.Errorf("SolarizedTheme.FormatStdout() should start with Stdout color code")
	}
	if !strings.Contains(solarizedResult, "test output") {
		t.Errorf("SolarizedTheme.FormatStdout() should contain the original text")
	}
}

func TestThemeFormatStderr(t *testing.T) {
	// Test with default theme (no colors)
	defaultTheme := DefaultTheme()
	if got := defaultTheme.FormatStderr("error"); got != "error" {
		t.Errorf("DefaultTheme.FormatStderr() = %q, want %q", got, "error")
	}

	// Test with Dracula theme (with colors)
	draculaTheme := DraculaTheme()
	stderrResult := draculaTheme.FormatStderr("error")
	if !strings.HasPrefix(stderrResult, draculaTheme.Stderr) {
		t.Errorf("DraculaTheme.FormatStderr() should start with Stderr color code")
	}
	if !strings.HasSuffix(stderrResult, draculaTheme.Reset) {
		t.Errorf("DraculaTheme.FormatStderr() should end with Reset code")
	}
	if !strings.Contains(stderrResult, "error") {
		t.Errorf("DraculaTheme.FormatStderr() should contain the original text")
	}

	// Test with Solarized theme
	solarizedTheme := SolarizedTheme()
	solarizedResult := solarizedTheme.FormatStderr("test error")
	if !strings.HasPrefix(solarizedResult, solarizedTheme.Stderr) {
		t.Errorf("SolarizedTheme.FormatStderr() should start with Stderr color code")
	}
	if !strings.Contains(solarizedResult, "test error") {
		t.Errorf("SolarizedTheme.FormatStderr() should contain the original text")
	}
}

func TestThemeFormatHistoryRecords(t *testing.T) {
	theme := DraculaTheme()

	// Test with empty records
	emptyResult := theme.FormatHistoryRecords([]HistoryRecord{})
	if !strings.Contains(emptyResult, "No login history found") {
		t.Errorf("FormatHistoryRecords(empty) should indicate no history found")
	}

	// Test with records
	records := []HistoryRecord{
		{
			ID:         1,
			HostKey:    "root@example.com:22",
			LoginCount: 5,
			Timestamp:  "2024-01-01 12:00:00",
			HasPubKey:  true,
		},
		{
			ID:         2,
			HostKey:    "admin@server.local:22",
			LoginCount: 2,
			Timestamp:  "2024-01-02 15:30:00",
			HasPubKey:  false,
		},
	}

	result := theme.FormatHistoryRecords(records)
	if !strings.Contains(result, "Login History") {
		t.Errorf("FormatHistoryRecords() should contain 'Login History' header")
	}
	if !strings.Contains(result, "root@example.com:22") {
		t.Errorf("FormatHistoryRecords() should contain first host")
	}
	if !strings.Contains(result, "admin@server.local:22") {
		t.Errorf("FormatHistoryRecords() should contain second host")
	}
	if !strings.Contains(result, "[key]") {
		t.Errorf("FormatHistoryRecords() should show [key] for hosts with pubkey")
	}
}

func TestThemeFormatHostsSimple(t *testing.T) {
	theme := SolarizedTheme()

	// Test with empty records
	emptyResult := theme.FormatHostsSimple([]HistoryRecord{})
	if !strings.Contains(emptyResult, "No saved hosts found") {
		t.Errorf("FormatHostsSimple(empty) should indicate no hosts found")
	}

	// Test with records
	records := []HistoryRecord{
		{
			ID:         1,
			HostKey:    "root@192.168.1.100:22",
			LoginCount: 3,
			Timestamp:  "2024-01-01 12:00:00",
			HasPubKey:  true,
		},
	}

	result := theme.FormatHostsSimple(records)
	if !strings.Contains(result, "Saved Hosts") {
		t.Errorf("FormatHostsSimple() should contain 'Saved Hosts' header")
	}
	if !strings.Contains(result, "[1]") {
		t.Errorf("FormatHostsSimple() should contain host ID")
	}
	if !strings.Contains(result, "192.168.1.100") {
		t.Errorf("FormatHostsSimple() should contain host address")
	}
	if !strings.Contains(result, "connect <id>") {
		t.Errorf("FormatHostsSimple() should contain usage hint")
	}
}
