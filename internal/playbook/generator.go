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

package playbook

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/warm3snow/sherlock/internal/ai"
)

// Generator provides AI-powered playbook generation.
type Generator struct {
	client ai.ModelClient
}

// flexiblePlaybookStep is used for parsing AI-generated JSON that might have
// flexible types for numeric fields (e.g., array instead of int).
type flexiblePlaybookStep struct {
	Name            string          `json:"name"`
	Command         string          `json:"command"`
	Description     string          `json:"description,omitempty"`
	ContinueOnError bool            `json:"continue_on_error"`
	Timeout         json.RawMessage `json:"timeout,omitempty"`
	ExpectedExitCode json.RawMessage `json:"expected_exit_code,omitempty"`
	RetryCount      json.RawMessage `json:"retry_count,omitempty"`
}

// flexiblePlaybook is used for parsing AI-generated playbooks with flexible types.
type flexiblePlaybook struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Category    Category             `json:"category"`
	Steps       []flexiblePlaybookStep `json:"steps"`
	Variables   map[string]string    `json:"variables,omitempty"`
	Tags        []string             `json:"tags,omitempty"`
	Author      string               `json:"author,omitempty"`
	Version     string               `json:"version,omitempty"`
}

// flexibleGenerationResult is used for parsing AI responses.
type flexibleGenerationResult struct {
	Playbook     *flexiblePlaybook `json:"playbook"`
	Explanation  string            `json:"explanation"`
	Warnings     []string          `json:"warnings"`
	Alternatives []string          `json:"alternatives"`
}

// parseFlexibleInt parses an int from JSON that might be an int or an array.
func parseFlexibleInt(raw json.RawMessage, defaultVal int) int {
	if len(raw) == 0 {
		return defaultVal
	}
	
	// Try parsing as int first
	var intVal int
	if err := json.Unmarshal(raw, &intVal); err == nil {
		return intVal
	}
	
	// Try parsing as array of ints
	var intArr []int
	if err := json.Unmarshal(raw, &intArr); err == nil && len(intArr) > 0 {
		return intArr[0]
	}
	
	return defaultVal
}

// toPlaybook converts flexiblePlaybook to Playbook.
func (fp *flexiblePlaybook) toPlaybook() *Playbook {
	if fp == nil {
		return nil
	}
	
	pb := &Playbook{
		Name:        fp.Name,
		Description: fp.Description,
		Category:    fp.Category,
		Variables:   fp.Variables,
		Tags:        fp.Tags,
		Author:      fp.Author,
		Version:     fp.Version,
		Steps:       make([]PlaybookStep, len(fp.Steps)),
	}
	
	for i, step := range fp.Steps {
		pb.Steps[i] = PlaybookStep{
			Name:             step.Name,
			Command:          step.Command,
			Description:      step.Description,
			ContinueOnError:  step.ContinueOnError,
			Timeout:          parseFlexibleInt(step.Timeout, 60),
			ExpectedExitCode: parseFlexibleInt(step.ExpectedExitCode, 0),
			RetryCount:       parseFlexibleInt(step.RetryCount, 0),
		}
	}
	
	return pb
}

// NewGenerator creates a new playbook generator.
func NewGenerator(client ai.ModelClient) *Generator {
	return &Generator{client: client}
}

// GenerationRequest represents a request to generate a playbook.
type GenerationRequest struct {
	Description  string            `json:"description"`   // Natural language description
	Category     Category          `json:"category"`      // Optional category hint
	Variables    map[string]string `json:"variables"`     // Optional predefined variables
	TargetOS     string            `json:"target_os"`     // linux, macos, etc.
	Requirements []string          `json:"requirements"`  // Additional requirements
}

// GenerationResult represents the result of playbook generation.
type GenerationResult struct {
	Playbook     *Playbook `json:"playbook"`
	Explanation  string    `json:"explanation"`   // AI's explanation of the playbook
	Warnings     []string  `json:"warnings"`      // Any warnings about the generated playbook
	Alternatives []string  `json:"alternatives"`  // Alternative approaches suggested
}

