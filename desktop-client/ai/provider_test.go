package ai

// This file verifies portfolio context mapping into the active catalog prompt.

import (
	"context"
	"strings"
	"testing"
	"time"

	"wails/analytics"
	"wails/portfolio"
	"wails/portfolioanalysis"
	"wails/stockanalysis"
)

type recordingProvider struct {
	requests  []ChatRequest
	responses []ChatResponse
}

const validStructuredResponse = `{"schema_version":1,"summary":"Structured summary.","observations":[{"category":"general_information","statement":"General context","supporting_fact_ids":[],"reported_values":[],"severity":"informational","confidence":"high","limitations":[],"explanation":"General information only.","citations":[]}],"limitations":[{"statement":"This is general information, not personalized investment advice.","fact_ids":[]}]}`

const validPortfolioResume = validStructuredResponse
const validPortfolioRisk = validStructuredResponse
const validPortfolioNews = validStructuredResponse
const validStockNews = validStructuredResponse
const validOrderAnalysis = validStructuredResponse

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
		CalculatedAt:       calculatedAt,
		BaseCurrency:       "USD",
		NetMarketValue:     portfolio.Money{Amount: 100, Currency: "USD", Available: true},
		GrossExposure:      portfolio.Money{Amount: 100, Currency: "USD", Available: true},
		TotalUnrealizedPnL: portfolio.Money{Amount: 5, Currency: "USD", Available: true},
		PositionWeights: []analytics.PositionWeight{
			{Weight: analytics.Ratio{Value: 0.5, Available: true}},
		},
		SectorAllocation: []analytics.Allocation{
			{Label: "Technology", Weight: analytics.Ratio{Value: 0.5, Available: true}},
		},
		EffectiveSecurityExposure: []analytics.EffectiveExposure{{
			MetricID: "effective.security.symbol_aapl.weight", Symbol: "AAPL", Name: "Apple Inc.",
			Weight: analytics.Ratio{Value: 0.6, Available: true},
		}},
		EffectiveSectorAllocation: []analytics.Allocation{{Label: "Technology", Weight: analytics.Ratio{Value: 0.6, Available: true}}},
		FundLookThrough: []portfolio.FundHoldings{{
			Fund: snapshot.Positions[0].Instrument, Available: true, Complete: true,
			SourceURL: "https://issuer.example/holdings.csv", SourceType: "official_issuer_full_holdings_csv",
			RetrievedAt: calculatedAt, EffectiveDate: calculatedAt, CoveragePercentage: portfolio.Percentage{Value: 100, Available: true}, Confidence: "high",
		}},
		EffectiveExposureComplete: true,
		TopThreeConcentration:     analytics.Ratio{Value: 0.5, Available: true},
		HHI:                       analytics.Ratio{Value: 0.25, Available: true},
	}

	response, err := counselor.PortfolioResume(context.Background(), snapshot, result)
	if err != nil {
		t.Fatalf("PortfolioResume returned an error: %v", err)
	}
	if response.Text != validPortfolioResume || response.Structured == nil || response.PromptID != PortfolioResumePromptAlias || response.PromptVersion != 3 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("expected one provider request, got %d", len(provider.requests))
	}
	request := provider.requests[0]
	userPrompt := request.Messages[1].Content
	for _, expected := range []string{
		"id=position.aapl.symbol",
		"id=position.aapl.weight",
		"id=allocation.sector.technology.weight",
		"id=effective.security.symbol_aapl.weight",
		"id=effective.sector.technology.weight",
		"id=position.aapl.look_through.coverage",
		"id=portfolio.top_three_concentration",
	} {
		if !strings.Contains(userPrompt, expected) {
			t.Fatalf("rendered prompt does not contain %q:\n%s", expected, userPrompt)
		}
	}
	if request.Settings.Temperature != 0 || request.Settings.MaxTokens != 2800 || request.ResponseSchema == nil {
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

func TestPortfolioRiskExecutesFocusedCatalogPrompt(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	provider := &recordingProvider{responses: []ChatResponse{{
		Message: Message{Role: "assistant", Content: validPortfolioRisk}, Model: "test-model",
	}}}
	counselor := NewCounselor(NewExecutor(catalog, provider), NewResearchToolsFactory(unusedResearchProvider{}, []string{"sec.gov"}, 5))
	result := portfolioanalysis.Result{
		CalculatedAt: time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC), BaseCurrency: "CHF",
		PeriodStart: time.Date(2025, time.October, 4, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, time.October, 3, 0, 0, 0, 0, time.UTC),
		Observations: 252, IncludedPositionCount: 2, TotalPositionCount: 2,
		ModeledTotalReturn: portfolioanalysis.Metric{Value: .08, Available: true}, AnnualizedVolatility: portfolioanalysis.Metric{Value: .16, Available: true},
		DownsideDeviation: portfolioanalysis.Metric{Value: .1, Available: true}, MaximumDrawdown: portfolioanalysis.Metric{Value: -.12, Available: true},
		Positions: []portfolioanalysis.PositionResult{{
			ContractID: 1, Symbol: "AAA", Name: "Alpha", SignedWeight: .6,
			TotalReturn: portfolioanalysis.Metric{Value: .1, Available: true}, AnnualizedVolatility: portfolioanalysis.Metric{Value: .2, Available: true},
			ReturnContribution: portfolioanalysis.Metric{Value: .06, Available: true}, RiskContribution: portfolioanalysis.Metric{Value: .7, Available: true},
		}},
		Correlations: []portfolioanalysis.Correlation{{LeftContractID: 1, LeftSymbol: "AAA", RightContractID: 2, RightSymbol: "BBB", Value: .35}},
		Scenarios: []portfolioanalysis.Scenario{{
			ID: "market_down_10", Name: "Broad market decline", Description: "All modeled instrument prices decline by 10%.",
			Impact: portfolio.Money{Amount: -100, Currency: "CHF", Available: true}, ImpactRatio: portfolioanalysis.Metric{Value: -.1, Available: true},
		}},
	}

	response, err := counselor.PortfolioRisk(context.Background(), result)
	if err != nil {
		t.Fatalf("PortfolioRisk returned an error: %v", err)
	}
	if response.Text != validPortfolioRisk || response.Structured == nil || response.PromptID != PortfolioRiskPromptAlias || response.PromptVersion != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	request := provider.requests[0]
	for _, expected := range []string{
		"id=risk.annualized_volatility", "id=position.contract_1.risk_contribution",
		"id=correlation.contract_1.contract_2", "id=scenario.market_down_10.impact_ratio",
	} {
		if !strings.Contains(request.Messages[1].Content, expected) {
			t.Fatalf("rendered prompt does not contain %q:\n%s", expected, request.Messages[1].Content)
		}
	}
	if request.Settings.Temperature != 0 || request.Settings.MaxTokens != 2800 || len(request.Tools) != 0 || request.ResponseSchema == nil {
		t.Fatalf("unexpected portfolio-risk request: %+v", request)
	}
}

