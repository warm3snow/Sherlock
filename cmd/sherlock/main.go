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

// Package main provides the main entry point for Sherlock CLI.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/peterh/liner"
	"golang.org/x/term"

	"github.com/warm3snow/sherlock/internal/agent"
	"github.com/warm3snow/sherlock/internal/ai"
	"github.com/warm3snow/sherlock/internal/analyzer"
	"github.com/warm3snow/sherlock/internal/cache"
	"github.com/warm3snow/sherlock/internal/config"
	"github.com/warm3snow/sherlock/internal/database"
	"github.com/warm3snow/sherlock/internal/history"
	"github.com/warm3snow/sherlock/internal/hosts"
	"github.com/warm3snow/sherlock/internal/theme"
	"github.com/warm3snow/sherlock/pkg/sshclient"
)

const (
	version     = "0.1.0"
	appName     = "Sherlock"
	description = "AI-powered SSH remote operations tool"
)

// ansiRegex matches ANSI color/style escape sequences (e.g., \033[31m, \033[1;32m).
// This handles the common color codes used in terminal prompts.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes ANSI escape codes from a string.
// Used when the liner library rejects prompts with control characters.
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// App represents the Sherlock application.
type App struct {
	cfg            *config.Config
	aiClient       ai.ModelClient
	agent          *agent.Agent
	sshClient      *sshclient.Client
	historyManager *history.Manager
	dbManager      *database.Manager
	cacheManager   *cache.Manager
	localClient    *sshclient.LocalClient
	theme          *theme.Theme
	ctx            context.Context
	cancel         context.CancelFunc
	sigChan        chan os.Signal
	liner          *liner.State

	// AI Enhanced features
	proactiveAnalyzer *analyzer.ProactiveAnalyzer
	toolRegistry      *agent.ToolRegistry
	
	// Non-interactive mode
	nonInteractive bool
	outputFormat   string
}

func main() {
	// Check for subcommands first
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "host":
			handleHostsCommand()
			return
		}
	}

	var (
		configPath      string
		showVersion     bool
		showHelp        bool
		providerFlag    string
		modelFlag       string
		baseURLFlag     string
		apiKeyFlag      string
		execCommand     string
		outputFormat    string
		nonInteractive  bool
	)

	flag.StringVar(&configPath, "config", "", "Path to configuration file")
	flag.StringVar(&configPath, "c", "", "Path to configuration file (shorthand)")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.BoolVar(&showVersion, "v", false, "Show version information (shorthand)")
	flag.BoolVar(&showHelp, "help", false, "Show help information")
	flag.BoolVar(&showHelp, "h", false, "Show help information (shorthand)")
	flag.StringVar(&providerFlag, "provider", "", "LLM provider (ollama, openai, deepseek, openai_compatible)")
	flag.StringVar(&modelFlag, "model", "", "Model name")
	flag.StringVar(&baseURLFlag, "base-url", "", "Base URL for LLM API")
	flag.StringVar(&apiKeyFlag, "api-key", "", "API key for LLM provider")
	flag.StringVar(&execCommand, "exec", "", "Execute command in non-interactive mode")
	flag.StringVar(&execCommand, "e", "", "Execute command in non-interactive mode (shorthand)")
	flag.StringVar(&outputFormat, "output", "text", "Output format: text, json")
	flag.StringVar(&outputFormat, "o", "text", "Output format: text, json (shorthand)")
	flag.BoolVar(&nonInteractive, "non-interactive", false, "Run in non-interactive mode")
	flag.BoolVar(&nonInteractive, "n", false, "Run in non-interactive mode (shorthand)")
	flag.Parse()
	
	// Auto-enable non-interactive mode if -exec is provided
	if execCommand != "" {
		nonInteractive = true
	}

	if showHelp {
		printHelp()
		return
	}

	if showVersion {
		fmt.Printf("%s version %s\n", appName, version)
		return
	}

	// Load configuration
	if configPath == "" {
		configPath = config.GetConfigPath()
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to load config: %v\n", err)
		cfg = config.DefaultConfig()
	}

	// Show warning if no SSH keys are found (detection is done in LoadConfig/DefaultConfig)
	if cfg.SSHKey.PrivateKeyPath == "" || cfg.SSHKey.PublicKeyPath == "" {
		fmt.Fprintln(os.Stderr, "Warning: No SSH keys found in ~/.ssh/ (tried id_ed25519 and id_rsa).")
		fmt.Fprintln(os.Stderr, "         Password authentication will be used for SSH connections.")
	}

	// Override config with command line flags
	if providerFlag != "" {
		cfg.LLM.Provider = config.LLMProviderType(providerFlag)
	}
	if modelFlag != "" {
		cfg.LLM.Model = modelFlag
	}
	if baseURLFlag != "" {
		cfg.LLM.BaseURL = baseURLFlag
	}
	if apiKeyFlag != "" {
		cfg.LLM.APIKey = apiKeyFlag
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid configuration: %v\n", err)
		fmt.Fprintln(os.Stderr, "Use --help for usage information or configure using a config file.")
		os.Exit(1)
	}

	// Create application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	// Only listen for SIGTERM for graceful shutdown
	signal.Notify(sigChan, syscall.SIGTERM)

	// Ignore SIGINT (Ctrl+C) at process level - let liner handle it via terminal
	signal.Ignore(syscall.SIGINT)

	app := &App{
		cfg:            cfg,
		ctx:            ctx,
		cancel:         cancel,
		sigChan:        sigChan,
		theme:          theme.GetTheme(cfg.UI.Theme),
		nonInteractive: nonInteractive,
		outputFormat:   outputFormat,
	}

	// Handle SIGTERM for graceful shutdown
	go func() {
		<-sigChan
		fmt.Println("\nReceived terminate signal, cleaning up...")
		app.cleanup()
		os.Exit(0)
	}()

	// Initialize AI client
	aiClient, err := ai.NewClient(ctx, &cfg.LLM)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize AI client: %v\n", err)
		os.Exit(1)
	}
	app.aiClient = aiClient
	app.agent = agent.NewAgent(aiClient)

	// Set custom shell commands from config whitelist
	if len(cfg.ShellCommands.Whitelist) > 0 {
		app.agent.SetCustomShellCommands(cfg.ShellCommands.Whitelist)
	}

	// Initialize AI Enhanced features based on config
	app.initAIEnhancedFeatures()

	// Initialize history manager
	historyMgr, err := history.NewManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize history manager: %v\n", err)
	}
	app.historyManager = historyMgr

	// Initialize database manager
	if historyMgr != nil {
		dbMgr, err := database.NewManager(historyMgr.DB())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to initialize database manager: %v\n", err)
		}
		app.dbManager = dbMgr
	}

	// Initialize cache manager
	if historyMgr != nil {
		cacheMgr, err := cache.NewManager(historyMgr.DB())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to initialize cache manager: %v\n", err)
		}
		app.cacheManager = cacheMgr
	}

	// Initialize local client for local command execution
	app.localClient = sshclient.NewLocalClient()

	// Run the application
	if nonInteractive {
		// Non-interactive mode: execute command and exit
		if err := app.runNonInteractive(execCommand); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			app.cleanup()
			os.Exit(1)
		}
	} else {
		// Interactive mode
		if err := app.run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			app.cleanup()
			os.Exit(1)
		}
	}

	app.cleanup()
}

// runNonInteractive executes a command in non-interactive mode.
func (a *App) runNonInteractive(command string) error {
	if command == "" {
		return fmt.Errorf("no command specified. Use -e or --exec to specify a command")
	}
	
	// Execute the command
	return a.handleInputNonInteractive(command)
}

// handleInputNonInteractive processes input in non-interactive mode.
func (a *App) handleInputNonInteractive(input string) error {
	// Handle built-in commands that return structured data
	switch strings.ToLower(input) {
	case "help":
		return a.printNonInteractiveHelp()
	case "status":
		return a.showStatusNonInteractive()
	}
	
	// Try enhanced commands first (host list, db list, etc.)
	if handled, err := a.handleEnhancedCommandNonInteractive(input); handled {
		return err
	}
	
	// Check if it's a whitelisted shell command - execute directly
	if isWhitelistedCommand(input) {
		return a.executeCommandNonInteractive(input)
	}
	
	// Parse as command request using AI
	cmdInfo, err := a.agent.ParseCommandRequest(a.ctx, input)
	if err != nil {
		return fmt.Errorf("failed to parse command: %w", err)
	}
	
	// Execute commands
	for _, cmd := range cmdInfo.Commands {
		if err := a.executeCommandNonInteractive(cmd); err != nil {
			return err
		}
	}
	
	return nil
}

