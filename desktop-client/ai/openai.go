package ai

// This file maps the provider-neutral counselor contract to OpenAI's Responses API.

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

const maxOpenAIResponseBytes = 4 << 20

// OpenAIProvider performs chat completion through OpenAI's Responses API.
type OpenAIProvider struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

type openAIResponseRequest struct {
	Model           string                 `json:"model"`
	Input           []openAIInputItem      `json:"input"`
	Tools           []openAIToolDefinition `json:"tools,omitempty"`
	Text            *openAITextConfig      `json:"text,omitempty"`
	Temperature     *float64               `json:"temperature,omitempty"`
	MaxOutputTokens int                    `json:"max_output_tokens,omitempty"`
	Store           bool                   `json:"store"`
}

type openAIInputItem struct {
	Type      string `json:"type,omitempty"`
	Role      string `json:"role,omitempty"`
	Content   string `json:"content,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

type openAIToolDefinition struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type openAITextConfig struct {
	Format openAITextFormat `json:"format"`
}

type openAITextFormat struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type openAIResponse struct {
	Status            string                 `json:"status"`
	Model             string                 `json:"model"`
	Output            []openAIResponseOutput `json:"output"`
	Error             *openAIError           `json:"error"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
}

type openAIResponseOutput struct {
	Type      string                  `json:"type"`
	Role      string                  `json:"role"`
	Content   []openAIResponseContent `json:"content"`
	CallID    string                  `json:"call_id"`
	Name      string                  `json:"name"`
	Arguments string                  `json:"arguments"`
	Action    *openAIWebSearchAction  `json:"action,omitempty"`
}

type openAIResponseContent struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Refusal string `json:"refusal"`
}

type openAIErrorEnvelope struct {
	Error *openAIError `json:"error"`
}

type openAIError struct {
	Message string `json:"message"`
}

// NewOpenAIProvider creates an OpenAI adapter with a bounded request duration.
func NewOpenAIProvider(baseURL, apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  strings.TrimSpace(apiKey),
		model:   strings.TrimSpace(model),
		client:  &http.Client{Timeout: 2 * time.Minute},
	}
}

// Chat sends a stateless Responses API request and maps text and function calls to the application boundary.
func (p *OpenAIProvider) Chat(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	if p.apiKey == "" {
		return ChatResponse{}, fmt.Errorf("OpenAI API access is unavailable because OPENAI_API_KEY is not configured")
	}
	input, err := toOpenAIInput(request.Messages)
	if err != nil {
		return ChatResponse{}, err
	}
	payload := openAIResponseRequest{
		Model:           p.model,
		Input:           input,
		Tools:           toOpenAITools(request.Tools),
		MaxOutputTokens: request.Settings.MaxTokens,
		Store:           false,
	}
	if openAIModelSupportsTemperature(p.model) {
		payload.Temperature = &request.Settings.Temperature
	}
	if request.ResponseSchema != nil {
		payload.Text = &openAITextConfig{Format: openAITextFormat{
			Type: "json_schema", Name: "counselor_response", Strict: true, Schema: request.ResponseSchema,
		}}
	}
	contents, err := json.Marshal(payload)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("encode OpenAI request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/responses", bytes.NewReader(contents))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create OpenAI request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")

	httpResponse, err := p.client.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return ChatResponse{}, ctx.Err()
		}
		return ChatResponse{}, fmt.Errorf("OpenAI is not reachable at the configured endpoint: %w", err)
	}
	defer httpResponse.Body.Close()
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxOpenAIResponseBytes+1))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read OpenAI response: %w", err)
	}
	if len(body) > maxOpenAIResponseBytes {
		return ChatResponse{}, fmt.Errorf("OpenAI response exceeded the %d-byte limit", maxOpenAIResponseBytes)
	}
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		var failure openAIErrorEnvelope
		_ = json.Unmarshal(body, &failure)
		message := httpResponse.Status
		if failure.Error != nil && strings.TrimSpace(failure.Error.Message) != "" {
			message = failure.Error.Message
		}
		return ChatResponse{}, fmt.Errorf("OpenAI request failed: %s", boundedProviderMessage(message))
	}

	var generated openAIResponse
	if err := json.Unmarshal(body, &generated); err != nil {
		return ChatResponse{}, fmt.Errorf("OpenAI returned an invalid response: %w", err)
	}
	if generated.Error != nil {
		return ChatResponse{}, fmt.Errorf("OpenAI request failed: %s", boundedProviderMessage(generated.Error.Message))
	}
	message, refusal, err := fromOpenAIOutput(generated.Output)
	if err != nil {
		return ChatResponse{}, err
	}
	if message.Content == "" && len(message.ToolCalls) == 0 {
		if refusal != "" {
			return ChatResponse{}, fmt.Errorf("OpenAI refused the request: %s", boundedProviderMessage(refusal))
		}
		return ChatResponse{}, fmt.Errorf("OpenAI returned no text or function call")
	}
	finishReason := generated.Status
	if len(message.ToolCalls) > 0 {
		finishReason = "tool_calls"
	} else if generated.IncompleteDetails != nil && generated.IncompleteDetails.Reason != "" {
		finishReason = generated.IncompleteDetails.Reason
	}
	model := strings.TrimSpace(generated.Model)
	if model == "" {
		model = p.model
	}
	return ChatResponse{Message: message, Model: model, FinishReason: finishReason}, nil
}

