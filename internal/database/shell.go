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
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/peterh/liner"
)

// Shell represents an interactive MySQL shell.
type Shell struct {
	client  *Client
	manager *Manager
	conn    *Connection
	out     io.Writer
	liner   *liner.State
}

// NewShell creates a new interactive MySQL shell.
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
	version, _ := s.client.GetVersion(ctx)
	currentDB, _ := s.client.GetCurrentDatabase(ctx)

	fmt.Fprintf(s.out, "Connected to MySQL %s\n", version)
	fmt.Fprintf(s.out, "Connection: %s\n", s.conn.DisplayName())
	if currentDB != "" {
		fmt.Fprintf(s.out, "Database: %s\n", currentDB)
	}
	fmt.Fprintln(s.out, "Type 'help' for available commands, 'exit' to quit.")
	fmt.Fprintln(s.out)

	var multiLineBuffer strings.Builder
	inMultiLine := false

	for {
		prompt := "mysql> "
		if inMultiLine {
			prompt = "    -> "
		}

		line, err := s.liner.Prompt(prompt)
		if err != nil {
			if err == liner.ErrPromptAborted {
				if inMultiLine {
					multiLineBuffer.Reset()
					inMultiLine = false
					fmt.Fprintln(s.out)
					continue
				}
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

		// Handle multi-line queries
		if inMultiLine {
			multiLineBuffer.WriteString(" ")
			multiLineBuffer.WriteString(line)
		} else {
			multiLineBuffer.WriteString(line)
		}

		query := multiLineBuffer.String()

		// Check if query is complete (ends with ; or is a special command)
		if !strings.HasSuffix(query, ";") && !s.isSpecialCommand(query) {
			inMultiLine = true
			continue
		}

		inMultiLine = false
		multiLineBuffer.Reset()

		// Remove trailing semicolon for special commands
		query = strings.TrimSuffix(query, ";")
		query = strings.TrimSpace(query)

		if query == "" {
			continue
		}

		s.liner.AppendHistory(query)

		// Handle special commands
		if s.handleSpecialCommand(ctx, query) {
			continue
		}

		// Execute SQL query
		if err := s.executeQuery(ctx, query); err != nil {
			fmt.Fprintf(s.out, "Error: %v\n", err)
		}
	}

	return nil
}

// isSpecialCommand checks if the input is a special shell command.
func (s *Shell) isSpecialCommand(input string) bool {
	lower := strings.ToLower(strings.TrimSpace(input))
	commands := []string{"exit", "quit", "help", "\\q", "\\h", "status", "\\s", "clear", "\\c"}
	for _, cmd := range commands {
		if lower == cmd || strings.HasPrefix(lower, cmd+" ") {
			return true
		}
	}
	return strings.HasPrefix(lower, "use ") ||
		strings.HasPrefix(lower, "\\u ")
}

// handleSpecialCommand handles shell-specific commands.
func (s *Shell) handleSpecialCommand(ctx context.Context, input string) bool {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "exit", "quit", "\\q":
		fmt.Fprintln(s.out, "Bye")
		os.Exit(0)
		return true

	case "help", "\\h":
		s.printHelp()
		return true

	case "status", "\\s":
		s.printStatus(ctx)
		return true

	case "clear", "\\c":
		// Clear screen (ANSI escape)
		fmt.Fprint(s.out, "\033[H\033[2J")
		return true

	case "use", "\\u":
		if len(args) == 0 {
			fmt.Fprintln(s.out, "Error: No database specified")
			return true
		}
		if err := s.client.UseDatabase(ctx, args[0]); err != nil {
			fmt.Fprintf(s.out, "Error: %v\n", err)
		} else {
			fmt.Fprintf(s.out, "Database changed to '%s'\n", args[0])
		}
		return true

	case "databases", "\\l":
		s.listDatabases(ctx)
		return true

	case "tables", "\\t":
		s.listTables(ctx)
		return true

	case "describe", "desc", "\\d":
		if len(args) == 0 {
			fmt.Fprintln(s.out, "Error: No table specified")
			return true
		}
		s.describeTable(ctx, args[0])
		return true

	case "processlist", "\\p":
		s.showProcessList(ctx)
		return true
	}

	return false
}

// executeQuery executes a SQL query and displays the results.
func (s *Shell) executeQuery(ctx context.Context, query string) error {
	result, err := s.client.Execute(ctx, query)
	if err != nil {
		return err
	}

	// Record query history
	if s.manager != nil {
		_ = s.manager.AddQueryHistory(ctx, s.conn.ID, query,
			result.Duration.Milliseconds(), result.RowsAffected)
	}

	if len(result.Columns) > 0 {
		s.printTable(result.Columns, result.Rows)
		fmt.Fprintf(s.out, "%d rows in set (%.3f sec)\n", len(result.Rows), result.Duration.Seconds())
	} else {
		fmt.Fprintf(s.out, "Query OK, %d rows affected (%.3f sec)\n",
			result.RowsAffected, result.Duration.Seconds())
	}

	return nil
}

