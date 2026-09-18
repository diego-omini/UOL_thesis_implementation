package ai

// This file executes catalog prompts, bounded tool loops, and final-response repair.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Executor renders prompt contracts and coordinates providers with application tools.
type Executor struct {
	catalog  *Catalog
	provider Provider
}

type completedChat struct {
	response ChatResponse
	messages []Message
}

// NewExecutor creates a generic prompt execution service.
func NewExecutor(catalog *Catalog, provider Provider) *Executor {
	return &Executor{catalog: catalog, provider: provider}
}

// Execute resolves an active alias and runs it with prompt-scoped tools.
func (e *Executor) Execute(ctx context.Context, alias string, parameters map[string]any, tools []Tool) (Response, error) {
	definition, err := e.catalog.resolve(alias)
	if err != nil {
		return Response{}, err
	}
	return e.executeDefinition(ctx, definition, parameters, tools)
}

// ExecuteVersion runs a retained prompt version directly for reproducible evaluation.
func (e *Executor) ExecuteVersion(
	ctx context.Context,
	promptID string,
	version int,
	parameters map[string]any,
	tools []Tool,
) (Response, error) {
	definition, err := e.catalog.version(promptID, version)
	if err != nil {
		return Response{}, err
	}
	return e.executeDefinition(ctx, definition, parameters, tools)
}

func (e *Executor) executeDefinition(
	ctx context.Context,
	definition PromptDefinition,
	parameters map[string]any,
	tools []Tool,
) (Response, error) {
	if err := validateParameters(definition.Parameters, parameters); err != nil {
		return Response{}, fmt.Errorf("prompt %s v%d parameters: %w", definition.ID, definition.Version, err)
	}
	systemPrompt, err := renderTemplate(definition.ID+"_system", definition.SystemTemplate, parameters)
	if err != nil {
		return Response{}, err
	}
	userPrompt, err := renderTemplate(definition.ID+"_user", definition.UserTemplate, parameters)
	if err != nil {
		return Response{}, err
	}
	systemPrompt += fmt.Sprintf("\n\nYour complete response must not exceed %d characters.", definition.Response.MaxCharacters)

	selectedTools, err := selectTools(definition.Tools, tools)
	if err != nil {
		return Response{}, fmt.Errorf("prompt %s v%d: %w", definition.ID, definition.Version, err)
	}
	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	settings := GenerationSettings{
		Temperature: definition.Response.Temperature,
		MaxTokens:   definition.Response.MaxTokens,
	}

	completed, err := e.runToolLoop(ctx, messages, selectedTools, definition.ToolSettings, settings)
	if err != nil {
		return Response{}, err
	}
	text := strings.TrimSpace(completed.response.Message.Content)
	if validationErr := validateFinalResponse(text, definition.Response); validationErr != nil {
		text, err = e.repair(ctx, completed.messages, definition.Response, validationErr)
		if err != nil {
			return Response{}, err
		}
	}
	return Response{
		Text:          text,
		Model:         completed.response.Model,
		PromptID:      definition.ID,
		PromptVersion: definition.Version,
	}, nil
}

func (e *Executor) runToolLoop(
	ctx context.Context,
	messages []Message,
	tools map[string]Tool,
	limits ToolSettings,
	settings GenerationSettings,
) (completedChat, error) {
	definitions := toolDefinitions(tools)
	rounds := 0
	calls := 0
	forcedFinal := false
	for {
		response, err := e.provider.Chat(ctx, ChatRequest{
			Messages: messages,
			Tools:    definitions,
			Settings: settings,
		})
		if err != nil {
			return completedChat{}, err
		}
		if len(response.Message.ToolCalls) == 0 {
			return completedChat{
				response: response,
				messages: append(messages, response.Message),
			}, nil
		}
		if forcedFinal {
			return completedChat{}, fmt.Errorf("provider requested tools after the research limit was reached")
		}

		messages = append(messages, response.Message)
		rounds++
		undeclaredToolRequested := false
		for _, call := range response.Message.ToolCalls {
			calls++
			if rounds > limits.MaxRounds || calls > limits.MaxCalls {
				messages = append(messages, Message{
					Role:     "tool",
					ToolName: call.Name,
					Content:  "Research limit reached. Complete the answer using evidence already collected and disclose the limitation.",
				})
				continue
			}
			tool, ok := tools[call.Name]
			if !ok {
				messages = append(messages, Message{
					Role:     "tool",
					ToolName: call.Name,
					Content:  fmt.Sprintf("Tool %q is not available. Do not request another tool; return the final answer directly using the supplied portfolio context and any evidence already collected.", call.Name),
				})
				undeclaredToolRequested = true
				continue
			}
			result, err := tool.Execute(ctx, call.Arguments)
			if err != nil {
				result = "Tool error: " + err.Error()
			}
			messages = append(messages, Message{Role: "tool", ToolName: call.Name, Content: result})
		}
		if undeclaredToolRequested || rounds >= limits.MaxRounds || calls >= limits.MaxCalls {
			definitions = nil
			forcedFinal = true
		}
	}
}

