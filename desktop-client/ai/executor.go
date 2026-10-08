package ai

// This file executes catalog prompts, bounded tool loops, and final-response repair.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Executor renders prompt contracts and coordinates providers with application tools.
type Executor struct {
	catalog      *Catalog
	provider     Provider
	providerName string
	recorder     ExecutionRecorder
}

// SetExecutionRecorder enables durable prompt audit records for subsequent executions.
func (e *Executor) SetExecutionRecorder(providerName string, recorder ExecutionRecorder) {
	e.providerName = strings.TrimSpace(providerName)
	e.recorder = recorder
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
	return e.executeDefinition(ctx, definition, parameters, tools, nil)
}

// ExecuteCounselor runs a structured counselor prompt against its authoritative fact set.
func (e *Executor) ExecuteCounselor(
	ctx context.Context,
	alias string,
	parameters map[string]any,
	tools []Tool,
	facts []CounselorFact,
) (Response, error) {
	definition, err := e.catalog.resolve(alias)
	if err != nil {
		return Response{}, err
	}
	if definition.Response.Schema != CounselorResponseSchema {
		return Response{}, fmt.Errorf("prompt %s v%d does not declare the counselor response schema", definition.ID, definition.Version)
	}
	return e.executeDefinition(ctx, definition, parameters, tools, facts)
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
	return e.executeDefinition(ctx, definition, parameters, tools, nil)
}

func (e *Executor) executeDefinition(
	ctx context.Context,
	definition PromptDefinition,
	parameters map[string]any,
	tools []Tool,
	facts []CounselorFact,
) (result Response, executionErr error) {
	startedAt := time.Now().UTC()
	contextAsOf := counselorContextAsOf(facts)
	var renderedSystemPrompt string
	var renderedUserPrompt string
	var transcript []Message
	var model string
	var modelSettings GenerationSettings
	var rawResponse string
	var repairResponse string
	var parsedResponse *CounselorResponse
	validationResult := CounselorValidationResult{}
	repairStatus := "not_attempted"
	defer func() {
		if e.recorder == nil {
			return
		}
		completedAt := result.CreatedAt
		if completedAt.IsZero() {
			completedAt = time.Now().UTC()
		}
		record := PromptExecution{
			StartedAt: startedAt, CompletedAt: completedAt, ContextAsOf: contextAsOf, Provider: e.providerName,
			Model: model, PromptID: definition.ID, PromptVersion: definition.Version,
			Parameters: parameters, SystemPrompt: renderedSystemPrompt, UserPrompt: renderedUserPrompt,
			Transcript: transcript, ModelSettings: modelSettings,
			RawResponse: rawResponse, RepairResponse: repairResponse, ResponseText: result.Text,
			ParsedResponse: parsedResponse, ValidationResult: validationResult, RepairStatus: repairStatus,
		}
		if executionErr != nil {
			record.ErrorText = executionErr.Error()
		}
		recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if recordErr := e.recorder.RecordPromptExecution(recordCtx, record); recordErr != nil && executionErr == nil {
			executionErr = fmt.Errorf("persist prompt execution: %w", recordErr)
		}
	}()
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
	renderedSystemPrompt = systemPrompt
	renderedUserPrompt = userPrompt

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
	modelSettings = settings
	var responseSchema map[string]any
	if definition.Response.Schema == CounselorResponseSchema {
		responseSchema = counselorJSONSchema()
	}

	transcript = append([]Message(nil), messages...)
	completed, err := e.runToolLoop(ctx, messages, selectedTools, definition.ToolSettings, settings, responseSchema)
	if err != nil {
		return Response{}, err
	}
	transcript = append([]Message(nil), completed.messages...)
	model = completed.response.Model
	rawResponse = strings.TrimSpace(completed.response.Message.Content)
	text := rawResponse
	parsed, validationErrors := validateExecutionResponse(text, definition.Response, facts, selectedTools)
	if len(validationErrors) > 0 {
		validationResult.InitialErrors = append([]string(nil), validationErrors...)
		repairStatus = "attempted"
		text, err = e.repair(ctx, completed.messages, definition.Response, strings.Join(validationErrors, "; "), responseSchema)
		if err != nil {
			repairStatus = "failed"
			return Response{}, err
		}
		repairResponse = text
		parsed, validationErrors = validateExecutionResponse(text, definition.Response, facts, selectedTools)
		if len(validationErrors) > 0 {
			validationResult.FinalErrors = append([]string(nil), validationErrors...)
			repairStatus = "failed"
			return Response{}, fmt.Errorf("provider could not produce a valid final response after one repair attempt: %s", strings.Join(validationErrors, "; "))
		}
		repairStatus = "succeeded"
	}
	validationResult.Valid = true
	parsedResponse = parsed
	result = Response{
		Text:          text,
		Structured:    parsed,
		Model:         completed.response.Model,
		PromptID:      definition.ID,
		PromptVersion: definition.Version,
		CreatedAt:     time.Now().UTC(),
		ContextAsOf:   contextAsOf,
	}
	return result, nil
}

