package main

// dto.go defines Wails-facing view models and maps domain results into them.

import (
	"time"

	"wails/analytics"
	"wails/broker"
	"wails/portfolio"
)

type ConnectionStatusView struct {
	Connected bool
	Message   string
}

type SessionStatusView struct {
	State            string
	GatewayReachable bool
	Authenticated    bool
	Connected        bool
	Competing        bool
	Paper            bool
	PaperKnown       bool
	Ready            bool
	Message          string
}

type MoneyView struct {
	Amount    float64
	Currency  string
	Available bool
}

type RatioView struct {
	Value     float64
	Available bool
}

type AccountSummaryView struct {
	AccountID      string
	NetLiquidation MoneyView
	CashBalance    MoneyView
}

type InstrumentView struct {
	ContractID int64
	Symbol     string
	Name       string
	AssetClass string
	Exchange   string
	Currency   string
	Sector     string
	Industry   string
}

type PositionView struct {
	Instrument        InstrumentView
	Quantity          float64
	AverageCost       MoneyView
	MarketPrice       MoneyView
	MarketValue       MoneyView
	BaseMarketValue   MoneyView
	UnrealizedPnL     MoneyView
	BaseUnrealizedPnL MoneyView
	PortfolioWeight   RatioView
}

type AllocationView struct {
	Label       string
	MarketValue MoneyView
	Exposure    MoneyView
	Weight      RatioView
}

type PositionWeightView struct {
	ContractID  int64
	Symbol      string
	Name        string
	MarketValue MoneyView
	Weight      RatioView
}

type DataWarningView struct {
	Code    string
	Message string
	Count   int
}

type ExchangeRateView struct {
	Currency          string
	BaseCurrency      string
	BaseAmountPerUnit float64
	Available         bool
	Source            string
	AsOf              string
}

type AnalyticsView struct {
	CalculatedAt          string
	BaseCurrency          string
	NetMarketValue        MoneyView
	GrossExposure         MoneyView
	TotalUnrealizedPnL    MoneyView
	LargestPositions      []PositionWeightView
	CurrencyAllocation    []AllocationView
	AssetClassAllocation  []AllocationView
	SectorAllocation      []AllocationView
	TopThreeConcentration RatioView
	HHI                   RatioView
	ExchangeRates         []ExchangeRateView
	Warnings              []DataWarningView
}

type PortfolioView struct {
	AccountSummary AccountSummaryView
	Positions      []PositionView
	Analytics      AnalyticsView
	Empty          bool
}

type AIResponseView struct {
	Text          string
	Provider      string
	Model         string
	PromptID      string
	PromptVersion int
}

func toConnectionStatusView(status bool, message string) ConnectionStatusView {
	return ConnectionStatusView{Connected: status, Message: message}
}

func toSessionStatusView(status broker.SessionStatus) SessionStatusView {
	return SessionStatusView{
		State:            string(status.State),
		GatewayReachable: status.GatewayReachable,
		Authenticated:    status.Authenticated,
		Connected:        status.Connected,
		Competing:        status.Competing,
		Paper:            status.Paper,
		PaperKnown:       status.PaperKnown,
		Ready:            status.Ready(),
		Message:          status.Message,
	}
}

