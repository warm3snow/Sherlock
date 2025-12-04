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
	"bufio"
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/warm3snow/Sherlock/internal/agent"
	"github.com/warm3snow/Sherlock/internal/ai"
	"github.com/warm3snow/Sherlock/internal/config"
	"github.com/warm3snow/Sherlock/internal/history"
	"github.com/warm3snow/Sherlock/pkg/sshclient"
)

const (
	version     = "0.1.0"
	appName     = "Sherlock"
	description = "AI-powered SSH remote operations tool"
)

// App represents the Sherlock application.
type App struct {
	cfg            *config.Config
	aiClient       ai.ModelClient
	agent          *agent.Agent
	sshClient      *sshclient.Client
	historyManager *history.Manager
  localClient  *sshclient.LocalClient
	ctx            context.Context
	cancel         context.CancelFunc
}

func main() {
	// Check for subcommands first
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hosts":
			handleHostsCommand()
			return
		}
	}

	var (
		configPath    string
		showVersion   bool
		showHelp      bool
		providerFlag  string
		modelFlag     string
		baseURLFlag   string
		apiKeyFlag    string
	)

	flag.StringVar(&configPath, "config", "", "Path to configuration file")
	flag.StringVar(&configPath, "c", "", "Path to configuration file (shorthand)")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.BoolVar(&showVersion, "v", false, "Show version information (shorthand)")
	flag.BoolVar(&showHelp, "help", false, "Show help information")
	flag.BoolVar(&showHelp, "h", false, "Show help information (shorthand)")
	flag.StringVar(&providerFlag, "provider", "", "LLM provider (ollama, openai, deepseek)")
	flag.StringVar(&modelFlag, "model", "", "Model name")
	flag.StringVar(&baseURLFlag, "base-url", "", "Base URL for LLM API")
	flag.StringVar(&apiKeyFlag, "api-key", "", "API key for LLM provider")
	flag.Parse()

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

	app := &App{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
	}

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, cleaning up...")
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

	// Initialize history manager
	historyMgr, err := history.NewManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize history manager: %v\n", err)
	}
	app.historyManager = historyMgr
	// Initialize local client for local command execution
	app.localClient = sshclient.NewLocalClient()

	// Run the application
	if err := app.run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		app.cleanup()
		os.Exit(1)
	}

	app.cleanup()
}

func (a *App) run() error {
	printBanner()
	fmt.Println("Type 'help' for available commands or describe what you want to do.")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	for {
		prompt := fmt.Sprintf("sherlock[%s]> ", a.localClient.HostInfoString())
		if a.sshClient != nil && a.sshClient.IsConnected() {
			prompt = fmt.Sprintf("sherlock[%s]> ", a.sshClient.HostInfoString())
		}

		fmt.Print(prompt)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		if err := a.handleInput(input); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
}

func (a *App) handleInput(input string) error {
	// Handle built-in commands
	switch strings.ToLower(input) {
	case "help":
		printCommandHelp()
		return nil
	case "exit", "quit", "q":
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
	case "hosts":
		return a.showHosts()
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

	// Check if connected
	if a.sshClient == nil || !a.sshClient.IsConnected() {
		// Try to parse as connection request
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

		fmt.Println("Not connected to any host. Use 'connect <host>' or describe a connection.")
		return nil
	}

	// Try to parse as connection request first
	if isConnectionRequest(input) {
		return a.handleConnect(input)
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
				return a.connectToHost(record.Host, record.Port, record.User)
			}
		}
	}

	// Parse connection request using AI
	fmt.Println("Parsing connection request...")

	connInfo, err := a.agent.ParseConnectionRequest(a.ctx, input)
	if err != nil {
		return fmt.Errorf("failed to parse connection request: %w", err)
	}

	return a.connectToHost(connInfo.Host, connInfo.Port, connInfo.User)
}

func (a *App) connectToHost(host string, port int, user string) error {
	fmt.Printf("Connecting to %s@%s:%d...\n", user, host, port)

	hostInfo := &sshclient.HostInfo{
		Host: host,
		Port: port,
		User: user,
	}

	// Always try key-based authentication first
	fmt.Println("Attempting key-based authentication...")
	clientCfg := &sshclient.Config{
		HostInfo:       hostInfo,
		PrivateKeyPath: a.cfg.SSHKey.PrivateKeyPath,
	}

	client, err := sshclient.NewClient(clientCfg)
	if err == nil {
		if err := client.Connect(a.ctx); err == nil {
			// Key auth succeeded
			if a.sshClient != nil {
				_ = a.sshClient.Close()
			}
			a.sshClient = client
			fmt.Printf("Successfully connected to %s using SSH key\n", client.HostInfoString())

			// Update history
			if a.historyManager != nil {
				_ = a.historyManager.AddRecord(host, port, user, true)
			}
			return nil
		}
		// Key auth failed, will fall through to password prompt
		fmt.Println("Key authentication failed, falling back to password...")
	}

	// Key auth failed, prompt for password
	fmt.Print("Password (or press Enter to cancel): ")
	reader := bufio.NewReader(os.Stdin)
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	if password == "" {
		fmt.Println("Connection cancelled.")
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
	fmt.Printf("Successfully connected to %s\n", client.HostInfoString())

	// Optionally add public key to authorized_keys
	pubKeyAdded := false
	if a.cfg.SSHKey.AutoAddToRemote {
		fmt.Println("Adding public key to remote authorized_keys...")
		if err := client.AddPublicKeyToAuthorizedKeys(a.ctx, a.cfg.SSHKey.PublicKeyPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to add public key: %v\n", err)
		} else {
			fmt.Println("Public key added successfully. Future connections can use key authentication.")
			pubKeyAdded = true
		}
	}

	// Update history
	if a.historyManager != nil {
		_ = a.historyManager.AddRecord(host, port, user, pubKeyAdded)
	}

	return nil
}

func (a *App) handleDirectCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}

	return a.executeCommand(cmd)
}

