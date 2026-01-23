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

// Package database provides MySQL database connection management.
package database

import (
	"fmt"
	"time"
)

// Connection represents a database connection configuration.
type Connection struct {
	ID           int64
	Host         string
	Port         int
	User         string
	Password     string // Encrypted
	DatabaseName string
	Alias        string
	Description  string
	Timestamp    time.Time
	LoginCount   int
	Groups       []Group
	Tags         []Tag
}

// ConnectionKey returns a unique key for the connection.
func (c *Connection) ConnectionKey() string {
	return fmt.Sprintf("%s@%s:%d/%s", c.User, c.Host, c.Port, c.DatabaseName)
}

// DSN returns the Data Source Name for MySQL connection.
func (c *Connection) DSN(password string) string {
	dbName := c.DatabaseName
	if dbName == "" {
		dbName = "mysql"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User, password, c.Host, c.Port, dbName)
}

// DisplayName returns the display name for the connection.
func (c *Connection) DisplayName() string {
	if c.Alias != "" {
		return c.Alias
	}
	return c.ConnectionKey()
}

// Group represents a database group.
type Group struct {
	ID          int64
	Name        string
	Description string
}

// Tag represents a tag for database connections.
type Tag struct {
	ID    int64
	Name  string
	Color string
}

// QueryHistory represents a database query history entry.
type QueryHistory struct {
	ID              int64
	DBID            int64
	Query           string
	ExecutionTimeMs int64
	RowsAffected    int64
	ExecutedAt      time.Time
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
	Host         string   `json:"host" yaml:"host"`
	Port         int      `json:"port" yaml:"port"`
	User         string   `json:"user" yaml:"user"`
	DatabaseName string   `json:"database_name,omitempty" yaml:"database_name,omitempty"`
	Alias        string   `json:"alias,omitempty" yaml:"alias,omitempty"`
	Description  string   `json:"description,omitempty" yaml:"description,omitempty"`
	Groups       []string `json:"groups,omitempty" yaml:"groups,omitempty"`
	Tags         []string `json:"tags,omitempty" yaml:"tags,omitempty"`
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
