// Package main implements a Model Context Protocol (MCP) server for Sherlock.
// This allows Sherlock to be integrated with Claude and other AI assistants
// that support the MCP protocol.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MCP Protocol Version
const (
	MCPVersion     = "2024-11-05"
	ServerName     = "sherlock-mcp"
	ServerVersion  = "1.0.0"
	SherlockBinary = "sherlock"
)

// JSON-RPC structures
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP structures
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion string            `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo        `json:"serverInfo"`
}

type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type ToolResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// MCPServer handles MCP protocol communication
type MCPServer struct {
	reader     *bufio.Reader
	writer     io.Writer
	mutex      sync.Mutex
	sherlockPath string
}

func NewMCPServer() *MCPServer {
	// Find sherlock binary
	sherlockPath := findSherlockBinary()
	
	return &MCPServer{
		reader:       bufio.NewReader(os.Stdin),
		writer:       os.Stdout,
		sherlockPath: sherlockPath,
	}
}

func findSherlockBinary() string {
	// Try to find sherlock in various locations
	candidates := []string{
		// Same directory as MCP server (build/sherlock-mcp -> build/sherlock)
		filepath.Join(filepath.Dir(os.Args[0]), "sherlock"),
		// Parent directory's build folder
		filepath.Join(filepath.Dir(os.Args[0]), "..", "build", "sherlock"),
		// Parent directory
		filepath.Join(filepath.Dir(os.Args[0]), "..", "sherlock"),
		// PATH
		"sherlock",
	}
	
	for _, path := range candidates {
		absPath, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if _, err := os.Stat(absPath); err == nil {
			return absPath
		}
	}
	
	// Try PATH lookup
	if absPath, err := exec.LookPath("sherlock"); err == nil {
		return absPath
	}
	
	// Default to same directory
	return filepath.Join(filepath.Dir(os.Args[0]), "sherlock")
}

func (s *MCPServer) Run() error {
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read error: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.sendError(nil, -32700, "Parse error", err.Error())
			continue
		}

		s.handleRequest(&req)
	}
}

func (s *MCPServer) handleRequest(req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "initialized":
		// Notification, no response needed
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	case "ping":
		s.sendResult(req.ID, map[string]interface{}{})
	default:
		s.sendError(req.ID, -32601, "Method not found", req.Method)
	}
}

func (s *MCPServer) handleInitialize(req *JSONRPCRequest) {
	result := InitializeResult{
		ProtocolVersion: MCPVersion,
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{
				ListChanged: false,
			},
		},
		ServerInfo: ServerInfo{
			Name:    ServerName,
			Version: ServerVersion,
		},
	}
	s.sendResult(req.ID, result)
}

func (s *MCPServer) handleToolsList(req *JSONRPCRequest) {
	tools := s.getAvailableTools()
	s.sendResult(req.ID, ToolsListResult{Tools: tools})
}

func (s *MCPServer) handleToolsCall(req *JSONRPCRequest) {
	var params CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "Invalid params", err.Error())
		return
	}

	result := s.executeTool(params.Name, params.Arguments)
	s.sendResult(req.ID, result)
}

func (s *MCPServer) getAvailableTools() []Tool {
	return []Tool{
		{
			Name:        "sherlock_execute",
			Description: "Execute a shell command on a remote host via SSH. The host must be configured in Sherlock's host list.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"host": {
						"type": "string",
						"description": "Host alias or connection string (user@host:port). If empty, uses the currently connected host."
					},
					"command": {
						"type": "string",
						"description": "The shell command to execute on the remote host"
					},
					"timeout": {
						"type": "integer",
						"description": "Command timeout in seconds (default: 60)",
						"default": 60
					}
				},
				"required": ["command"]
			}`),
		},
		{
			Name:        "sherlock_analyze",
			Description: "Use AI to analyze command output, logs, or error messages. Provides intelligent diagnosis and suggestions.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"content": {
						"type": "string",
						"description": "The content to analyze (command output, logs, error messages, etc.)"
					},
					"context": {
						"type": "string",
						"description": "Additional context about what the content represents"
					}
				},
				"required": ["content"]
			}`),
		},
		{
			Name:        "sherlock_diagnose",
			Description: "Diagnose errors or issues using AI. Provides root cause analysis and fix suggestions.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"error": {
						"type": "string",
						"description": "The error message or problematic output to diagnose"
					},
					"command": {
						"type": "string",
						"description": "The command that produced the error (optional)"
					}
				},
				"required": ["error"]
			}`),
		},
		{
			Name:        "sherlock_batch_execute",
			Description: "Execute a command on multiple hosts simultaneously. Supports filtering by group or tag.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command": {
						"type": "string",
						"description": "The shell command to execute on all target hosts"
					},
					"group": {
						"type": "string",
						"description": "Execute only on hosts in this group"
					},
					"tag": {
						"type": "string",
						"description": "Execute only on hosts with this tag"
					},
					"all": {
						"type": "boolean",
						"description": "Execute on all configured hosts",
						"default": false
					}
				},
				"required": ["command"]
			}`),
		},
		{
			Name:        "sherlock_health_check",
			Description: "Perform health check on one or more hosts. Checks CPU, memory, disk, and services.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"host": {
						"type": "string",
						"description": "Host alias to check. If empty, checks all hosts."
					},
					"level": {
						"type": "string",
						"enum": ["quick", "standard", "deep"],
						"description": "Check depth level: quick (basic), standard (normal), deep (comprehensive)",
						"default": "standard"
					}
				}
			}`),
		},
		{
			Name:        "sherlock_upload",
			Description: "Upload a local file to a remote host via SFTP.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"host": {
						"type": "string",
						"description": "Host alias or connection string"
					},
					"local_path": {
						"type": "string",
						"description": "Local file path to upload"
					},
					"remote_path": {
						"type": "string",
						"description": "Remote destination path"
					},
					"recursive": {
						"type": "boolean",
						"description": "Upload directory recursively",
						"default": false
					}
				},
				"required": ["local_path", "remote_path"]
			}`),
		},
		{
			Name:        "sherlock_download",
			Description: "Download a file from a remote host via SFTP.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"host": {
						"type": "string",
						"description": "Host alias or connection string"
					},
					"remote_path": {
						"type": "string",
						"description": "Remote file path to download"
					},
					"local_path": {
						"type": "string",
						"description": "Local destination path"
					},
					"recursive": {
						"type": "boolean",
						"description": "Download directory recursively",
						"default": false
					}
				},
				"required": ["remote_path", "local_path"]
			}`),
		},
		{
			Name:        "sherlock_hosts_list",
			Description: "List all configured SSH hosts with their connection details and status.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"group": {
						"type": "string",
						"description": "Filter by group name"
					},
					"tag": {
						"type": "string",
						"description": "Filter by tag"
					}
				}
			}`),
		},
		{
			Name:        "sherlock_hosts_add",
			Description: "Add a new SSH host to Sherlock's configuration.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"alias": {
						"type": "string",
						"description": "Unique alias for the host"
					},
					"host": {
						"type": "string",
						"description": "Hostname or IP address"
					},
					"port": {
						"type": "integer",
						"description": "SSH port (default: 22)",
						"default": 22
					},
					"user": {
						"type": "string",
						"description": "SSH username"
					},
					"password": {
						"type": "string",
						"description": "SSH password (optional, key auth preferred)"
					},
					"key_path": {
						"type": "string",
						"description": "Path to SSH private key file"
					},
					"group": {
						"type": "string",
						"description": "Group to assign the host to"
					},
					"tags": {
						"type": "array",
						"items": {"type": "string"},
						"description": "Tags to assign to the host"
					}
				},
				"required": ["alias", "host", "user"]
			}`),
		},
		{
			Name:        "sherlock_tunnel_create",
			Description: "Create an SSH tunnel for port forwarding (local or remote).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"host": {
						"type": "string",
						"description": "SSH host alias for the tunnel"
					},
					"type": {
						"type": "string",
						"enum": ["local", "remote"],
						"description": "Tunnel type: local (-L) or remote (-R) forwarding"
					},
					"local_port": {
						"type": "integer",
						"description": "Local port number"
					},
					"remote_host": {
						"type": "string",
						"description": "Remote host to forward to (default: localhost)",
						"default": "localhost"
					},
					"remote_port": {
						"type": "integer",
						"description": "Remote port number"
					}
				},
				"required": ["host", "type", "local_port", "remote_port"]
			}`),
		},
		{
			Name:        "sherlock_tunnel_list",
			Description: "List all active SSH tunnels.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		{
			Name:        "sherlock_tunnel_close",
			Description: "Close an SSH tunnel.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {
						"type": "string",
						"description": "Tunnel ID or local port to close"
					}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "sherlock_playbook_list",
			Description: "List available automation playbooks.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		{
			Name:        "sherlock_playbook_run",
			Description: "Run an automation playbook on target hosts.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"name": {
						"type": "string",
						"description": "Playbook name to execute"
					},
					"host": {
						"type": "string",
						"description": "Target host alias (optional, uses playbook default if not specified)"
					},
					"vars": {
						"type": "object",
						"description": "Variables to pass to the playbook"
					}
				},
				"required": ["name"]
			}`),
		},
		{
			Name:        "sherlock_advisor",
			Description: "Get AI-powered operational advice and analysis for system issues or optimization.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "Your question or issue to get advice on"
					},
					"host": {
						"type": "string",
						"description": "Host context for the advice (optional)"
					}
				},
				"required": ["query"]
			}`),
		},
		{
			Name:        "sherlock_snippet_list",
			Description: "List saved command snippets.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"category": {
						"type": "string",
						"description": "Filter by category"
					}
				}
			}`),
		},
		{
			Name:        "sherlock_snippet_run",
			Description: "Execute a saved command snippet.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"name": {
						"type": "string",
						"description": "Snippet name to execute"
					},
					"host": {
						"type": "string",
						"description": "Target host (optional)"
					}
				},
				"required": ["name"]
			}`),
		},
		{
			Name:        "sherlock_database_list",
			Description: "List configured database connections.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		{
			Name:        "sherlock_database_query",
			Description: "Execute a SQL query on a configured database.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"alias": {
						"type": "string",
						"description": "Database alias"
					},
					"query": {
						"type": "string",
						"description": "SQL query to execute"
					}
				},
				"required": ["alias", "query"]
			}`),
		},
		{
			Name:        "sherlock_cache_list",
			Description: "List configured cache (Redis) connections.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {}
			}`),
		},
		{
			Name:        "sherlock_cache_command",
			Description: "Execute a Redis command on a configured cache.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"alias": {
						"type": "string",
						"description": "Cache alias"
					},
					"command": {
						"type": "string",
						"description": "Redis command to execute (e.g., 'GET key', 'KEYS *')"
					}
				},
				"required": ["alias", "command"]
			}`),
		},
	}
}

