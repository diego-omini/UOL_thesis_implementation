package analytics

// analytics_test.go verifies portfolio analytics, currency conversion, and warning behavior.

import (
	"math"
	"testing"
	"time"

	"wails/portfolio"
)

func TestCalculateUsesBaseCurrencyAndAbsoluteExposure(t *testing.T) {
	calculatedAt := time.Date(2026, time.September, 6, 12, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	snapshot := portfolio.Portfolio{
		AccountSummary: portfolio.AccountSummary{
			NetLiquidation: availableMoney(1000, "CHF"),
		},
		Positions: []portfolio.Position{
			{
				Instrument:        portfolio.Instrument{ContractID: 1, Symbol: "AAA", AssetClass: "STK", Sector: "Technology"},
				MarketPrice:       availableMoney(100, "USD"),
				MarketValue:       availableMoney(100, "USD"),
				BaseMarketValue:   availableMoney(90, "CHF"),
				UnrealizedPnL:     availableMoney(10, "USD"),
				BaseUnrealizedPnL: availableMoney(9, "CHF"),
			},
			{
				Instrument:    portfolio.Instrument{ContractID: 2, Symbol: "BBB", AssetClass: "STK", Sector: "Financials"},
				MarketPrice:   availableMoney(10, "CHF"),
				MarketValue:   availableMoney(60, "CHF"),
				UnrealizedPnL: availableMoney(6, "CHF"),
			},
			{
				Instrument:    portfolio.Instrument{ContractID: 3, Symbol: "CCC", AssetClass: "STK", Sector: "Technology"},
				MarketPrice:   availableMoney(5, "CHF"),
				MarketValue:   availableMoney(-50, "CHF"),
				UnrealizedPnL: availableMoney(-5, "CHF"),
			},
		},
	}

	result := Calculate(snapshot, calculatedAt)

	assertMoney(t, result.NetMarketValue, 100, "CHF", true)
	assertMoney(t, result.GrossExposure, 200, "CHF", true)
	assertMoney(t, result.TotalUnrealizedPnL, 10, "CHF", true)
	assertRatio(t, result.PositionWeights[0].Weight, 0.45, true)
	assertRatio(t, result.PositionWeights[1].Weight, 0.30, true)
	assertRatio(t, result.PositionWeights[2].Weight, 0.25, true)
	assertRatio(t, result.TopThreeConcentration, 1, true)
	assertRatio(t, result.HHI, 0.355, true)

	if !result.CalculatedAt.Equal(calculatedAt.UTC()) {
		t.Fatalf("unexpected calculation time: %s", result.CalculatedAt)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", result.Warnings)
	}

	assertAllocation(t, result.CurrencyAllocation, "CHF", 10, 110, 0.55)
	assertAllocation(t, result.CurrencyAllocation, "USD", 90, 90, 0.45)
	assertAllocation(t, result.SectorAllocation, "Technology", 40, 140, 0.70)
	assertAllocation(t, result.SectorAllocation, "Financials", 60, 60, 0.30)

	if len(result.LargestPositions) != 3 || result.LargestPositions[0].Symbol != "AAA" {
		t.Fatalf("positions were not sorted by exposure: %#v", result.LargestPositions)
	}
}

func TestCalculateRejectsUnconvertedForeignValues(t *testing.T) {
	snapshot := portfolio.Portfolio{
		AccountSummary: portfolio.AccountSummary{
			NetLiquidation: availableMoney(1000, "CHF"),
		},
		Positions: []portfolio.Position{
			{
				Instrument:    portfolio.Instrument{Symbol: "AAA"},
				MarketValue:   availableMoney(100, "USD"),
				UnrealizedPnL: availableMoney(10, "USD"),
			},
		},
	}

	result := Calculate(snapshot, time.Time{})

	if result.NetMarketValue.Available || result.GrossExposure.Available {
		t.Fatal("portfolio totals must be unavailable without a base-currency market value")
	}
	if result.TotalUnrealizedPnL.Available {
		t.Fatal("profit and loss must be unavailable without a base-currency value")
	}
	assertWarning(t, result.Warnings, "missing_base_market_value", 1)
	assertWarning(t, result.Warnings, "missing_base_unrealized_pnl", 1)
	assertWarning(t, result.Warnings, "missing_asset_class", 1)
	assertWarning(t, result.Warnings, "missing_sector", 1)
}

func TestCalculateConvertsMultipleCurrenciesUsingLedgerRates(t *testing.T) {
	asOf := time.Date(2026, time.September, 7, 12, 0, 0, 0, time.UTC)
	snapshot := portfolio.Portfolio{
		AccountSummary: portfolio.AccountSummary{
			NetLiquidation: availableMoney(1000, "GBP"),
		},
		ExchangeRates: []portfolio.ExchangeRate{
			{
				Currency:          "USD",
				BaseCurrency:      "GBP",
				BaseAmountPerUnit: 0.75,
				Available:         true,
				Source:            "IBKR account ledger",
				AsOf:              asOf,
			},
			{
				Currency:          "EUR",
				BaseCurrency:      "GBP",
				BaseAmountPerUnit: 0.85,
				Available:         true,
				Source:            "IBKR account ledger",
				AsOf:              asOf,
			},
		},
		Positions: []portfolio.Position{
			{
				Instrument:    portfolio.Instrument{ContractID: 1, Symbol: "USD", AssetClass: "STK", Sector: "Technology"},
				MarketPrice:   availableMoney(100, "USD"),
				MarketValue:   availableMoney(100, "USD"),
				UnrealizedPnL: availableMoney(10, "USD"),
			},
			{
				Instrument:    portfolio.Instrument{ContractID: 2, Symbol: "EUR", AssetClass: "STK", Sector: "Financials"},
				MarketPrice:   availableMoney(100, "EUR"),
				MarketValue:   availableMoney(200, "EUR"),
				UnrealizedPnL: availableMoney(-20, "EUR"),
			},
			{
				Instrument:    portfolio.Instrument{ContractID: 3, Symbol: "GBP", AssetClass: "STK", Sector: "Utilities"},
				MarketPrice:   availableMoney(25, "GBP"),
				MarketValue:   availableMoney(50, "GBP"),
				UnrealizedPnL: availableMoney(5, "GBP"),
			},
		},
	}

	result := Calculate(snapshot, asOf)

	assertMoney(t, result.NetMarketValue, 295, "GBP", true)
	assertMoney(t, result.GrossExposure, 295, "GBP", true)
	assertMoney(t, result.TotalUnrealizedPnL, -4.5, "GBP", true)
	assertRatio(t, result.PositionWeights[0].Weight, 75.0/295.0, true)
	assertRatio(t, result.PositionWeights[1].Weight, 170.0/295.0, true)
	assertRatio(t, result.PositionWeights[2].Weight, 50.0/295.0, true)
	if len(result.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", result.Warnings)
	}
	if len(result.ExchangeRates) != 2 || result.ExchangeRates[0].Source != "IBKR account ledger" {
		t.Fatalf("unexpected exchange-rate metadata: %#v", result.ExchangeRates)
	}
}

func availableMoney(amount float64, currency string) portfolio.Money {
	return portfolio.Money{Amount: amount, Currency: currency, Available: true}
}

func assertMoney(t *testing.T, got portfolio.Money, amount float64, currency string, available bool) {
	t.Helper()
	if !closeEnough(got.Amount, amount) || got.Currency != currency || got.Available != available {
		t.Fatalf("unexpected money value: %#v", got)
	}
}

func assertRatio(t *testing.T, got Ratio, value float64, available bool) {
	t.Helper()
	if !closeEnough(got.Value, value) || got.Available != available {
		t.Fatalf("unexpected ratio: %#v", got)
	}
}

func assertAllocation(t *testing.T, allocations []Allocation, label string, marketValue, exposure, weight float64) {
	t.Helper()
	for _, allocation := range allocations {
		if allocation.Label != label {
			continue
		}
		assertMoney(t, allocation.MarketValue, marketValue, "CHF", true)
		assertMoney(t, allocation.Exposure, exposure, "CHF", true)
		assertRatio(t, allocation.Weight, weight, true)
		return
	}
	t.Fatalf("allocation %q was not found", label)
}

func assertWarning(t *testing.T, warnings []DataWarning, code string, count int) {
	t.Helper()
	for _, warning := range warnings {
		if warning.Code == code {
			if warning.Count != count {
				t.Fatalf("warning %q count = %d, want %d", code, warning.Count, count)
			}
			return
		}
	}
	t.Fatalf("warning %q was not found", code)
}

func closeEnough(left, right float64) bool {
	return math.Abs(left-right) < 0.000001
}
