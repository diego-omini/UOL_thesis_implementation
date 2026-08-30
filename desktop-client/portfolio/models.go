// Package portfolio defines domain models used independently from IBKR and persistence DTOs.
package portfolio

// Money represents a monetary amount in a specific currency.
type Money struct {
	Amount   float64
	Currency string
}

// Instrument describes a tradable financial instrument held in the portfolio.
type Instrument struct {
	ContractID int64
	Symbol     string
	Name       string
	AssetClass string
	Exchange   string
	Currency   string
	Sector     string
	Industry   string
}

// Position represents the portfolio's holding and valuation data for one instrument.
type Position struct {
	Instrument    Instrument
	Quantity      float64
	AverageCost   Money
	MarketPrice   Money
	MarketValue   Money
	UnrealizedPnL Money
}

// AccountSummary contains high-level account values needed by the portfolio view.
type AccountSummary struct {
	AccountID      string
	NetLiquidation Money
	CashBalance    Money
}

// Portfolio combines account-level information with the current open positions.
type Portfolio struct {
	AccountSummary AccountSummary
	Positions      []Position
}