func (s *MCPServer) executeTool(name string, args map[string]interface{}) ToolResult {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var output string
	var err error

	switch name {
	case "sherlock_execute":
		output, err = s.executeCommand(ctx, args)
	case "sherlock_analyze":
		output, err = s.analyzeContent(ctx, args)
	case "sherlock_diagnose":
		output, err = s.diagnoseError(ctx, args)
	case "sherlock_batch_execute":
		output, err = s.batchExecute(ctx, args)
	case "sherlock_health_check":
		output, err = s.healthCheck(ctx, args)
	case "sherlock_upload":
		output, err = s.uploadFile(ctx, args)
	case "sherlock_download":
		output, err = s.downloadFile(ctx, args)
	case "sherlock_hosts_list":
		output, err = s.hostsList(ctx, args)
	case "sherlock_hosts_add":
		output, err = s.hostsAdd(ctx, args)
	case "sherlock_tunnel_create":
		output, err = s.tunnelCreate(ctx, args)
	case "sherlock_tunnel_list":
		output, err = s.tunnelList(ctx, args)
	case "sherlock_tunnel_close":
		output, err = s.tunnelClose(ctx, args)
	case "sherlock_playbook_list":
		output, err = s.playbookList(ctx, args)
	case "sherlock_playbook_run":
		output, err = s.playbookRun(ctx, args)
	case "sherlock_advisor":
		output, err = s.advisor(ctx, args)
	case "sherlock_snippet_list":
		output, err = s.snippetList(ctx, args)
	case "sherlock_snippet_run":
		output, err = s.snippetRun(ctx, args)
	case "sherlock_database_list":
		output, err = s.databaseList(ctx, args)
	case "sherlock_database_query":
		output, err = s.databaseQuery(ctx, args)
	case "sherlock_cache_list":
		output, err = s.cacheList(ctx, args)
	case "sherlock_cache_command":
		output, err = s.cacheCommand(ctx, args)
	default:
		return ToolResult{
			Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", name)}},
			IsError: true,
		}
	}

	if err != nil {
		return ToolResult{
			Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}
	}

	return ToolResult{
		Content: []ContentItem{{Type: "text", Text: output}},
	}
}

