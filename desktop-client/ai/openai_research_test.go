package ai

// This file verifies isolated OpenAI web-search requests and source URL enforcement.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIResearchProviderSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %q", request.Header.Get("Authorization"))
		}
		var payload openAIResearchRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "gpt-6-luna" || payload.ToolChoice != "required" || payload.Store {
			t.Fatalf("unexpected request settings: %+v", payload)
		}
		if len(payload.Tools) != 1 || payload.Tools[0].Type != "web_search" || !payload.Tools[0].ExternalWebAccess {
			t.Fatalf("web search was not enabled: %+v", payload.Tools)
		}
		if len(payload.Tools[0].Filters.AllowedDomains) != 2 || payload.Tools[0].Filters.AllowedDomains[0] != "sec.gov" {
			t.Fatalf("source domains were not enforced: %+v", payload.Tools[0].Filters)
		}
		if !strings.Contains(payload.Input, "Apple Inc. AAPL") || strings.Contains(payload.Input, "portfolio") || strings.Contains(payload.Input, "quantity") {
			t.Fatalf("research request contained unexpected context: %q", payload.Input)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"status":"completed",
			"output":[
				{"type":"web_search_call","action":{"type":"search","sources":[{"type":"url","url":"https://www.sec.gov/example"}]}},
				{"type":"message","content":[{"type":"output_text","text":"{\"results\":[{\"title\":\"Filing\",\"url\":\"https://www.sec.gov/example\",\"content\":\"Evidence\"},{\"title\":\"Invented\",\"url\":\"https://www.sec.gov/invented\",\"content\":\"Ignore\"}]}"}]}
			]
		}`))
	}))
	defer server.Close()

	provider := NewOpenAIResearchProvider(server.URL+"/v1", "test-key", "gpt-6-luna", []string{"sec.gov", "reuters.com"})
	results, err := provider.Search(context.Background(), "Apple Inc. AAPL official filings", 5)
	if err != nil {
		t.Fatalf("Search returned an error: %v", err)
	}
	if len(results) != 1 || results[0].URL != "https://www.sec.gov/example" || results[0].Content != "Evidence" {
		t.Fatalf("unexpected verified results: %+v", results)
	}
}

func TestOpenAIResearchProviderFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload openAIResearchRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(payload.Tools[0].Filters.AllowedDomains) != 1 || payload.Tools[0].Filters.AllowedDomains[0] != "www.sec.gov" {
			t.Fatalf("fetch was not restricted to the source domain: %+v", payload.Tools[0].Filters)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"status":"completed",
			"output":[
				{"type":"web_search_call","action":{"type":"open_page","sources":[{"type":"url","url":"https://www.sec.gov/example/"}]}},
				{"type":"message","content":[{"type":"output_text","text":"{\"title\":\"Filing\",\"url\":\"https://www.sec.gov/example\",\"content\":\"Detailed evidence\"}"}]}
			]
		}`))
	}))
	defer server.Close()

	provider := NewOpenAIResearchProvider(server.URL, "test-key", "gpt-6-luna", []string{"sec.gov"})
	source, err := provider.Fetch(context.Background(), "https://www.sec.gov/example")
	if err != nil {
		t.Fatalf("Fetch returned an error: %v", err)
	}
	if source.Title != "Filing" || source.URL != "https://www.sec.gov/example" || source.Content != "Detailed evidence" {
		t.Fatalf("unexpected fetched source: %+v", source)
	}
}

func TestOpenAIResearchProviderRejectsUnverifiedSources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{
			"status":"completed",
			"output":[
				{"type":"web_search_call","action":{"type":"search","sources":[{"type":"url","url":"https://www.sec.gov/actual"}]}},
				{"type":"message","content":[{"type":"output_text","text":"{\"results\":[{\"title\":\"Invented\",\"url\":\"https://www.sec.gov/invented\",\"content\":\"Unsupported\"}]}"}]}
			]
		}`))
	}))
	defer server.Close()

	provider := NewOpenAIResearchProvider(server.URL, "test-key", "gpt-6-luna", []string{"sec.gov"})
	if _, err := provider.Search(context.Background(), "test query", 5); err == nil || !strings.Contains(err.Error(), "verifiable source URLs") {
		t.Fatalf("expected an unverified-source error, got %v", err)
	}
}

func TestOpenAIResearchProviderRequiresAPIKey(t *testing.T) {
	provider := NewOpenAIResearchProvider("https://api.openai.com/v1", "", "gpt-6-luna", []string{"sec.gov"})
	if _, err := provider.Search(context.Background(), "test query", 5); err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatalf("expected a missing-key error, got %v", err)
	}
}