func TestPortfolioRiskRejectsInsufficientHistory(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	counselor := NewCounselor(NewExecutor(catalog, &recordingProvider{}), nil)
	if _, err := counselor.PortfolioRisk(context.Background(), portfolioanalysis.Result{Observations: 19, IncludedPositionCount: 1}); err == nil {
		t.Fatal("expected insufficient-history error")
	}
}

func TestPortfolioRiskFactsPreservePercentUnitsAndRequiredLimitations(t *testing.T) {
	facts := portfolioRiskFacts(portfolioanalysis.Result{
		CalculatedAt: time.Now(), BaseCurrency: "CHF", Observations: 20, IncludedPositionCount: 1, TotalPositionCount: 2,
		AnnualizedVolatility: portfolioanalysis.Metric{Value: .125, Available: true},
		Warnings:             []string{"One position lacked sufficient history."},
	})
	var volatility, warning *CounselorFact
	for index := range facts {
		switch facts[index].ID {
		case "risk.annualized_volatility":
			volatility = &facts[index]
		case "warning.portfolio_risk.1":
			warning = &facts[index]
		}
	}
	if volatility == nil || volatility.NumericValue == nil || *volatility.NumericValue != 12.5 || volatility.Unit != "percent" {
		t.Fatalf("unexpected volatility fact: %+v", volatility)
	}
	if warning == nil || warning.Available || !warning.RequiredLimitation || warning.Limitation == "" {
		t.Fatalf("unexpected warning fact: %+v", warning)
	}
}

