package analytics

// This file verifies deterministic direct and indirect effective-exposure aggregation.

import (
	"math"
	"strings"
	"testing"
	"time"

	"wails/portfolio"
)

func TestCalculateWithFundHoldingsCombinesDirectAndRepeatedIndirectExposureWithoutDoubleCounting(t *testing.T) {
	snapshot := portfolio.Portfolio{
		AccountSummary: portfolio.AccountSummary{NetLiquidation: availableMoney(1000, "USD")},
		Positions: []portfolio.Position{
			{Instrument: portfolio.Instrument{ContractID: 1, Symbol: "AAPL", Name: "Apple", AssetClass: "STK", Sector: "Technology"}, MarketPrice: availableMoney(1, "USD"), MarketValue: availableMoney(400, "USD"), UnrealizedPnL: availableMoney(0, "USD")},
			{Instrument: portfolio.Instrument{ContractID: 2, Symbol: "FUNDA", Name: "Fund A ETF", AssetClass: "ETF"}, MarketPrice: availableMoney(1, "USD"), MarketValue: availableMoney(300, "USD"), UnrealizedPnL: availableMoney(0, "USD")},
			{Instrument: portfolio.Instrument{ContractID: 3, Symbol: "FUNDB", Name: "Fund B ETF", AssetClass: "ETF"}, MarketPrice: availableMoney(1, "USD"), MarketValue: availableMoney(300, "USD"), UnrealizedPnL: availableMoney(0, "USD")},
		},
	}
	holdings := []portfolio.FundHoldings{
		completeFund(snapshot.Positions[1].Instrument, []portfolio.FundHolding{{SecurityID: "US0378331005", Symbol: "AAPL", Name: "Apple", Sector: "Technology", Weight: 0.5}, {SecurityID: "US5949181045", Symbol: "MSFT", Name: "Microsoft", Sector: "Technology", Weight: 0.5}}),
		completeFund(snapshot.Positions[2].Instrument, []portfolio.FundHolding{{SecurityID: "US0378331005", Symbol: "AAPL", Name: "Apple", Sector: "Technology", Weight: 0.25}, {SecurityID: "US0231351067", Symbol: "AMZN", Name: "Amazon", Sector: "Consumer Discretionary", Weight: 0.75}}),
	}

	result := CalculateWithFundHoldings(snapshot, holdings, time.Now())
	assertEffective(t, result.EffectiveSecurityExposure, "AAPL", 625, 0.625)
	assertEffective(t, result.EffectiveSecurityExposure, "MSFT", 150, 0.15)
	assertEffective(t, result.EffectiveSecurityExposure, "AMZN", 225, 0.225)
	if len(result.EffectiveSecurityExposure) != 3 || !result.EffectiveExposureComplete {
		t.Fatalf("fund wrappers were counted or result was incomplete: %#v", result.EffectiveSecurityExposure)
	}
	sector := result.EffectiveSectorAllocation[0]
	if sector.Label != "Technology" || sector.MarketValue.Currency != "USD" || math.Abs(sector.MarketValue.Amount-775) > 0.000001 || math.Abs(sector.Weight.Value-0.775) > 0.000001 {
		t.Fatalf("unexpected effective sector exposure: %#v", result.EffectiveSectorAllocation)
	}
}

func TestCalculateWithFundHoldingsKeepsPartialAndUnavailableExposureVisible(t *testing.T) {
	snapshot := portfolio.Portfolio{
		AccountSummary: portfolio.AccountSummary{NetLiquidation: availableMoney(1000, "USD")},
		Positions: []portfolio.Position{
			{Instrument: portfolio.Instrument{ContractID: 2, Symbol: "PART", Name: "Partial ETF", AssetClass: "ETF"}, MarketPrice: availableMoney(1, "USD"), MarketValue: availableMoney(600, "USD"), UnrealizedPnL: availableMoney(0, "USD")},
			{Instrument: portfolio.Instrument{ContractID: 3, Symbol: "MISS", Name: "Missing Fund", AssetClass: "FUND"}, MarketPrice: availableMoney(1, "USD"), MarketValue: availableMoney(400, "USD"), UnrealizedPnL: availableMoney(0, "USD")},
		},
	}
	partial := completeFund(snapshot.Positions[0].Instrument, []portfolio.FundHolding{{Symbol: "AAA", Name: "Alpha", Weight: 0.75}})
	partial.Complete = false
	partial.CoveragePercentage = portfolio.Percentage{Value: 75, Available: true}
	unavailable := portfolio.FundHoldings{Fund: snapshot.Positions[1].Instrument, SourceType: "unsupported", Confidence: "unavailable"}

	result := CalculateWithFundHoldings(snapshot, []portfolio.FundHoldings{partial, unavailable}, time.Now())
	assertEffective(t, result.EffectiveSecurityExposure, "AAA", 450, 0.45)
	unknownValue := 0.0
	for _, exposure := range result.EffectiveSecurityExposure {
		if strings.HasPrefix(exposure.SecurityID, "unknown:fund:") {
			unknownValue += exposure.Exposure.Amount
		}
	}
	if math.Abs(unknownValue-550) > 0.000001 || result.EffectiveExposureComplete {
		t.Fatalf("unknown exposure was hidden or completeness was overstated: %#v", result)
	}
}

func completeFund(instrument portfolio.Instrument, holdings []portfolio.FundHolding) portfolio.FundHoldings {
	return portfolio.FundHoldings{
		Fund: instrument, Available: true, Complete: true, SourceURL: "https://issuer.example/holdings.csv",
		SourceType: "official_issuer_full_holdings_csv", EffectiveDate: time.Now(), RetrievedAt: time.Now(),
		CoveragePercentage: portfolio.Percentage{Value: 100, Available: true}, Confidence: "high", Holdings: holdings,
	}
}

func assertEffective(t *testing.T, exposures []EffectiveExposure, symbol string, value, weight float64) {
	t.Helper()
	for _, exposure := range exposures {
		if exposure.Symbol == symbol {
			if math.Abs(exposure.MarketValue.Amount-value) > 0.000001 || math.Abs(exposure.Weight.Value-weight) > 0.000001 {
				t.Fatalf("unexpected %s exposure: %#v", symbol, exposure)
			}
			return
		}
	}
	t.Fatalf("missing effective exposure for %s: %#v", symbol, exposures)
}