// Tool implementations

func (s *MCPServer) runSherlockCommand(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, s.sherlockPath, args...)
	cmd.Env = append(os.Environ(), "SHERLOCK_MCP_MODE=1")
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return string(output), fmt.Errorf("command failed with exit code %d: %s", exitErr.ExitCode(), string(output))
		}
		return "", err
	}
	return string(output), nil
}

func (s *MCPServer) executeCommand(ctx context.Context, args map[string]interface{}) (string, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return "", fmt.Errorf("command is required")
	}

	host, _ := args["host"].(string)
	
	cmdArgs := []string{"-e", command, "-o", "json"}
	if host != "" {
		// TODO: Add host connection support
		_ = host
	}

	return s.runSherlockCommand(ctx, cmdArgs...)
}

func (s *MCPServer) analyzeContent(ctx context.Context, args map[string]interface{}) (string, error) {
	content, _ := args["content"].(string)
	if content == "" {
		return "", fmt.Errorf("content is required")
	}

	// Use analyze command with the content
	cmdArgs := []string{"-e", fmt.Sprintf("analyze %s", content), "-o", "json"}
	return s.runSherlockCommand(ctx, cmdArgs...)
}

func (s *MCPServer) diagnoseError(ctx context.Context, args map[string]interface{}) (string, error) {
	errorMsg, _ := args["error"].(string)
	if errorMsg == "" {
		return "", fmt.Errorf("error message is required")
	}

	cmdArgs := []string{"-e", fmt.Sprintf("diagnose %s", errorMsg), "-o", "json"}
	return s.runSherlockCommand(ctx, cmdArgs...)
}