// printTable prints query results in a table format.
func (s *Shell) printTable(columns []string, rows [][]interface{}) {
	if len(columns) == 0 {
		return
	}

	w := tabwriter.NewWriter(s.out, 0, 0, 2, ' ', 0)

	// Print header
	header := make([]string, len(columns))
	for i, col := range columns {
		header[i] = col
	}
	fmt.Fprintln(w, strings.Join(header, "\t"))

	// Print separator
	sep := make([]string, len(columns))
	for i := range columns {
		sep[i] = "--------"
	}
	fmt.Fprintln(w, strings.Join(sep, "\t"))

	// Print rows
	for _, row := range rows {
		values := make([]string, len(row))
		for i, v := range row {
			if v == nil {
				values[i] = "NULL"
			} else {
				values[i] = fmt.Sprintf("%v", v)
			}
		}
		fmt.Fprintln(w, strings.Join(values, "\t"))
	}

	w.Flush()
}

// printHelp prints help information.
func (s *Shell) printHelp() {
	fmt.Fprintln(s.out, "MySQL Shell Commands:")
	fmt.Fprintln(s.out, "")
	fmt.Fprintln(s.out, "  help, \\h           Show this help message")
	fmt.Fprintln(s.out, "  exit, quit, \\q     Exit the shell")
	fmt.Fprintln(s.out, "  status, \\s         Show server status")
	fmt.Fprintln(s.out, "  clear, \\c          Clear the screen")
	fmt.Fprintln(s.out, "  use <db>, \\u <db>  Switch to database")
	fmt.Fprintln(s.out, "  databases, \\l      List all databases")
	fmt.Fprintln(s.out, "  tables, \\t         List tables in current database")
	fmt.Fprintln(s.out, "  describe <tbl>     Describe table structure")
	fmt.Fprintln(s.out, "  processlist, \\p    Show process list")
	fmt.Fprintln(s.out, "")
	fmt.Fprintln(s.out, "All other input is executed as SQL queries.")
	fmt.Fprintln(s.out, "End SQL statements with ';'")
}

// printStatus prints server status.
func (s *Shell) printStatus(ctx context.Context) {
	fmt.Fprintln(s.out, "--------------")
	fmt.Fprintf(s.out, "Connection: %s\n", s.conn.DisplayName())

	version, _ := s.client.GetVersion(ctx)
	fmt.Fprintf(s.out, "Server version: %s\n", version)

	currentDB, _ := s.client.GetCurrentDatabase(ctx)
	if currentDB != "" {
		fmt.Fprintf(s.out, "Current database: %s\n", currentDB)
	}

	fmt.Fprintf(s.out, "Connection ID: %s@%s:%d\n", s.conn.User, s.conn.Host, s.conn.Port)
	fmt.Fprintln(s.out, "--------------")
}

// listDatabases lists all databases.
func (s *Shell) listDatabases(ctx context.Context) {
	databases, err := s.client.ListDatabases(ctx)
	if err != nil {
		fmt.Fprintf(s.out, "Error: %v\n", err)
		return
	}

	columns := []string{"Database"}
	rows := make([][]interface{}, len(databases))
	for i, db := range databases {
		rows[i] = []interface{}{db}
	}
	s.printTable(columns, rows)
	fmt.Fprintf(s.out, "%d databases\n", len(databases))
}

// listTables lists all tables in the current database.
func (s *Shell) listTables(ctx context.Context) {
	tables, err := s.client.ListTables(ctx)
	if err != nil {
		fmt.Fprintf(s.out, "Error: %v\n", err)
		return
	}

	currentDB, _ := s.client.GetCurrentDatabase(ctx)
	columns := []string{fmt.Sprintf("Tables_in_%s", currentDB)}
	rows := make([][]interface{}, len(tables))
	for i, tbl := range tables {
		rows[i] = []interface{}{tbl}
	}
	s.printTable(columns, rows)
	fmt.Fprintf(s.out, "%d tables\n", len(tables))
}

// describeTable describes a table structure.
func (s *Shell) describeTable(ctx context.Context, tableName string) {
	result, err := s.client.DescribeTable(ctx, tableName)
	if err != nil {
		fmt.Fprintf(s.out, "Error: %v\n", err)
		return
	}
	s.printTable(result.Columns, result.Rows)
}

// showProcessList shows the process list.
func (s *Shell) showProcessList(ctx context.Context) {
	result, err := s.client.GetProcessList(ctx)
	if err != nil {
		fmt.Fprintf(s.out, "Error: %v\n", err)
		return
	}
	s.printTable(result.Columns, result.Rows)
}
