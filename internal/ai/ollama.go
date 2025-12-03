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
	"net/url"
	"runtime/debug"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// OllamaOptions stores Ollama-specific options.
type OllamaOptions struct {
	Temperature float32  `json:"temperature,omitempty"`
	TopP        float32  `json:"top_p,omitempty"`
	Stop        []string `json:"stop,omitempty"`
	Seed        int      `json:"seed,omitempty"`
}

// OllamaConfig stores configuration for Ollama client.
type OllamaConfig struct {
	BaseURL    string         `json:"base_url"`
	Timeout    time.Duration  `json:"timeout"`
	Model      string         `json:"model"`
	Format     json.RawMessage `json:"format,omitempty"`
	KeepAlive  *time.Duration `json:"keep_alive,omitempty"`
	Options    *OllamaOptions `json:"options,omitempty"`
	HTTPClient *http.Client   `json:"-"`
}

// OllamaChatModel implements model.ChatModel for Ollama.
type OllamaChatModel struct {
	httpClient *http.Client
	config     *OllamaConfig
	baseURL    *url.URL
}

// NewOllamaChatModel creates a new Ollama chat model.
func NewOllamaChatModel(_ context.Context, config *OllamaConfig) (*OllamaChatModel, error) {
	if config == nil {
		return nil, errors.New("config must not be nil")
	}

	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434"
	}

	baseURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: config.Timeout}
	}

	return &OllamaChatModel{
		httpClient: httpClient,
		config:     config,
		baseURL:    baseURL,
	}, nil
}

// ollamaChatRequest represents a request to Ollama's chat API.
type ollamaChatRequest struct {
	Model     string                 `json:"model"`
	Messages  []ollamaMessage        `json:"messages"`
	Stream    bool                   `json:"stream"`
	Format    json.RawMessage        `json:"format,omitempty"`
	Options   map[string]any         `json:"options,omitempty"`
	KeepAlive string                 `json:"keep_alive,omitempty"`
}

// ollamaMessage represents a message in Ollama format.
type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaChatResponse represents a response from Ollama's chat API.
type ollamaChatResponse struct {
	Model     string        `json:"model"`
	CreatedAt string        `json:"created_at"`
	Message   ollamaMessage `json:"message"`
	Done      bool          `json:"done"`
	DoneReason string       `json:"done_reason,omitempty"`
	TotalDuration int64     `json:"total_duration,omitempty"`
	LoadDuration  int64     `json:"load_duration,omitempty"`
	PromptEvalCount int     `json:"prompt_eval_count,omitempty"`
	EvalCount   int         `json:"eval_count,omitempty"`
}

// Generate generates a response from the model.
func (m *OllamaChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
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

	outMsg := &schema.Message{
		Role:    schema.RoleType(resp.Message.Role),
		Content: resp.Message.Content,
		ResponseMeta: &schema.ResponseMeta{
			FinishReason: resp.DoneReason,
			Usage: &schema.TokenUsage{
				PromptTokens:     resp.PromptEvalCount,
				CompletionTokens: resp.EvalCount,
				TotalTokens:      resp.PromptEvalCount + resp.EvalCount,
			},
		},
	}

	cbOutput := &model.CallbackOutput{
		Message: outMsg,
		Config:  cbInput.Config,
		TokenUsage: &model.TokenUsage{
			PromptTokens:     resp.PromptEvalCount,
			CompletionTokens: resp.EvalCount,
			TotalTokens:      resp.PromptEvalCount + resp.EvalCount,
		},
	}

	_ = callbacks.OnEnd(ctx, cbOutput)
	return outMsg, nil
}

// Stream generates a streaming response from the model.
func (m *OllamaChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
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

		err := m.doStreamRequest(ctx, req, func(resp *ollamaChatResponse) error {
			outMsg := &schema.Message{
				Role:    schema.RoleType(resp.Message.Role),
				Content: resp.Message.Content,
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

func (m *OllamaChatModel) genRequest(stream bool, input []*schema.Message, _ ...model.Option) (*ollamaChatRequest, *model.CallbackInput, error) {
	messages := make([]ollamaMessage, 0, len(input))
	for _, msg := range input {
		messages = append(messages, ollamaMessage{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}

	options := make(map[string]any)
	if m.config.Options != nil {
		if m.config.Options.Temperature > 0 {
			options["temperature"] = m.config.Options.Temperature
		}
		if m.config.Options.TopP > 0 {
			options["top_p"] = m.config.Options.TopP
		}
		if len(m.config.Options.Stop) > 0 {
			options["stop"] = m.config.Options.Stop
		}
		if m.config.Options.Seed > 0 {
			options["seed"] = m.config.Options.Seed
		}
	}

	req := &ollamaChatRequest{
		Model:    m.config.Model,
		Messages: messages,
		Stream:   stream,
		Format:   m.config.Format,
		Options:  options,
	}

	if m.config.KeepAlive != nil {
		req.KeepAlive = m.config.KeepAlive.String()
	}

	var temp float32
	if m.config.Options != nil {
		temp = m.config.Options.Temperature
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

func (m *OllamaChatModel) doRequest(ctx context.Context, req *ollamaChatRequest) (*ollamaChatResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := m.baseURL.JoinPath("/api/chat").String()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var chatResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &chatResp, nil
}

func (m *OllamaChatModel) doStreamRequest(ctx context.Context, req *ollamaChatRequest, handler func(*ollamaChatResponse) error) error {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := m.baseURL.JoinPath("/api/chat").String()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

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
		var chatResp ollamaChatResponse
		if err := decoder.Decode(&chatResp); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to decode response: %w", err)
		}

		if err := handler(&chatResp); err != nil {
			return err
		}

		if chatResp.Done {
			break
		}
	}

	return nil
}

// GetType returns the type of the model.
func (m *OllamaChatModel) GetType() string {
	return "Ollama"
}

// IsCallbacksEnabled returns true if callbacks are enabled.
func (m *OllamaChatModel) IsCallbacksEnabled() bool {
	return true
}

// BindTools binds tools to the model (not implemented for basic chat).
func (m *OllamaChatModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

// Verify interface compliance.
var _ model.ChatModel = (*OllamaChatModel)(nil)
