package stockanalysis

// This file verifies deterministic market-risk and proposed-order calculations.

import (
	"math"
	"testing"
	"time"

	"wails/portfolio"
)

func TestCalculateBuyImpactAndMarketMetrics(t *testing.T) {
	points := make([]portfolio.StockHistoryPoint, 250)
	start := time.Date(2025, time.September, 1, 0, 0, 0, 0, time.UTC)
	for index := range points {
		price := 100 + float64(index)*0.2
		if index == 125 {
			price = 90
		}
		points[index] = portfolio.StockHistoryPoint{Time: start.AddDate(0, 0, index), Close: price}
	}
	snapshot := portfolio.StockSnapshot{
		Instrument: portfolio.Instrument{ContractID: 42, Symbol: "TEST", Name: "Test AG", AssetClass: "STK", Currency: "USD"},
		Currency:   "USD", AskPrice: portfolio.OptionalNumber{Value: 150, Available: true},
		BidPrice:   portfolio.OptionalNumber{Value: 149, Available: true},
		Low52Week:  portfolio.OptionalNumber{Value: 80, Available: true},
		High52Week: portfolio.OptionalNumber{Value: 160, Available: true},
	}
	account := portfolio.Portfolio{
		AccountSummary: portfolio.AccountSummary{
			NetLiquidation: portfolio.Money{Amount: 20000, Currency: "CHF", Available: true},
			CashBalance:    portfolio.Money{Amount: 5000, Currency: "CHF", Available: true},
		},
		Positions: []portfolio.Position{{
			Instrument: portfolio.Instrument{ContractID: 42, Currency: "USD"}, Quantity: 10,
			MarketValue: portfolio.Money{Amount: 1500, Currency: "USD", Available: true},
		}},
		ExchangeRates: []portfolio.ExchangeRate{{Currency: "USD", BaseCurrency: "CHF", BaseAmountPerUnit: .8, Available: true}},
	}

	result, err := Calculate(snapshot, portfolio.StockHistory{Points: points}, account, "BUY", 5, start)
	if err != nil {
		t.Fatalf("Calculate() error: %v", err)
	}
	if result.BaseOrderValue.Amount != 600 || result.BaseOrderValue.Currency != "CHF" {
		t.Fatalf("unexpected base order value: %#v", result.BaseOrderValue)
	}
	if !result.CashUsage.Available || math.Abs(result.CashUsage.Value-.12) > 1e-9 {
		t.Fatalf("unexpected cash usage: %#v", result.CashUsage)
	}
	if result.ResultingQuantity != 15 || !result.ResultingWeight.Available || result.HistoryObservations != 250 {
		t.Fatalf("unexpected order or history result: %#v", result)
	}
	if !result.TotalReturn.Available || !result.AnnualizedVolatility.Available || !result.MaximumDrawdown.Available || result.MaximumDrawdown.Value >= 0 {
		t.Fatalf("market metrics unavailable or invalid: %#v", result)
	}
	if !result.DistanceFromSMA50.Available || !result.DistanceFromSMA200.Available || !result.PositionIn52WeekRange.Available {
		t.Fatalf("derived comparison metrics unavailable: %#v", result)
	}
}

func TestCalculateRejectsOversell(t *testing.T) {
	snapshot := portfolio.StockSnapshot{
		Instrument: portfolio.Instrument{ContractID: 42, AssetClass: "STK"}, Currency: "USD",
		BidPrice: portfolio.OptionalNumber{Value: 10, Available: true},
	}
	account := portfolio.Portfolio{Positions: []portfolio.Position{{Instrument: portfolio.Instrument{ContractID: 42}, Quantity: 2}}}
	if _, err := Calculate(snapshot, portfolio.StockHistory{}, account, "SELL", 3, time.Now()); err == nil {
		t.Fatal("Calculate() accepted an oversell")
	}
}

func TestCalculateFirstPositionProducesFullResultingWeight(t *testing.T) {
	now := time.Now()
	snapshot := portfolio.StockSnapshot{
		Instrument: portfolio.Instrument{ContractID: 42, AssetClass: "STK", Currency: "USD"}, Currency: "USD",
		AskPrice: portfolio.OptionalNumber{Value: 10, Available: true},
	}
	account := portfolio.Portfolio{AccountSummary: portfolio.AccountSummary{
		NetLiquidation: portfolio.Money{Amount: 1000, Currency: "USD", Available: true},
		CashBalance:    portfolio.Money{Amount: 1000, Currency: "USD", Available: true},
	}}
	result, err := Calculate(snapshot, portfolio.StockHistory{Points: []portfolio.StockHistoryPoint{{Time: now, Close: 10}}}, account, "BUY", 5, now)
	if err != nil {
		t.Fatalf("Calculate() error: %v", err)
	}
	if !result.CurrentWeight.Available || result.CurrentWeight.Value != 0 || !result.ResultingWeight.Available || result.ResultingWeight.Value != 1 {
		t.Fatalf("unexpected first-position weights: current=%#v resulting=%#v", result.CurrentWeight, result.ResultingWeight)
	}
}

func TestCalculateUsesLatestCloseForDuplicateDate(t *testing.T) {
	start := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	snapshot := portfolio.StockSnapshot{Instrument: portfolio.Instrument{ContractID: 42, AssetClass: "STK", Currency: "USD"}, Currency: "USD", AskPrice: portfolio.OptionalNumber{Value: 12, Available: true}}
	account := portfolio.Portfolio{AccountSummary: portfolio.AccountSummary{CashBalance: portfolio.Money{Amount: 100, Currency: "USD", Available: true}, NetLiquidation: portfolio.Money{Amount: 100, Currency: "USD", Available: true}}}
	history := portfolio.StockHistory{Points: []portfolio.StockHistoryPoint{
		{Time: start, Close: 10},
		{Time: start.Add(2 * time.Hour), Close: 20},
		{Time: start.AddDate(0, 0, 1), Close: 30},
	}}
	result, err := Calculate(snapshot, history, account, "BUY", 1, start)
	if err != nil {
		t.Fatal(err)
	}
	if result.HistoryObservations != 2 || math.Abs(result.TotalReturn.Value-.5) > 1e-9 {
		t.Fatalf("duplicate date distorted history metrics: %+v", result)
	}
}
