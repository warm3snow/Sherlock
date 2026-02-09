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

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/warm3snow/sherlock/internal/ai"
)

// CommandPredictor predicts the next likely command based on context and history.
type CommandPredictor struct {
	mu sync.RWMutex

	aiClient ai.ModelClient

	// History of executed commands
	history []PredictorHistoryEntry

	// Maximum history entries to keep
	maxHistory int

	// Cached predictions
	cachedPredictions []Prediction
	lastPredictionAt  time.Time
	cacheValidFor     time.Duration

	// Machine context for OS-aware predictions
	machineContext *MachineContext

	// Current working state
	currentWorkflow string // e.g., "deploying", "debugging", "monitoring"

	// Enable/disable prediction
	enabled bool
}

// PredictorHistoryEntry represents a command in prediction history.
type PredictorHistoryEntry struct {
	Command   string    `json:"command"`
	Output    string    `json:"output"`      // Truncated output
	ExitCode  int       `json:"exit_code"`
	Timestamp time.Time `json:"timestamp"`
	Context   string    `json:"context"`     // Additional context (e.g., directory, host)
}

// Prediction represents a predicted command.
type Prediction struct {
	Command     string  `json:"command"`     // The predicted command
	Description string  `json:"description"` // Why this command might be useful
	Confidence  float64 `json:"confidence"`  // Confidence score (0-1)
	Reason      string  `json:"reason"`      // Explanation for the prediction
	Category    string  `json:"category"`    // follow_up, related, common
}

// PredictorConfig holds configuration for the predictor.
type PredictorConfig struct {
	MaxHistory    int           `json:"max_history"`
	CacheValidFor time.Duration `json:"cache_valid_for"`
	Enabled       bool          `json:"enabled"`
}

// DefaultPredictorConfig returns default predictor configuration.
func DefaultPredictorConfig() *PredictorConfig {
	return &PredictorConfig{
		MaxHistory:    20,
		CacheValidFor: 30 * time.Second,
		Enabled:       true,
	}
}

// NewCommandPredictor creates a new command predictor.
func NewCommandPredictor(client ai.ModelClient, config *PredictorConfig) *CommandPredictor {
	if config == nil {
		config = DefaultPredictorConfig()
	}

	return &CommandPredictor{
		aiClient:          client,
		history:           make([]PredictorHistoryEntry, 0, config.MaxHistory),
		maxHistory:        config.MaxHistory,
		cachedPredictions: make([]Prediction, 0),
		cacheValidFor:     config.CacheValidFor,
		enabled:           config.Enabled,
	}
}

const systemPromptPredict = `You are Sherlock, an AI assistant for SSH operations.
Based on the user's command history and current context, predict the most likely next commands.

Consider:
1. Common command sequences (e.g., after 'git pull', likely 'npm install' or 'make')
2. Error recovery patterns (if last command failed, suggest fixes)
3. Workflow continuity (if user is debugging, suggest diagnostic commands)
4. Operating system specifics

Provide 3-5 predictions ordered by likelihood.

Respond in JSON format only:
{
  "predictions": [
    {
      "command": "the predicted command",
      "description": "what this command does",
      "confidence": 0.85,
      "reason": "why this prediction makes sense",
      "category": "follow_up|related|common"
    }
  ],
  "detected_workflow": "deploying|debugging|monitoring|exploring|configuring|none"
}`

// RecordCommand records an executed command in history.
func (p *CommandPredictor) RecordCommand(command, output string, exitCode int, ctx string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry := PredictorHistoryEntry{
		Command:   command,
		Output:    truncatePredictorOutput(output, 200),
		ExitCode:  exitCode,
		Timestamp: time.Now(),
		Context:   ctx,
	}

	p.history = append(p.history, entry)

	// Apply sliding window
	if len(p.history) > p.maxHistory {
		p.history = p.history[len(p.history)-p.maxHistory:]
	}

	// Invalidate cache
	p.cachedPredictions = nil
}

// truncatePredictorOutput truncates output for prediction context.
func truncatePredictorOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + "..."
}

