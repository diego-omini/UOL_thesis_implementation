package ai

// This file implements Ollama's authenticated web-search and page-fetch APIs.

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

const maxResearchResponseBytes = 8 << 20

// OllamaResearchProvider accesses Ollama-hosted web research independently from local inference.
type OllamaResearchProvider struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

type ollamaSearchResponse struct {
	Results []SearchResult `json:"results"`
	Error   string         `json:"error"`
}

type ollamaFetchResponse struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Error   string `json:"error"`
}

// NewOllamaResearchProvider creates the hosted research adapter.
func NewOllamaResearchProvider(baseURL, apiKey string) *OllamaResearchProvider {
	return &OllamaResearchProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  strings.TrimSpace(apiKey),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Search returns public web results for a privacy-filtered query.
func (p *OllamaResearchProvider) Search(ctx context.Context, query string, maximum int) ([]SearchResult, error) {
	var response ollamaSearchResponse
	if err := p.post(ctx, "/web_search", map[string]any{
		"query":       query,
		"max_results": maximum,
	}, &response); err != nil {
		return nil, err
	}
	if response.Error != "" {
		return nil, fmt.Errorf("Ollama web search failed: %s", boundedProviderMessage(response.Error))
	}
	return response.Results, nil
}

// Fetch returns readable page content for an approved result URL.
func (p *OllamaResearchProvider) Fetch(ctx context.Context, sourceURL string) (FetchedSource, error) {
	var response ollamaFetchResponse
	if err := p.post(ctx, "/web_fetch", map[string]string{"url": sourceURL}, &response); err != nil {
		return FetchedSource{}, err
	}
	if response.Error != "" {
		return FetchedSource{}, fmt.Errorf("Ollama web fetch failed: %s", boundedProviderMessage(response.Error))
	}
	return FetchedSource{Title: response.Title, URL: sourceURL, Content: response.Content}, nil
}

func (p *OllamaResearchProvider) post(ctx context.Context, path string, payload any, target any) error {
	if p.apiKey == "" {
		return fmt.Errorf("Ollama web research is unavailable because OLLAMA_API_KEY is not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Ollama research request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Ollama research request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+p.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("Ollama web research is not reachable: %w", err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxResearchResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read Ollama research response: %w", err)
	}
	if len(contents) > maxResearchResponseBytes {
		return fmt.Errorf("Ollama web research response exceeded the %d-byte limit", maxResearchResponseBytes)
	}
	if err := json.Unmarshal(contents, target); err != nil {
		return fmt.Errorf("Ollama web research returned an invalid response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Ollama web research failed with %s", response.Status)
	}
	return nil
}
