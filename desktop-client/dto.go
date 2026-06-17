// Wails-facing types definition
package main

import "wails/portfolio"

type ConnectionStatusView struct {
	Connected bool
	Message   string
}

type MoneyView struct {
	Amount   float64
	Currency string
}

type AccountSummaryView struct {
	AccountID      string
	NetLiquidation MoneyView
	CashBalance    MoneyView
}

type InstrumentView struct {
	Symbol     string
	Name       string
	AssetClass string
	Exchange   string
	Currency   string
}

type PositionView struct {
	Instrument    InstrumentView
	Quantity      float64
	AverageCost   MoneyView
	MarketPrice   MoneyView
	MarketValue   MoneyView
	UnrealizedPnL MoneyView
}

type PortfolioView struct {
	AccountSummary AccountSummaryView
	Positions      []PositionView
	Empty          bool
}

func toConnectionStatusView(status bool, message string) ConnectionStatusView {
	return ConnectionStatusView{Connected: status, Message: message}
}

func toPortfolioView(p portfolio.Portfolio) PortfolioView {
	positions := make([]PositionView, 0, len(p.Positions))

	for _, pos := range p.Positions {
		positions = append(positions, PositionView{
			Instrument:    InstrumentView(pos.Instrument),
			Quantity:      pos.Quantity,
			AverageCost:   toMoneyView(pos.AverageCost),
			MarketPrice:   toMoneyView(pos.MarketPrice),
			MarketValue:   toMoneyView(pos.MarketValue),
			UnrealizedPnL: toMoneyView(pos.UnrealizedPnL),
		})
	}

	return PortfolioView{
		AccountSummary: AccountSummaryView{
			AccountID:      p.AccountSummary.AccountID,
			NetLiquidation: toMoneyView(p.AccountSummary.NetLiquidation),
			CashBalance:    toMoneyView(p.AccountSummary.CashBalance),
		},
		Positions: positions,
		Empty:     len(positions) == 0,
	}
}

func toMoneyView(m portfolio.Money) MoneyView {
	return MoneyView{Amount: m.Amount, Currency: m.Currency}
}
