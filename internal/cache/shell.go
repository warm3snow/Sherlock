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
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/peterh/liner"
)

// Shell represents an interactive Redis shell.
type Shell struct {
	client  *Client
	manager *Manager
	conn    *Connection
	out     io.Writer
	liner   *liner.State
}

// NewShell creates a new interactive Redis shell.
func NewShell(client *Client, manager *Manager, conn *Connection) *Shell {
	return &Shell{
		client:  client,
		manager: manager,
		conn:    conn,
		out:     os.Stdout,
	}
}

// Run starts the interactive shell.
func (s *Shell) Run(ctx context.Context) error {
	s.liner = liner.NewLiner()
	defer s.liner.Close()

	s.liner.SetCtrlCAborts(true)

	// Print welcome message
	version, _ := s.client.GetServerVersion(ctx)
	dbSize, _ := s.client.DBSize(ctx)

	fmt.Fprintf(s.out, "Connected to Redis %s\n", version)
	fmt.Fprintf(s.out, "Connection: %s\n", s.conn.DisplayName())
	fmt.Fprintf(s.out, "Database: %d (%d keys)\n", s.conn.DatabaseIndex, dbSize)
	fmt.Fprintln(s.out, "Type 'help' for available commands, 'exit' to quit.")
	fmt.Fprintln(s.out)

	for {
		prompt := fmt.Sprintf("%s:%d> ", s.conn.Host, s.conn.DatabaseIndex)

		line, err := s.liner.Prompt(prompt)
		if err != nil {
			if err == liner.ErrPromptAborted {
				fmt.Fprintln(s.out, "Use 'exit' to quit.")
				continue
			}
			// EOF or other error
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		s.liner.AppendHistory(line)

		// Handle special commands
		if s.handleSpecialCommand(ctx, line) {
			continue
		}

		// Execute Redis command
		if err := s.executeCommand(ctx, line); err != nil {
			fmt.Fprintf(s.out, "(error) %v\n", err)
		}
	}

	return nil
}

// handleSpecialCommand handles shell-specific commands.
func (s *Shell) handleSpecialCommand(ctx context.Context, input string) bool {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false
	}

	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "exit", "quit":
		fmt.Fprintln(s.out, "Bye")
		os.Exit(0)
		return true

	case "help":
		s.printHelp()
		return true

	case "clear":
		// Clear screen (ANSI escape)
		fmt.Fprint(s.out, "\033[H\033[2J")
		return true

	case "info":
		s.printInfo(ctx, parts[1:])
		return true

	case "status":
		s.printStatus(ctx)
		return true

	case "scan":
		s.scanKeys(ctx, parts[1:])
		return true
	}

	return false
}

// executeCommand executes a Redis command and displays the results.
func (s *Shell) executeCommand(ctx context.Context, cmdStr string) error {
	args := ParseArgs(cmdStr)
	if len(args) == 0 {
		return nil
	}

	start := time.Now()
	result, err := s.client.Execute(ctx, args...)
	if err != nil {
		return err
	}

	// Record command history
	if s.manager != nil {
		_ = s.manager.AddCommandHistory(ctx, s.conn.ID, cmdStr)
	}

	// Format and print result
	fmt.Fprintln(s.out, FormatValue(result.Value))
	fmt.Fprintf(s.out, "(%.3f sec)\n", time.Since(start).Seconds())

	return nil
}

// printHelp prints help information.
func (s *Shell) printHelp() {
	fmt.Fprintln(s.out, "Redis Shell Commands:")
	fmt.Fprintln(s.out, "")
	fmt.Fprintln(s.out, "  help           Show this help message")
	fmt.Fprintln(s.out, "  exit, quit     Exit the shell")
	fmt.Fprintln(s.out, "  clear          Clear the screen")
	fmt.Fprintln(s.out, "  info [section] Show server information")
	fmt.Fprintln(s.out, "  status         Show connection status")
	fmt.Fprintln(s.out, "  scan [pattern] Scan keys matching pattern")
	fmt.Fprintln(s.out, "")
	fmt.Fprintln(s.out, "Common Redis Commands:")
	fmt.Fprintln(s.out, "")
	fmt.Fprintln(s.out, "  String:")
	fmt.Fprintln(s.out, "    GET key              Get value")
	fmt.Fprintln(s.out, "    SET key value        Set value")
	fmt.Fprintln(s.out, "    MGET key [key...]    Get multiple values")
	fmt.Fprintln(s.out, "    INCR/DECR key        Increment/decrement")
	fmt.Fprintln(s.out, "")
	fmt.Fprintln(s.out, "  Key:")
	fmt.Fprintln(s.out, "    KEYS pattern         Find keys")
	fmt.Fprintln(s.out, "    DEL key [key...]     Delete keys")
	fmt.Fprintln(s.out, "    EXISTS key           Check if key exists")
	fmt.Fprintln(s.out, "    TYPE key             Get key type")
	fmt.Fprintln(s.out, "    TTL key              Get time to live")
	fmt.Fprintln(s.out, "    EXPIRE key seconds   Set expiration")
	fmt.Fprintln(s.out, "")
	fmt.Fprintln(s.out, "  Hash:")
	fmt.Fprintln(s.out, "    HGET key field       Get hash field")
	fmt.Fprintln(s.out, "    HSET key field val   Set hash field")
	fmt.Fprintln(s.out, "    HGETALL key          Get all fields")
	fmt.Fprintln(s.out, "")
	fmt.Fprintln(s.out, "  List:")
	fmt.Fprintln(s.out, "    LPUSH/RPUSH key val  Push to list")
	fmt.Fprintln(s.out, "    LPOP/RPOP key        Pop from list")
	fmt.Fprintln(s.out, "    LRANGE key 0 -1      Get list elements")
	fmt.Fprintln(s.out, "")
	fmt.Fprintln(s.out, "  Set:")
	fmt.Fprintln(s.out, "    SADD key member      Add to set")
	fmt.Fprintln(s.out, "    SMEMBERS key         Get all members")
	fmt.Fprintln(s.out, "")
	fmt.Fprintln(s.out, "  Sorted Set:")
	fmt.Fprintln(s.out, "    ZADD key score mem   Add with score")
	fmt.Fprintln(s.out, "    ZRANGE key 0 -1      Get range")
	fmt.Fprintln(s.out, "")
	fmt.Fprintln(s.out, "All standard Redis commands are supported.")
}