// executeCommandNonInteractive executes a command and outputs result.
func (a *App) executeCommandNonInteractive(cmd string) error {
	var result *sshclient.ExecuteResult
	
	// Use SSH client if connected, otherwise use local client
	if a.sshClient != nil && a.sshClient.IsConnected() {
		result = a.sshClient.Execute(a.ctx, cmd)
	} else {
		result = a.localClient.Execute(a.ctx, cmd)
	}
	
	if a.outputFormat == "json" {
		return a.outputJSON(map[string]interface{}{
			"command":   cmd,
			"stdout":    result.Stdout,
			"stderr":    result.Stderr,
			"exit_code": result.ExitCode,
			"error":     errorToString(result.Error),
		})
	}
	
	// Text output
	if result.Stdout != "" {
		fmt.Print(result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)
	}
	
	if result.Error != nil {
		return result.Error
	}
	
	return nil
}

// handleEnhancedCommandNonInteractive handles enhanced commands in non-interactive mode.
func (a *App) handleEnhancedCommandNonInteractive(input string) (bool, error) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false, nil
	}
	
	cmd := strings.ToLower(parts[0])
	args := parts[1:]
	
	switch cmd {
	// Host commands
	case "host", "hosts", "hl":
		return true, a.handleHostsNonInteractive(args)
	case "ha":
		return true, a.handleHostAddNonInteractive(args)
	case "hr":
		return true, a.handleHostRemoveNonInteractive(args)
	
	// Database commands
	case "db", "database", "dl":
		return true, a.handleDatabaseNonInteractive(args)
	case "da":
		return true, a.handleDatabaseAddNonInteractive(args)
	case "dr":
		return true, a.handleDatabaseRemoveNonInteractive(args)
	
	// Cache commands
	case "cache", "cl":
		return true, a.handleCacheNonInteractive(args)
	case "ca":
		return true, a.handleCacheAddNonInteractive(args)
	case "cr":
		return true, a.handleCacheRemoveNonInteractive(args)
	
	// Tunnel commands
	case "tunnel", "tun":
		return true, a.handleTunnelNonInteractive(args)
	
	// Playbook commands
	case "playbook":
		return true, a.handlePlaybookNonInteractive(args)
	
	// Snippet commands
	case "snippet", "snip":
		return true, a.handleSnippetNonInteractive(args)
	
	// Batch commands
	case "batch":
		return true, a.handleBatchNonInteractive(args)
	
	// Health check
	case "health", "check":
		return true, a.handleHealthCheckNonInteractive(args)
	
	// Analyze
	case "analyze":
		return true, a.handleAnalyzeNonInteractive(args)
	
	// Diagnose
	case "diagnose":
		return true, a.handleDiagnoseNonInteractive(args)
	
	// Advisor
	case "advisor":
		return true, a.handleAdvisorNonInteractive(args)
	}
	
	return false, nil
}

// showStatusNonInteractive shows status in non-interactive mode.
func (a *App) showStatusNonInteractive() error {
	status := map[string]interface{}{
		"version":      version,
		"llm_provider": string(a.cfg.LLM.Provider),
		"llm_model":    a.cfg.LLM.Model,
		"theme":        a.cfg.UI.Theme,
		"connected":    a.sshClient != nil && a.sshClient.IsConnected(),
		"ai_enhanced": map[string]bool{
			"memory":     a.cfg.AIEnhanced.EnableMemory,
			"analysis":   a.cfg.AIEnhanced.EnableProactiveAnalysis,
			"prediction": a.cfg.AIEnhanced.EnablePrediction,
			"tool_call":  a.cfg.AIEnhanced.EnableToolCalling,
		},
	}
	
	if a.sshClient != nil && a.sshClient.IsConnected() {
		status["host"] = a.sshClient.HostInfoString()
	} else {
		status["host"] = a.localClient.HostInfoString()
	}
	
	return a.outputJSON(status)
}

// printNonInteractiveHelp shows help for non-interactive mode.
func (a *App) printNonInteractiveHelp() error {
	help := map[string]interface{}{
		"usage": "sherlock -e <command> [-o json]",
		"commands": map[string]string{
			"host, hl":         "List all hosts",
			"ha <connection>":  "Add host",
			"hr <id>":          "Remove host",
			"db, dl":           "List databases",
			"da <connection>":  "Add database",
			"dr <id>":          "Remove database",
			"cache, cl":        "List caches",
			"ca <connection>":  "Add cache",
			"cr <id>":          "Remove cache",
			"tunnel list":      "List tunnels",
			"tunnel create":    "Create tunnel",
			"tunnel close":     "Close tunnel",
			"playbook list":    "List playbooks",
			"playbook run":     "Run playbook",
			"snippet list":     "List snippets",
			"snippet run":      "Run snippet",
			"batch <cmd>":      "Batch execute",
			"health <host>":    "Health check",
			"analyze <cmd>":    "Analyze command output",
			"diagnose <error>": "Diagnose error",
			"advisor <query>":  "AI advisor",
			"status":           "Show status",
		},
		"options": map[string]string{
			"-e, --exec":           "Command to execute",
			"-o, --output":         "Output format (text, json)",
			"-n, --non-interactive": "Non-interactive mode",
		},
	}
	
	if a.outputFormat == "json" {
		return a.outputJSON(help)
	}
	
	fmt.Println("Sherlock Non-Interactive Mode")
	fmt.Println("Usage: sherlock -e <command> [-o json]")
	fmt.Println()
	fmt.Println("Commands:")
	for cmd, desc := range help["commands"].(map[string]string) {
		fmt.Printf("  %-20s %s\n", cmd, desc)
	}
	return nil
}

// outputJSON outputs data as JSON.
func (a *App) outputJSON(data interface{}) error {
	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(output))
	return nil
}

// errorToString converts error to string, returns empty string for nil.
func errorToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (a *App) run() error {
	a.printBanner()
	fmt.Println(a.theme.FormatInfo("Type 'help' for available commands or describe what you want to do."))
	fmt.Println()

	// Initialize liner for readline-like functionality
	a.liner = liner.NewLiner()
	defer a.liner.Close()

	// Set up tab completion
	a.liner.SetCompleter(a.completer)

	// Disable Ctrl+C exit - only allow exit via 'exit' command
	a.liner.SetCtrlCAborts(false)

	for {
		var hostStr string
		if a.sshClient != nil && a.sshClient.IsConnected() {
			hostStr = a.sshClient.HostInfoString()
		} else {
			hostStr = a.localClient.HostInfoString()
		}
		prompt := a.theme.FormatPrompt("sherlock[", hostStr, "]> ")

		input, err := a.liner.Prompt(prompt)
		if err == liner.ErrInvalidPrompt {
			// Prompt contains ANSI codes that liner rejects (e.g., when input is redirected).
			// Fall back to a plain prompt without escape sequences.
			input, err = a.liner.Prompt(stripANSI(prompt))
		}
		if err != nil {
			if err == liner.ErrPromptAborted {
				// Ctrl+C pressed, just print a newline and continue
				fmt.Println()
				continue
			}
			if err == io.EOF {
				// Only exit on EOF (e.g., when stdin is closed)
				fmt.Println("\nGoodbye!")
				return nil
			}
			return fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Add to liner history
		a.liner.AppendHistory(input)

		if err := a.handleInput(input); err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", a.theme.FormatError("Error: "+err.Error()))
		}
	}
}

func (a *App) handleInput(input string) error {
	// Handle built-in commands
	switch strings.ToLower(input) {
	case "help":
		a.printCommandHelp()
		return nil
	case "exit", "quit", "q":
		// If connected to remote host, only disconnect the session
		if a.sshClient != nil && a.sshClient.IsConnected() {
			fmt.Println("Exiting remote session...")
			return a.disconnect()
		}
		// Otherwise, exit the entire program
		a.cleanup()
		fmt.Println("Goodbye!")
		os.Exit(0)
	case "disconnect":
		return a.disconnect()
	case "status":
		a.showStatus()
		return nil
	case "history":
		return a.showHistory("")
	}

	// Try enhanced commands first
	if handled, err := a.handleEnhancedCommand(input); handled {
		return err
	}

	// Check for history command with search query
	if strings.HasPrefix(strings.ToLower(input), "history ") {
		query := strings.TrimPrefix(input, "history ")
		query = strings.TrimPrefix(query, "History ")
		return a.showHistory(query)
	}

	// Check for special prefixes
	if strings.HasPrefix(input, "connect ") || strings.HasPrefix(input, "ssh ") {
		return a.handleConnect(input)
	}

	if strings.HasPrefix(input, "$") {
		return a.handleDirectCommand(strings.TrimPrefix(input, "$"))
	}

	// Try to parse as connection request first
	if isConnectionRequest(input) {
		return a.handleConnect(input)
	}

	// Check if it's a history query in natural language
	if isHistoryRequest(input) {
		return a.handleHistoryRequest(input)
	}

	// Check if it's a hosts query in natural language
	if isHostsRequest(input) {
		return a.showHosts()
	}

	// Check if it's a whitelisted shell command - bypass LLM parsing for speed
	if isWhitelistedCommand(input) {
		return a.executeCommand(input)
	}

	// Try to parse as sherlock internal command using natural language
	if cmdInfo, err := a.agent.ParseSherlockCommand(a.ctx, input); err == nil && cmdInfo != nil {
		return a.executeSherlockCommand(cmdInfo)
	}

	// Parse as command request (works both locally and remotely)
	return a.handleCommandRequest(input)
}

