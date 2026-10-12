//go:build showcase

package main

// showcase_enabled_test.go verifies controlled portfolio fixture selection.

import (
	"strings"
	"testing"

	"wails/analytics"
	"wails/evaluation"
)

func TestLoadShowcaseScenario(t *testing.T) {
	t.Setenv("SHOWCASE_SCENARIO", "incomplete-data")

	scenario, err := loadShowcaseScenario()
	if err != nil {
		t.Fatal(err)
	}
	if scenario.ID != "incomplete-data" {
		t.Fatalf("unexpected showcase selection: %#v", scenario)
	}
}

func TestShowcaseAppReturnsFixtureWithoutBroker(t *testing.T) {
	t.Setenv("SHOWCASE_SCENARIO", "concentrated-multicurrency")
	scenario, err := loadShowcaseScenario()
	if err != nil {
		t.Fatal(err)
	}
	app := &App{showcase: scenario}

	if status := app.GetSessionStatus(); !status.Ready || !status.Paper {
		t.Fatalf("showcase session is not paper-ready: %#v", status)
	}
	view, err := app.GetPortfolio()
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Positions) != len(scenario.Portfolio.Positions) {
		t.Fatalf("position count = %d, want %d", len(view.Positions), len(scenario.Portfolio.Positions))
	}
	if len(view.Analytics.EffectiveSecurityExposure) == 0 || view.Analytics.EffectiveExposureComplete {
		t.Fatalf("expected explicit incomplete effective exposure, got %#v", view.Analytics)
	}
	history, err := app.GetPortfolioHistory("30d")
	if err != nil {
		t.Fatal(err)
	}
	if history.Range != "30d" || len(history.AccountValueSeries) != 0 || len(history.TrackingSeries) != 0 {
		t.Fatalf("unexpected showcase history: %#v", history)
	}
}

func TestAdditionalShowcaseScenarioComposition(t *testing.T) {
	scenarios, err := loadShowcaseScenarios()
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]evaluation.FrozenScenario, len(scenarios))
	for _, scenario := range scenarios {
		byID[scenario.ID] = scenario
		if scenario.ID == "multicurrency-10-stock" {
			continue
		}
		result := analytics.Calculate(scenario.Portfolio, scenario.SnapshotAt)
		for _, position := range result.PositionWeights {
			if !position.Weight.Available || position.Weight.Value >= 0.10 {
				t.Fatalf("scenario %s position %s weight = %#v, want an available weight below 10%%", scenario.ID, position.Symbol, position.Weight)
			}
		}
	}

	diversified := byID["large-diversified"]
	if len(diversified.Portfolio.Positions) < 20 {
		t.Fatalf("large diversified positions = %d, want at least 20", len(diversified.Portfolio.Positions))
	}
	korean := byID["south-korea-20-stock"]
	if len(korean.Portfolio.Positions) < 20 {
		t.Fatalf("South Korean positions = %d, want at least 20", len(korean.Portfolio.Positions))
	}
	for _, position := range korean.Portfolio.Positions {
		if position.Instrument.Currency != "KRW" || position.Instrument.Exchange != "KRX" || position.Instrument.AssetClass != "STK" {
			t.Fatalf("South Korean scenario contains a non-KRX stock: %#v", position.Instrument)
		}
	}
	etfs := byID["etf-only-12"]
	if len(etfs.Portfolio.Positions) < 10 {
		t.Fatalf("ETF positions = %d, want at least 10", len(etfs.Portfolio.Positions))
	}
	for _, position := range etfs.Portfolio.Positions {
		if !strings.EqualFold(position.Instrument.AssetClass, "ETF") {
			t.Fatalf("ETF scenario contains a non-ETF position: %#v", position.Instrument)
		}
	}

	multicurrency := byID["multicurrency-10-stock"]
	if len(multicurrency.Portfolio.Positions) != 10 {
		t.Fatalf("multicurrency positions = %d, want 10", len(multicurrency.Portfolio.Positions))
	}
	currencies := make(map[string]struct{}, len(multicurrency.Portfolio.Positions))
	for _, position := range multicurrency.Portfolio.Positions {
		if !strings.EqualFold(position.Instrument.AssetClass, "STK") {
			t.Fatalf("multicurrency scenario contains a non-stock position: %#v", position.Instrument)
		}
		currencies[strings.ToUpper(position.Instrument.Currency)] = struct{}{}
	}
	if len(currencies) != len(multicurrency.Portfolio.Positions) {
		t.Fatalf("multicurrency currencies = %d, want one distinct currency per position", len(currencies))
	}
	rates := make(map[string]struct{}, len(multicurrency.Portfolio.ExchangeRates))
	for _, rate := range multicurrency.Portfolio.ExchangeRates {
		if !rate.Available || !strings.EqualFold(rate.BaseCurrency, "CHF") || rate.BaseAmountPerUnit <= 0 {
			t.Fatalf("multicurrency scenario contains an unusable CHF exchange rate: %#v", rate)
		}
		rates[strings.ToUpper(rate.Currency)] = struct{}{}
	}
	for currency := range currencies {
		if currency == "CHF" {
			continue
		}
		if _, exists := rates[currency]; !exists {
			t.Fatalf("multicurrency scenario is missing a CHF exchange rate for %s", currency)
		}
	}
	result := analytics.Calculate(multicurrency.Portfolio, multicurrency.SnapshotAt)
	for _, position := range result.PositionWeights {
		if !position.Weight.Available || position.Weight.Value < 0.099999 || position.Weight.Value > 0.100001 {
			t.Fatalf("multicurrency position %s weight = %#v, want 10%%", position.Symbol, position.Weight)
		}
	}
}

func TestLoadShowcaseScenarioRejectsUnknownSelection(t *testing.T) {
	t.Setenv("SHOWCASE_SCENARIO", "unknown")

	if _, err := loadShowcaseScenario(); err == nil {
		t.Fatal("expected unknown scenario to be rejected")
	}
}