func (s *MCPServer) batchExecute(ctx context.Context, args map[string]interface{}) (string, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return "", fmt.Errorf("command is required")
	}

	batchCmd := fmt.Sprintf("batch %s", command)
	
	if group, ok := args["group"].(string); ok && group != "" {
		batchCmd += fmt.Sprintf(" --group %s", group)
	}
	if tag, ok := args["tag"].(string); ok && tag != "" {
		batchCmd += fmt.Sprintf(" --tag %s", tag)
	}
	if all, ok := args["all"].(bool); ok && all {
		batchCmd += " --all"
	}
	
	return s.runSherlockCommand(ctx, "-e", batchCmd, "-o", "json")
}

func (s *MCPServer) healthCheck(ctx context.Context, args map[string]interface{}) (string, error) {
	healthCmd := "health"
	
	if host, ok := args["host"].(string); ok && host != "" {
		healthCmd += " " + host
	}
	if level, ok := args["level"].(string); ok && level != "" {
		healthCmd += " --level " + level
	}

	return s.runSherlockCommand(ctx, "-e", healthCmd, "-o", "json")
}

func (s *MCPServer) uploadFile(ctx context.Context, args map[string]interface{}) (string, error) {
	localPath, _ := args["local_path"].(string)
	remotePath, _ := args["remote_path"].(string)
	if localPath == "" || remotePath == "" {
		return "", fmt.Errorf("local_path and remote_path are required")
	}

	uploadCmd := fmt.Sprintf("upload %s %s", localPath, remotePath)
	
	if recursive, ok := args["recursive"].(bool); ok && recursive {
		uploadCmd += " -r"
	}

	return s.runSherlockCommand(ctx, "-e", uploadCmd, "-o", "json")
}

func (s *MCPServer) downloadFile(ctx context.Context, args map[string]interface{}) (string, error) {
	remotePath, _ := args["remote_path"].(string)
	localPath, _ := args["local_path"].(string)
	if localPath == "" || remotePath == "" {
		return "", fmt.Errorf("local_path and remote_path are required")
	}

	downloadCmd := fmt.Sprintf("download %s %s", remotePath, localPath)
	
	if recursive, ok := args["recursive"].(bool); ok && recursive {
		downloadCmd += " -r"
	}

	return s.runSherlockCommand(ctx, "-e", downloadCmd, "-o", "json")
}

func (s *MCPServer) hostsList(ctx context.Context, args map[string]interface{}) (string, error) {
	return s.runSherlockCommand(ctx, "-e", "hl", "-o", "json")
}

func (s *MCPServer) hostsAdd(ctx context.Context, args map[string]interface{}) (string, error) {
	alias, _ := args["alias"].(string)
	host, _ := args["host"].(string)
	user, _ := args["user"].(string)
	
	if alias == "" || host == "" || user == "" {
		return "", fmt.Errorf("alias, host, and user are required")
	}

	port := 22
	if p, ok := args["port"].(float64); ok {
		port = int(p)
	}
	
	addCmd := fmt.Sprintf("ha %s@%s:%d --alias %s", user, host, port, alias)
	
	if group, ok := args["group"].(string); ok && group != "" {
		addCmd += " --group " + group
	}

	return s.runSherlockCommand(ctx, "-e", addCmd, "-o", "json")
}