func (a *App) handleConnect(input string) error {
	// Check if input is a numeric ID (connect to saved host by ID)
	trimmedInput := strings.TrimSpace(input)
	// Handle "connect <id>" pattern
	if strings.HasPrefix(strings.ToLower(trimmedInput), "connect ") {
		idStr := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(trimmedInput), "connect "))
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil && a.historyManager != nil {
			record, err := a.historyManager.GetRecordByID(id)
			if err == nil {
				return a.connectToHostWithInfo(&agent.SmartConnectionInfo{
					Type: agent.ConnectionTypeSSH,
					Host: record.Host,
					Port: record.Port,
					User: record.User,
				})
			}
		}
	}

	// Use smart connection parsing to determine connection type
	fmt.Println(a.theme.FormatInfo("Analyzing connection request..."))

	connInfo, err := a.agent.ParseSmartConnectionRequest(a.ctx, input)
	if err != nil {
		return fmt.Errorf("failed to parse connection request: %w", err)
	}

	// Route to appropriate handler based on connection type
	switch connInfo.Type {
	case agent.ConnectionTypeSSH:
		return a.connectToHostWithInfo(connInfo)
	case agent.ConnectionTypeDatabase:
		return a.connectToDatabase(connInfo)
	case agent.ConnectionTypeCache:
		return a.connectToCache(connInfo)
	default:
		// Default to SSH connection
		return a.connectToHostWithInfo(connInfo)
	}
}

// connectToDatabase connects to a MySQL database.
func (a *App) connectToDatabase(connInfo *agent.SmartConnectionInfo) error {
	fmt.Printf("%s %s@%s:%d...\n", a.theme.FormatInfo("Connecting to MySQL database"), connInfo.User, connInfo.Host, connInfo.Port)

	// Get password if not provided
	password := connInfo.Password
	if password == "" {
		fmt.Print("Enter password: ")
		pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		password = string(pwBytes)
	}

	// Create database connection
	conn := &database.Connection{
		Host:         connInfo.Host,
		Port:         connInfo.Port,
		User:         connInfo.User,
		DatabaseName: connInfo.DatabaseName,
		Alias:        connInfo.Alias,
		Description:  connInfo.Description,
	}

	client, err := database.NewClient(conn, password)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	fmt.Println(a.theme.FormatSuccess("Successfully connected to MySQL database"))

	// Record connection if manager is available
	if a.dbManager != nil {
		conn.Password = password
		savedConn, err := a.dbManager.RecordLogin(a.ctx, conn)
		if err == nil && savedConn != nil {
			// Handle group if specified
			if connInfo.Group != "" {
				// Create group if not exists
				_, err := a.dbManager.GetGroupByName(a.ctx, connInfo.Group)
				if err != nil {
					group := &database.Group{Name: connInfo.Group}
					_ = a.dbManager.CreateGroup(a.ctx, group)
				}
				// Add to group
				_ = a.dbManager.AddConnectionToGroupByName(a.ctx, savedConn.ID, connInfo.Group)
			}
			if connInfo.Alias != "" || connInfo.Group != "" {
				fmt.Printf("  已保存: [%d] %s", savedConn.ID, savedConn.DisplayName())
				if connInfo.Alias != "" {
					fmt.Printf(" 别名: %s", connInfo.Alias)
				}
				if connInfo.Group != "" {
					fmt.Printf(" 分组: %s", connInfo.Group)
				}
				fmt.Println()
			}
		}
	}

	// Start interactive shell
	shell := database.NewShell(client, a.dbManager, conn)
	return shell.Run(a.ctx)
}

// connectToCache connects to a Redis cache.
func (a *App) connectToCache(connInfo *agent.SmartConnectionInfo) error {
	fmt.Printf("%s %s:%d...\n", a.theme.FormatInfo("Connecting to Redis cache"), connInfo.Host, connInfo.Port)

	// Get password if auth is required but not provided
	password := connInfo.Password

	// Create cache connection
	conn := &cache.Connection{
		Host:          connInfo.Host,
		Port:          connInfo.Port,
		DatabaseIndex: 0,
		Alias:         connInfo.Alias,
		Description:   connInfo.Description,
	}

	client, err := cache.NewClient(conn, password)
	if err != nil {
		// If connection failed, might need password
		if password == "" && strings.Contains(err.Error(), "NOAUTH") {
			fmt.Print("Enter password: ")
			pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return fmt.Errorf("failed to read password: %w", err)
			}
			password = string(pwBytes)
			client, err = cache.NewClient(conn, password)
			if err != nil {
				return fmt.Errorf("failed to connect to cache: %w", err)
			}
		} else {
			return fmt.Errorf("failed to connect to cache: %w", err)
		}
	}

	fmt.Println(a.theme.FormatSuccess("Successfully connected to Redis cache"))

	// Record connection if manager is available
	if a.cacheManager != nil {
		conn.Password = password
		savedConn, err := a.cacheManager.RecordLogin(a.ctx, conn)
		if err == nil && savedConn != nil {
			// Handle group if specified
			if connInfo.Group != "" {
				// Create group if not exists
				_, err := a.cacheManager.GetGroupByName(a.ctx, connInfo.Group)
				if err != nil {
					group := &cache.Group{Name: connInfo.Group}
					_ = a.cacheManager.CreateGroup(a.ctx, group)
				}
				// Add to group
				_ = a.cacheManager.AddConnectionToGroupByName(a.ctx, savedConn.ID, connInfo.Group)
			}
			if connInfo.Alias != "" || connInfo.Group != "" {
				fmt.Printf("  已保存: [%d] %s", savedConn.ID, savedConn.DisplayName())
				if connInfo.Alias != "" {
					fmt.Printf(" 别名: %s", connInfo.Alias)
				}
				if connInfo.Group != "" {
					fmt.Printf(" 分组: %s", connInfo.Group)
				}
				fmt.Println()
			}
		}
	}

	// Start interactive shell
	shell := cache.NewShell(client, a.cacheManager, conn)
	return shell.Run(a.ctx)
}