func openAIModelSupportsTemperature(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range []string{"gpt-5", "gpt-6", "o1", "o3", "o4"} {
		if strings.HasPrefix(model, prefix) {
			return false
		}
	}
	return true
}

func toOpenAIInput(messages []Message) ([]openAIInputItem, error) {
	items := make([]openAIInputItem, 0, len(messages))
	for _, message := range messages {
		if message.Role == "tool" {
			if strings.TrimSpace(message.ToolCallID) == "" {
				return nil, fmt.Errorf("map OpenAI tool result: missing tool call ID for %q", message.ToolName)
			}
			items = append(items, openAIInputItem{Type: "function_call_output", CallID: message.ToolCallID, Output: message.Content})
			continue
		}
		switch message.Role {
		case "system", "developer", "user", "assistant":
		default:
			return nil, fmt.Errorf("map OpenAI message: unsupported role %q", message.Role)
		}
		if message.Content != "" {
			items = append(items, openAIInputItem{Role: message.Role, Content: message.Content})
		}
		for _, call := range message.ToolCalls {
			if strings.TrimSpace(call.ID) == "" {
				return nil, fmt.Errorf("map OpenAI function call %q: missing call ID", call.Name)
			}
			arguments, err := json.Marshal(call.Arguments)
			if err != nil {
				return nil, fmt.Errorf("encode OpenAI function call %q arguments: %w", call.Name, err)
			}
			items = append(items, openAIInputItem{
				Type: "function_call", CallID: call.ID, Name: call.Name, Arguments: string(arguments),
			})
		}
	}
	return items, nil
}

func toOpenAITools(tools []ToolDefinition) []openAIToolDefinition {
	mapped := make([]openAIToolDefinition, 0, len(tools))
	for _, tool := range tools {
		mapped = append(mapped, openAIToolDefinition{
			Type: "function", Name: tool.Name, Description: tool.Description, Parameters: tool.Parameters,
		})
	}
	return mapped
}

func fromOpenAIOutput(output []openAIResponseOutput) (Message, string, error) {
	message := Message{Role: "assistant"}
	var textParts []string
	var refusals []string
	for _, item := range output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				switch content.Type {
				case "output_text":
					textParts = append(textParts, content.Text)
				case "refusal":
					refusals = append(refusals, content.Refusal)
				}
			}
		case "function_call":
			var arguments map[string]any
			if err := json.Unmarshal([]byte(item.Arguments), &arguments); err != nil {
				return Message{}, "", fmt.Errorf("OpenAI returned invalid arguments for function %q: %w", item.Name, err)
			}
			message.ToolCalls = append(message.ToolCalls, ToolCall{ID: item.CallID, Name: item.Name, Arguments: arguments})
		}
	}
	message.Content = strings.Join(textParts, "")
	return message, strings.Join(refusals, " "), nil
}