func (s *MCPServer) tunnelCreate(ctx context.Context, args map[string]interface{}) (string, error) {
	host, _ := args["host"].(string)
	tunnelType, _ := args["type"].(string)
	localPort, _ := args["local_port"].(float64)
	remotePort, _ := args["remote_port"].(float64)
	
	if host == "" || tunnelType == "" || localPort == 0 || remotePort == 0 {
		return "", fmt.Errorf("host, type, local_port, and remote_port are required")
	}

	remoteHost, _ := args["remote_host"].(string)
	if remoteHost == "" {
		remoteHost = "localhost"
	}

	tunnelCmd := fmt.Sprintf("tunnel %s %d:%s:%d", tunnelType, int(localPort), remoteHost, int(remotePort))
	return s.runSherlockCommand(ctx, "-e", tunnelCmd, "-o", "json")
}

func (s *MCPServer) tunnelList(ctx context.Context, args map[string]interface{}) (string, error) {
	return s.runSherlockCommand(ctx, "-e", "tunnel list", "-o", "json")
}

func (s *MCPServer) tunnelClose(ctx context.Context, args map[string]interface{}) (string, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return "", fmt.Errorf("tunnel id is required")
	}
	return s.runSherlockCommand(ctx, "-e", fmt.Sprintf("tunnel close %s", id), "-o", "json")
}

func (s *MCPServer) playbookList(ctx context.Context, args map[string]interface{}) (string, error) {
	return s.runSherlockCommand(ctx, "-e", "playbook list", "-o", "json")
}

func (s *MCPServer) playbookRun(ctx context.Context, args map[string]interface{}) (string, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return "", fmt.Errorf("playbook name is required")
	}

	playbookCmd := fmt.Sprintf("playbook run %s", name)
	
	if host, ok := args["host"].(string); ok && host != "" {
		playbookCmd += " --host " + host
	}

	return s.runSherlockCommand(ctx, "-e", playbookCmd, "-o", "json")
}

func (s *MCPServer) advisor(ctx context.Context, args map[string]interface{}) (string, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return "", fmt.Errorf("query is required")
	}

	return s.runSherlockCommand(ctx, "-e", fmt.Sprintf("advisor %s", query), "-o", "json")
}

func (s *MCPServer) snippetList(ctx context.Context, args map[string]interface{}) (string, error) {
	return s.runSherlockCommand(ctx, "-e", "snippet list", "-o", "json")
}

func (s *MCPServer) snippetRun(ctx context.Context, args map[string]interface{}) (string, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return "", fmt.Errorf("snippet name is required")
	}

	return s.runSherlockCommand(ctx, "-e", fmt.Sprintf("snippet run %s", name), "-o", "json")
}

func (s *MCPServer) databaseList(ctx context.Context, args map[string]interface{}) (string, error) {
	return s.runSherlockCommand(ctx, "-e", "dl", "-o", "json")
}

func (s *MCPServer) databaseQuery(ctx context.Context, args map[string]interface{}) (string, error) {
	alias, _ := args["alias"].(string)
	query, _ := args["query"].(string)
	
	if alias == "" || query == "" {
		return "", fmt.Errorf("alias and query are required")
	}

	// Database query in non-interactive mode is complex, return info message
	return fmt.Sprintf(`{"info": "Database query requires interactive connection. Use 'dc %s' to connect first, then run your query."}`, alias), nil
}

func (s *MCPServer) cacheList(ctx context.Context, args map[string]interface{}) (string, error) {
	return s.runSherlockCommand(ctx, "-e", "cl", "-o", "json")
}

func (s *MCPServer) cacheCommand(ctx context.Context, args map[string]interface{}) (string, error) {
	alias, _ := args["alias"].(string)
	command, _ := args["command"].(string)
	
	if alias == "" || command == "" {
		return "", fmt.Errorf("alias and command are required")
	}

	// Cache command in non-interactive mode is complex, return info message
	return fmt.Sprintf(`{"info": "Cache command requires interactive connection. Use 'cc %s' to connect first, then run your command."}`, alias), nil
}

// Response helpers

func (s *MCPServer) sendResult(id interface{}, result interface{}) {
	s.send(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *MCPServer) sendError(id interface{}, code int, message string, data interface{}) {
	s.send(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	})
}

func (s *MCPServer) send(response JSONRPCResponse) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	data, err := json.Marshal(response)
	if err != nil {
		return
	}

	fmt.Fprintln(s.writer, string(data))
}

func main() {
	server := NewMCPServer()
	if err := server.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}