// connectToHostWithInfo connects to an SSH host with full connection info including alias and group.
func (a *App) connectToHostWithInfo(connInfo *agent.SmartConnectionInfo) error {
	host := connInfo.Host
	port := connInfo.Port
	user := connInfo.User

	fmt.Printf("%s %s@%s:%d...\n", a.theme.FormatInfo("Connecting to"), user, host, port)

	// Store last host for reconnect feature
	lastHost.host = host
	lastHost.port = port
	lastHost.user = user

	hostInfo := &sshclient.HostInfo{
		Host: host,
		Port: port,
		User: user,
	}

	// Always try key-based authentication first
	fmt.Println(a.theme.FormatInfo("Attempting key-based authentication..."))
	clientCfg := &sshclient.Config{
		HostInfo:       hostInfo,
		PrivateKeyPath: a.cfg.SSHKey.PrivateKeyPath,
	}

	var keyAuthErr error
	client, err := sshclient.NewClient(clientCfg)
	if err != nil {
		keyAuthErr = err
	} else {
		if connectErr := client.Connect(a.ctx); connectErr == nil {
			// Key auth succeeded
			if a.sshClient != nil {
				_ = a.sshClient.Close()
			}
			a.sshClient = client
			fmt.Printf("%s %s\n", a.theme.FormatSuccess("Successfully connected to"), client.HostInfoString()+" using SSH key")

			// Detect remote machine context for AI command generation
			a.detectRemoteMachineContext()

			// Update AI contexts for the new host
			a.updateAIContextForHost()

			// Update history and handle alias/group
			a.saveHostWithAliasAndGroup(host, port, user, true, connInfo.Alias, connInfo.Group, connInfo.Description)
			return nil
		} else {
			keyAuthErr = connectErr
		}
	}

	// Key auth failed, show the error and fall through to password prompt
	if keyAuthErr != nil {
		fmt.Printf("%s %v\n", a.theme.FormatWarning("Key authentication failed:"), keyAuthErr)
	} else {
		fmt.Println(a.theme.FormatWarning("Key authentication failed"))
	}
	fmt.Println(a.theme.FormatInfo("Falling back to password authentication..."))

	// Key auth failed, prompt for password (use secure password input)
	fmt.Print(a.theme.FormatInfo("Password (or press Enter to cancel): "))
	pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println() // Add newline after password input
	if err != nil {
		fmt.Printf("%s %v\n", a.theme.FormatWarning("Failed to read password:"), err)
		return nil
	}
	password := strings.TrimSpace(string(pwBytes))

	if password == "" {
		fmt.Println(a.theme.FormatInfo("Connection cancelled."))
		return nil
	}

	// Create SSH client with password
	clientCfg = &sshclient.Config{
		HostInfo:       hostInfo,
		Password:       password,
		PrivateKeyPath: a.cfg.SSHKey.PrivateKeyPath,
	}

	client, err = sshclient.NewClient(clientCfg)
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %w", err)
	}

	// Connect
	if err := client.Connect(a.ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Close existing connection if any
	if a.sshClient != nil {
		_ = a.sshClient.Close()
	}

	a.sshClient = client
	fmt.Printf("%s %s\n", a.theme.FormatSuccess("Successfully connected to"), client.HostInfoString())

	// Detect remote machine context for AI command generation
	a.detectRemoteMachineContext()

	// Update AI contexts for the new host
	a.updateAIContextForHost()

	// Optionally add public key to authorized_keys
	pubKeyAdded := false
	if a.cfg.SSHKey.AutoAddToRemote {
		fmt.Println(a.theme.FormatInfo("Adding public key to remote authorized_keys..."))
		if err := client.AddPublicKeyToAuthorizedKeys(a.ctx, a.cfg.SSHKey.PublicKeyPath); err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", a.theme.FormatWarning("Warning: Failed to add public key:"), err)
		} else {
			fmt.Println(a.theme.FormatSuccess("Public key added successfully. Future connections can use key authentication."))
			pubKeyAdded = true
		}
	}

	// Update history and handle alias/group
	a.saveHostWithAliasAndGroup(host, port, user, pubKeyAdded, connInfo.Alias, connInfo.Group, connInfo.Description)

	return nil
}

// saveHostWithAliasAndGroup saves host to history with optional alias and group
func (a *App) saveHostWithAliasAndGroup(host string, port int, user string, hasPubKey bool, alias, group, description string) {
	if a.historyManager != nil {
		_ = a.historyManager.AddRecord(host, port, user, hasPubKey)
	}

	// If alias or group specified, update the host record
	if (alias != "" || group != "" || description != "") && hostsManager != nil {
		ctx := context.Background()
		// Find the host by connection info
		hostList, err := hostsManager.ListHosts(ctx, nil)
		if err == nil {
			for _, h := range hostList {
				if h.Host == host && h.Port == port && h.User == user {
					// Update alias and description
					if alias != "" {
						h.Alias = alias
					}
					if description != "" {
						h.Description = description
					}
					_ = hostsManager.UpdateHost(ctx, &h)

					// Handle group
					if group != "" {
						// Create group if not exists
						_, err := hostsManager.GetGroupByName(ctx, group)
						if err != nil {
							g := &hosts.Group{Name: group}
							_ = hostsManager.CreateGroup(ctx, g)
						}
						// Add to group
						_ = hostsManager.AddHostToGroupByName(ctx, h.ID, group)
					}

					if alias != "" || group != "" {
						fmt.Printf("  已保存: [%d] %s", h.ID, h.DisplayName())
						if alias != "" {
							fmt.Printf(" 别名: %s", alias)
						}
						if group != "" {
							fmt.Printf(" 分组: %s", group)
						}
						fmt.Println()
					}
					break
				}
			}
		}
	}
}

func (a *App) connectToHost(host string, port int, user string) error {
	return a.connectToHostWithInfo(&agent.SmartConnectionInfo{
		Type: agent.ConnectionTypeSSH,
		Host: host,
		Port: port,
		User: user,
	})
}

func (a *App) handleDirectCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}

	return a.executeCommand(cmd)
}

// commonShellCommands is a whitelist of common shell commands that can be executed directly
// without going through LLM parsing, to speed up command execution.
var commonShellCommands = map[string]bool{
	// File operations
	"ls": true, "ll": true, "la": true, "l": true,
	"cat": true, "head": true, "tail": true, "less": true, "more": true,
	"pwd": true, "cd": true,
	"cp": true, "mv": true, "rm": true, "mkdir": true, "rmdir": true,
	"touch": true, "chmod": true, "chown": true,
	"find": true, "locate": true, "which": true, "whereis": true,
	"ln": true, "file": true, "stat": true,
	"tree": true, "du": true, "df": true,
	// Text processing
	"grep": true, "egrep": true, "fgrep": true,
	"sed": true, "awk": true, "cut": true, "sort": true, "uniq": true,
	"wc": true, "diff": true, "tr": true, "tee": true,
	"xargs": true, "column": true,
	// System info
	"date": true, "cal": true, "uptime": true, "w": true, "who": true, "whoami": true,
	"hostname": true, "uname": true, "arch": true,
	"id": true, "groups": true, "users": true, "last": true, "lastlog": true,
	"env": true, "printenv": true, "set": true, "export": true,
	"locale": true, "timedatectl": true,
	// Process management
	"ps": true, "top": true, "htop": true, "pgrep": true, "pidof": true,
	"kill": true, "killall": true, "pkill": true,
	"jobs": true, "bg": true, "fg": true, "nohup": true,
	"nice": true, "renice": true, "time": true,
	// Network
	"ping": true, "curl": true, "wget": true,
	"netstat": true, "ss": true, "ip": true, "ifconfig": true,
	"nslookup": true, "dig": true, "host": true,
	"route": true, "traceroute": true, "tracepath": true,
	"telnet": true, "nc": true, "nmap": true,
	"iptables": true, "firewall-cmd": true,
	// Disk & filesystem
	"mount": true, "umount": true, "fdisk": true, "lsblk": true,
	"blkid": true, "parted": true,
	// Package management
	"apt": true, "apt-get": true, "apt-cache": true, "dpkg": true,
	"yum": true, "dnf": true, "rpm": true,
	"pacman": true, "zypper": true,
	"pip": true, "pip3": true, "npm": true, "yarn": true, "go": true,
	// Service management
	"systemctl": true, "service": true, "journalctl": true,
	"chkconfig": true,
	// Archive
	"tar": true, "gzip": true, "gunzip": true, "zip": true, "unzip": true,
	"bzip2": true, "xz": true, "7z": true,
	// Memory & performance
	"free": true, "vmstat": true, "iostat": true, "mpstat": true,
	"sar": true, "dmesg": true, "lscpu": true, "lsmem": true,
	// User management
	"useradd": true, "userdel": true, "usermod": true,
	"groupadd": true, "groupdel": true, "groupmod": true,
	"passwd": true, "chpasswd": true,
	// Other common commands
	"echo": true, "printf": true, "clear": true, "reset": true,
	"man": true, "info": true, "help": true,
	"history": true, "alias": true, "unalias": true,
	"source": true, "bash": true, "sh": true, "zsh": true,
	"sudo": true, "su": true,
	"crontab": true, "at": true,
	"git": true, "svn": true,
	"docker": true, "docker-compose": true, "kubectl": true,
	"make": true, "cmake": true, "gcc": true, "g++": true,
	"python": true, "python3": true, "java": true, "node": true,
	"vim": true, "vi": true, "nano": true, "emacs": true,
	"screen": true, "tmux": true,
	"ssh": true, "scp": true, "rsync": true, "sftp": true,
	"perl": true, "ruby": true,
	"lsof": true, "strace": true, "ltrace": true,
}

// isWhitelistedCommand checks if the input starts with a whitelisted shell command.
// Returns true if it's a whitelisted command that can be executed directly.
func isWhitelistedCommand(input string) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return false
	}

	// Extract the first word (command name)
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false
	}

	cmd := parts[0]

	// Handle sudo prefix
	if cmd == "sudo" && len(parts) > 1 {
		cmd = parts[1]
	}

	return commonShellCommands[cmd]
}