const systemPromptGenerate = `You are Sherlock, an AI assistant specializing in creating automated operations playbooks.
Your task is to generate a complete, executable playbook based on the user's natural language description.

A playbook consists of:
1. Name: A short, descriptive name (use-kebab-case)
2. Description: What the playbook does
3. Category: One of (inspect, cleanup, deploy, backup, recover, custom)
4. Steps: A list of executable shell commands with descriptions
5. Variables: Placeholders that can be customized (use {{variable_name}} syntax)

Guidelines:
- Generate practical, tested shell commands
- Add proper error handling (set -e where appropriate)
- Include verification steps after critical operations
- Use variables for paths, service names, etc. that might change
- Consider the target OS when generating commands
- Add comments/descriptions for complex steps
- Set appropriate timeouts for long-running operations
- Mark dangerous operations (delete, restart) with continue_on_error: false

Support both English and Chinese descriptions.

CRITICAL: All numeric fields MUST be integers, NOT arrays.
- "timeout" must be an integer (e.g., 60)
- "expected_exit_code" must be an integer (e.g., 0), NOT an array like [0]
- "retry_count" must be an integer (e.g., 0)

Respond in JSON format only:
{
  "playbook": {
    "name": "playbook-name",
    "description": "What this playbook does",
    "category": "deploy",
    "steps": [
      {
        "name": "step-name",
        "command": "shell command to execute",
        "description": "What this step does",
        "continue_on_error": false,
        "timeout": 60,
        "expected_exit_code": 0,
        "retry_count": 0
      }
    ],
    "variables": {
      "variable_name": "default_value"
    },
    "tags": ["tag1", "tag2"]
  },
  "explanation": "Brief explanation of the approach taken",
  "warnings": ["Any warnings or cautions"],
  "alternatives": ["Alternative approaches if any"]
}`

const systemPromptModify = `You are Sherlock, an AI assistant specializing in modifying operations playbooks.
Your task is to modify an existing playbook based on the user's instructions.

Current playbook:
%s

User's modification request will follow.

Respond in the same JSON format as the original playbook generation.
Keep existing steps unless explicitly asked to change them.
Add new steps as requested while maintaining logical order.`

// Generate creates a new playbook from a natural language description.
func (g *Generator) Generate(ctx context.Context, req *GenerationRequest) (*GenerationResult, error) {
	prompt := g.buildGenerationPrompt(req)

	messages := []*schema.Message{
		schema.SystemMessage(systemPromptGenerate),
		schema.UserMessage(prompt),
	}

	response, err := g.client.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("AI generation failed: %w", err)
	}

	content := strings.TrimSpace(response.Content)
	content = extractJSON(content)

	// Use flexible parsing to handle AI's inconsistent types
	var flexResult flexibleGenerationResult
	if err := json.Unmarshal([]byte(content), &flexResult); err != nil {
		return nil, fmt.Errorf("failed to parse generation result: %w", err)
	}

	// Convert to standard result
	result := &GenerationResult{
		Playbook:     flexResult.Playbook.toPlaybook(),
		Explanation:  flexResult.Explanation,
		Warnings:     flexResult.Warnings,
		Alternatives: flexResult.Alternatives,
	}

	// Set defaults and validate
	if result.Playbook != nil {
		g.setPlaybookDefaults(result.Playbook)
		if err := g.validatePlaybook(result.Playbook); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Validation warning: %v", err))
		}
	}

	return result, nil
}

// buildGenerationPrompt builds the prompt for playbook generation.
func (g *Generator) buildGenerationPrompt(req *GenerationRequest) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Create a playbook for: %s\n", req.Description))

	if req.TargetOS != "" {
		sb.WriteString(fmt.Sprintf("\nTarget OS: %s\n", req.TargetOS))
	}

	if req.Category != "" {
		sb.WriteString(fmt.Sprintf("\nCategory: %s\n", req.Category))
	}

	if len(req.Variables) > 0 {
		sb.WriteString("\nPredefined variables:\n")
		for k, v := range req.Variables {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", k, v))
		}
	}

	if len(req.Requirements) > 0 {
		sb.WriteString("\nAdditional requirements:\n")
		for _, r := range req.Requirements {
			sb.WriteString(fmt.Sprintf("  - %s\n", r))
		}
	}

	return sb.String()
}

// setPlaybookDefaults sets default values for the playbook.
func (g *Generator) setPlaybookDefaults(pb *Playbook) {
	if pb.Version == "" {
		pb.Version = "1.0"
	}
	if pb.Author == "" {
		pb.Author = "AI Generated"
	}
	if pb.Category == "" {
		pb.Category = CategoryCustom
	}
	pb.CreatedAt = time.Now()
	pb.UpdatedAt = time.Now()

	// Set step defaults
	for i := range pb.Steps {
		if pb.Steps[i].Timeout == 0 {
			pb.Steps[i].Timeout = 60
		}
	}
}

// validatePlaybook performs basic validation on the playbook.
func (g *Generator) validatePlaybook(pb *Playbook) error {
	if pb.Name == "" {
		return fmt.Errorf("playbook name is required")
	}
	if len(pb.Steps) == 0 {
		return fmt.Errorf("playbook must have at least one step")
	}

	for i, step := range pb.Steps {
		if step.Command == "" {
			return fmt.Errorf("step %d has no command", i+1)
		}
	}

	return nil
}

