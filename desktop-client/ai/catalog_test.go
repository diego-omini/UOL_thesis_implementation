package ai

// This file verifies version resolution, generic execution, parameter validation, and response repair.

import (
	"context"
	"strings"
	"testing"
)

type testTool struct {
	arguments map[string]any
}

func (t *testTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "lookup", Description: "Lookup test data.", Parameters: map[string]any{"type": "object"}}
}

func (t *testTool) Execute(_ context.Context, arguments map[string]any) (string, error) {
	t.arguments = arguments
	return "tool evidence", nil
}

func TestEmbeddedCatalogResolvesActivePortfolioPrompt(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	definition, err := catalog.resolve(PortfolioResumePromptAlias)
	if err != nil {
		t.Fatalf("resolve returned an error: %v", err)
	}
	if definition.ID != PortfolioResumePromptAlias || definition.Version != 2 || definition.Response.MaxCharacters != 4000 {
		t.Fatalf("unexpected prompt definition: %+v", definition)
	}
	if _, err := catalog.version(PortfolioResumePromptAlias, 1); err != nil {
		t.Fatalf("retained prompt version is unavailable: %v", err)
	}
	if _, err := catalog.version(PortfolioResumePromptAlias, 2); err != nil {
		t.Fatalf("active prompt version is unavailable: %v", err)
	}
}

func TestEmbeddedCatalogResolvesPortfolioNewsPrompt(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	definition, err := catalog.resolve(PortfolioNewsPromptAlias)
	if err != nil {
		t.Fatalf("resolve returned an error: %v", err)
	}
	if definition.ID != PortfolioNewsPromptAlias || definition.Version != 1 || definition.Response.MaxCharacters != 12000 {
		t.Fatalf("unexpected prompt definition: %+v", definition)
	}
}

func TestEmbeddedCatalogResolvesStockNewsPrompt(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	definition, err := catalog.resolve(StockNewsPromptAlias)
	if err != nil {
		t.Fatalf("resolve returned an error: %v", err)
	}
	if definition.ID != StockNewsPromptAlias || definition.Version != 1 || definition.Response.MaxCharacters != 6500 {
		t.Fatalf("unexpected prompt definition: %+v", definition)
	}
}

func TestExecutorRejectsMissingAndUnexpectedParameters(t *testing.T) {
	executor := testExecutor(&recordingProvider{}, 100)

	if _, err := executor.Execute(context.Background(), "active_test", map[string]any{}, nil); err == nil || !strings.Contains(err.Error(), "required parameter") {
		t.Fatalf("expected a missing parameter error, got %v", err)
	}
	if _, err := executor.Execute(context.Background(), "active_test", map[string]any{"name": "value", "extra": true}, nil); err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("expected an unexpected parameter error, got %v", err)
	}
}

func TestExecutorRepairsOverlongResponse(t *testing.T) {
	provider := &recordingProvider{responses: []ChatResponse{
		{Message: Message{Role: "assistant", Content: "response that is too long"}, Model: "test-model"},
		{Message: Message{Role: "assistant", Content: "short"}, Model: "test-model"},
	}}
	executor := testExecutor(provider, 5)

	response, err := executor.Execute(context.Background(), "active_test", map[string]any{"name": "portfolio"}, nil)
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	if response.Text != "short" {
		t.Fatalf("response was not repaired: %q", response.Text)
	}
	if len(provider.requests) != 2 || provider.requests[1].Settings.Temperature != 0 {
		t.Fatalf("expected a deterministic response-repair request: %+v", provider.requests)
	}
	if response.PromptID != "test" || response.PromptVersion != 2 {
		t.Fatalf("prompt metadata was not returned: %+v", response)
	}
}

func TestExecutorRepairsTextualToolEnvelope(t *testing.T) {
	provider := &recordingProvider{responses: []ChatResponse{
		{Message: Message{Role: "assistant", Content: `{"name":"analyze_portfolio","parameters":{"currency":"GBP"}}`}, Model: "test-model"},
		{Message: Message{Role: "assistant", Content: "## Overall assessment\nFinal analysis."}, Model: "test-model"},
	}}
	definition := PromptDefinition{
		ID: "markdown_test", Version: 1, Description: "Markdown test.",
		SystemTemplate: "System", UserTemplate: "User",
		Response: ResponseSettings{
			Format: "markdown", RequiredSections: []string{"## Overall assessment"},
			MaxCharacters: 1000, Temperature: 0.1, MaxTokens: 100,
		},
	}
	catalog := &Catalog{
		prompts: map[promptKey]PromptDefinition{{id: "markdown_test", version: 1}: definition},
		aliases: map[string]PromptReference{"markdown_test": {ID: "markdown_test", Version: 1}},
	}

	response, err := NewExecutor(catalog, provider).Execute(context.Background(), "markdown_test", nil, nil)
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	if response.Text != "## Overall assessment\nFinal analysis." || len(provider.requests) != 2 {
		t.Fatalf("textual tool envelope was not repaired: response=%+v requests=%d", response, len(provider.requests))
	}
	repairPrompt := provider.requests[1].Messages[len(provider.requests[1].Messages)-1].Content
	if !strings.Contains(repairPrompt, "tool-call envelope") || len(provider.requests[1].Tools) != 0 {
		t.Fatalf("unexpected repair request: %+v", provider.requests[1])
	}
}