func (a *App) handleCommandRequest(input string) error {
	// Check if it's a direct command with $ prefix (bypass AI)
	isDirectCommand := strings.HasPrefix(strings.TrimSpace(input), "$")

	// Check if it's a whitelisted shell command (bypass AI for speed)
	if !isDirectCommand && isWhitelistedCommand(input) {
		// Execute whitelisted command directly without AI parsing
		return a.executeCommand(input)
	}

	// Show "thinking..." for AI-processed commands
	if !isDirectCommand {
		fmt.Print(a.theme.FormatInfo("🤔 AI Thinking..."))
	}

	// Parse command using AI (or direct execution for $ prefix)
	cmdInfo, err := a.agent.ParseCommandRequest(a.ctx, input)
	if err != nil {
		if !isDirectCommand {
			fmt.Println() // Clear the "thinking..." line
		}
		return fmt.Errorf("failed to parse command request: %w", err)
	}

	// Clear "thinking..." and show generated command
	if !isDirectCommand {
		fmt.Print("\r\033[K") // Clear the line
		if cmdInfo.Description != "" {
			fmt.Printf("%s %s\n", a.theme.FormatInfo("📝"), cmdInfo.Description)
		}
		for _, cmd := range cmdInfo.Commands {
			fmt.Printf("%s %s\n", a.theme.FormatInfo(">"), a.theme.FormatCommand(cmd))
		}
	}

	// Confirm if needed for dangerous operations
	if cmdInfo.NeedsConfirm {
		confirm, err := a.liner.Prompt(a.theme.FormatWarning("⚠️  This operation may be dangerous. Continue? [y/N]: "))
		if err != nil {
			fmt.Println(a.theme.FormatInfo("Operation cancelled."))
			return nil
		}
		confirm = strings.TrimSpace(strings.ToLower(confirm))
		if confirm != "y" && confirm != "yes" {
			fmt.Println(a.theme.FormatInfo("Operation cancelled."))
			return nil
		}
	}

	// Execute commands
	for _, cmd := range cmdInfo.Commands {
		if err := a.executeCommand(cmd); err != nil {
			return err
		}
	}

	return nil
}

func (a *App) executeCommand(cmd string) error {
	// Check if this is an interactive command that needs PTY support
	if sshclient.IsInteractiveCommand(cmd) {
		return a.executeInteractiveCommand(cmd)
	}

	var result *sshclient.ExecuteResult

	// Use SSH client if connected, otherwise use local client
	if a.sshClient != nil && a.sshClient.IsConnected() {
		result = a.sshClient.Execute(a.ctx, cmd)
	} else {
		result = a.localClient.Execute(a.ctx, cmd)
	}

	if result.Stdout != "" {
		fmt.Print(a.theme.FormatStdout(result.Stdout))
	}
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, a.theme.FormatStderr(result.Stderr))
	}

	if result.Error != nil {
		// Record command execution in memory and predictor before returning
		a.recordCommandExecution(cmd, result)
		// Run proactive analysis on error
		a.runProactiveAnalysis(cmd, result)
		return result.Error
	}

	if result.ExitCode != 0 {
		fmt.Printf("(exit code: %d)\n", result.ExitCode)
	}

	// Record command execution in agent memory and predictor
	a.recordCommandExecution(cmd, result)

	// Run proactive analysis
	a.runProactiveAnalysis(cmd, result)

	return nil
}

// recordCommandExecution records a command execution in memory and predictor.
func (a *App) recordCommandExecution(cmd string, result *sshclient.ExecuteResult) {
	output := result.Stdout
	if result.Stderr != "" {
		if output != "" {
			output += "\n"
		}
		output += result.Stderr
	}

	// Record in agent memory
	a.agent.RecordCommandExecution(cmd, output, result.ExitCode)

	// Record in predictor
	if a.agent.IsPredictionEnabled() {
		var hostCtx string
		if a.sshClient != nil && a.sshClient.IsConnected() {
			hostCtx = a.sshClient.HostInfoString()
		} else {
			hostCtx = "local"
		}
		a.agent.GetPredictor().RecordCommand(cmd, output, result.ExitCode, hostCtx)
	}
}

// runProactiveAnalysis runs proactive analysis on command output.
func (a *App) runProactiveAnalysis(cmd string, result *sshclient.ExecuteResult) {
	if a.proactiveAnalyzer == nil || !a.proactiveAnalyzer.IsEnabled() {
		return
	}

	proactiveResult := a.proactiveAnalyzer.AnalyzeCommandOutput(
		a.ctx, cmd, result.Stdout, result.Stderr, result.ExitCode,
	)

	if proactiveResult != nil && proactiveResult.ShouldShow {
		fmt.Print(analyzer.FormatProactiveResult(proactiveResult))
	}
}

// executeInteractiveCommand executes an interactive command with PTY support.
func (a *App) executeInteractiveCommand(cmd string) error {
	fmt.Println(a.theme.FormatInfo("Running in interactive mode. Press Ctrl+C to exit."))

	// Use SSH client if connected, otherwise use local client
	if a.sshClient != nil && a.sshClient.IsConnected() {
		return a.sshClient.ExecuteInteractive(a.ctx, cmd)
	}
	return a.localClient.ExecuteInteractive(a.ctx, cmd)
}

func (a *App) disconnect() error {
	if a.sshClient == nil {
		fmt.Println("Not connected to any host.")
		return nil
	}

	if err := a.sshClient.Close(); err != nil {
		return fmt.Errorf("failed to disconnect: %w", err)
	}

	a.sshClient = nil
	fmt.Println("Disconnected.")

	// Reset machine context to local
	a.resetToLocalMachineContext()

	return nil
}

// detectRemoteMachineContext detects the remote machine's OS and updates the agent context.
func (a *App) detectRemoteMachineContext() {
	if a.sshClient == nil || !a.sshClient.IsConnected() {
		return
	}

	ctx := &agent.MachineContext{
		IsRemote: true,
	}

	// Detect OS using uname
	result := a.sshClient.Execute(a.ctx, "uname -s")
	if result.Error == nil {
		osName := strings.TrimSpace(result.Stdout)
		switch strings.ToLower(osName) {
		case "linux":
			ctx.OS = "Linux"
		case "darwin":
			ctx.OS = "macOS"
		default:
			ctx.OS = osName
		}
	}

	// Detect architecture
	result = a.sshClient.Execute(a.ctx, "uname -m")
	if result.Error == nil {
		ctx.Arch = strings.TrimSpace(result.Stdout)
	}

	// Detect hostname
	result = a.sshClient.Execute(a.ctx, "hostname")
	if result.Error == nil {
		ctx.Hostname = strings.TrimSpace(result.Stdout)
	}

	// Detect Linux distribution if applicable
	if ctx.OS == "Linux" {
		result = a.sshClient.Execute(a.ctx, "cat /etc/os-release 2>/dev/null | grep -E '^(ID|VERSION_ID)=' | head -2")
		if result.Error == nil && result.Stdout != "" {
			lines := strings.Split(result.Stdout, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "ID=") {
					distro := strings.TrimPrefix(line, "ID=")
					distro = strings.Trim(distro, "\"")
					ctx.Distribution = distro
					break
				}
			}
		}
	}

	// Update agent context
	a.agent.SetMachineContext(ctx)

	// Show detected context
	fmt.Printf("%s OS: %s", a.theme.FormatInfo("🖥️  Detected:"), ctx.OS)
	if ctx.Distribution != "" {
		fmt.Printf(" (%s)", ctx.Distribution)
	}
	if ctx.Arch != "" {
		fmt.Printf(", Arch: %s", ctx.Arch)
	}
	fmt.Println()
}

// resetToLocalMachineContext resets the agent context to local machine.
func (a *App) resetToLocalMachineContext() {
	// The agent will detect local context automatically
	a.agent.SetMachineContext(nil)
}

func (a *App) showStatus() {
	fmt.Println(a.theme.FormatTableHeader("=== Sherlock Status ==="))
	fmt.Printf("%s %s\n", a.theme.FormatInfo("Version:"), version)
	fmt.Printf("%s %s\n", a.theme.FormatInfo("LLM Provider:"), a.cfg.LLM.Provider)
	fmt.Printf("%s %s\n", a.theme.FormatInfo("LLM Model:"), a.cfg.LLM.Model)
	fmt.Printf("%s %s\n", a.theme.FormatInfo("Theme:"), a.cfg.UI.Theme)

	if a.sshClient != nil && a.sshClient.IsConnected() {
		fmt.Printf("%s %s\n", a.theme.FormatInfo("Connected to:"), a.theme.FormatSuccess(a.sshClient.HostInfoString()+" (remote)"))
	} else {
		fmt.Printf("%s %s\n", a.theme.FormatInfo("Connected to:"), a.localClient.HostInfoString()+" (local)")
	}

	// AI Enhanced features status
	fmt.Println()
	fmt.Println(a.theme.FormatTableHeader("=== AI Enhanced ==="))
	cfg := &a.cfg.AIEnhanced
	boolStatus := func(b bool) string {
		if b {
			return "✓ 已启用"
		}
		return "✗ 未启用"
	}
	fmt.Printf("%s %s\n", a.theme.FormatInfo("会话记忆:"), boolStatus(cfg.EnableMemory))
	fmt.Printf("%s %s\n", a.theme.FormatInfo("主动分析:"), boolStatus(cfg.EnableProactiveAnalysis))
	fmt.Printf("%s %s\n", a.theme.FormatInfo("命令预测:"), boolStatus(cfg.EnablePrediction))
	fmt.Printf("%s %s\n", a.theme.FormatInfo("Tool Calling:"), boolStatus(cfg.EnableToolCalling))

	if a.agent.IsMemoryEnabled() {
		mem := a.agent.GetMemory()
		fmt.Printf("%s 消息: %d, 命令: %d\n", a.theme.FormatInfo("  记忆状态:"), mem.MessageCount(), mem.CommandCount())
	}
}