// counselorContextAsOf finds the newest authoritative timestamp supplied to a counselor prompt.
func counselorContextAsOf(facts []CounselorFact) time.Time {
	var newest time.Time
	for _, fact := range facts {
		id := strings.ToLower(strings.TrimSpace(fact.ID))
		if !strings.Contains(id, "timestamp") && !strings.Contains(id, "range_end") && !strings.HasSuffix(id, ".as_of") && !strings.HasSuffix(id, ".last_at") {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(fact.TextValue))
		if err == nil && parsed.After(newest) {
			newest = parsed.UTC()
		}
	}
	return newest
}

func (e *Executor) runToolLoop(
	ctx context.Context,
	messages []Message,
	tools map[string]Tool,
	limits ToolSettings,
	settings GenerationSettings,
	responseSchema map[string]any,
) (completedChat, error) {
	definitions := toolDefinitions(tools)
	rounds := 0
	calls := 0
	forcedFinal := false
	for {
		response, err := e.provider.Chat(ctx, ChatRequest{
			Messages: messages, Tools: definitions, ResponseSchema: responseSchema, Settings: settings,
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
	validationErr string,
	responseSchema map[string]any,
) (string, error) {
	sections := "none"
	if len(settings.RequiredSections) > 0 {
		sections = strings.Join(settings.RequiredSections, ", ")
	}
	repairInstruction := fmt.Sprintf(
		"Your previous response was not a valid final answer: %v. Return the final answer only in %s format, include these required sections: %s, and keep it within %d characters. Do not emit, call, or describe a tool or function.",
		validationErr,
		settings.Format,
		sections,
		settings.MaxCharacters,
	)
	if settings.Schema == CounselorResponseSchema {
		repairInstruction += ` Return one concise JSON object with schema_version 1, a non-empty summary, one to five complete observations, and at least one response limitation. Every observation must have a non-empty statement and explanation and must include all array fields. Use only the category, severity, and confidence enum values declared in the schema. This is a tool-free repair: use no sourced_external_evidence category, URLs, or citations; citations must be empty arrays. A supporting_fact_ids or limitation fact_ids entry must be only the exact identifier following id= in the supplied facts, never the whole fact line. reported_values may contain only supplied numeric facts; copy the value and exact unit token, and also list the same fact ID in supporting_fact_ids. Never put text or unavailable facts in reported_values. Include every fact named by a "required limitation ... is missing" validation error in a response-level limitation fact_ids array. If an observation cannot be repaired safely, omit it rather than inventing support. Do not copy placeholder text.`
	}
	messages = append(messages, Message{
		Role:    "user",
		Content: repairInstruction,
	})
	response, err := e.provider.Chat(ctx, ChatRequest{
		Messages: messages, ResponseSchema: responseSchema,
		Settings: GenerationSettings{Temperature: 0, MaxTokens: settings.MaxTokens},
	})
	if err != nil {
		return "", fmt.Errorf("repair invalid final response: %w", err)
	}
	if len(response.Message.ToolCalls) > 0 {
		return "", fmt.Errorf("provider returned a tool call while repairing the final response")
	}
	repaired := strings.TrimSpace(response.Message.Content)
	return repaired, nil
}

func validateExecutionResponse(text string, settings ResponseSettings, facts []CounselorFact, tools map[string]Tool) (*CounselorResponse, []string) {
	if settings.Schema == CounselorResponseSchema {
		if utf8.RuneCountInString(text) > settings.MaxCharacters {
			return nil, []string{fmt.Sprintf("response exceeds the %d-character limit", settings.MaxCharacters)}
		}
		return decodeAndValidateCounselorResponse(text, facts, tools)
	}
	if err := validateFinalResponse(text, settings); err != nil {
		return nil, []string{err.Error()}
	}
	return nil, nil
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
