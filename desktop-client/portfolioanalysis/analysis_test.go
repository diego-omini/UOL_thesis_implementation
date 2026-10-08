package portfolioanalysis

// This file verifies modeled portfolio risk, contribution, correlation, and stress calculations.

import (
	"math"
	"testing"
	"time"

	"wails/analytics"
	"wails/portfolio"
)

func TestCalculateModelsPortfolioRiskAndScenarios(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	snapshot := portfolio.Portfolio{
		AccountSummary: portfolio.AccountSummary{NetLiquidation: portfolio.Money{Amount: 1000, Currency: "CHF", Available: true}},
		Positions: []portfolio.Position{
			{Instrument: portfolio.Instrument{ContractID: 1, Symbol: "AAA", AssetClass: "STK", Currency: "CHF", Sector: "Technology"}, MarketValue: portfolio.Money{Amount: 600, Currency: "CHF", Available: true}},
			{Instrument: portfolio.Instrument{ContractID: 2, Symbol: "BBB", AssetClass: "STK", Currency: "USD", Sector: "Financials"}, MarketValue: portfolio.Money{Amount: 400, Currency: "CHF", Available: true}},
		},
	}
	current := analytics.Calculate(snapshot, start)
	histories := map[int64]portfolio.StockHistory{}
	for _, position := range snapshot.Positions {
		points := make([]portfolio.StockHistoryPoint, 31)
		for i := range points {
			growth := 1.01
			if position.Instrument.ContractID == 2 {
				growth = 0.995
			}
			points[i] = portfolio.StockHistoryPoint{Time: start.AddDate(0, 0, i), Close: 100 * math.Pow(growth, float64(i))}
		}
		histories[position.Instrument.ContractID] = portfolio.StockHistory{Points: points}
	}

	result := Calculate(snapshot, current, histories, start.AddDate(1, 0, 0))
	if result.Observations != 30 || len(result.Positions) != 2 || len(result.Correlations) != 1 || len(result.Scenarios) != 4 {
		t.Fatalf("unexpected analysis dimensions: %+v", result)
	}
	if !result.ModeledTotalReturn.Available || result.ModeledTotalReturn.Value <= 0 || !result.MaximumDrawdown.Available {
		t.Fatalf("expected modeled metrics: %+v", result)
	}
	if got := result.Scenarios[0].Impact.Amount; math.Abs(got-(-100)) > 1e-9 {
		t.Fatalf("broad-market impact = %v, want -100", got)
	}
	if len(result.Warnings) < 2 {
		t.Fatalf("expected methodology and FX warnings: %+v", result.Warnings)
	}
}

func TestCalculateRejectsInsufficientCommonHistory(t *testing.T) {
	snapshot := portfolio.Portfolio{Positions: []portfolio.Position{{Instrument: portfolio.Instrument{ContractID: 1, Symbol: "AAA"}}}}
	result := Calculate(snapshot, analytics.Result{}, nil, time.Now())
	if result.ModeledTotalReturn.Available || len(result.Warnings) == 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestDailyReturnsUsesLatestCloseForDuplicateDate(t *testing.T) {
	start := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	returns, _ := dailyReturns([]portfolio.StockHistoryPoint{
		{Time: start, Close: 10},
		{Time: start.Add(time.Hour), Close: 20},
		{Time: start.AddDate(0, 0, 1), Close: 30},
	})
	if len(returns) != 1 || math.Abs(returns["2026-01-02"]-.5) > 1e-9 {
		t.Fatalf("duplicate date distorted returns: %#v", returns)
	}
}
