package ai

// This file verifies portfolio context mapping into the active catalog prompt.

import (
	"context"
	"strings"
	"testing"
	"time"

	"wails/analytics"
	"wails/portfolio"
)

type recordingProvider struct {
	requests  []ChatRequest
	responses []ChatResponse
}

const validPortfolioResume = `## Overall assessment
Summary.
## Material findings
Finding.
## Diversifying factors and strengths
Factors.
## Risk-management considerations
Considerations.
## Data coverage and sources
Coverage.`

const validPortfolioNews = `## Portfolio news
### AAPL — Apple Inc.
No reliable recent item was found.
## Coverage and sources
Covered one of one positions.`

const validStockNews = `## Latest company news
### AAPL — Apple Inc.
No reliable recent item was found.
## Coverage and sources
Research completed at the supplied timestamp.`

func (p *recordingProvider) Chat(_ context.Context, request ChatRequest) (ChatResponse, error) {
	p.requests = append(p.requests, request)
	if len(p.responses) == 0 {
		return ChatResponse{Message: Message{Role: "assistant", Content: "summary"}, Model: "test-model"}, nil
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

type unusedResearchProvider struct{}

func (unusedResearchProvider) Search(context.Context, string, int) ([]SearchResult, error) {
	return nil, nil
}

func (unusedResearchProvider) Fetch(context.Context, string) (FetchedSource, error) {
	return FetchedSource{}, nil
}

func TestPortfolioResumeExecutesActiveCatalogPrompt(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	provider := &recordingProvider{responses: []ChatResponse{{
		Message: Message{Role: "assistant", Content: validPortfolioResume}, Model: "test-model",
	}}}
	researchTools := NewResearchToolsFactory(unusedResearchProvider{}, []string{"sec.gov"}, 5)
	counselor := NewCounselor(NewExecutor(catalog, provider), researchTools)
	calculatedAt := time.Date(2026, time.September, 17, 12, 0, 0, 0, time.UTC)

	snapshot := portfolio.Portfolio{Positions: []portfolio.Position{{
		Instrument: portfolio.Instrument{
			Symbol: "AAPL", Name: "Apple Inc.", AssetClass: "STK", Sector: "Technology", Currency: "USD",
		},
		Quantity: 3,
	}}}
	result := analytics.Result{
		CalculatedAt: calculatedAt,
		BaseCurrency: "USD",
		PositionWeights: []analytics.PositionWeight{
			{Weight: analytics.Ratio{Value: 0.5, Available: true}},
		},
		SectorAllocation: []analytics.Allocation{
			{Label: "Technology", Weight: analytics.Ratio{Value: 0.5, Available: true}},
		},
		TopThreeConcentration: analytics.Ratio{Value: 0.5, Available: true},
		HHI:                   analytics.Ratio{Value: 0.25, Available: true},
	}

	response, err := counselor.PortfolioResume(context.Background(), snapshot, result)
	if err != nil {
		t.Fatalf("PortfolioResume returned an error: %v", err)
	}
	if response.Text != validPortfolioResume || response.PromptID != PortfolioResumePromptAlias || response.PromptVersion != 2 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("expected one provider request, got %d", len(provider.requests))
	}
	request := provider.requests[0]
	userPrompt := request.Messages[1].Content
	for _, expected := range []string{
		"AAPL | Apple Inc.",
		"portfolio weight: 50.0%",
		"Technology: 50.0%",
		"Top-three concentration: 50.0%",
	} {
		if !strings.Contains(userPrompt, expected) {
			t.Fatalf("rendered prompt does not contain %q:\n%s", expected, userPrompt)
		}
	}
	if request.Settings.Temperature != 0.2 || request.Settings.MaxTokens != 1200 {
		t.Fatalf("unexpected response settings: %+v", request.Settings)
	}
	if len(request.Tools) != 2 {
		t.Fatalf("expected two research tools, got %+v", request.Tools)
	}
}

func TestPortfolioResumeRejectsEmptyPortfolio(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	counselor := NewCounselor(NewExecutor(catalog, &recordingProvider{}), nil)
	if _, err := counselor.PortfolioResume(context.Background(), portfolio.Portfolio{}, analytics.Result{}); err == nil {
		t.Fatal("expected an empty portfolio error")
	}
}

func TestPortfolioNewsExecutesActiveCatalogPrompt(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	provider := &recordingProvider{responses: []ChatResponse{{
		Message: Message{Role: "assistant", Content: validPortfolioNews}, Model: "test-model",
	}}}
	researchTools := NewResearchToolsFactory(unusedResearchProvider{}, []string{"sec.gov"}, 5)
	counselor := NewCounselor(NewExecutor(catalog, provider), researchTools)
	researchTime := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	snapshot := portfolio.Portfolio{Positions: []portfolio.Position{
		{Instrument: portfolio.Instrument{Symbol: "AAPL", Name: "Apple Inc.", AssetClass: "STK"}},
		{Instrument: portfolio.Instrument{Symbol: "MSFT", Name: "Microsoft Corp.", AssetClass: "STK"}},
	}}

	response, err := counselor.PortfolioNews(context.Background(), snapshot, researchTime)
	if err != nil {
		t.Fatalf("PortfolioNews returned an error: %v", err)
	}
	if response.Text != validPortfolioNews || response.PromptID != PortfolioNewsPromptAlias || response.PromptVersion != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	request := provider.requests[0]
	userPrompt := request.Messages[1].Content
	for _, expected := range []string{
		"2026-09-20T12:00:00Z",
		"AAPL | Apple Inc. | asset class: STK",
		"MSFT | Microsoft Corp. | asset class: STK",
	} {
		if !strings.Contains(userPrompt, expected) {
			t.Fatalf("rendered prompt does not contain %q:\n%s", expected, userPrompt)
		}
	}
	if strings.Contains(userPrompt, "quantity") || strings.Contains(userPrompt, "portfolio weight") {
		t.Fatalf("portfolio-news prompt contains private position data:\n%s", userPrompt)
	}
	if request.Settings.Temperature != 0.1 || request.Settings.MaxTokens != 3500 || len(request.Tools) != 2 {
		t.Fatalf("unexpected portfolio-news request: %+v", request)
	}
}

func TestPortfolioNewsRejectsEmptyPortfolio(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	counselor := NewCounselor(NewExecutor(catalog, &recordingProvider{}), nil)
	if _, err := counselor.PortfolioNews(context.Background(), portfolio.Portfolio{}, time.Now()); err == nil {
		t.Fatal("expected an empty portfolio error")
	}
}

func TestStockNewsExecutesActiveCatalogPrompt(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	provider := &recordingProvider{responses: []ChatResponse{{
		Message: Message{Role: "assistant", Content: validStockNews}, Model: "test-model",
	}}}
	researchTools := NewResearchToolsFactory(unusedResearchProvider{}, []string{"sec.gov"}, 5)
	counselor := NewCounselor(NewExecutor(catalog, provider), researchTools)
	researchTime := time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC)
	instrument := portfolio.Instrument{Symbol: "AAPL", Name: "Apple Inc.", AssetClass: "STK"}

	response, err := counselor.StockNews(context.Background(), instrument, researchTime)
	if err != nil {
		t.Fatalf("StockNews returned an error: %v", err)
	}
	if response.Text != validStockNews || response.PromptID != StockNewsPromptAlias || response.PromptVersion != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	request := provider.requests[0]
	userPrompt := request.Messages[1].Content
	for _, expected := range []string{"2026-09-25T12:00:00Z", "AAPL | Apple Inc. | asset class: STK"} {
		if !strings.Contains(userPrompt, expected) {
			t.Fatalf("rendered prompt does not contain %q:\n%s", expected, userPrompt)
		}
	}
	if request.Settings.Temperature != 0.1 || request.Settings.MaxTokens != 2000 || len(request.Tools) != 2 {
		t.Fatalf("unexpected stock-news request: %+v", request)
	}
}

func TestStockNewsRejectsMissingPublicIdentity(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	counselor := NewCounselor(NewExecutor(catalog, &recordingProvider{}), nil)
	if _, err := counselor.StockNews(context.Background(), portfolio.Instrument{}, time.Now()); err == nil {
		t.Fatal("expected a missing stock identity error")
	}
}
