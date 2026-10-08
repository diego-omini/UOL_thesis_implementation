package positionanalysis

// This file verifies deterministic selected-position risk calculations and validation.

import (
	"math"
	"testing"
	"time"

	"wails/analytics"
	"wails/portfolio"
)

func TestCalculatePositionRisk(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	points := make([]portfolio.StockHistoryPoint, 220)
	for index := range points {
		price := 100 + float64(index)*0.1
		if index == 100 {
			price = 80
		}
		points[index] = portfolio.StockHistoryPoint{Time: start.AddDate(0, 0, index), Close: price}
	}
	instrument := portfolio.Instrument{ContractID: 42, Symbol: "TEST", Name: "Test AG", AssetClass: "STK", Currency: "USD"}
	position := portfolio.Position{Instrument: instrument, Quantity: 10, AverageCost: portfolio.Money{Amount: 90, Currency: "USD", Available: true}}
	snapshot := portfolio.StockSnapshot{Instrument: instrument, Low52Week: portfolio.OptionalNumber{Value: 75, Available: true}, High52Week: portfolio.OptionalNumber{Value: 130, Available: true}, AsOf: start.AddDate(0, 0, 220)}

	result, err := Calculate(snapshot, portfolio.StockHistory{Points: points, DelaySeconds: 900}, position, analytics.Ratio{Value: .25, Available: true}, start.AddDate(0, 0, 221))
	if err != nil {
		t.Fatalf("Calculate() error: %v", err)
	}
	if result.HistoryObservations != 220 || !result.TotalReturn.Available || !result.AnnualizedVolatility.Available || !result.MaximumDrawdown.Available {
		t.Fatalf("market metrics unavailable: %#v", result)
	}
	if !result.PortfolioWeight.Available || math.Abs(result.PortfolioWeight.Value-.25) > 1e-9 || len(result.Warnings) != 1 {
		t.Fatalf("position context not preserved: %#v", result)
	}
}

func TestCalculateRejectsMismatchedPosition(t *testing.T) {
	snapshot := portfolio.StockSnapshot{Instrument: portfolio.Instrument{ContractID: 42, AssetClass: "STK"}}
	position := portfolio.Position{Instrument: portfolio.Instrument{ContractID: 7}}
	if _, err := Calculate(snapshot, portfolio.StockHistory{}, position, analytics.Ratio{}, time.Now()); err == nil {
		t.Fatal("Calculate() accepted a mismatched holding")
	}
}

func TestCalculateUsesLatestCloseForDuplicateDate(t *testing.T) {
	start := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	instrument := portfolio.Instrument{ContractID: 42, AssetClass: "STK"}
	history := portfolio.StockHistory{Points: []portfolio.StockHistoryPoint{
		{Time: start, Close: 10},
		{Time: start.Add(time.Hour), Close: 20},
		{Time: start.AddDate(0, 0, 1), Close: 30},
	}}
	result, err := Calculate(portfolio.StockSnapshot{Instrument: instrument}, history, portfolio.Position{Instrument: instrument}, analytics.Ratio{}, start)
	if err != nil {
		t.Fatal(err)
	}
	if result.HistoryObservations != 2 || math.Abs(result.TotalReturn.Value-.5) > 1e-9 {
		t.Fatalf("duplicate date distorted history metrics: %+v", result)
	}
}
