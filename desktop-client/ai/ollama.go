package ai

// This file implements provider-neutral chat and tool calling through Ollama's local HTTP API.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxOllamaResponseBytes = 4 << 20

// OllamaProvider performs chat completion through an Ollama server.
type OllamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

type ollamaChatRequest struct {
	Model    string                 `json:"model"`
	Messages []ollamaMessage        `json:"messages"`
	Tools    []ollamaToolDefinition `json:"tools,omitempty"`
	Stream   bool                   `json:"stream"`
	Options  ollamaOptions          `json:"options"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaFunctionCall `json:"function"`
}

type ollamaFunctionCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ollamaToolDefinition struct {
	Type     string                   `json:"type"`
	Function ollamaFunctionDefinition `json:"function"`
}

type ollamaFunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"num_predict"`
}

type ollamaChatResponse struct {
	Model   string        `json:"model"`
	Message ollamaMessage `json:"message"`
	Error   string        `json:"error"`
}

// NewOllamaProvider creates an Ollama adapter with a bounded request duration.
func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	return &OllamaProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 2 * time.Minute},
	}
}

// Chat sends a non-streaming conversation and maps content and tool calls to the application boundary.
func (p *OllamaProvider) Chat(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	payload, err := json.Marshal(ollamaChatRequest{
		Model:    p.model,
		Messages: toOllamaMessages(request.Messages),
		Tools:    toOllamaTools(request.Tools),
		Stream:   false,
		Options: ollamaOptions{
			Temperature: request.Settings.Temperature,
			MaxTokens:   request.Settings.MaxTokens,
		},
	})
	if err != nil {
		return ChatResponse{}, fmt.Errorf("encode Ollama request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat", bytes.NewReader(payload))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create Ollama request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	httpResponse, err := p.client.Do(httpRequest)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("Ollama is not reachable at %s: %w", p.baseURL, err)
	}
	defer httpResponse.Body.Close()
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxOllamaResponseBytes))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read Ollama response: %w", err)
	}

	var generated ollamaChatResponse
	if err := json.Unmarshal(body, &generated); err != nil {
		return ChatResponse{}, fmt.Errorf("Ollama returned an invalid response: %w", err)
	}
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(generated.Error)
		if message == "" {
			message = httpResponse.Status
		}
		return ChatResponse{}, fmt.Errorf("Ollama request failed: %s", message)
	}
	if generated.Error != "" {
		return ChatResponse{}, fmt.Errorf("Ollama request failed: %s", generated.Error)
	}

	return ChatResponse{
		Message: fromOllamaMessage(generated.Message),
		Model:   generated.Model,
	}, nil
}

func toOllamaMessages(messages []Message) []ollamaMessage {
	mapped := make([]ollamaMessage, 0, len(messages))
	for _, message := range messages {
		toolCalls := make([]ollamaToolCall, 0, len(message.ToolCalls))
		for _, call := range message.ToolCalls {
			toolCalls = append(toolCalls, ollamaToolCall{Function: ollamaFunctionCall{
				Name:      call.Name,
				Arguments: call.Arguments,
			}})
		}
		mapped = append(mapped, ollamaMessage{
			Role:      message.Role,
			Content:   message.Content,
			ToolName:  message.ToolName,
			ToolCalls: toolCalls,
		})
	}
	return mapped
}

func fromOllamaMessage(message ollamaMessage) Message {
	toolCalls := make([]ToolCall, 0, len(message.ToolCalls))
	for _, call := range message.ToolCalls {
		toolCalls = append(toolCalls, ToolCall{
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		})
	}
	return Message{
		Role:      message.Role,
		Content:   message.Content,
		ToolName:  message.ToolName,
		ToolCalls: toolCalls,
	}
}

func toOllamaTools(tools []ToolDefinition) []ollamaToolDefinition {
	mapped := make([]ollamaToolDefinition, 0, len(tools))
	for _, tool := range tools {
		mapped = append(mapped, ollamaToolDefinition{
			Type: "function",
			Function: ollamaFunctionDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}
	return mapped
}
