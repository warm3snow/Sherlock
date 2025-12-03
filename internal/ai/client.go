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

// Package ai provides AI model integration for Sherlock using the Eino framework.
package ai

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/warm3snow/Sherlock/internal/config"
)

// ModelClient interface defines methods for interacting with LLM models.
type ModelClient interface {
	// Generate generates a response from the model.
	Generate(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	// Stream generates a streaming response from the model.
	Stream(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error)
	// GetModel returns the underlying model.
	GetModel() model.ChatModel
	// Close cleans up any resources.
	Close() error
}

// Client wraps an LLM model client.
type Client struct {
	model    model.ChatModel
	provider config.LLMProviderType
}

// NewClient creates a new AI client based on the configuration.
func NewClient(ctx context.Context, cfg *config.LLMConfig) (ModelClient, error) {
	switch cfg.Provider {
	case config.ProviderOllama:
		return newOllamaClient(ctx, cfg)
	case config.ProviderOpenAI:
		return newOpenAIClient(ctx, cfg)
	case config.ProviderDeepSeek:
		return newDeepSeekClient(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

// Generate generates a response from the model.
func (c *Client) Generate(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	return c.model.Generate(ctx, messages)
}

// Stream generates a streaming response from the model.
func (c *Client) Stream(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	return c.model.Stream(ctx, messages)
}

// GetModel returns the underlying model.
func (c *Client) GetModel() model.ChatModel {
	return c.model
}

// Close cleans up any resources.
func (c *Client) Close() error {
	return nil
}

// ollama client implementation
func newOllamaClient(ctx context.Context, cfg *config.LLMConfig) (*Client, error) {
	ollamaCfg := &OllamaConfig{
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: 60 * time.Second,
	}

	if cfg.Temperature > 0 {
		ollamaCfg.Options = &OllamaOptions{
			Temperature: cfg.Temperature,
		}
	}

	chatModel, err := NewOllamaChatModel(ctx, ollamaCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Ollama client: %w", err)
	}

	return &Client{
		model:    chatModel,
		provider: config.ProviderOllama,
	}, nil
}

// openai client implementation
func newOpenAIClient(ctx context.Context, cfg *config.LLMConfig) (*Client, error) {
	openaiCfg := &OpenAIConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: 60 * time.Second,
	}

	if cfg.Temperature > 0 {
		temp := cfg.Temperature
		openaiCfg.Temperature = &temp
	}

	chatModel, err := NewOpenAIChatModel(ctx, openaiCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI client: %w", err)
	}

	return &Client{
		model:    chatModel,
		provider: config.ProviderOpenAI,
	}, nil
}

// deepseek client implementation
func newDeepSeekClient(ctx context.Context, cfg *config.LLMConfig) (*Client, error) {
	deepseekCfg := &DeepSeekConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: 60 * time.Second,
	}

	if cfg.Temperature > 0 {
		temp := cfg.Temperature
		deepseekCfg.Temperature = &temp
	}

	chatModel, err := NewDeepSeekChatModel(ctx, deepseekCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create DeepSeek client: %w", err)
	}

	return &Client{
		model:    chatModel,
		provider: config.ProviderDeepSeek,
	}, nil
}

// ParseConnectionIntent parses a natural language request to extract SSH connection information.
type ConnectionIntent struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password,omitempty"`
}

// CommandIntent represents a parsed command intent.
type CommandIntent struct {
	Commands    []string `json:"commands"`
	Description string   `json:"description"`
	NeedsConfirm bool    `json:"needs_confirm"`
}

// Verify interface compliance.
var _ ModelClient = (*Client)(nil)

// ErrNoResponse indicates the model did not return a response.
var ErrNoResponse = errors.New("no response from model")
