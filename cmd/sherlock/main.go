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
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/warm3snow/Sherlock/internal/agent"
	"github.com/warm3snow/Sherlock/internal/ai"
	"github.com/warm3snow/Sherlock/internal/config"
	"github.com/warm3snow/Sherlock/pkg/sshclient"
)

const (
	version     = "0.1.0"
	appName     = "Sherlock"
	description = "AI-powered SSH remote operations tool"
)

// App represents the Sherlock application.
type App struct {
	cfg       *config.Config
	aiClient  ai.ModelClient
	agent     *agent.Agent
	sshClient *sshclient.Client
	ctx       context.Context
	cancel    context.CancelFunc
}

func main() {
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
		prompt := "sherlock> "
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
		fmt.Println("Not connected to any host. Use 'connect <host>' or describe a connection.")
		return nil
	}

	// Parse as command request
	return a.handleCommandRequest(input)
}

func (a *App) handleConnect(input string) error {
	// Parse connection request using AI
	fmt.Println("Parsing connection request...")

	connInfo, err := a.agent.ParseConnectionRequest(a.ctx, input)
	if err != nil {
		return fmt.Errorf("failed to parse connection request: %w", err)
	}

	fmt.Printf("Connecting to %s@%s:%d...\n", connInfo.User, connInfo.Host, connInfo.Port)

	// Prompt for password
	fmt.Print("Password (leave empty to use SSH key): ")
	reader := bufio.NewReader(os.Stdin)
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	// Create SSH client
	clientCfg := &sshclient.Config{
		HostInfo:       connInfo.ToHostInfo(),
		Password:       password,
		PrivateKeyPath: a.cfg.SSHKey.PrivateKeyPath,
	}

	client, err := sshclient.NewClient(clientCfg)
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
	if a.cfg.SSHKey.AutoAddToRemote && password != "" {
		fmt.Println("Adding public key to remote authorized_keys...")
		if err := client.AddPublicKeyToAuthorizedKeys(a.ctx, a.cfg.SSHKey.PublicKeyPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to add public key: %v\n", err)
		} else {
			fmt.Println("Public key added successfully. Future connections can use key authentication.")
		}
	}

	return nil
}

func (a *App) handleDirectCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}

	if a.sshClient == nil || !a.sshClient.IsConnected() {
		return fmt.Errorf("not connected to any host")
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
	result := a.sshClient.Execute(a.ctx, cmd)

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
		fmt.Printf("Connected to: %s\n", a.sshClient.HostInfoString())
	} else {
		fmt.Println("SSH Status: Not connected")
	}
}

func (a *App) cleanup() {
	if a.sshClient != nil {
		_ = a.sshClient.Close()
	}
	if a.aiClient != nil {
		_ = a.aiClient.Close()
	}
	a.cancel()
}

func isConnectionRequest(input string) bool {
	lower := strings.ToLower(input)
	keywords := []string{"connect", "ssh", "login", "log in"}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	// Check for user@host pattern
	if strings.Contains(input, "@") {
		return true
	}
	return false
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

Usage: sherlock [options]

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
  disconnect              Disconnect from current host

Connection:
  connect <host>          Connect to a remote host
  ssh user@host:port      Connect using SSH-like syntax
  Or describe in natural language, e.g., "connect to server 192.168.1.100 as root"

Commands (when connected):
  $<command>              Execute a command directly, e.g., $ls -la
  Or describe in natural language, e.g., "show me disk usage"
`)
}