func TestPortfolioRiskFactsBoundCorrelationContext(t *testing.T) {
	result := portfolioanalysis.Result{CalculatedAt: time.Now(), BaseCurrency: "CHF"}
	for index := 0; index < 13; index++ {
		result.Correlations = append(result.Correlations, portfolioanalysis.Correlation{
			LeftContractID: int64(index + 1), RightContractID: int64(index + 101), LeftSymbol: "LEFT", RightSymbol: "RIGHT", Value: .5,
		})
	}
	facts := portfolioRiskFacts(result)
	correlations := 0
	coverageWarning := false
	for _, fact := range facts {
		if strings.HasPrefix(fact.ID, "correlation.contract_") {
			correlations++
		}
		if fact.ID == "warning.portfolio_risk.correlation_coverage" && fact.RequiredLimitation {
			coverageWarning = true
		}
	}
	if correlations != 12 || !coverageWarning {
		t.Fatalf("unexpected bounded correlation facts: correlations=%d warning=%v", correlations, coverageWarning)
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
	if response.Text != validPortfolioNews || response.Structured == nil || response.PromptID != PortfolioNewsPromptAlias || response.PromptVersion != 2 {
		t.Fatalf("unexpected response: %+v", response)
	}
	request := provider.requests[0]
	userPrompt := request.Messages[1].Content
	for _, expected := range []string{
		"id=research.timestamp",
		"id=position.aapl.symbol",
		"id=position.msft.symbol",
	} {
		if !strings.Contains(userPrompt, expected) {
			t.Fatalf("rendered prompt does not contain %q:\n%s", expected, userPrompt)
		}
	}
	if strings.Contains(userPrompt, "quantity") || strings.Contains(userPrompt, "portfolio weight") {
		t.Fatalf("portfolio-news prompt contains private position data:\n%s", userPrompt)
	}
	if request.Settings.Temperature != 0 || request.Settings.MaxTokens != 4200 || len(request.Tools) != 2 || request.ResponseSchema == nil {
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
	if response.Text != validStockNews || response.Structured == nil || response.PromptID != StockNewsPromptAlias || response.PromptVersion != 2 {
		t.Fatalf("unexpected response: %+v", response)
	}
	request := provider.requests[0]
	userPrompt := request.Messages[1].Content
	for _, expected := range []string{"id=research.timestamp", "id=instrument.aapl.symbol", "text=Apple Inc."} {
		if !strings.Contains(userPrompt, expected) {
			t.Fatalf("rendered prompt does not contain %q:\n%s", expected, userPrompt)
		}
	}
	if request.Settings.Temperature != 0 || request.Settings.MaxTokens != 2800 || len(request.Tools) != 2 || request.ResponseSchema == nil {
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

func TestOrderAnalysisExecutesActiveCatalogPrompt(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog returned an error: %v", err)
	}
	provider := &recordingProvider{responses: []ChatResponse{{
		Message: Message{Role: "assistant", Content: validOrderAnalysis}, Model: "test-model",
	}}}
	counselor := NewCounselor(NewExecutor(catalog, provider), NewResearchToolsFactory(unusedResearchProvider{}, []string{"sec.gov"}, 5))
	result := stockanalysis.Result{
		CalculatedAt: time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC),
		Instrument:   portfolio.Instrument{Symbol: "NESN", Name: "Nestlé S.A.", Exchange: "SIX", Currency: "CHF"},
		Side:         "BUY", Quantity: 4,
		EstimatedPrice:        portfolio.Money{Amount: 72.5, Currency: "CHF", Available: true},
		EstimatedOrderValue:   portfolio.Money{Amount: 290, Currency: "CHF", Available: true},
		BaseOrderValue:        portfolio.Money{Amount: 290, Currency: "CHF", Available: true},
		CashBalance:           portfolio.Money{Amount: 1000, Currency: "CHF", Available: true},
		CashUsage:             stockanalysis.Metric{Value: .29, Available: true},
		CurrentQuantity:       2,
		ResultingQuantity:     6,
		CurrentWeight:         stockanalysis.Metric{Value: .05, Available: true},
		ResultingWeight:       stockanalysis.Metric{Value: .075, Available: true},
		WeightChange:          stockanalysis.Metric{Value: .025, Available: true},
		HistoryStart:          time.Date(2025, time.September, 26, 0, 0, 0, 0, time.UTC),
		HistoryEnd:            time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC),
		HistoryObservations:   252,
		TotalReturn:           stockanalysis.Metric{Value: -.1, Available: true},
		AnnualizedVolatility:  stockanalysis.Metric{Value: .2, Available: true},
		MaximumDrawdown:       stockanalysis.Metric{Value: -.15, Available: true},
		PositionIn52WeekRange: stockanalysis.Metric{Value: .4, Available: true},
		DistanceFromSMA50:     stockanalysis.Metric{Value: -.02, Available: true},
		DistanceFromSMA200:    stockanalysis.Metric{Value: .03, Available: true},
		MarketDataStatus:      "live",
	}
	response, err := counselor.OrderAnalysis(context.Background(), result)
	if err != nil {
		t.Fatalf("OrderAnalysis returned an error: %v", err)
	}
	if response.Text != validOrderAnalysis || response.PromptID != OrderAnalysisPromptAlias {
		t.Fatalf("unexpected response: %+v", response)
	}
	request := provider.requests[0]
	for _, expected := range []string{"id=instrument.nesn.symbol", "id=order.side", "id=order.cash_usage", "id=position.resulting_weight"} {
		if !strings.Contains(request.Messages[1].Content, expected) {
			t.Fatalf("rendered prompt does not contain %q:\n%s", expected, request.Messages[1].Content)
		}
	}
	if request.Settings.Temperature != 0 || request.Settings.MaxTokens != 3600 || len(request.Tools) != 2 || request.ResponseSchema == nil {
		t.Fatalf("unexpected order-analysis request: %+v", request)
	}
}
