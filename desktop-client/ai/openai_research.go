package ai

// This file implements privacy-isolated research through OpenAI's hosted web-search tool.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const openAIResearchMaxOutputTokens = 4000

// OpenAIResearchProvider searches public sources without receiving portfolio or order context.
type OpenAIResearchProvider struct {
	baseURL        string
	apiKey         string
	model          string
	allowedDomains []string
	client         *http.Client
}

type openAIResearchRequest struct {
	Model           string                `json:"model"`
	Input           string                `json:"input"`
	Tools           []openAIWebSearchTool `json:"tools"`
	ToolChoice      string                `json:"tool_choice"`
	Include         []string              `json:"include"`
	Text            openAITextConfig      `json:"text"`
	MaxOutputTokens int                   `json:"max_output_tokens"`
	Store           bool                  `json:"store"`
}

type openAIWebSearchTool struct {
	Type              string                 `json:"type"`
	Filters           openAIWebSearchFilters `json:"filters"`
	SearchContextSize string                 `json:"search_context_size"`
	ExternalWebAccess bool                   `json:"external_web_access"`
}

type openAIWebSearchFilters struct {
	AllowedDomains []string `json:"allowed_domains"`
}

type openAIWebSearchAction struct {
	Sources []openAIWebSearchSource `json:"sources"`
}

type openAIWebSearchSource struct {
	URL string `json:"url"`
}

type openAIResearchResults struct {
	Results []SearchResult `json:"results"`
}

type openAIFetchedResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// NewOpenAIResearchProvider creates a hosted search adapter that shares the selected OpenAI credentials.
func NewOpenAIResearchProvider(baseURL, apiKey, model string, allowedDomains []string) *OpenAIResearchProvider {
	domains := make([]string, 0, len(allowedDomains))
	for _, domain := range allowedDomains {
		if normalized := normalizeDomain(domain); normalized != "" {
			domains = append(domains, normalized)
		}
	}
	return &OpenAIResearchProvider{
		baseURL:        strings.TrimRight(baseURL, "/"),
		apiKey:         strings.TrimSpace(apiKey),
		model:          strings.TrimSpace(model),
		allowedDomains: domains,
		client:         &http.Client{Timeout: 30 * time.Second},
	}
}

// Search performs one required web search and returns only URLs reported by the hosted tool.
func (p *OpenAIResearchProvider) Search(ctx context.Context, query string, maximum int) ([]SearchResult, error) {
	if maximum < 1 {
		return []SearchResult{}, nil
	}
	if maximum > 10 {
		maximum = 10
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"results": map[string]any{
				"type":     "array",
				"maxItems": maximum,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title":   map[string]any{"type": "string"},
						"url":     map[string]any{"type": "string"},
						"content": map[string]any{"type": "string"},
					},
					"required":             []string{"title", "url", "content"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"results"},
		"additionalProperties": false,
	}
	prompt := fmt.Sprintf(
		"Search the live web for this exact public-market research query: %q. Return at most %d distinct, relevant sources. Use each source's exact URL and title, and summarize only factual evidence supported by that source. Do not invent URLs or facts.",
		strings.TrimSpace(query), maximum,
	)
	text, sources, err := p.run(ctx, prompt, "research_results", schema, p.allowedDomains)
	if err != nil {
		return nil, err
	}
	var decoded openAIResearchResults
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return nil, fmt.Errorf("OpenAI web search returned invalid structured results: %w", err)
	}
	verified := make([]SearchResult, 0, len(decoded.Results))
	for _, result := range decoded.Results {
		if _, ok := sources[researchURLKey(result.URL)]; !ok {
			continue
		}
		verified = append(verified, result)
		if len(verified) >= maximum {
			break
		}
	}
	if len(decoded.Results) > 0 && len(verified) == 0 {
		return nil, fmt.Errorf("OpenAI web search returned results without verifiable source URLs")
	}
	return verified, nil
}

