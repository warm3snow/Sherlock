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

package cache

import (
	"context"
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client represents a Redis client.
type Client struct {
	client *redis.Client
	conn   *Connection
}

// NewClient creates a new Redis client.
func NewClient(conn *Connection, password string) (*Client, error) {
	opts := &redis.Options{
		Addr:     conn.Addr(),
		Password: password,
		DB:       conn.DatabaseIndex,
	}

	if conn.UseTLS {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Client{
		client: client,
		conn:   conn,
	}, nil
}

// Close closes the Redis connection.
func (c *Client) Close() error {
	return c.client.Close()
}

// Ping tests the Redis connection.
func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Client returns the underlying go-redis client.
func (c *Client) Client() *redis.Client {
	return c.client
}

// CommandResult represents the result of a Redis command.
type CommandResult struct {
	Value    interface{}
	Duration time.Duration
}

// Execute executes a Redis command.
func (c *Client) Execute(ctx context.Context, args ...interface{}) (*CommandResult, error) {
	start := time.Now()

	cmd := c.client.Do(ctx, args...)
	if cmd.Err() != nil && cmd.Err() != redis.Nil {
		return nil, cmd.Err()
	}

	return &CommandResult{
		Value:    cmd.Val(),
		Duration: time.Since(start),
	}, nil
}

// Get gets a string value.
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

// Set sets a string value.
func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.client.Set(ctx, key, value, expiration).Err()
}

// Del deletes keys.
func (c *Client) Del(ctx context.Context, keys ...string) (int64, error) {
	return c.client.Del(ctx, keys...).Result()
}

// Keys returns keys matching a pattern.
func (c *Client) Keys(ctx context.Context, pattern string) ([]string, error) {
	return c.client.Keys(ctx, pattern).Result()
}

// Type returns the type of a key.
func (c *Client) Type(ctx context.Context, key string) (string, error) {
	return c.client.Type(ctx, key).Result()
}

// TTL returns the TTL of a key.
func (c *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.client.TTL(ctx, key).Result()
}

// Exists checks if keys exist.
func (c *Client) Exists(ctx context.Context, keys ...string) (int64, error) {
	return c.client.Exists(ctx, keys...).Result()
}

// Info returns server information.
func (c *Client) Info(ctx context.Context, section ...string) (string, error) {
	return c.client.Info(ctx, section...).Result()
}

// DBSize returns the number of keys in the current database.
func (c *Client) DBSize(ctx context.Context) (int64, error) {
	return c.client.DBSize(ctx).Result()
}

// FlushDB removes all keys from the current database.
func (c *Client) FlushDB(ctx context.Context) error {
	return c.client.FlushDB(ctx).Err()
}

// Select changes the selected database.
func (c *Client) Select(ctx context.Context, db int) error {
	// Note: go-redis doesn't support SELECT on the same connection
	// This is a limitation - would need to create a new client
	return fmt.Errorf("SELECT not supported - create a new connection with different database")
}

// Scan iterates over keys.
func (c *Client) Scan(ctx context.Context, cursor uint64, match string, count int64) ([]string, uint64, error) {
	return c.client.Scan(ctx, cursor, match, count).Result()
}

// HGetAll gets all fields and values from a hash.
func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, key).Result()
}

// HSet sets fields in a hash.
func (c *Client) HSet(ctx context.Context, key string, values ...interface{}) (int64, error) {
	return c.client.HSet(ctx, key, values...).Result()
}

// LRange gets a range of elements from a list.
func (c *Client) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.LRange(ctx, key, start, stop).Result()
}

// SMembers gets all members of a set.
func (c *Client) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.client.SMembers(ctx, key).Result()
}

// ZRangeWithScores gets a range of members with scores from a sorted set.
func (c *Client) ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	return c.client.ZRangeWithScores(ctx, key, start, stop).Result()
}

