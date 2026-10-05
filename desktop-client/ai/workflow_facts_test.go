package ai

// This file verifies focused workflow catalog contracts, stable facts, units, and required limitations.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"wails/analytics"
	"wails/portfolio"
	"wails/portfoliohistory"
	"wails/positionanalysis"
)

func TestFocusedWorkflowCatalogDefinitionsAreToolFree(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog() error: %v", err)
	}
	aliases := []string{CurrencyExposurePromptAlias, DiversificationPromptAlias, PortfolioHistoryPromptAlias, PositionRiskPromptAlias, DataQualityPromptAlias}
	for _, alias := range aliases {
		definition, err := catalog.resolve(alias)
		if err != nil {
			t.Fatalf("resolve(%q) error: %v", alias, err)
		}
		if definition.Version != 1 || definition.Response.Schema != CounselorResponseSchema || len(definition.Tools) != 0 {
			t.Fatalf("unexpected focused workflow definition for %s: %#v", alias, definition)
		}
	}
	if len(catalog.aliases) < 10 {
		t.Fatalf("catalog aliases = %d, want at least 10", len(catalog.aliases))
	}
}

func TestFocusedWorkflowDoesNotReceiveAvailableResearchTool(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog() error: %v", err)
	}
	provider := &recordingProvider{responses: []ChatResponse{{Message: Message{Role: "assistant", Content: validStructuredResponse}, Model: "test-model"}}}
	executor := NewExecutor(catalog, provider)
	facts := []CounselorFact{textFact("currency.base_currency", CategoryDeterministicDerivedFact, "Account base currency", "GBP")}
	if _, err := executor.ExecuteCounselor(context.Background(), CurrencyExposurePromptAlias, map[string]any{"facts": facts}, []Tool{&testTool{}}, facts); err != nil {
		t.Fatalf("ExecuteCounselor() error: %v", err)
	}
	if len(provider.requests) != 1 || len(provider.requests[0].Tools) != 0 {
		t.Fatalf("tool-free workflow received tools: %#v", provider.requests)
	}
}

func TestCurrencyFactsPreserveRateProvenanceAndUnits(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	snapshot := portfolio.Portfolio{Positions: []portfolio.Position{{Instrument: portfolio.Instrument{ContractID: 7, Currency: "USD"}}}}
	result := analytics.Result{CalculatedAt: now, BaseCurrency: "GBP", CurrencyAllocation: []analytics.Allocation{{Label: "USD", Weight: analytics.Ratio{Value: .4, Available: true}}}, ExchangeRates: []portfolio.ExchangeRate{{Currency: "USD", BaseCurrency: "GBP", BaseAmountPerUnit: .75, Available: true, Source: "IBKR portfolio ledger", AsOf: now}}}
	facts := currencyExposureFacts(snapshot, result)
	assertFact(t, facts, "currency.allocation.usd.weight", "percent", true)
	assertFact(t, facts, "currency.rate.usd.base_amount_per_unit", "GBP_per_USD", true)
	if factText(facts, "currency.rate.usd.source") != "IBKR portfolio ledger" || factText(facts, "currency.rate.usd.as_of") == "" {
		t.Fatalf("currency provenance missing: %#v", facts)
	}
}

func TestHistoryFactsRequireNonReturnLimitation(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	summary := portfoliohistory.Summary{From: now.AddDate(0, -1, 0), To: now, Warnings: []string{"Account-value changes are descriptive and are not investment returns because deposits and withdrawals are unavailable."}, Series: []portfoliohistory.SeriesSummary{{Product: "account_value", BaseCurrency: "GBP", ObservationCount: 2, FirstAt: now.AddDate(0, 0, -1), LastAt: now, FirstValue: portfoliohistory.Metric{Value: 100, Currency: "GBP", Available: true}, LastValue: portfoliohistory.Metric{Value: 105, Currency: "GBP", Available: true}, AbsoluteChange: portfoliohistory.Metric{Value: 5, Currency: "GBP", Available: true}, Minimum: portfoliohistory.Metric{Value: 100, Currency: "GBP", Available: true}, Maximum: portfoliohistory.Metric{Value: 105, Currency: "GBP", Available: true}, Sources: []string{portfoliohistory.SourceIBKRPortfolioAnalyst}}}}
	facts := portfolioHistoryFacts(summary)
	assertFact(t, facts, "history.series.1.absolute_change", "GBP", true)
	fact := findFact(facts, "warning.portfolio_history.1")
	if fact == nil || !fact.RequiredLimitation || !strings.Contains(fact.Limitation, "not investment returns") {
		t.Fatalf("return limitation missing: %#v", fact)
	}
}

