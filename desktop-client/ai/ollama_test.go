package ai

// This file verifies Ollama chat mapping and user-facing API failures.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaProviderChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		var payload ollamaChatRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "test-model" || payload.Stream || len(payload.Messages) != 2 || len(payload.Tools) != 1 {
			t.Fatalf("unexpected request: %+v", payload)
		}
		if payload.Options.Temperature != 0.2 || payload.Options.MaxTokens != 400 {
			t.Fatalf("unexpected generation options: %+v", payload.Options)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"model":"test-model","message":{"role":"assistant","content":"portfolio summary"},"done":true}`))
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL+"/api", "test-model")
	response, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "system", Content: "system"}, {Role: "user", Content: "prompt"}},
		Tools: []ToolDefinition{{
			Name: "test_tool", Description: "Test.", Parameters: map[string]any{"type": "object"},
		}},
		Settings: GenerationSettings{Temperature: 0.2, MaxTokens: 400},
	})
	if err != nil {
		t.Fatalf("Chat returned an error: %v", err)
	}
	if response.Message.Content != "portfolio summary" || response.Model != "test-model" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestOllamaProviderMapsToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"model":"test","message":{"role":"assistant","tool_calls":[{"function":{"name":"search_instrument","arguments":{"ticker":"AAPL"}}}]}}`))
	}))
	defer server.Close()
	provider := NewOllamaProvider(server.URL, "test")
	response, err := provider.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("Chat returned an error: %v", err)
	}
	if len(response.Message.ToolCalls) != 1 || response.Message.ToolCalls[0].Name != "search_instrument" {
		t.Fatalf("tool calls were not mapped: %+v", response)
	}
}

func TestOllamaProviderReportsAPIErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"error":"model not found"}`))
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL+"/api", "missing")
	_, err := provider.Chat(context.Background(), ChatRequest{})
	if err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("expected a model error, got %v", err)
	}
}

func TestOllamaProviderReportsUnreachableServer(t *testing.T) {
	provider := NewOllamaProvider("http://127.0.0.1:1/api", "test-model")
	_, err := provider.Chat(context.Background(), ChatRequest{})
	if err == nil || !strings.Contains(err.Error(), "Ollama is not reachable") {
		t.Fatalf("expected an unreachable server error, got %v", err)
	}
}