// Modify modifies an existing playbook based on instructions.
func (g *Generator) Modify(ctx context.Context, pb *Playbook, instruction string) (*GenerationResult, error) {
	// Serialize current playbook for context
	pbJSON, err := json.MarshalIndent(pb, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize playbook: %w", err)
	}

	systemPrompt := fmt.Sprintf(systemPromptModify, string(pbJSON))

	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(instruction),
	}

	response, err := g.client.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("AI modification failed: %w", err)
	}

	content := strings.TrimSpace(response.Content)
	content = extractJSON(content)

	// Use flexible parsing to handle AI's inconsistent types
	var flexResult flexibleGenerationResult
	if err := json.Unmarshal([]byte(content), &flexResult); err != nil {
		return nil, fmt.Errorf("failed to parse modification result: %w", err)
	}

	// Convert to standard result
	result := &GenerationResult{
		Playbook:     flexResult.Playbook.toPlaybook(),
		Explanation:  flexResult.Explanation,
		Warnings:     flexResult.Warnings,
		Alternatives: flexResult.Alternatives,
	}

	// Preserve original ID and metadata
	if result.Playbook != nil {
		result.Playbook.ID = pb.ID
		result.Playbook.CreatedAt = pb.CreatedAt
		result.Playbook.UpdatedAt = time.Now()
		result.Playbook.UsageCount = pb.UsageCount

		if err := g.validatePlaybook(result.Playbook); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Validation warning: %v", err))
		}
	}

	return result, nil
}

// GenerateFromTemplate generates a playbook from a template type.
func (g *Generator) GenerateFromTemplate(ctx context.Context, templateType string, params map[string]string) (*GenerationResult, error) {
	templates := map[string]string{
		"deploy-django": "创建一个部署 Django 应用的 playbook，包括拉取代码、安装依赖、迁移数据库、收集静态文件、重启服务",
		"deploy-nodejs": "创建一个部署 Node.js 应用的 playbook，包括拉取代码、npm install、pm2 重启",
		"deploy-docker": "创建一个 Docker 容器部署的 playbook，包括拉取镜像、停止旧容器、启动新容器、健康检查",
		"backup-mysql":  "创建一个 MySQL 数据库备份的 playbook，使用 mysqldump，包括压缩和上传到远程存储",
		"backup-files":  "创建一个文件备份的 playbook，打包重要目录并保留最近 N 个备份",
		"cleanup-logs":  "创建一个日志清理的 playbook，清理超过指定天数的日志文件并统计释放空间",
		"cleanup-docker": "创建一个 Docker 清理的 playbook，清理悬空镜像、停止容器、未使用的网络和卷",
		"inspect-system": "创建一个系统巡检的 playbook，检查 CPU、内存、磁盘、网络、服务状态",
		"inspect-security": "创建一个安全巡检的 playbook，检查端口、用户、权限、日志异常",
	}

	description, ok := templates[templateType]
	if !ok {
		return nil, fmt.Errorf("unknown template type: %s, available: %v", templateType, getTemplateTypes(templates))
	}

	// Append params to description
	if len(params) > 0 {
		var paramDesc []string
		for k, v := range params {
			paramDesc = append(paramDesc, fmt.Sprintf("%s=%s", k, v))
		}
		description += fmt.Sprintf(" (参数: %s)", strings.Join(paramDesc, ", "))
	}

	return g.Generate(ctx, &GenerationRequest{
		Description: description,
		Category:    inferCategory(templateType),
	})
}

// getTemplateTypes returns available template types.
func getTemplateTypes(templates map[string]string) []string {
	types := make([]string, 0, len(templates))
	for k := range templates {
		types = append(types, k)
	}
	return types
}

// inferCategory infers category from template type.
func inferCategory(templateType string) Category {
	if strings.HasPrefix(templateType, "deploy-") {
		return CategoryDeploy
	}
	if strings.HasPrefix(templateType, "backup-") {
		return CategoryBackup
	}
	if strings.HasPrefix(templateType, "cleanup-") {
		return CategoryCleanup
	}
	if strings.HasPrefix(templateType, "inspect-") {
		return CategoryInspect
	}
	if strings.HasPrefix(templateType, "recover-") {
		return CategoryRecover
	}
	return CategoryCustom
}

