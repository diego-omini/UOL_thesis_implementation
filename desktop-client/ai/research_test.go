package ai

// This file verifies search privacy, source filtering, and fetch-session restrictions.

import (
	"context"
	"strings"
	"testing"
)

type recordingResearchProvider struct {
	query      string
	results    []SearchResult
	fetchedURL string
}

func (p *recordingResearchProvider) Search(_ context.Context, query string, _ int) ([]SearchResult, error) {
	p.query = query
	return p.results, nil
}

func (p *recordingResearchProvider) Fetch(_ context.Context, sourceURL string) (FetchedSource, error) {
	p.fetchedURL = sourceURL
	return FetchedSource{Title: "Filing", URL: sourceURL, Content: "Evidence"}, nil
}

func TestInstrumentSearchUsesOnlyPublicIdentityAndFixedTopic(t *testing.T) {
	provider := &recordingResearchProvider{results: []SearchResult{
		{Title: "SEC filing", URL: "https://www.sec.gov/example", Content: "Primary evidence"},
		{Title: "Untrusted", URL: "https://example.com/story", Content: "Ignored"},
	}}
	tools := NewResearchToolsFactory(provider, []string{"sec.gov"}, 5).ForInstruments([]PublicInstrument{
		{Name: "Apple Inc.", Ticker: "AAPL"},
	})

	result, err := tools[0].Execute(context.Background(), map[string]any{
		"name": "Apple Inc.", "ticker": "AAPL", "topic": "company_risk",
	})
	if err != nil {
		t.Fatalf("search returned an error: %v", err)
	}
	if strings.Contains(provider.query, "%") || strings.Contains(provider.query, "portfolio") || strings.Contains(provider.query, "quantity") {
		t.Fatalf("search query leaked private context: %q", provider.query)
	}
	if !strings.Contains(provider.query, "Apple Inc. AAPL") || !strings.Contains(result, "sec.gov") || !strings.Contains(result, "retrieved_at") || strings.Contains(result, "example.com") {
		t.Fatalf("unexpected filtered research: query=%q result=%s", provider.query, result)
	}

	fetched, err := tools[1].Execute(context.Background(), map[string]any{"url": "https://www.sec.gov/example"})
	if err != nil || !strings.Contains(fetched, "Evidence") || !strings.Contains(fetched, "retrieved_at") {
		t.Fatalf("approved fetch failed: result=%q err=%v", fetched, err)
	}
	if _, err := tools[1].Execute(context.Background(), map[string]any{"url": "https://www.sec.gov/not-searched"}); err == nil {
		t.Fatal("expected an unsearched URL to be rejected")
	}
}

func TestInstrumentSearchRejectsUnknownIdentity(t *testing.T) {
	provider := &recordingResearchProvider{}
	tool := NewResearchToolsFactory(provider, []string{"sec.gov"}, 5).ForInstruments([]PublicInstrument{
		{Name: "Apple Inc.", Ticker: "AAPL"},
	})[0]
	if _, err := tool.Execute(context.Background(), map[string]any{
		"name": "Other Corp", "ticker": "OTHER", "topic": "company_risk",
	}); err == nil {
		t.Fatal("expected an unknown instrument to be rejected")
	}
}

func TestInstrumentSearchEnforcesResultAndContentLimits(t *testing.T) {
	provider := &recordingResearchProvider{results: []SearchResult{
		{Title: strings.Repeat("A", 400), URL: "https://www.sec.gov/first", Content: strings.Repeat("B", 3000)},
		{Title: "Second", URL: "https://www.sec.gov/second", Content: "must not be returned"},
	}}
	tool := NewResearchToolsFactory(provider, []string{"sec.gov"}, 1).ForInstruments([]PublicInstrument{
		{Name: "Apple Inc.", Ticker: "AAPL"},
	})[0]

	result, err := tool.Execute(context.Background(), map[string]any{
		"name": "Apple Inc.", "ticker": "AAPL", "topic": "company_risk",
	})
	if err != nil {
		t.Fatalf("search returned an error: %v", err)
	}
	if strings.Contains(result, "/second") || len([]rune(result)) > 2400 {
		t.Fatalf("research limits were not enforced: %s", result)
	}
}
