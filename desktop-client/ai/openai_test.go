package ai

// This file verifies OpenAI Responses API mapping, tool-call correlation, and bounded failures.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIProviderChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %q", request.Header.Get("Authorization"))
		}
		var payload openAIResponseRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "test-model" || payload.Store || len(payload.Input) != 2 || len(payload.Tools) != 1 {
			t.Fatalf("unexpected request: %+v", payload)
		}
		if payload.Text == nil || payload.Text.Format.Type != "json_schema" || !payload.Text.Format.Strict {
			t.Fatalf("structured output was not configured: %+v", payload.Text)
		}
		if payload.Temperature == nil || *payload.Temperature != 0.2 || payload.MaxOutputTokens != 400 {
			t.Fatalf("unexpected generation settings: %+v", payload)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"resp_test","status":"completed","model":"test-model-2026-01-01","service_tier":"default","reasoning":{"effort":"low"},"usage":{"input_tokens":120,"input_tokens_details":{"cached_tokens":20,"cache_write_tokens":0},"output_tokens":35,"output_tokens_details":{"reasoning_tokens":10},"total_tokens":155},"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"{\"summary\":\"ok\"}"}]}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(server.URL+"/v1", "test-key", "test-model")
	response, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "system", Content: "system"}, {Role: "user", Content: "prompt"}},
		Tools: []ToolDefinition{{
			Name: "test_tool", Description: "Test.", Parameters: map[string]any{"type": "object"},
		}},
		ResponseSchema: map[string]any{"type": "object"},
		Settings:       GenerationSettings{Temperature: 0.2, MaxTokens: 400, ReasoningEffort: "low"},
	})
	if err != nil {
		t.Fatalf("Chat returned an error: %v", err)
	}
	if response.Message.Content != `{"summary":"ok"}` || response.Model != "test-model-2026-01-01" || response.FinishReason != "completed" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.ResponseID != "resp_test" || response.ServiceTier != "default" || response.EffectiveReasoningEffort != "low" || response.Usage.TotalTokens != 155 || response.Usage.CachedInputTokens != 20 || response.Usage.ReasoningTokens != 10 {
		t.Fatalf("response metadata was not retained: %+v", response)
	}
}

func TestOpenAIProviderOmitsTemperatureForReasoningModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload openAIResponseRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "gpt-6-luna" || payload.Temperature != nil {
			t.Fatalf("reasoning-model request included temperature: %+v", payload)
		}
		if payload.Reasoning == nil || payload.Reasoning.Effort != "low" {
			t.Fatalf("reasoning effort was not configured: %+v", payload.Reasoning)
		}
		_, _ = writer.Write([]byte(`{"status":"completed","model":"gpt-6-luna","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(server.URL, "test-key", "gpt-6-luna")
	if _, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "prompt"}},
		Settings: GenerationSettings{Temperature: 0.7, MaxTokens: 100, ReasoningEffort: "low"},
	}); err != nil {
		t.Fatalf("Chat returned an error: %v", err)
	}
}

func TestOpenAIProviderMapsFunctionCallsAndOutputs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload openAIResponseRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(payload.Input) != 3 {
			t.Fatalf("unexpected input items: %+v", payload.Input)
		}
		if payload.Input[1].Type != "function_call" || payload.Input[1].CallID != "call_123" || payload.Input[1].Name != "search_instrument" {
			t.Fatalf("function call was not replayed: %+v", payload.Input[1])
		}
		if payload.Input[2].Type != "function_call_output" || payload.Input[2].CallID != "call_123" || payload.Input[2].Output != "result" {
			t.Fatalf("function output was not mapped: %+v", payload.Input[2])
		}
		_, _ = writer.Write([]byte(`{"status":"completed","model":"test-model","output":[{"type":"function_call","call_id":"call_456","name":"fetch_source","arguments":"{\"url\":\"https://example.test\"}"}]}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(server.URL, "test-key", "test-model")
	response, err := provider.Chat(context.Background(), ChatRequest{Messages: []Message{
		{Role: "user", Content: "prompt"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_123", Name: "search_instrument", Arguments: map[string]any{"ticker": "AAPL"}}}},
		{Role: "tool", ToolName: "search_instrument", ToolCallID: "call_123", Content: "result"},
	}})
	if err != nil {
		t.Fatalf("Chat returned an error: %v", err)
	}
	if response.FinishReason != "tool_calls" || len(response.Message.ToolCalls) != 1 {
		t.Fatalf("unexpected function-call response: %+v", response)
	}
	call := response.Message.ToolCalls[0]
	if call.ID != "call_456" || call.Name != "fetch_source" || call.Arguments["url"] != "https://example.test" {
		t.Fatalf("function call was not mapped: %+v", call)
	}
}

func TestOpenAIProviderRequiresAPIKey(t *testing.T) {
	_, err := NewOpenAIProvider("https://api.openai.com/v1", "", "test-model").Chat(context.Background(), ChatRequest{})
	if err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("expected a missing-key error, got %v", err)
	}
}

func TestOpenAIProviderReportsBoundedAPIErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(writer).Encode(map[string]any{"error": map[string]string{"message": strings.Repeat("x", 2000)}})
	}))
	defer server.Close()

	_, err := NewOpenAIProvider(server.URL, "test-key", "test-model").Chat(context.Background(), ChatRequest{})
	if err == nil || len(err.Error()) > 600 || !strings.HasSuffix(err.Error(), "...") {
		t.Fatalf("provider error was not bounded: length=%d error=%v", len(err.Error()), err)
	}
}
