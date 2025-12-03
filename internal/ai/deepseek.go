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

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultDeepSeekBaseURL = "https://api.deepseek.com/v1"
)

// DeepSeekConfig stores configuration for DeepSeek client.
type DeepSeekConfig struct {
	APIKey      string         `json:"api_key"`
	BaseURL     string         `json:"base_url"`
	Model       string         `json:"model"`
	Temperature *float32       `json:"temperature,omitempty"`
	MaxTokens   *int           `json:"max_tokens,omitempty"`
	Timeout     time.Duration  `json:"timeout"`
	HTTPClient  *http.Client   `json:"-"`
}

// DeepSeekChatModel implements model.ChatModel for DeepSeek.
type DeepSeekChatModel struct {
	httpClient *http.Client
	config     *DeepSeekConfig
}

// NewDeepSeekChatModel creates a new DeepSeek chat model.
func NewDeepSeekChatModel(_ context.Context, config *DeepSeekConfig) (*DeepSeekChatModel, error) {
	if config == nil {
		return nil, errors.New("config must not be nil")
	}

	if config.APIKey == "" {
		return nil, errors.New("API key is required")
	}

	if config.BaseURL == "" {
		config.BaseURL = defaultDeepSeekBaseURL
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: config.Timeout}
	}

	return &DeepSeekChatModel{
		httpClient: httpClient,
		config:     config,
	}, nil
}

// deepSeekChatRequest represents a request to DeepSeek's chat API.
// DeepSeek uses OpenAI-compatible API format.
type deepSeekChatRequest struct {
	Model       string              `json:"model"`
	Messages    []deepSeekMessage   `json:"messages"`
	Temperature *float32            `json:"temperature,omitempty"`
	MaxTokens   *int                `json:"max_tokens,omitempty"`
	Stream      bool                `json:"stream"`
}

// deepSeekMessage represents a message in DeepSeek format.
type deepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// deepSeekChatResponse represents a response from DeepSeek's chat API.
type deepSeekChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// deepSeekStreamResponse represents a streaming response from DeepSeek's chat API.
type deepSeekStreamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
}

// Generate generates a response from the model.
func (m *DeepSeekChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	ctx = callbacks.EnsureRunInfo(ctx, m.GetType(), components.ComponentOfChatModel)

	req, cbInput, err := m.genRequest(false, input, opts...)
	if err != nil {
		return nil, fmt.Errorf("error generating request: %w", err)
	}

	ctx = callbacks.OnStart(ctx, cbInput)

	resp, err := m.doRequest(ctx, req)
	if err != nil {
		_ = callbacks.OnError(ctx, err)
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, ErrNoResponse
	}

	choice := resp.Choices[0]
	outMsg := &schema.Message{
		Role:    schema.RoleType(choice.Message.Role),
		Content: choice.Message.Content,
		ResponseMeta: &schema.ResponseMeta{
			FinishReason: choice.FinishReason,
			Usage: &schema.TokenUsage{
				PromptTokens:     resp.Usage.PromptTokens,
				CompletionTokens: resp.Usage.CompletionTokens,
				TotalTokens:      resp.Usage.TotalTokens,
			},
		},
	}

	cbOutput := &model.CallbackOutput{
		Message: outMsg,
		Config:  cbInput.Config,
		TokenUsage: &model.TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	_ = callbacks.OnEnd(ctx, cbOutput)
	return outMsg, nil
}

// Stream generates a streaming response from the model.
func (m *DeepSeekChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	ctx = callbacks.EnsureRunInfo(ctx, m.GetType(), components.ComponentOfChatModel)

	req, cbInput, err := m.genRequest(true, input, opts...)
	if err != nil {
		return nil, fmt.Errorf("error generating request: %w", err)
	}

	ctx = callbacks.OnStart(ctx, cbInput)

	sr, sw := schema.Pipe[*model.CallbackOutput](1)
	go func(ctx context.Context, conf *model.Config) {
		defer func() {
			if panicErr := recover(); panicErr != nil {
				sw.Send(nil, fmt.Errorf("panic: %v, stack: %s", panicErr, string(debug.Stack())))
			}
			sw.Close()
		}()

		err := m.doStreamRequest(ctx, req, func(resp *deepSeekStreamResponse) error {
			if len(resp.Choices) == 0 {
				return nil
			}

			choice := resp.Choices[0]
			outMsg := &schema.Message{
				Role:    schema.Assistant,
				Content: choice.Delta.Content,
			}

			cbOutput := &model.CallbackOutput{
				Message: outMsg,
				Config:  conf,
			}

			sw.Send(cbOutput, nil)
			return nil
		})

		if err != nil {
			sw.Send(nil, err)
		}
	}(ctx, cbInput.Config)

	ctx, s := callbacks.OnEndWithStreamOutput(ctx, sr)

	outStream := schema.StreamReaderWithConvert(s,
		func(src *model.CallbackOutput) (*schema.Message, error) {
			if src.Message == nil {
				return nil, schema.ErrNoValue
			}
			return src.Message, nil
		})

	return outStream, nil
}

func (m *DeepSeekChatModel) genRequest(stream bool, input []*schema.Message, _ ...model.Option) (*deepSeekChatRequest, *model.CallbackInput, error) {
	messages := make([]deepSeekMessage, 0, len(input))
	for _, msg := range input {
		messages = append(messages, deepSeekMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}

	req := &deepSeekChatRequest{
		Model:       m.config.Model,
		Messages:    messages,
		Temperature: m.config.Temperature,
		MaxTokens:   m.config.MaxTokens,
		Stream:      stream,
	}

	var temp float32
	if m.config.Temperature != nil {
		temp = *m.config.Temperature
	}

	cbInput := &model.CallbackInput{
		Messages: input,
		Config: &model.Config{
			Model:       m.config.Model,
			Temperature: temp,
		},
	}

	return req, cbInput, nil
}

func (m *DeepSeekChatModel) doRequest(ctx context.Context, req *deepSeekChatRequest) (*deepSeekChatResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := m.config.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.config.APIKey)

	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var chatResp deepSeekChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &chatResp, nil
}

func (m *DeepSeekChatModel) doStreamRequest(ctx context.Context, req *deepSeekChatRequest, handler func(*deepSeekStreamResponse) error) error {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := m.config.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+m.config.APIKey)

	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		var chatResp deepSeekStreamResponse
		if err := decoder.Decode(&chatResp); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to decode response: %w", err)
		}

		if err := handler(&chatResp); err != nil {
			return err
		}
	}

	return nil
}

// GetType returns the type of the model.
func (m *DeepSeekChatModel) GetType() string {
	return "DeepSeek"
}

// IsCallbacksEnabled returns true if callbacks are enabled.
func (m *DeepSeekChatModel) IsCallbacksEnabled() bool {
	return true
}

// BindTools binds tools to the model (not implemented for basic chat).
func (m *DeepSeekChatModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

// Verify interface compliance.
var _ model.ChatModel = (*DeepSeekChatModel)(nil)