func (a *App) cleanup() {
	if a.sshClient != nil {
		_ = a.sshClient.Close()
	}
	if a.aiClient != nil {
		_ = a.aiClient.Close()
	}
	if a.historyManager != nil {
		_ = a.historyManager.Close()
	}
	a.cancel()
}

// initAIEnhancedFeatures initializes AI enhanced features based on configuration.
func (a *App) initAIEnhancedFeatures() {
	cfg := &a.cfg.AIEnhanced

	// Initialize conversation memory
	if cfg.EnableMemory {
		memCfg := &agent.MemoryConfig{
			MaxMessages:       cfg.MemoryWindowSize,
			MaxCommandHistory: cfg.MaxCommandHistory,
			EnablePersistence: false, // Use in-memory for now
		}
		if memCfg.MaxMessages == 0 {
			memCfg.MaxMessages = 20
		}
		if memCfg.MaxCommandHistory == 0 {
			memCfg.MaxCommandHistory = 50
		}
		memory, err := agent.NewConversationMemory(nil, memCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to initialize conversation memory: %v\n", err)
		} else {
			a.agent.SetMemory(memory)
		}
	}

	// Initialize command predictor
	if cfg.EnablePrediction {
		predCfg := &agent.PredictorConfig{
			MaxHistory:    cfg.MaxCommandHistory,
			CacheValidFor: 30 * 1000000000, // 30 seconds in nanoseconds
			Enabled:       true,
		}
		if predCfg.MaxHistory == 0 {
			predCfg.MaxHistory = 20
		}
		predictor := agent.NewCommandPredictor(a.aiClient, predCfg)
		a.agent.SetPredictor(predictor)
	}

	// Initialize proactive analyzer
	if cfg.EnableProactiveAnalysis {
		analyzerCfg := &analyzer.ProactiveAnalyzerConfig{
			Enabled:          true,
			AnalyzeOnError:   cfg.AnalyzeOnError,
			AnalyzeOnWarning: cfg.AnalyzeOnWarning,
		}
		a.proactiveAnalyzer = analyzer.NewProactiveAnalyzer(a.aiClient, analyzerCfg)
	}

	// Initialize tool registry (only if tool calling is enabled)
	if cfg.EnableToolCalling {
		// Tool registry needs an SSH executor; set up with local client initially
		a.toolRegistry = agent.NewToolRegistry(nil)
		a.agent.SetToolRegistry(a.toolRegistry)
		// Set confirmation function
		a.toolRegistry.SetConfirmFunc(func(msg string) bool {
			confirm, err := a.liner.Prompt(a.theme.FormatWarning(fmt.Sprintf("⚠️  AI wants to: %s. Allow? [y/N]: ", msg)))
			if err != nil {
				return false
			}
			confirm = strings.TrimSpace(strings.ToLower(confirm))
			return confirm == "y" || confirm == "yes"
		})
	}

	// Print AI enhanced features status
	var enabledFeatures []string
	if cfg.EnableMemory {
		enabledFeatures = append(enabledFeatures, "会话记忆")
	}
	if cfg.EnableProactiveAnalysis {
		enabledFeatures = append(enabledFeatures, "主动分析")
	}
	if cfg.EnablePrediction {
		enabledFeatures = append(enabledFeatures, "命令预测")
	}
	if cfg.EnableToolCalling {
		enabledFeatures = append(enabledFeatures, "Tool Calling")
	}
	if len(enabledFeatures) > 0 {
		fmt.Printf("%s AI 增强: %s\n", a.theme.FormatInfo("🧠"), strings.Join(enabledFeatures, ", "))
	}
}

// updateAIContextForHost updates AI-related contexts when connecting to a new host.
func (a *App) updateAIContextForHost() {
	hostInfo := ""
	if a.sshClient != nil && a.sshClient.IsConnected() {
		hostInfo = a.sshClient.HostInfoString()
	}

	// Update memory host context
	a.agent.UpdateHostContext(hostInfo)

	// Update tool registry executor with SSH client adapter
	if a.toolRegistry != nil && a.sshClient != nil {
		a.toolRegistry.SetExecutor(&sshClientAdapter{client: a.sshClient})
	}

	// Update predictor machine context
	if a.agent.IsPredictionEnabled() && a.agent.GetMachineContext() != nil {
		a.agent.GetPredictor().SetMachineContext(a.agent.GetMachineContext())
	}
}

// sshClientAdapter adapts sshclient.Client to the agent.SSHExecutor interface.
type sshClientAdapter struct {
	client *sshclient.Client
}

func (a *sshClientAdapter) Execute(ctx context.Context, command string) *agent.ExecuteResult {
	result := a.client.Execute(ctx, command)
	return &agent.ExecuteResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
		Error:    result.Error,
	}
}

func (a *sshClientAdapter) IsConnected() bool {
	return a.client.IsConnected()
}

func (a *sshClientAdapter) HostInfoString() string {
	return a.client.HostInfoString()
}

// completer provides tab completion for commands and file paths.
func (a *App) completer(line string) []string {
	// Get current working directory
	var cwd string
	if a.sshClient != nil && a.sshClient.IsConnected() {
		cwd = a.sshClient.GetCwd()
	} else {
		cwd = a.localClient.GetCwd()
	}

	// Split the line to find the last token
	parts := strings.Fields(line)
	if len(parts) == 0 {
		// Complete shell commands when line is empty
		return a.completeCommands("")
	}

	// Check if we're completing the first word (command) or arguments
	lastPart := parts[len(parts)-1]
	
	// If the line ends with a space, we're starting a new token (likely a path)
	if strings.HasSuffix(line, " ") {
		// Complete file paths from current directory
		return a.completeFilePaths(cwd, "", line)
	}

	// If there's only one part, complete as command
	if len(parts) == 1 {
		return a.completeCommands(lastPart)
	}

	// Otherwise, complete as file path
	return a.completeFilePaths(cwd, lastPart, line)
}

// completeCommands returns completions for shell commands.
func (a *App) completeCommands(prefix string) []string {
	// Built-in commands and shortcuts
	builtinCommands := []string{
		"help", "exit", "quit", "disconnect", "status", "history", "connect",
		"list",                                 // Unified list
		"host", "hl", "ha", "hr", "hc",         // Host commands
		"db", "dl", "da", "dr", "dc",           // Database commands
		"cache", "cl", "ca", "cr", "cc",        // Cache commands
		"batch", "upload", "download", "check", "monitor",
	}

	// Common shell commands
	shellCommands := []string{
		"pwd", "mkdir", "rmdir", "rm", "cp", "mv",
		"touch", "cat", "head", "tail", "less", "more", "find", "grep",
		"ps", "top", "htop", "df", "du", "free", "uname", "hostname",
		"ping", "curl", "wget", "ssh", "scp", "rsync",
		"git", "docker", "kubectl", "systemctl", "service",
		"vim", "nano", "vi", "echo", "export", "env", "clear",
	}

	allCommands := append(builtinCommands, shellCommands...)
	var completions []string

	prefix = strings.ToLower(prefix)
	for _, cmd := range allCommands {
		if strings.HasPrefix(cmd, prefix) {
			completions = append(completions, cmd)
		}
	}

	return completions
}