func (a *App) handleCommandRequest(input string) error {
	// Parse command using AI
	cmdInfo, err := a.agent.ParseCommandRequest(a.ctx, input)
	if err != nil {
		return fmt.Errorf("failed to parse command request: %w", err)
	}

	fmt.Printf("Commands to execute:\n")
	for i, cmd := range cmdInfo.Commands {
		fmt.Printf("  %d. %s\n", i+1, cmd)
	}
	fmt.Printf("Description: %s\n", cmdInfo.Description)

	// Confirm if needed
	if cmdInfo.NeedsConfirm {
		fmt.Print("\n⚠️  This operation may be dangerous. Continue? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(strings.ToLower(confirm))
		if confirm != "y" && confirm != "yes" {
			fmt.Println("Operation cancelled.")
			return nil
		}
	}

	// Execute commands
	for _, cmd := range cmdInfo.Commands {
		fmt.Printf("\n$ %s\n", cmd)
		if err := a.executeCommand(cmd); err != nil {
			return err
		}
	}

	return nil
}

func (a *App) executeCommand(cmd string) error {
	var result *sshclient.ExecuteResult

	// Use SSH client if connected, otherwise use local client
	if a.sshClient != nil && a.sshClient.IsConnected() {
		result = a.sshClient.Execute(a.ctx, cmd)
	} else {
		result = a.localClient.Execute(a.ctx, cmd)
	}

	if result.Stdout != "" {
		fmt.Print(result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintf(os.Stderr, "%s", result.Stderr)
	}

	if result.Error != nil {
		return result.Error
	}

	if result.ExitCode != 0 {
		fmt.Printf("(exit code: %d)\n", result.ExitCode)
	}

	return nil
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
	return nil
}

func (a *App) showStatus() {
	fmt.Println("=== Sherlock Status ===")
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("LLM Provider: %s\n", a.cfg.LLM.Provider)
	fmt.Printf("LLM Model: %s\n", a.cfg.LLM.Model)

	if a.sshClient != nil && a.sshClient.IsConnected() {
		fmt.Printf("Connected to: %s (remote)\n", a.sshClient.HostInfoString())
	} else {
		fmt.Printf("Connected to: %s (local)\n", a.localClient.HostInfoString())
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
	keywords := []string{"hosts", "主机", "服务器", "saved hosts", "show hosts", "list hosts", "查看主机", "显示主机", "all hosts"}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func (a *App) showHistory(query string) error {
	if a.historyManager == nil {
		fmt.Println("History feature is not available.")
		return nil
	}

	var records []history.Record
	if query == "" {
		records = a.historyManager.GetRecords()
	} else {
		records = a.historyManager.SearchRecords(query)
	}

	fmt.Print(history.FormatRecords(records))
	return nil
}

func (a *App) showHosts() error {
	if a.historyManager == nil {
		fmt.Println("Hosts feature is not available.")
		return nil
	}

	records := a.historyManager.GetRecords()
	fmt.Print(history.FormatHostsSimple(records))
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
	fmt.Print(history.FormatHostsSimple(records))
}

func printBanner() {
	fmt.Print(`
  _____ _    _ ______ _____  _      ____   _____ _  __
 / ____| |  | |  ____|  __ \| |    / __ \ / ____| |/ /
| (___ | |__| | |__  | |__) | |   | |  | | |    | ' / 
 \___ \|  __  |  __| |  _  /| |   | |  | | |    |  <  
 ____) | |  | | |____| | \ \| |___| |__| | |____| . \ 
|_____/|_|  |_|______|_|  \_\______\____/ \_____|_|\_\
                                                      
AI-powered SSH Remote Operations Tool
`)
}

func printHelp() {
	fmt.Printf(`%s - %s

Usage: sherlock [options] [command]

Commands:
  hosts                   Show all saved hosts

Options:
  -c, --config <path>     Path to configuration file
  -v, --version           Show version information
  -h, --help              Show this help message
  --provider <provider>   LLM provider (ollama, openai, deepseek)
  --model <model>         Model name
  --base-url <url>        Base URL for LLM API
  --api-key <key>         API key for LLM provider

Examples:
  sherlock                           Start interactive mode with default config
  sherlock hosts                     Show all saved hosts
  sherlock --provider ollama         Use Ollama as LLM provider
  sherlock -c ~/.config/sherlock/config.json

For more information, visit: https://github.com/warm3snow/Sherlock
`, appName, description)
}

func printCommandHelp() {
	fmt.Print(`
Available commands:
  help                    Show this help message
  exit, quit, q           Exit Sherlock
  status                  Show current status
  hosts                   Show all saved hosts
  history                 Show login history
  history <query>         Search login history
  disconnect              Disconnect from remote host (switch to local mode)

Connection:
  connect <host>          Connect to a remote host
  connect <id>            Connect to a saved host by ID
  ssh user@host:port      Connect using SSH-like syntax
  Or describe in natural language, e.g., "connect to server 192.168.1.100 as root"
  Note: If you have logged in before with SSH key, no password will be required.

Hosts:
  hosts                   Show all saved hosts with IDs
  Or use natural language, e.g., "show my hosts" or "显示主机"

History:
  history                 Show all login history
  history <query>         Search history by host, user, or pattern
  Or use natural language, e.g., "show my login history"

Commands (local or remote):
  $<command>              Execute a command directly, e.g., $ls -la
  Or describe in natural language, e.g., "show me disk usage"

Note: When not connected to a remote host, commands are executed locally.
`)
}
