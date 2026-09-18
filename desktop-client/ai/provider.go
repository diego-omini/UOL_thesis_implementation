// Package ai defines the provider-neutral boundary for read-only portfolio counseling.
package ai

// This file defines provider-neutral chat messages, tool calls, and generation settings.

import "context"

// GenerationSettings controls provider-independent model generation behavior.
type GenerationSettings struct {
	Temperature float64
	MaxTokens   int
}

// Message is one provider-neutral conversation item.
type Message struct {
	Role      string
	Content   string
	ToolName  string
	ToolCalls []ToolCall
}

// ToolCall is a model request to execute one declared application tool.
type ToolCall struct {
	Name      string
	Arguments map[string]any
}

// ToolDefinition describes one callable application tool to a model.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ChatRequest contains a complete provider-independent model invocation.
type ChatRequest struct {
	Messages []Message
	Tools    []ToolDefinition
	Settings GenerationSettings
}

// ChatResponse contains the next model message and provider metadata.
type ChatResponse struct {
	Message Message
	Model   string
}

// Response is the completed prompt result returned to application services.
type Response struct {
	Text          string
	Model         string
	PromptID      string
	PromptVersion int
}

// Provider performs chat completion without receiving broker or trading capabilities.
type Provider interface {
	Chat(ctx context.Context, request ChatRequest) (ChatResponse, error)
}

// Tool executes one capability explicitly declared by a prompt definition.
type Tool interface {
	Definition() ToolDefinition
	Execute(ctx context.Context, arguments map[string]any) (string, error)
}
