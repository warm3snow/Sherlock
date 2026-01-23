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

// Package hosts provides enhanced host management for Sherlock.
package hosts

import "time"

// Host represents an enhanced host record.
type Host struct {
	ID          int64
	Host        string
	Port        int
	User        string
	Alias       string // Optional alias for easy reference
	Description string // Optional description
	Timestamp   time.Time
	HasPubKey   bool
	LoginCount  int
	Groups      []Group           // Associated groups
	Tags        []Tag             // Associated tags
	EnvVars     map[string]string // Environment variables
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// HostKey returns a unique key for the host (user@host:port).
func (h *Host) HostKey() string {
	return h.User + "@" + h.Host + ":" + string(rune(h.Port+'0'))
}

// DisplayName returns the alias if set, otherwise the host key.
func (h *Host) DisplayName() string {
	if h.Alias != "" {
		return h.Alias
	}
	return h.HostKey()
}

// Group represents a host group.
type Group struct {
	ID          int64
	Name        string
	Description string
	HostCount   int // Number of hosts in this group
	CreatedAt   time.Time
}

// Tag represents a host tag.
type Tag struct {
	ID    int64
	Name  string
	Color string // Hex color code
}

// HostFilter defines search/filter criteria.
type HostFilter struct {
	Query     string   // Free text search (searches host, user, alias, description)
	Groups    []string // Filter by group names
	Tags      []string // Filter by tag names
	HasPubKey *bool    // Filter by public key status
}

// ExportFormat defines export formats.
type ExportFormat string

const (
	ExportJSON      ExportFormat = "json"
	ExportYAML      ExportFormat = "yaml"
	ExportSSHConfig ExportFormat = "ssh_config"
)

// ExportData represents exported hosts data.
type ExportData struct {
	Version string         `json:"version" yaml:"version"`
	Hosts   []ExportedHost `json:"hosts" yaml:"hosts"`
	Groups  []ExportedGroup `json:"groups" yaml:"groups"`
	Tags    []ExportedTag   `json:"tags" yaml:"tags"`
}

// ExportedHost represents a host in export format.
type ExportedHost struct {
	Host        string            `json:"host" yaml:"host"`
	Port        int               `json:"port" yaml:"port"`
	User        string            `json:"user" yaml:"user"`
	Alias       string            `json:"alias,omitempty" yaml:"alias,omitempty"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Groups      []string          `json:"groups,omitempty" yaml:"groups,omitempty"`
	Tags        []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	EnvVars     map[string]string `json:"env_vars,omitempty" yaml:"env_vars,omitempty"`
}

// ExportedGroup represents a group in export format.
type ExportedGroup struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// ExportedTag represents a tag in export format.
type ExportedTag struct {
	Name  string `json:"name" yaml:"name"`
	Color string `json:"color,omitempty" yaml:"color,omitempty"`
}

// CommandHistoryEntry represents a command history entry.
type CommandHistoryEntry struct {
	ID         int64
	HostID     int64
	Command    string
	ExitCode   *int
	ExecutedAt time.Time
}

// EnvVar represents an environment variable.
type EnvVar struct {
	ID     int64
	HostID int64
	Name   string
	Value  string
}