// Predict predicts the next likely commands.
func (p *CommandPredictor) Predict(ctx context.Context) ([]Prediction, error) {
	if !p.enabled {
		return nil, nil
	}

	p.mu.RLock()
	// Check cache
	if len(p.cachedPredictions) > 0 && time.Since(p.lastPredictionAt) < p.cacheValidFor {
		predictions := make([]Prediction, len(p.cachedPredictions))
		copy(predictions, p.cachedPredictions)
		p.mu.RUnlock()
		return predictions, nil
	}
	p.mu.RUnlock()

	// Generate new predictions
	predictions, workflow, err := p.generatePredictions(ctx)
	if err != nil {
		return nil, err
	}

	// Update cache
	p.mu.Lock()
	p.cachedPredictions = predictions
	p.lastPredictionAt = time.Now()
	p.currentWorkflow = workflow
	p.mu.Unlock()

	return predictions, nil
}

// generatePredictions generates predictions using AI.
func (p *CommandPredictor) generatePredictions(ctx context.Context) ([]Prediction, string, error) {
	prompt := p.buildPredictionPrompt()

	messages := []*schema.Message{
		schema.SystemMessage(systemPromptPredict),
		schema.UserMessage(prompt),
	}

	response, err := p.aiClient.Generate(ctx, messages)
	if err != nil {
		// Fall back to rule-based predictions
		return p.ruleBasedPredictions(), "", nil
	}

	content := strings.TrimSpace(response.Content)
	content = extractPredictorJSON(content)

	var result struct {
		Predictions      []Prediction `json:"predictions"`
		DetectedWorkflow string       `json:"detected_workflow"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return p.ruleBasedPredictions(), "", nil
	}

	return result.Predictions, result.DetectedWorkflow, nil
}

// buildPredictionPrompt builds the prompt for prediction.
func (p *CommandPredictor) buildPredictionPrompt() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var sb strings.Builder

	// Add machine context
	if p.machineContext != nil {
		sb.WriteString(fmt.Sprintf("Target System: %s (%s)\n", p.machineContext.OS, p.machineContext.Arch))
		if p.machineContext.Distribution != "" {
			sb.WriteString(fmt.Sprintf("Distribution: %s\n", p.machineContext.Distribution))
		}
	}

	sb.WriteString("\nRecent Command History:\n")

	// Include last N commands
	start := 0
	if len(p.history) > 10 {
		start = len(p.history) - 10
	}

	for i, entry := range p.history[start:] {
		status := "✓"
		if entry.ExitCode != 0 {
			status = fmt.Sprintf("✗(%d)", entry.ExitCode)
		}
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, status, entry.Command))
		if entry.ExitCode != 0 && entry.Output != "" {
			sb.WriteString(fmt.Sprintf("   Error: %s\n", entry.Output))
		}
	}

	sb.WriteString("\nPredict the next likely commands the user might want to execute.")

	return sb.String()
}

// ruleBasedPredictions provides fallback rule-based predictions.
func (p *CommandPredictor) ruleBasedPredictions() []Prediction {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.history) == 0 {
		return p.defaultPredictions()
	}

	last := p.history[len(p.history)-1]
	predictions := make([]Prediction, 0)

	// If last command failed, suggest diagnostic commands
	if last.ExitCode != 0 {
		predictions = append(predictions, Prediction{
			Command:     "echo $?",
			Description: "Check exit code",
			Confidence:  0.8,
			Reason:      "Last command failed, check exact error",
			Category:    "follow_up",
		})
	}

	// Common follow-up patterns
	patterns := map[string][]Prediction{
		"git pull": {
			{Command: "npm install", Description: "Install dependencies", Confidence: 0.7, Reason: "Common after git pull", Category: "follow_up"},
			{Command: "make build", Description: "Build the project", Confidence: 0.6, Reason: "Common after git pull", Category: "follow_up"},
		},
		"git status": {
			{Command: "git add .", Description: "Stage all changes", Confidence: 0.7, Reason: "Common after checking status", Category: "follow_up"},
			{Command: "git diff", Description: "View changes", Confidence: 0.6, Reason: "Common after checking status", Category: "follow_up"},
		},
		"docker ps": {
			{Command: "docker logs", Description: "View container logs", Confidence: 0.7, Reason: "Common after listing containers", Category: "follow_up"},
			{Command: "docker stats", Description: "View container stats", Confidence: 0.6, Reason: "Common after listing containers", Category: "follow_up"},
		},
		"df -h": {
			{Command: "du -sh /*", Description: "Find large directories", Confidence: 0.8, Reason: "Common after checking disk usage", Category: "follow_up"},
		},
		"top": {
			{Command: "free -h", Description: "Check memory usage", Confidence: 0.7, Reason: "Common after viewing processes", Category: "related"},
			{Command: "iostat", Description: "Check I/O stats", Confidence: 0.6, Reason: "Common after viewing processes", Category: "related"},
		},
	}

	// Match patterns
	for pattern, preds := range patterns {
		if strings.HasPrefix(last.Command, pattern) {
			predictions = append(predictions, preds...)
			break
		}
	}

	// Add some common commands if we don't have enough predictions
	if len(predictions) < 3 {
		predictions = append(predictions, p.defaultPredictions()...)
	}

	// Limit to 5 predictions
	if len(predictions) > 5 {
		predictions = predictions[:5]
	}

	return predictions
}

// defaultPredictions returns default predictions when no context is available.
func (p *CommandPredictor) defaultPredictions() []Prediction {
	return []Prediction{
		{Command: "ls -la", Description: "List files with details", Confidence: 0.5, Reason: "Common exploration command", Category: "common"},
		{Command: "pwd", Description: "Print working directory", Confidence: 0.4, Reason: "Common exploration command", Category: "common"},
		{Command: "df -h", Description: "Check disk usage", Confidence: 0.4, Reason: "Common monitoring command", Category: "common"},
	}
}

// SetMachineContext updates the machine context for predictions.
func (p *CommandPredictor) SetMachineContext(ctx *MachineContext) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.machineContext = ctx
	p.cachedPredictions = nil // Invalidate cache
}

// GetCurrentWorkflow returns the detected current workflow.
func (p *CommandPredictor) GetCurrentWorkflow() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentWorkflow
}

// SetEnabled enables or disables prediction.
func (p *CommandPredictor) SetEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = enabled
}

// IsEnabled returns whether prediction is enabled.
func (p *CommandPredictor) IsEnabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.enabled
}

// ClearHistory clears the command history.
func (p *CommandPredictor) ClearHistory() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.history = make([]PredictorHistoryEntry, 0, p.maxHistory)
	p.cachedPredictions = nil
	p.currentWorkflow = ""
}

// GetHistory returns the command history.
func (p *CommandPredictor) GetHistory() []PredictorHistoryEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]PredictorHistoryEntry, len(p.history))
	copy(result, p.history)
	return result
}

// PredictAsync performs prediction asynchronously.
func (p *CommandPredictor) PredictAsync(ctx context.Context, callback func([]Prediction, error)) {
	go func() {
		predictions, err := p.Predict(ctx)
		callback(predictions, err)
	}()
}

// FormatPredictions formats predictions for display.
func FormatPredictions(predictions []Prediction) string {
	if len(predictions) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n💡 Suggested next commands:\n")

	for i, pred := range predictions {
		confidence := "●"
		if pred.Confidence >= 0.8 {
			confidence = "●●●"
		} else if pred.Confidence >= 0.6 {
			confidence = "●●"
		}

		sb.WriteString(fmt.Sprintf("  %d. %s %s\n", i+1, confidence, pred.Command))
		if pred.Description != "" {
			sb.WriteString(fmt.Sprintf("     └─ %s\n", pred.Description))
		}
	}

	return sb.String()
}

// GetTopPrediction returns the highest confidence prediction.
func GetTopPrediction(predictions []Prediction) *Prediction {
	if len(predictions) == 0 {
		return nil
	}

	top := &predictions[0]
	for i := 1; i < len(predictions); i++ {
		if predictions[i].Confidence > top.Confidence {
			top = &predictions[i]
		}
	}

	return top
}

// extractPredictorJSON extracts JSON from response.
func extractPredictorJSON(content string) string {
	// Try to find JSON in markdown code blocks
	start := strings.Index(content, "```")
	if start >= 0 {
		end := strings.LastIndex(content, "```")
		if end > start {
			inner := content[start+3 : end]
			// Remove potential language identifier
			if idx := strings.Index(inner, "\n"); idx >= 0 {
				inner = inner[idx+1:]
			}
			return strings.TrimSpace(inner)
		}
	}

	// Try to find raw JSON
	start = strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		return content[start : end+1]
	}

	return content
}