// ConfigGet gets configuration parameters.
func (c *Client) ConfigGet(ctx context.Context, parameter string) (map[string]string, error) {
	return c.client.ConfigGet(ctx, parameter).Result()
}

// ClientList returns information about client connections.
func (c *Client) ClientList(ctx context.Context) (string, error) {
	return c.client.ClientList(ctx).Result()
}

// SlowLogGet gets entries from the slowlog.
func (c *Client) SlowLogGet(ctx context.Context, num int64) ([]redis.SlowLog, error) {
	return c.client.SlowLogGet(ctx, num).Result()
}

// ServerInfo parses and returns server info as a map.
func (c *Client) ServerInfo(ctx context.Context) (map[string]map[string]string, error) {
	info, err := c.Info(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]map[string]string)
	var currentSection string

	lines := strings.Split(info, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "#") {
			currentSection = strings.TrimPrefix(line, "# ")
			result[currentSection] = make(map[string]string)
			continue
		}

		if currentSection != "" {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				result[currentSection][parts[0]] = parts[1]
			}
		}
	}

	return result, nil
}

// GetServerVersion returns the Redis server version.
func (c *Client) GetServerVersion(ctx context.Context) (string, error) {
	info, err := c.ServerInfo(ctx)
	if err != nil {
		return "", err
	}

	if server, ok := info["Server"]; ok {
		if version, ok := server["redis_version"]; ok {
			return version, nil
		}
	}

	return "unknown", nil
}

// GetMemoryUsage returns memory usage information.
func (c *Client) GetMemoryUsage(ctx context.Context) (map[string]string, error) {
	info, err := c.ServerInfo(ctx)
	if err != nil {
		return nil, err
	}

	if memory, ok := info["Memory"]; ok {
		return memory, nil
	}

	return nil, fmt.Errorf("memory info not found")
}

// ParseArgs parses a command string into arguments.
func ParseArgs(cmdStr string) []interface{} {
	var args []interface{}
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, ch := range cmdStr {
		switch {
		case ch == '"' || ch == '\'':
			if inQuote && ch == quoteChar {
				inQuote = false
				quoteChar = 0
			} else if !inQuote {
				inQuote = true
				quoteChar = ch
			} else {
				current.WriteRune(ch)
			}
		case ch == ' ' && !inQuote:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// FormatValue formats a Redis value for display.
func FormatValue(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return "(nil)"
	case string:
		return fmt.Sprintf("\"%s\"", v)
	case int64:
		return fmt.Sprintf("(integer) %d", v)
	case []interface{}:
		if len(v) == 0 {
			return "(empty array)"
		}
		var lines []string
		for i, item := range v {
			lines = append(lines, fmt.Sprintf("%d) %s", i+1, FormatValue(item)))
		}
		return strings.Join(lines, "\n")
	case []string:
		if len(v) == 0 {
			return "(empty array)"
		}
		var lines []string
		for i, item := range v {
			lines = append(lines, fmt.Sprintf("%d) \"%s\"", i+1, item))
		}
		return strings.Join(lines, "\n")
	case map[string]string:
		if len(v) == 0 {
			return "(empty hash)"
		}
		var lines []string
		i := 1
		for key, val := range v {
			lines = append(lines, fmt.Sprintf("%d) \"%s\"", i, key))
			lines = append(lines, fmt.Sprintf("%d) \"%s\"", i+1, val))
			i += 2
		}
		return strings.Join(lines, "\n")
	case []redis.Z:
		if len(v) == 0 {
			return "(empty sorted set)"
		}
		var lines []string
		for i, z := range v {
			lines = append(lines, fmt.Sprintf("%d) \"%v\"", i*2+1, z.Member))
			lines = append(lines, fmt.Sprintf("%d) \"%s\"", i*2+2, strconv.FormatFloat(z.Score, 'f', -1, 64)))
		}
		return strings.Join(lines, "\n")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// TestConnection tests if a connection is working.
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
