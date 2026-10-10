// Package ai defines the provider-neutral boundary for read-only portfolio counseling.
package ai

// This file defines provider-neutral chat messages, tool calls, and generation settings.

import (
	"context"
	"time"
)

// GenerationSettings controls provider-independent model generation behavior.
type GenerationSettings struct {
	Temperature float64 `yaml:"temperature" json:"temperature"`
	MaxTokens   int     `yaml:"max_tokens" json:"max_tokens"`
	Seed        int     `yaml:"seed" json:"seed"`
	ContextSize int     `yaml:"context_size" json:"context_size"`
}

// Message is one provider-neutral conversation item.
type Message struct {
	Role       string
	Content    string
	ToolName   string
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall
}

// ToolCall is a model request to execute one declared application tool.
type ToolCall struct {
	ID        string `json:"id,omitempty"`
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
	Messages       []Message
	Tools          []ToolDefinition
	ResponseSchema map[string]any
	Settings       GenerationSettings
}

// ChatResponse contains the next model message and provider metadata.
type ChatResponse struct {
	Message      Message
	Model        string
	FinishReason string
}

// Response is the completed prompt result returned to application services.
type Response struct {
	Text          string
	Structured    *CounselorResponse
	Model         string
	PromptID      string
	PromptVersion int
	CreatedAt     time.Time
	ContextAsOf   time.Time
}

// PromptExecution contains the sanitized prompt inputs and outcome retained for reproducibility.
type PromptExecution struct {
	StartedAt        time.Time
	CompletedAt      time.Time
	ContextAsOf      time.Time
	Provider         string
	Model            string
	PromptID         string
	PromptVersion    int
	Parameters       map[string]any
	SystemPrompt     string
	UserPrompt       string
	Transcript       []Message
	ModelSettings    GenerationSettings
	RawResponse      string
	RepairResponse   string
	ResponseText     string
	ParsedResponse   *CounselorResponse
	ValidationResult CounselorValidationResult
	RepairStatus     string
	ErrorText        string
}

// ExecutionRecorder persists prompt executions without receiving model or application capabilities.
type ExecutionRecorder interface {
	RecordPromptExecution(ctx context.Context, execution PromptExecution) error
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