// toPortfolioView assembles the complete frontend payload from portfolio and analytics domain results.
func toPortfolioView(p portfolio.Portfolio, result analytics.Result) PortfolioView {
	positions := make([]PositionView, 0, len(p.Positions))

	for index, pos := range p.Positions {
		weight := RatioView{}
		if index < len(result.PositionWeights) {
			weight = toRatioView(result.PositionWeights[index].Weight)
		}

		positions = append(positions, PositionView{
			Instrument:        InstrumentView(pos.Instrument),
			Quantity:          pos.Quantity,
			AverageCost:       toMoneyView(pos.AverageCost),
			MarketPrice:       toMoneyView(pos.MarketPrice),
			MarketValue:       toMoneyView(pos.MarketValue),
			BaseMarketValue:   toMoneyView(pos.BaseMarketValue),
			UnrealizedPnL:     toMoneyView(pos.UnrealizedPnL),
			BaseUnrealizedPnL: toMoneyView(pos.BaseUnrealizedPnL),
			PortfolioWeight:   weight,
		})
	}

	return PortfolioView{
		AccountSummary: AccountSummaryView{
			AccountID:      p.AccountSummary.AccountID,
			NetLiquidation: toMoneyView(p.AccountSummary.NetLiquidation),
			CashBalance:    toMoneyView(p.AccountSummary.CashBalance),
		},
		Positions: positions,
		Analytics: toAnalyticsView(result),
		Empty:     len(positions) == 0,
	}
}

func toMoneyView(m portfolio.Money) MoneyView {
	return MoneyView{Amount: m.Amount, Currency: m.Currency, Available: m.Available}
}

func toRatioView(ratio analytics.Ratio) RatioView {
	return RatioView{Value: ratio.Value, Available: ratio.Available}
}

// toAnalyticsView maps the complete analytics result into Wails-safe view models.
func toAnalyticsView(result analytics.Result) AnalyticsView {
	return AnalyticsView{
		CalculatedAt:          result.CalculatedAt.Format(time.RFC3339),
		BaseCurrency:          result.BaseCurrency,
		NetMarketValue:        toMoneyView(result.NetMarketValue),
		GrossExposure:         toMoneyView(result.GrossExposure),
		TotalUnrealizedPnL:    toMoneyView(result.TotalUnrealizedPnL),
		LargestPositions:      toPositionWeightViews(result.LargestPositions),
		CurrencyAllocation:    toAllocationViews(result.CurrencyAllocation),
		AssetClassAllocation:  toAllocationViews(result.AssetClassAllocation),
		SectorAllocation:      toAllocationViews(result.SectorAllocation),
		TopThreeConcentration: toRatioView(result.TopThreeConcentration),
		HHI:                   toRatioView(result.HHI),
		ExchangeRates:         toExchangeRateViews(result.ExchangeRates),
		Warnings:              toWarningViews(result.Warnings),
	}
}

func toExchangeRateViews(rates []portfolio.ExchangeRate) []ExchangeRateView {
	views := make([]ExchangeRateView, 0, len(rates))
	for _, rate := range rates {
		views = append(views, ExchangeRateView{
			Currency:          rate.Currency,
			BaseCurrency:      rate.BaseCurrency,
			BaseAmountPerUnit: rate.BaseAmountPerUnit,
			Available:         rate.Available,
			Source:            rate.Source,
			AsOf:              rate.AsOf.Format(time.RFC3339),
		})
	}
	return views
}

func toPositionWeightViews(positions []analytics.PositionWeight) []PositionWeightView {
	views := make([]PositionWeightView, 0, len(positions))
	for _, position := range positions {
		views = append(views, PositionWeightView{
			ContractID:  position.ContractID,
			Symbol:      position.Symbol,
			Name:        position.Name,
			MarketValue: toMoneyView(position.MarketValue),
			Weight:      toRatioView(position.Weight),
		})
	}
	return views
}

func toAllocationViews(allocations []analytics.Allocation) []AllocationView {
	views := make([]AllocationView, 0, len(allocations))
	for _, allocation := range allocations {
		views = append(views, AllocationView{
			Label:       allocation.Label,
			MarketValue: toMoneyView(allocation.MarketValue),
			Exposure:    toMoneyView(allocation.Exposure),
			Weight:      toRatioView(allocation.Weight),
		})
	}
	return views
}

func toWarningViews(warnings []analytics.DataWarning) []DataWarningView {
	views := make([]DataWarningView, 0, len(warnings))
	for _, warning := range warnings {
		views = append(views, DataWarningView(warning))
	}
	return views
}
