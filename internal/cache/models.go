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

// Package cache provides Redis cache connection management.
package cache

import (
	"fmt"
	"time"
)

// Connection represents a Redis cache connection configuration.
type Connection struct {
	ID            int64
	Host          string
	Port          int
	Password      string // Encrypted
	DatabaseIndex int
	Alias         string
	Description   string
	Timestamp     time.Time
	LoginCount    int
	UseTLS        bool
	Groups        []Group
	Tags          []Tag
}

// ConnectionKey returns a unique key for the connection.
func (c *Connection) ConnectionKey() string {
	return fmt.Sprintf("%s:%d/%d", c.Host, c.Port, c.DatabaseIndex)
}

// DisplayName returns the display name for the connection.
func (c *Connection) DisplayName() string {
	if c.Alias != "" {
		return c.Alias
	}
	return c.ConnectionKey()
}

// Addr returns the Redis server address.
func (c *Connection) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Group represents a cache group.
type Group struct {
	ID          int64
	Name        string
	Description string
}

// Tag represents a tag for cache connections.
type Tag struct {
	ID    int64
	Name  string
	Color string
}

// CommandHistory represents a cache command history entry.
type CommandHistory struct {
	ID         int64
	CacheID    int64
	Command    string
	ExecutedAt time.Time
}

// ListFilter defines filters for listing connections.
type ListFilter struct {
	GroupID   int64
	GroupName string
	TagID     int64
	TagName   string
	Search    string
}

// ExportFormat defines the export format type.
type ExportFormat string

const (
	ExportJSON ExportFormat = "json"
	ExportYAML ExportFormat = "yaml"
)

// ExportData represents the exported data structure.
type ExportData struct {
	Version     string             `json:"version" yaml:"version"`
	Connections []ExportConnection `json:"connections" yaml:"connections"`
	Groups      []ExportGroup      `json:"groups" yaml:"groups"`
	Tags        []ExportTag        `json:"tags" yaml:"tags"`
}

// ExportConnection represents an exported connection.
type ExportConnection struct {
	Host          string   `json:"host" yaml:"host"`
	Port          int      `json:"port" yaml:"port"`
	DatabaseIndex int      `json:"database_index,omitempty" yaml:"database_index,omitempty"`
	UseTLS        bool     `json:"use_tls,omitempty" yaml:"use_tls,omitempty"`
	Alias         string   `json:"alias,omitempty" yaml:"alias,omitempty"`
	Description   string   `json:"description,omitempty" yaml:"description,omitempty"`
	Groups        []string `json:"groups,omitempty" yaml:"groups,omitempty"`
	Tags          []string `json:"tags,omitempty" yaml:"tags,omitempty"`
}

// ExportGroup represents an exported group.
type ExportGroup struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// ExportTag represents an exported tag.
type ExportTag struct {
	Name  string `json:"name" yaml:"name"`
	Color string `json:"color,omitempty" yaml:"color,omitempty"`
}