// SuggestImprovements suggests improvements for an existing playbook.
func (g *Generator) SuggestImprovements(ctx context.Context, pb *Playbook) ([]string, error) {
	pbJSON, err := json.MarshalIndent(pb, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to serialize playbook: %w", err)
	}

	prompt := fmt.Sprintf(`Analyze this playbook and suggest improvements for reliability, efficiency, and best practices:

%s

Provide 3-5 specific, actionable suggestions. Focus on:
1. Error handling and recovery
2. Performance optimization
3. Security considerations
4. Monitoring and logging
5. Idempotency

Respond with a JSON array of strings:
["suggestion1", "suggestion2", "suggestion3"]`, string(pbJSON))

	messages := []*schema.Message{
		schema.SystemMessage("You are a DevOps expert. Analyze playbooks and suggest improvements."),
		schema.UserMessage(prompt),
	}

	response, err := g.client.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("AI suggestion failed: %w", err)
	}

	content := strings.TrimSpace(response.Content)
	content = extractJSON(content)

	var suggestions []string
	if err := json.Unmarshal([]byte(content), &suggestions); err != nil {
		// Try to extract suggestions from free text
		return extractSuggestions(response.Content), nil
	}

	return suggestions, nil
}

// extractSuggestions extracts suggestions from free-form text.
func extractSuggestions(text string) []string {
	var suggestions []string
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Match numbered items or bullet points
		if matched, _ := regexp.MatchString(`^[\d\-\*•]`, line); matched {
			// Clean up the line
			line = strings.TrimLeft(line, "0123456789.-*• \t")
			if len(line) > 10 {
				suggestions = append(suggestions, line)
			}
		}
	}
	return suggestions
}

// FormatGenerationResult formats the generation result for display.
func FormatGenerationResult(result *GenerationResult) string {
	var sb strings.Builder

	if result.Playbook == nil {
		sb.WriteString("❌ Failed to generate playbook\n")
		return sb.String()
	}

	pb := result.Playbook
	sb.WriteString(fmt.Sprintf("\n✨ Generated Playbook: %s\n", pb.Name))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("📋 Description: %s\n", pb.Description))
	sb.WriteString(fmt.Sprintf("📁 Category: %s\n", pb.Category))

	if len(pb.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("🏷️  Tags: %s\n", strings.Join(pb.Tags, ", ")))
	}

	sb.WriteString(fmt.Sprintf("\n📝 Steps (%d):\n", len(pb.Steps)))
	for i, step := range pb.Steps {
		icon := "  "
		if step.ContinueOnError {
			icon = "⚠️"
		}
		sb.WriteString(fmt.Sprintf("  %d. %s %s\n", i+1, icon, step.Name))
		sb.WriteString(fmt.Sprintf("     └─ %s\n", step.Command))
		if step.Description != "" {
			sb.WriteString(fmt.Sprintf("        %s\n", step.Description))
		}
	}

	if len(pb.Variables) > 0 {
		sb.WriteString("\n🔧 Variables:\n")
		for k, v := range pb.Variables {
			sb.WriteString(fmt.Sprintf("  • {{%s}}: %s\n", k, v))
		}
	}

	if result.Explanation != "" {
		sb.WriteString(fmt.Sprintf("\n💡 Explanation:\n  %s\n", result.Explanation))
	}

	if len(result.Warnings) > 0 {
		sb.WriteString("\n⚠️  Warnings:\n")
		for _, w := range result.Warnings {
			sb.WriteString(fmt.Sprintf("  • %s\n", w))
		}
	}

	if len(result.Alternatives) > 0 {
		sb.WriteString("\n🔄 Alternatives:\n")
		for _, a := range result.Alternatives {
			sb.WriteString(fmt.Sprintf("  • %s\n", a))
		}
	}

	return sb.String()
}

// ListTemplates returns available playbook templates.
func ListTemplates() map[string]string {
	return map[string]string{
		"deploy-django":    "Deploy Django application",
		"deploy-nodejs":    "Deploy Node.js application",
		"deploy-docker":    "Deploy Docker container",
		"backup-mysql":     "Backup MySQL database",
		"backup-files":     "Backup files and directories",
		"cleanup-logs":     "Clean up old log files",
		"cleanup-docker":   "Clean up Docker resources",
		"inspect-system":   "System health inspection",
		"inspect-security": "Security inspection",
	}
}

// extractJSON extracts JSON from markdown code blocks or raw text.
func extractJSON(content string) string {
	jsonBlockRe := regexp.MustCompile("```(?:json)?\\s*([\\s\\S]*?)```")
	if matches := jsonBlockRe.FindStringSubmatch(content); len(matches) == 2 {
		return strings.TrimSpace(matches[1])
	}

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		return content[start : end+1]
	}

	return content
}