// completeFilePaths returns completions for file paths.
func (a *App) completeFilePaths(cwd, prefix, fullLine string) []string {
	var completions []string

	// Determine the directory to search and the file prefix
	searchDir := cwd
	filePrefix := prefix

	// Handle absolute paths
	if strings.HasPrefix(prefix, "/") {
		searchDir = filepath.Dir(prefix)
		filePrefix = filepath.Base(prefix)
		if prefix == "/" || strings.HasSuffix(prefix, "/") {
			searchDir = prefix
			if strings.HasSuffix(searchDir, "/") && searchDir != "/" {
				searchDir = searchDir[:len(searchDir)-1]
			}
			filePrefix = ""
		}
	} else if strings.HasPrefix(prefix, "~/") {
		// Handle home directory
		homeDir, err := os.UserHomeDir()
		if err == nil {
			expandedPrefix := filepath.Join(homeDir, prefix[2:])
			searchDir = filepath.Dir(expandedPrefix)
			filePrefix = filepath.Base(expandedPrefix)
			if strings.HasSuffix(prefix, "/") {
				searchDir = expandedPrefix
				if strings.HasSuffix(searchDir, "/") {
					searchDir = searchDir[:len(searchDir)-1]
				}
				filePrefix = ""
			}
		}
	} else if strings.Contains(prefix, "/") {
		// Handle relative path with directories
		searchDir = filepath.Join(cwd, filepath.Dir(prefix))
		filePrefix = filepath.Base(prefix)
		if strings.HasSuffix(prefix, "/") {
			searchDir = filepath.Join(cwd, prefix)
			if strings.HasSuffix(searchDir, "/") {
				searchDir = searchDir[:len(searchDir)-1]
			}
			filePrefix = ""
		}
	}

	// If connected to remote, use remote file listing
	if a.sshClient != nil && a.sshClient.IsConnected() {
		return a.completeRemoteFilePaths(searchDir, filePrefix, prefix, fullLine)
	}

	// Local file completion
	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return completions
	}

	// Build the prefix for the full line (everything before the file we're completing)
	linePrefix := fullLine
	if len(prefix) > 0 {
		linePrefix = strings.TrimSuffix(fullLine, prefix)
	}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, filePrefix) {
			var completion string
			if strings.HasPrefix(prefix, "/") {
				completion = filepath.Join(searchDir, name)
			} else if strings.HasPrefix(prefix, "~/") {
				homeDir, _ := os.UserHomeDir()
				relPath, _ := filepath.Rel(homeDir, filepath.Join(searchDir, name))
				completion = "~/" + relPath
			} else if strings.Contains(prefix, "/") {
				completion = filepath.Join(filepath.Dir(prefix), name)
			} else {
				completion = name
			}
			
			// Add trailing slash for directories
			if entry.IsDir() {
				completion += "/"
			}
			
			completions = append(completions, linePrefix+completion)
		}
	}

	return completions
}

// completeRemoteFilePaths returns completions for remote file paths.
func (a *App) completeRemoteFilePaths(searchDir, filePrefix, prefix, fullLine string) []string {
	var completions []string

	// Execute ls command on remote host to get file list
	result := a.sshClient.Execute(a.ctx, fmt.Sprintf("ls -1a %s 2>/dev/null", sshclient.ShellEscape(searchDir)))
	if result.Error != nil || result.ExitCode != 0 {
		return completions
	}

	// Build the prefix for the full line (everything before the file we're completing)
	linePrefix := fullLine
	if len(prefix) > 0 {
		linePrefix = strings.TrimSuffix(fullLine, prefix)
	}

	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	for _, name := range lines {
		name = strings.TrimSpace(name)
		if name == "" || name == "." || name == ".." {
			continue
		}
		if strings.HasPrefix(name, filePrefix) {
			var completion string
			if strings.HasPrefix(prefix, "/") {
				completion = filepath.Join(searchDir, name)
			} else if strings.Contains(prefix, "/") {
				completion = filepath.Join(filepath.Dir(prefix), name)
			} else {
				completion = name
			}
			
			// Check if it's a directory (execute stat on remote)
			checkPath := filepath.Join(searchDir, name)
			checkResult := a.sshClient.Execute(a.ctx, fmt.Sprintf("test -d %s && echo 'dir'", sshclient.ShellEscape(checkPath)))
			if strings.TrimSpace(checkResult.Stdout) == "dir" {
				completion += "/"
			}
			
			completions = append(completions, linePrefix+completion)
		}
	}

	return completions
}

func isConnectionRequest(input string) bool {
	lower := strings.ToLower(input)
	keywords := []string{"connect", "ssh", "login", "log in", "连接", "登录", "登陆"}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	// Check for user@host pattern
	if strings.Contains(input, "@") {
		return true
	}
	// Check for valid IP address pattern
	if containsValidIP(input) {
		return true
	}
	return false
}

// containsValidIP checks if the input contains a valid IPv4 address.
func containsValidIP(input string) bool {
	// Find potential IP address patterns
	ipPattern := regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
	matches := ipPattern.FindAllString(input, -1)
	for _, match := range matches {
		if net.ParseIP(match) != nil {
			return true
		}
	}
	return false
}

