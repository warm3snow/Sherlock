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

package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Client represents a MySQL database client using GORM.
type Client struct {
	db   *gorm.DB
	conn *Connection
}

// NewClient creates a new MySQL client.
func NewClient(conn *Connection, password string) (*Client, error) {
	dsn := conn.DSN(password)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL: %w", err)
	}

	// Get underlying sql.DB and set connection pool settings
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying DB: %w", err)
	}

	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return &Client{
		db:   db,
		conn: conn,
	}, nil
}

// Ping tests the database connection.
func (c *Client) Ping(ctx context.Context) error {
	sqlDB, err := c.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Close closes the database connection.
func (c *Client) Close() error {
	sqlDB, err := c.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// DB returns the underlying GORM database.
func (c *Client) DB() *gorm.DB {
	return c.db
}

// QueryResult represents the result of a query.
type QueryResult struct {
	Columns      []string
	Rows         [][]interface{}
	RowsAffected int64
	Duration     time.Duration
}

// Execute executes a SQL query and returns the results.
func (c *Client) Execute(ctx context.Context, query string) (*QueryResult, error) {
	start := time.Now()
	query = strings.TrimSpace(query)

	result := &QueryResult{}

	// Determine if it's a SELECT query
	upperQuery := strings.ToUpper(query)
	isSelect := strings.HasPrefix(upperQuery, "SELECT") ||
		strings.HasPrefix(upperQuery, "SHOW") ||
		strings.HasPrefix(upperQuery, "DESCRIBE") ||
		strings.HasPrefix(upperQuery, "DESC") ||
		strings.HasPrefix(upperQuery, "EXPLAIN")

	if isSelect {
		rows, err := c.db.WithContext(ctx).Raw(query).Rows()
		if err != nil {
			return nil, fmt.Errorf("query failed: %w", err)
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("failed to get columns: %w", err)
		}
		result.Columns = columns

		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				return nil, fmt.Errorf("failed to scan row: %w", err)
			}

			// Convert []byte to string for display
			for i, v := range values {
				if b, ok := v.([]byte); ok {
					values[i] = string(b)
				}
			}

			result.Rows = append(result.Rows, values)
		}
	} else {
		// Non-SELECT query (INSERT, UPDATE, DELETE, etc.)
		tx := c.db.WithContext(ctx).Exec(query)
		if tx.Error != nil {
			return nil, fmt.Errorf("query failed: %w", tx.Error)
		}
		result.RowsAffected = tx.RowsAffected
	}

	result.Duration = time.Since(start)
	return result, nil
}

// ListDatabases returns all database names.
func (c *Client) ListDatabases(ctx context.Context) ([]string, error) {
	var databases []string
	result := c.db.WithContext(ctx).Raw("SHOW DATABASES").Scan(&databases)
	if result.Error != nil {
		return nil, result.Error
	}
	return databases, nil
}

// ListTables returns all table names in the current database.
func (c *Client) ListTables(ctx context.Context) ([]string, error) {
	var tables []string
	result := c.db.WithContext(ctx).Raw("SHOW TABLES").Scan(&tables)
	if result.Error != nil {
		return nil, result.Error
	}
	return tables, nil
}

// DescribeTable returns the structure of a table.
func (c *Client) DescribeTable(ctx context.Context, tableName string) (*QueryResult, error) {
	return c.Execute(ctx, fmt.Sprintf("DESCRIBE `%s`", tableName))
}

// UseDatabase switches to a different database.
func (c *Client) UseDatabase(ctx context.Context, dbName string) error {
	return c.db.WithContext(ctx).Exec(fmt.Sprintf("USE `%s`", dbName)).Error
}

// GetCurrentDatabase returns the current database name.
func (c *Client) GetCurrentDatabase(ctx context.Context) (string, error) {
	var dbName string
	result := c.db.WithContext(ctx).Raw("SELECT DATABASE()").Scan(&dbName)
	if result.Error != nil {
		return "", result.Error
	}
	return dbName, nil
}

// GetVersion returns the MySQL server version.
func (c *Client) GetVersion(ctx context.Context) (string, error) {
	var version string
	result := c.db.WithContext(ctx).Raw("SELECT VERSION()").Scan(&version)
	if result.Error != nil {
		return "", result.Error
	}
	return version, nil
}

// GetStatus returns MySQL server status variables.
func (c *Client) GetStatus(ctx context.Context) (map[string]string, error) {
	rows, err := c.db.WithContext(ctx).Raw("SHOW STATUS").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	status := make(map[string]string)
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			continue
		}
		status[name] = value
	}
	return status, nil
}

// GetVariables returns MySQL server variables.
func (c *Client) GetVariables(ctx context.Context, pattern string) (map[string]string, error) {
	query := "SHOW VARIABLES"
	if pattern != "" {
		query += fmt.Sprintf(" LIKE '%s'", pattern)
	}

	rows, err := c.db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	variables := make(map[string]string)
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			continue
		}
		variables[name] = value
	}
	return variables, nil
}

// GetProcessList returns the current process list.
func (c *Client) GetProcessList(ctx context.Context) (*QueryResult, error) {
	return c.Execute(ctx, "SHOW PROCESSLIST")
}

// KillProcess kills a process by ID.
func (c *Client) KillProcess(ctx context.Context, processID int64) error {
	return c.db.WithContext(ctx).Exec(fmt.Sprintf("KILL %d", processID)).Error
}

// TestConnection tests if the connection is working.
func TestConnection(conn *Connection, password string) error {
	client, err := NewClient(conn, password)
	if err != nil {
		return err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return client.Ping(ctx)
}

// RawDB returns the underlying sql.DB for advanced operations.
func (c *Client) RawDB() (*sql.DB, error) {
	return c.db.DB()
}