// Fetch retrieves evidence for one URL already accepted by the application research session.
func (p *OpenAIResearchProvider) Fetch(ctx context.Context, sourceURL string) (FetchedSource, error) {
	parsed, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return FetchedSource{}, fmt.Errorf("OpenAI web fetch requires an absolute HTTPS URL")
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title":   map[string]any{"type": "string"},
			"url":     map[string]any{"type": "string"},
			"content": map[string]any{"type": "string"},
		},
		"required":             []string{"title", "url", "content"},
		"additionalProperties": false,
	}
	prompt := fmt.Sprintf(
		"Use live web search to retrieve this exact public source URL: %s. Return its title, the exact URL, and a detailed factual summary of the page content. If the page cannot be retrieved, return empty title and content fields without substituting another URL.",
		sourceURL,
	)
	text, sources, err := p.run(ctx, prompt, "fetched_source", schema, []string{strings.ToLower(parsed.Hostname())})
	if err != nil {
		return FetchedSource{}, err
	}
	if _, ok := sources[researchURLKey(sourceURL)]; !ok {
		return FetchedSource{}, fmt.Errorf("OpenAI web fetch did not retrieve the requested source URL")
	}
	var decoded openAIFetchedResult
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return FetchedSource{}, fmt.Errorf("OpenAI web fetch returned an invalid structured result: %w", err)
	}
	if researchURLKey(decoded.URL) != researchURLKey(sourceURL) {
		return FetchedSource{}, fmt.Errorf("OpenAI web fetch returned a different source URL")
	}
	return FetchedSource{Title: decoded.Title, URL: sourceURL, Content: decoded.Content}, nil
}

// run executes a stateless hosted-tool request and returns structured text plus consulted URLs.
func (p *OpenAIResearchProvider) run(
	ctx context.Context,
	prompt string,
	formatName string,
	schema map[string]any,
	allowedDomains []string,
) (string, map[string]struct{}, error) {
	if p.apiKey == "" {
		return "", nil, fmt.Errorf("OpenAI web research is unavailable because OPENAI_API_KEY is not configured")
	}
	payload := openAIResearchRequest{
		Model: p.model,
		Input: prompt,
		Tools: []openAIWebSearchTool{{
			Type:              "web_search",
			Filters:           openAIWebSearchFilters{AllowedDomains: allowedDomains},
			SearchContextSize: "medium",
			ExternalWebAccess: true,
		}},
		ToolChoice:      "required",
		Include:         []string{"web_search_call.action.sources"},
		Text:            openAITextConfig{Format: openAITextFormat{Type: "json_schema", Name: formatName, Strict: true, Schema: schema}},
		MaxOutputTokens: openAIResearchMaxOutputTokens,
		Store:           false,
	}
	contents, err := json.Marshal(payload)
	if err != nil {
		return "", nil, fmt.Errorf("encode OpenAI web research request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/responses", bytes.NewReader(contents))
	if err != nil {
		return "", nil, fmt.Errorf("create OpenAI web research request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+p.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		return "", nil, fmt.Errorf("OpenAI web research is not reachable: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxOpenAIResponseBytes+1))
	if err != nil {
		return "", nil, fmt.Errorf("read OpenAI web research response: %w", err)
	}
	if len(body) > maxOpenAIResponseBytes {
		return "", nil, fmt.Errorf("OpenAI web research response exceeded the %d-byte limit", maxOpenAIResponseBytes)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var failure openAIErrorEnvelope
		_ = json.Unmarshal(body, &failure)
		message := response.Status
		if failure.Error != nil && strings.TrimSpace(failure.Error.Message) != "" {
			message = failure.Error.Message
		}
		return "", nil, fmt.Errorf("OpenAI web research failed: %s", boundedProviderMessage(message))
	}
	var generated openAIResponse
	if err := json.Unmarshal(body, &generated); err != nil {
		return "", nil, fmt.Errorf("OpenAI web research returned an invalid response: %w", err)
	}
	if generated.Error != nil {
		return "", nil, fmt.Errorf("OpenAI web research failed: %s", boundedProviderMessage(generated.Error.Message))
	}
	if generated.Status != "completed" {
		return "", nil, fmt.Errorf("OpenAI web research did not complete: %s", boundedProviderMessage(generated.Status))
	}
	var textParts []string
	sources := make(map[string]struct{})
	for _, item := range generated.Output {
		for _, content := range item.Content {
			if content.Type == "output_text" {
				textParts = append(textParts, content.Text)
			}
		}
		if item.Action != nil {
			for _, source := range item.Action.Sources {
				if key := researchURLKey(source.URL); key != "" {
					sources[key] = struct{}{}
				}
			}
		}
	}
	text := strings.Join(textParts, "")
	if strings.TrimSpace(text) == "" {
		return "", nil, fmt.Errorf("OpenAI web research returned no structured text")
	}
	return text, sources, nil
}

func researchURLKey(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return ""
	}
	parsed.Scheme = "https"
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed.String()
}
