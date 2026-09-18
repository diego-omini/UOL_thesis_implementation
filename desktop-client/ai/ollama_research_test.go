package ai

// This file verifies authenticated Ollama web-search and fetch requests.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaResearchProviderSearchAndFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("missing authorization header")
		}
		switch request.URL.Path {
		case "/api/web_search":
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode search request: %v", err)
			}
			if body["query"] != "Apple AAPL filings" {
				t.Fatalf("unexpected query: %+v", body)
			}
			_, _ = writer.Write([]byte(`{"results":[{"title":"Filing","url":"https://sec.gov/a","content":"Evidence"}]}`))
		case "/api/web_fetch":
			_, _ = writer.Write([]byte(`{"title":"Filing","content":"Full evidence"}`))
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewOllamaResearchProvider(server.URL+"/api", "secret")
	results, err := provider.Search(context.Background(), "Apple AAPL filings", 3)
	if err != nil || len(results) != 1 {
		t.Fatalf("Search failed: results=%+v err=%v", results, err)
	}
	fetched, err := provider.Fetch(context.Background(), results[0].URL)
	if err != nil || fetched.Content != "Full evidence" {
		t.Fatalf("Fetch failed: result=%+v err=%v", fetched, err)
	}
}

func TestOllamaResearchProviderRequiresAPIKey(t *testing.T) {
	provider := NewOllamaResearchProvider("https://ollama.com/api", "")
	_, err := provider.Search(context.Background(), "test", 3)
	if err == nil || !strings.Contains(err.Error(), "OLLAMA_API_KEY") {
		t.Fatalf("expected a missing API key error, got %v", err)
	}
}