func isHistoryRequest(input string) bool {
	lower := strings.ToLower(input)
	keywords := []string{"history", "历史", "登录记录", "login history", "connection history", "show history", "list history", "查看历史", "显示历史"}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func isHostsRequest(input string) bool {
	lower := strings.ToLower(input)
	keywords := []string{"host", "hosts", "主机", "服务器", "saved hosts", "show hosts", "list hosts", "查看主机", "显示主机", "all hosts"}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func (a *App) showHistory(query string) error {
	if a.historyManager == nil {
		fmt.Println(a.theme.FormatWarning("History feature is not available."))
		return nil
	}

	var records []history.Record
	if query == "" {
		records = a.historyManager.GetRecords()
	} else {
		records = a.historyManager.SearchRecords(query)
	}

	// Convert to theme records
	themeRecords := make([]theme.HistoryRecord, len(records))
	for i, r := range records {
		themeRecords[i] = theme.HistoryRecord{
			ID:         r.ID,
			HostKey:    r.HostKey(),
			LoginCount: r.LoginCount,
			Timestamp:  r.Timestamp.Format("2006-01-02 15:04:05"),
			HasPubKey:  r.HasPubKey,
		}
	}

	fmt.Print(a.theme.FormatHistoryRecords(themeRecords))
	return nil
}

func (a *App) showHosts() error {
	if a.historyManager == nil {
		fmt.Println(a.theme.FormatWarning("Hosts feature is not available."))
		return nil
	}

	records := a.historyManager.GetRecords()

	// Convert to theme records
	themeRecords := make([]theme.HistoryRecord, len(records))
	for i, r := range records {
		themeRecords[i] = theme.HistoryRecord{
			ID:         r.ID,
			HostKey:    r.HostKey(),
			LoginCount: r.LoginCount,
			Timestamp:  r.Timestamp.Format("2006-01-02 15:04:05"),
			HasPubKey:  r.HasPubKey,
		}
	}

	fmt.Print(a.theme.FormatHostsSimple(themeRecords))
	return nil
}

func (a *App) handleHistoryRequest(input string) error {
	// Extract any search query from the natural language input
	lower := strings.ToLower(input)

	// Try to extract a search term
	searchPrefixes := []string{"search for ", "find ", "query ", "look for ", "搜索", "查找"}
	var query string
	for _, prefix := range searchPrefixes {
		if idx := strings.Index(lower, prefix); idx != -1 {
			query = strings.TrimSpace(input[idx+len(prefix):])
			break
		}
	}

	return a.showHistory(query)
}

// handleHostsCommand handles the 'sherlock hosts' subcommand.
func handleHostsCommand() {
	historyMgr, err := history.NewManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to initialize history manager: %v\n", err)
		os.Exit(1)
	}
	defer historyMgr.Close()

	records := historyMgr.GetRecords()

	// Use default theme for subcommand output
	t := theme.DefaultTheme()
	themeRecords := make([]theme.HistoryRecord, len(records))
	for i, r := range records {
		themeRecords[i] = theme.HistoryRecord{
			ID:         r.ID,
			HostKey:    r.HostKey(),
			LoginCount: r.LoginCount,
			Timestamp:  r.Timestamp.Format("2006-01-02 15:04:05"),
			HasPubKey:  r.HasPubKey,
		}
	}
	fmt.Print(t.FormatHostsSimple(themeRecords))
}

func (a *App) printBanner() {
	banner := `
  _____ _    _ ______ _____  _      ____   _____ _  __
 / ____| |  | |  ____|  __ \| |    / __ \ / ____| |/ /
| (___ | |__| | |__  | |__) | |   | |  | | |    | ' / 
 \___ \|  __  |  __| |  _  /| |   | |  | | |    |  <  
 ____) | |  | | |____| | \ \| |___| |__| | |____| . \ 
|_____/|_|  |_|______|_|  \_\______\____/ \_____|_|\_\
`
	subtitle := "AI-powered SSH Remote Operations Tool"

	fmt.Println(a.theme.FormatBanner(banner))
	fmt.Println(a.theme.FormatBannerSubtitle(subtitle))
}

func printHelp() {
	fmt.Printf(`%s - %s

Usage: sherlock [options] [command]

Interactive Mode:
  sherlock                           Start interactive mode with default config
  sherlock --provider ollama         Use Ollama as LLM provider

Non-Interactive Mode:
  sherlock -e <command>              Execute command and exit
  sherlock -e <command> -o json      Execute and output JSON
  sherlock -n -e "hl"                List hosts in non-interactive mode

Commands:
  host                    Show all saved hosts

Options:
  -c, --config <path>     Path to configuration file
  -v, --version           Show version information
  -h, --help              Show this help message
  --provider <provider>   LLM provider (ollama, openai, deepseek, openai_compatible)
  --model <model>         Model name
  --base-url <url>        Base URL for LLM API
  --api-key <key>         API key for LLM provider
  
Non-Interactive Options:
  -e, --exec <cmd>        Execute command in non-interactive mode
  -o, --output <format>   Output format: text (default), json
  -n, --non-interactive   Run in non-interactive mode

Non-Interactive Commands:
  hl, host                List all SSH hosts
  dl, db                  List all database connections
  cl, cache               List all cache connections
  tunnel list             List active tunnels
  playbook list           List available playbooks
  snippet list            List saved snippets
  batch <cmd> --all       Execute command on all hosts
  health [host]           Run health check
  analyze <cmd>           Execute and analyze output
  diagnose <error>        Diagnose error message
  advisor <query>         Get AI operational advice

Examples:
  sherlock -e "hl"                       List hosts
  sherlock -e "hl" -o json               List hosts as JSON
  sherlock -e "batch uptime --all"       Run uptime on all hosts
  sherlock -e "analyze df -h"            Analyze disk usage
  sherlock -e "health"                   Health check all hosts

For more information, visit: https://github.com/warm3snow/Sherlock
`, appName, description)
}

func (a *App) printCommandHelp() {
	fmt.Println()
	fmt.Println(a.theme.FormatTableHeader("🎯 核心功能 (演示亮点):"))
	fmt.Printf("  %s              %s\n", a.theme.FormatCommand("advisor"), a.theme.FormatDescription("🤖 AI智能运维助手 - 主动发现问题并给出优化建议"))
	fmt.Printf("  %s              %s\n", a.theme.FormatCommand("inspect"), a.theme.FormatDescription("🏥 一键健康巡检 - 批量检查所有主机并生成报告"))
	fmt.Printf("  %s             %s\n", a.theme.FormatCommand("quickfix"), a.theme.FormatDescription("⚡ 快速故障恢复 - AI诊断+一键修复"))
	fmt.Printf("  %s             %s\n", a.theme.FormatCommand("playbook"), a.theme.FormatDescription("📜 自动化运维剧本 - 预定义操作一键执行"))
	fmt.Printf("  %s            %s\n", a.theme.FormatCommand("dashboard"), a.theme.FormatDescription("📊 可视化仪表盘 - 多主机状态总览"))
	fmt.Printf("  %s                %s\n", a.theme.FormatCommand("audit"), a.theme.FormatDescription("📝 操作审计日志 - 完整追溯+合规报表"))

	fmt.Println()
	fmt.Println(a.theme.FormatTableHeader("基础命令:"))
	fmt.Printf("  %s                        %s\n", a.theme.FormatCommand("help"), a.theme.FormatDescription("显示帮助"))
	fmt.Printf("  %s                 %s\n", a.theme.FormatCommand("exit, quit"), a.theme.FormatDescription("退出"))
	fmt.Printf("  %s                      %s\n", a.theme.FormatCommand("status"), a.theme.FormatDescription("显示状态"))
	fmt.Printf("  %s                        %s\n", a.theme.FormatCommand("list"), a.theme.FormatDescription("列出所有连接"))

	fmt.Println()
	fmt.Println(a.theme.FormatTableHeader("主机管理 (快捷键: hl/ha/hr/hc):"))
	fmt.Printf("  %s                 %s\n", a.theme.FormatCommand("host, hl"), a.theme.FormatDescription("列出主机"))
	fmt.Printf("  %s               %s\n", a.theme.FormatCommand("hc <id>"), a.theme.FormatDescription("连接主机"))
	fmt.Printf("  %s        %s\n", a.theme.FormatCommand("ha <user@host:port>"), a.theme.FormatDescription("添加主机"))
	fmt.Printf("  %s               %s\n", a.theme.FormatCommand("hr <id>"), a.theme.FormatDescription("删除主机"))

	fmt.Println()
	fmt.Println(a.theme.FormatTableHeader("批量操作与文件传输:"))
	fmt.Printf("  %s     %s\n", a.theme.FormatCommand("batch <cmd> --group=<g>"), a.theme.FormatDescription("批量执行命令"))
	fmt.Printf("  %s   %s\n", a.theme.FormatCommand("upload <local> <remote>"), a.theme.FormatDescription("上传文件"))
	fmt.Printf("  %s %s\n", a.theme.FormatCommand("download <remote> <local>"), a.theme.FormatDescription("下载文件"))

	fmt.Println()
	fmt.Println(a.theme.FormatTableHeader("AI分析与诊断:"))
	fmt.Printf("  %s            %s\n", a.theme.FormatCommand("analyze <cmd>"), a.theme.FormatDescription("执行命令并AI分析输出"))
	fmt.Printf("  %s           %s\n", a.theme.FormatCommand("diagnose <cmd>"), a.theme.FormatDescription("诊断错误并建议修复"))

	fmt.Println()
	fmt.Println(a.theme.FormatTableHeader("🧠 AI 增强功能:"))
	fmt.Printf("  %s      %s\n", a.theme.FormatCommand("predict, pred"), a.theme.FormatDescription("查看命令预测建议"))
	fmt.Printf("  %s       %s\n", a.theme.FormatCommand("memory, mem"), a.theme.FormatDescription("会话记忆管理 (clear/new/history)"))
	fmt.Printf("  %s  %s\n", a.theme.FormatCommand("playbook generate"), a.theme.FormatDescription("AI 生成运维剧本"))
	fmt.Printf("  %s  %s\n", a.theme.FormatCommand("playbook template"), a.theme.FormatDescription("查看可用剧本模板"))
	fmt.Printf("  %s   %s\n", a.theme.FormatCommand("playbook improve"), a.theme.FormatDescription("AI 分析改进剧本"))

	fmt.Println()
	fmt.Println(a.theme.FormatTableHeader("其他功能:"))
	fmt.Printf("  %s                  %s\n", a.theme.FormatCommand("db, cache"), a.theme.FormatDescription("数据库/缓存连接管理"))
	fmt.Printf("  %s            %s\n", a.theme.FormatCommand("snippet, snip"), a.theme.FormatDescription("命令片段管理"))
	fmt.Printf("  %s            %s\n", a.theme.FormatCommand("session, sess"), a.theme.FormatDescription("多会话管理"))
	fmt.Printf("  %s              %s\n", a.theme.FormatCommand("tunnel, tun"), a.theme.FormatDescription("SSH隧道管理"))
	fmt.Printf("  %s          %s\n", a.theme.FormatCommand("import ssh-config"), a.theme.FormatDescription("从SSH配置导入主机"))

	fmt.Println()
	fmt.Println(a.theme.FormatTableHeader("命令执行:"))
	fmt.Printf("  %s                  %s\n", a.theme.FormatCommand("$<command>"), a.theme.FormatDescription("直接执行命令"))
	fmt.Printf("  %s\n", a.theme.FormatDescription("或使用自然语言，如 \"查看磁盘使用情况\""))
}

// executeSherlockCommand executes a parsed sherlock internal command.
func (a *App) executeSherlockCommand(cmdInfo *agent.SherlockCommandInfo) error {
	// Build the command string based on parsed info
	var args []string

	switch cmdInfo.Command {
	case "host":
		args = append(args, cmdInfo.Action)
		args = append(args, cmdInfo.Args...)
		return a.handleHostsEnhanced(args)

	case "db":
		args = append(args, cmdInfo.Action)
		args = append(args, cmdInfo.Args...)
		return a.handleDatabase(args)

	case "cache":
		args = append(args, cmdInfo.Action)
		args = append(args, cmdInfo.Args...)
		return a.handleCache(args)

	case "check":
		return a.handleCheck(cmdInfo.Args)

	case "connect":
		if len(cmdInfo.Args) > 0 {
			return a.handleConnect("connect " + cmdInfo.Args[0])
		}
		return fmt.Errorf("connect requires a host ID or alias")

	case "batch":
		return a.handleBatch(cmdInfo.Args)

	case "upload":
		return a.handleUpload(cmdInfo.Args)

	case "download":
		return a.handleDownload(cmdInfo.Args)

	default:
		return fmt.Errorf("unknown sherlock command: %s", cmdInfo.Command)
	}
}