func (e *Executor) repair(
	ctx context.Context,
	messages []Message,
	settings ResponseSettings,
	validationErr error,
) (string, error) {
	sections := "none"
	if len(settings.RequiredSections) > 0 {
		sections = strings.Join(settings.RequiredSections, ", ")
	}
	messages = append(messages, Message{
		Role: "user",
		Content: fmt.Sprintf(
			"Your previous response was not a valid final answer: %v. Return the final answer only in %s format, include these required sections: %s, and keep it within %d characters. Do not emit, call, or describe a tool or function.",
			validationErr,
			settings.Format,
			sections,
			settings.MaxCharacters,
		),
	})
	response, err := e.provider.Chat(ctx, ChatRequest{
		Messages: messages,
		Settings: GenerationSettings{Temperature: 0, MaxTokens: settings.MaxTokens},
	})
	if err != nil {
		return "", fmt.Errorf("repair invalid final response: %w", err)
	}
	if len(response.Message.ToolCalls) > 0 {
		return "", fmt.Errorf("provider returned a tool call while repairing the final response")
	}
	repaired := strings.TrimSpace(response.Message.Content)
	if err := validateFinalResponse(repaired, settings); err != nil {
		return "", fmt.Errorf("provider could not produce a valid final response after one repair attempt: %w", err)
	}
	return repaired, nil
}

func validateFinalResponse(text string, settings ResponseSettings) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("response is empty")
	}
	if utf8.RuneCountInString(text) > settings.MaxCharacters {
		return fmt.Errorf("response exceeds the %d-character limit", settings.MaxCharacters)
	}
	if settings.Format != "json" && looksLikeToolEnvelope(text) {
		return fmt.Errorf("response is a textual tool-call envelope rather than a final answer")
	}
	switch settings.Format {
	case "json":
		if !json.Valid([]byte(text)) {
			return fmt.Errorf("response is not valid JSON")
		}
	case "markdown":
		for _, section := range settings.RequiredSections {
			if !strings.Contains(text, section) {
				return fmt.Errorf("response is missing required section %q", section)
			}
		}
	}
	return nil
}

func looksLikeToolEnvelope(text string) bool {
	var envelope map[string]json.RawMessage
	if json.Unmarshal([]byte(text), &envelope) != nil {
		return false
	}
	_, hasName := envelope["name"]
	_, hasParameters := envelope["parameters"]
	_, hasArguments := envelope["arguments"]
	return hasName && (hasParameters || hasArguments)
}

func selectTools(names []string, available []Tool) (map[string]Tool, error) {
	byName := make(map[string]Tool, len(available))
	for _, tool := range available {
		name := tool.Definition().Name
		if _, exists := byName[name]; exists {
			return nil, fmt.Errorf("duplicate available tool %q", name)
		}
		byName[name] = tool
	}
	selected := make(map[string]Tool, len(names))
	for _, name := range names {
		tool, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("required tool %q is unavailable", name)
		}
		selected[name] = tool
	}
	return selected, nil
}

func toolDefinitions(tools map[string]Tool) []ToolDefinition {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	definitions := make([]ToolDefinition, 0, len(names))
	for _, name := range names {
		definitions = append(definitions, tools[name].Definition())
	}
	return definitions
}