func TestExecutorRunsRetainedVersionDirectly(t *testing.T) {
	executor := testExecutor(&recordingProvider{}, 100)
	response, err := executor.ExecuteVersion(context.Background(), "test", 2, map[string]any{"name": "portfolio"}, nil)
	if err != nil {
		t.Fatalf("ExecuteVersion returned an error: %v", err)
	}
	if response.PromptVersion != 2 {
		t.Fatalf("unexpected prompt version: %+v", response)
	}
}

func TestExecutorRunsDeclaredToolLoop(t *testing.T) {
	provider := &recordingProvider{responses: []ChatResponse{
		{
			Message: Message{Role: "assistant", ToolCalls: []ToolCall{{Name: "lookup", Arguments: map[string]any{"query": "test"}}}},
			Model:   "test-model",
		},
		{Message: Message{Role: "assistant", Content: "grounded answer"}, Model: "test-model"},
	}}
	tool := &testTool{}
	definition := PromptDefinition{
		ID: "tool_test", Version: 1, Description: "Tool test.",
		SystemTemplate: "System", UserTemplate: "User",
		Tools: []string{"lookup"}, ToolSettings: ToolSettings{MaxRounds: 2, MaxCalls: 2},
		Response: ResponseSettings{Format: "text", MaxCharacters: 100, Temperature: 0.1, MaxTokens: 50},
	}
	catalog := &Catalog{
		prompts: map[promptKey]PromptDefinition{{id: "tool_test", version: 1}: definition},
		aliases: map[string]PromptReference{"tool_test": {ID: "tool_test", Version: 1}},
	}

	response, err := NewExecutor(catalog, provider).Execute(context.Background(), "tool_test", nil, []Tool{tool})
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	if response.Text != "grounded answer" || tool.arguments["query"] != "test" {
		t.Fatalf("tool loop did not complete: response=%+v arguments=%+v", response, tool.arguments)
	}
	if len(provider.requests) != 2 || provider.requests[1].Messages[len(provider.requests[1].Messages)-1].Content != "tool evidence" {
		t.Fatalf("tool result was not returned to the provider: %+v", provider.requests)
	}
}

func TestExecutorRecoversFromUndeclaredToolCall(t *testing.T) {
	provider := &recordingProvider{responses: []ChatResponse{
		{
			Message: Message{Role: "assistant", ToolCalls: []ToolCall{{Name: "analyze_portfolio"}}},
			Model:   "test-model",
		},
		{Message: Message{Role: "assistant", Content: "final portfolio answer"}, Model: "test-model"},
	}}
	tool := &testTool{}
	definition := PromptDefinition{
		ID: "tool_test", Version: 1, Description: "Tool test.",
		SystemTemplate: "System", UserTemplate: "User",
		Tools: []string{"lookup"}, ToolSettings: ToolSettings{MaxRounds: 2, MaxCalls: 2},
		Response: ResponseSettings{Format: "text", MaxCharacters: 100, Temperature: 0.1, MaxTokens: 50},
	}
	catalog := &Catalog{
		prompts: map[promptKey]PromptDefinition{{id: "tool_test", version: 1}: definition},
		aliases: map[string]PromptReference{"tool_test": {ID: "tool_test", Version: 1}},
	}

	response, err := NewExecutor(catalog, provider).Execute(context.Background(), "tool_test", nil, []Tool{tool})
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	if response.Text != "final portfolio answer" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if len(provider.requests) != 2 || len(provider.requests[1].Tools) != 0 {
		t.Fatalf("expected one tool-free recovery request: %+v", provider.requests)
	}
	messages := provider.requests[1].Messages
	toolResult := messages[len(messages)-1]
	if toolResult.Role != "tool" || toolResult.ToolName != "analyze_portfolio" || !strings.Contains(toolResult.Content, "not available") {
		t.Fatalf("unexpected undeclared-tool result: %+v", toolResult)
	}
	if tool.arguments != nil {
		t.Fatalf("declared tool should not have executed: %+v", tool.arguments)
	}
}

func testExecutor(provider Provider, maxCharacters int) *Executor {
	definition := PromptDefinition{
		ID:          "test",
		Version:     2,
		Description: "Test prompt.",
		Parameters: []ParameterDefinition{
			{Name: "name", Type: "string", Required: true, Description: "Name to analyze."},
		},
		SystemTemplate: "System for {{.name}}",
		UserTemplate:   "Analyze {{.name}}",
		Response: ResponseSettings{
			Format:        "text",
			MaxCharacters: maxCharacters,
			Temperature:   0.3,
			MaxTokens:     100,
		},
	}
	catalog := &Catalog{
		prompts: map[promptKey]PromptDefinition{{id: "test", version: 2}: definition},
		aliases: map[string]PromptReference{"active_test": {ID: "test", Version: 2}},
	}
	return NewExecutor(catalog, provider)
}