func TestPositionRiskFactsUseDeterministicPercentMetrics(t *testing.T) {
	result := positionanalysis.Result{CalculatedAt: time.Now(), Instrument: portfolio.Instrument{ContractID: 9, Symbol: "TEST", Name: "Test AG"}, Quantity: 4, PortfolioWeight: positionanalysis.Metric{Value: .2, Available: true}, AnnualizedVolatility: positionanalysis.Metric{Value: .31, Available: true}, HistoryObservations: 250, MarketDataStatus: "Delayed"}
	facts := positionRiskFacts(result)
	assertFact(t, facts, "position_risk.portfolio_weight", "percent", true)
	fact := findFact(facts, "position_risk.annualized_volatility")
	if fact == nil || fact.NumericValue == nil || *fact.NumericValue != 31 {
		t.Fatalf("unexpected volatility fact: %#v", fact)
	}
}

func TestDiversificationFactsBoundEffectiveSecurityContext(t *testing.T) {
	exposures := make([]analytics.EffectiveExposure, 15)
	for index := range exposures {
		exposures[index] = analytics.EffectiveExposure{MetricID: fmt.Sprintf("effective.security.%d.weight", index+1), Name: fmt.Sprintf("Security %d", index+1), Weight: analytics.Ratio{Value: .01, Available: true}}
	}
	facts := diversificationFacts(portfolio.Portfolio{}, analytics.Result{CalculatedAt: time.Now(), EffectiveSecurityExposure: exposures, EffectiveExposureComplete: true})
	if findFact(facts, "effective.security.13.weight") != nil {
		t.Fatalf("effective-security context was not bounded: %#v", facts)
	}
	coverage := findFact(facts, "effective.security_coverage")
	if coverage == nil || !coverage.RequiredLimitation || factText(facts, "effective.security_count_total") != "" {
		t.Fatalf("bounded coverage facts missing: %#v", facts)
	}
	assertFact(t, facts, "effective.security_count_total", "count", true)
	assertFact(t, facts, "effective.security_count_supplied", "count", true)
}

func TestDataQualityFactsDoNotDuplicateFundProvenance(t *testing.T) {
	result := analytics.Result{CalculatedAt: time.Now(), FundLookThrough: []portfolio.FundHoldings{{Fund: portfolio.Instrument{Symbol: "FUND"}, Available: true, Complete: true, SourceType: "issuer CSV", SourceURL: "https://example.test/fund.csv", Confidence: "high"}}}
	facts := dataQualityFacts(portfolio.Portfolio{}, result)
	seen := map[string]bool{}
	for _, fact := range facts {
		if seen[fact.ID] {
			t.Fatalf("duplicate fact ID %q: %#v", fact.ID, facts)
		}
		seen[fact.ID] = true
	}
}

func assertFact(t *testing.T, facts []CounselorFact, id, unit string, available bool) {
	t.Helper()
	fact := findFact(facts, id)
	if fact == nil || fact.Unit != unit || fact.Available != available {
		t.Fatalf("fact %s = %#v", id, fact)
	}
}

func findFact(facts []CounselorFact, id string) *CounselorFact {
	for index := range facts {
		if facts[index].ID == id {
			return &facts[index]
		}
	}
	return nil
}

func factText(facts []CounselorFact, id string) string {
	if fact := findFact(facts, id); fact != nil {
		return fact.TextValue
	}
	return ""
}