// printInfo prints server information.
func (s *Shell) printInfo(ctx context.Context, sections []string) {
	section := ""
	if len(sections) > 0 {
		section = sections[0]
	}

	info, err := s.client.Info(ctx, section)
	if err != nil {
		fmt.Fprintf(s.out, "(error) %v\n", err)
		return
	}

	fmt.Fprintln(s.out, info)
}

// printStatus prints connection status.
func (s *Shell) printStatus(ctx context.Context) {
	fmt.Fprintln(s.out, "--------------")
	fmt.Fprintf(s.out, "Connection: %s\n", s.conn.DisplayName())
	fmt.Fprintf(s.out, "Address: %s\n", s.conn.Addr())
	fmt.Fprintf(s.out, "Database: %d\n", s.conn.DatabaseIndex)

	version, _ := s.client.GetServerVersion(ctx)
	fmt.Fprintf(s.out, "Server version: %s\n", version)

	dbSize, _ := s.client.DBSize(ctx)
	fmt.Fprintf(s.out, "Keys in database: %d\n", dbSize)

	if memory, err := s.client.GetMemoryUsage(ctx); err == nil {
		if used, ok := memory["used_memory_human"]; ok {
			fmt.Fprintf(s.out, "Memory used: %s\n", used)
		}
	}

	fmt.Fprintln(s.out, "--------------")
}

// scanKeys scans and displays keys.
func (s *Shell) scanKeys(ctx context.Context, args []string) {
	pattern := "*"
	if len(args) > 0 {
		pattern = args[0]
	}

	var allKeys []string
	var cursor uint64

	for {
		keys, newCursor, err := s.client.Scan(ctx, cursor, pattern, 100)
		if err != nil {
			fmt.Fprintf(s.out, "(error) %v\n", err)
			return
		}

		allKeys = append(allKeys, keys...)
		cursor = newCursor

		if cursor == 0 {
			break
		}

		// Limit to 1000 keys for display
		if len(allKeys) >= 1000 {
			fmt.Fprintln(s.out, "(limited to 1000 keys)")
			break
		}
	}

	if len(allKeys) == 0 {
		fmt.Fprintln(s.out, "(empty)")
		return
	}

	for i, key := range allKeys {
		keyType, _ := s.client.Type(ctx, key)
		ttl, _ := s.client.TTL(ctx, key)

		ttlStr := "no expire"
		if ttl > 0 {
			ttlStr = ttl.String()
		} else if ttl < 0 {
			ttlStr = "no expire"
		}

		fmt.Fprintf(s.out, "%d) %s (%s, ttl: %s)\n", i+1, key, keyType, ttlStr)
	}

	fmt.Fprintf(s.out, "\n%d keys\n", len(allKeys))
}

// InspectKey inspects a key and shows its value based on type.
func (s *Shell) InspectKey(ctx context.Context, key string) error {
	keyType, err := s.client.Type(ctx, key)
	if err != nil {
		return err
	}

	fmt.Fprintf(s.out, "Key: %s\n", key)
	fmt.Fprintf(s.out, "Type: %s\n", keyType)

	ttl, _ := s.client.TTL(ctx, key)
	if ttl > 0 {
		fmt.Fprintf(s.out, "TTL: %s\n", ttl)
	} else {
		fmt.Fprintln(s.out, "TTL: no expiry")
	}

	fmt.Fprintln(s.out, "Value:")

	switch keyType {
	case "string":
		val, err := s.client.Get(ctx, key)
		if err != nil {
			return err
		}
		fmt.Fprintf(s.out, "  %s\n", val)

	case "list":
		vals, err := s.client.LRange(ctx, key, 0, 99)
		if err != nil {
			return err
		}
		for i, v := range vals {
			fmt.Fprintf(s.out, "  %d) %s\n", i+1, v)
		}
		if len(vals) == 100 {
			fmt.Fprintln(s.out, "  ... (truncated)")
		}

	case "set":
		vals, err := s.client.SMembers(ctx, key)
		if err != nil {
			return err
		}
		for i, v := range vals {
			fmt.Fprintf(s.out, "  %d) %s\n", i+1, v)
		}

	case "hash":
		vals, err := s.client.HGetAll(ctx, key)
		if err != nil {
			return err
		}
		i := 1
		for k, v := range vals {
			fmt.Fprintf(s.out, "  %d) %s: %s\n", i, k, v)
			i++
		}

	case "zset":
		vals, err := s.client.ZRangeWithScores(ctx, key, 0, 99)
		if err != nil {
			return err
		}
		for i, z := range vals {
			fmt.Fprintf(s.out, "  %d) %v (score: %s)\n", i+1, z.Member, strconv.FormatFloat(z.Score, 'f', -1, 64))
		}
		if len(vals) == 100 {
			fmt.Fprintln(s.out, "  ... (truncated)")
		}

	default:
		fmt.Fprintf(s.out, "  (unknown type: %s)\n", keyType)
	}

	return nil
}
