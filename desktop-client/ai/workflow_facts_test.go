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
	"wails/stockanalysis"
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
		if definition.Version != 1 || definition.Response.Format != "markdown" || definition.Response.Schema != "" || len(definition.Tools) != 0 {
			t.Fatalf("unexpected focused workflow definition for %s: %#v", alias, definition)
		}
	}
	if len(catalog.aliases) < 10 {
		t.Fatalf("catalog aliases = %d, want at least 10", len(catalog.aliases))
	}
}

func TestPromptNumericValueUsesPlainTwoDecimalPrecision(t *testing.T) {
	large := numberFact("history.value", CategoryDeterministicDerivedFact, "History value", 1.009493375e+06, "GBP", moneyAbsoluteTolerance, defaultRelativeTolerance)
	percentage := numberFact("portfolio.weight", CategoryDeterministicDerivedFact, "Weight", 6.619693851887816, "percent", percentAbsoluteTolerance, 0)

	if large.PromptNumericValue() != "1009493.38" {
		t.Fatalf("large prompt value = %q", large.PromptNumericValue())
	}
	if percentage.PromptNumericValue() != "6.62" {
		t.Fatalf("percentage prompt value = %q", percentage.PromptNumericValue())
	}
}

func TestHistoryPromptRendersPlainTwoDecimalFacts(t *testing.T) {
	catalog, err := NewCatalog()
	if err != nil {
		t.Fatalf("NewCatalog() error: %v", err)
	}
	definition, err := catalog.resolve(PortfolioHistoryPromptAlias)
	if err != nil {
		t.Fatalf("resolve history prompt: %v", err)
	}
	facts := []CounselorFact{
		numberFact("history.value", CategoryDeterministicDerivedFact, "History value", 1.009493375e+06, "GBP", moneyAbsoluteTolerance, defaultRelativeTolerance),
		numberFact("history.percent", CategoryDeterministicDerivedFact, "History percentage", 6.619693851887816, "percent", percentAbsoluteTolerance, 0),
	}
	prompt, err := renderTemplate("history_test", definition.UserTemplate, map[string]any{"facts": facts})
	if err != nil {
		t.Fatalf("render history prompt: %v", err)
	}
	if !strings.Contains(prompt, "value=1009493.38 | unit=GBP") || !strings.Contains(prompt, "value=6.62 | unit=percent") || strings.Contains(strings.ToLower(prompt), "e+") {
		t.Fatalf("history prompt contains unformatted numeric facts: %s", prompt)
	}
}

func TestPortfolioResumeFactsBoundPositionAndEffectiveSecurityContext(t *testing.T) {
	positions := make([]portfolio.Position, 15)
	weights := make([]analytics.PositionWeight, 15)
	exposures := make([]analytics.EffectiveExposure, 15)
	for index := range positions {
		positions[index] = portfolio.Position{Instrument: portfolio.Instrument{ContractID: int64(index + 1), Symbol: fmt.Sprintf("P%d", index+1)}}
		weights[index] = analytics.PositionWeight{Weight: analytics.Ratio{Value: float64(index + 1), Available: true}}
		exposures[index] = analytics.EffectiveExposure{MetricID: fmt.Sprintf("effective.security.%d.weight", index+1), Name: fmt.Sprintf("Security %d", index+1), Weight: analytics.Ratio{Value: .01, Available: true}}
	}
	facts := portfolioResumeFacts(portfolio.Portfolio{Positions: positions}, analytics.Result{CalculatedAt: time.Now(), PositionWeights: weights, EffectiveSecurityExposure: exposures, EffectiveExposureComplete: true})
	if findFact(facts, "position.contract_1.symbol") != nil || findFact(facts, "position.contract_15.symbol") == nil {
		t.Fatalf("portfolio position facts were not limited by largest weight: %#v", facts)
	}
	for _, id := range []string{"warning.portfolio_resume.position_coverage", "effective.security_coverage"} {
		fact := findFact(facts, id)
		if fact == nil || !fact.RequiredLimitation {
			t.Fatalf("missing bounded-context limitation %s: %#v", id, facts)
		}
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

func TestNewsFactsRequireCoverageForEverySuppliedIdentity(t *testing.T) {
	snapshot := portfolio.Portfolio{Positions: []portfolio.Position{
		{Instrument: portfolio.Instrument{ContractID: 1, Symbol: "AAA", Name: "Alpha"}},
		{Instrument: portfolio.Instrument{ContractID: 2, Symbol: "BBB", Name: "Beta"}},
	}}
	facts := portfolioNewsFacts(snapshot, time.Now())
	for _, id := range []string{"position.contract_1.symbol", "position.contract_2.symbol"} {
		fact := findFact(facts, id)
		if fact == nil || !fact.RequiredObservation {
			t.Fatalf("identity coverage is not required for %s: %#v", id, facts)
		}
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
	result := positionanalysis.Result{CalculatedAt: time.Now(), Instrument: portfolio.Instrument{ContractID: 9, Symbol: "TEST", Name: "Test AG"}, Quantity: 4, MarketValue: portfolio.Money{Amount: 80, Currency: "CHF", Available: true}, PortfolioWeight: positionanalysis.Metric{Value: .2, Available: true}, AnnualizedVolatility: positionanalysis.Metric{Value: .31, Available: true}, HistoryObservations: 250, MarketDataStatus: "Delayed"}
	facts := positionRiskFacts(result)
	assertFact(t, facts, "position_risk.portfolio_weight", "percent", true)
	fact := findFact(facts, "position_risk.annualized_volatility")
	if fact == nil || fact.NumericValue == nil || *fact.NumericValue != 31 {
		t.Fatalf("unexpected volatility fact: %#v", fact)
	}
	if marketValue := findFact(facts, "position_risk.market_value"); marketValue == nil || marketValue.Kind != string(CategorySuppliedFact) {
		t.Fatalf("broker-reported market value was not marked supplied: %#v", marketValue)
	}
}

func TestOrderFactsSeparateBrokerAndCalculatedValues(t *testing.T) {
	result := stockanalysis.Result{
		CalculatedAt: time.Now(), Instrument: portfolio.Instrument{ContractID: 4, Symbol: "TEST", Name: "Test AG", Currency: "CHF"}, Side: "BUY", Quantity: 2,
		EstimatedPrice: portfolio.Money{Amount: 10, Currency: "CHF", Available: true}, EstimatedOrderValue: portfolio.Money{Amount: 20, Currency: "CHF", Available: true}, CashBalance: portfolio.Money{Amount: 100, Currency: "CHF", Available: true},
	}
	facts := orderAnalysisFacts(result)
	for _, id := range []string{"order.estimated_price", "portfolio.cash_balance", "position.current_quantity"} {
		if fact := findFact(facts, id); fact == nil || fact.Kind != string(CategorySuppliedFact) {
			t.Fatalf("broker fact %s has the wrong category: %#v", id, fact)
		}
	}
	if fact := findFact(facts, "order.estimated_value"); fact == nil || fact.Kind != string(CategoryDeterministicDerivedFact) {
		t.Fatalf("calculated order value has the wrong category: %#v", fact)
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
