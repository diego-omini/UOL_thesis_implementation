// Package portfolio defines domain models used independently from IBKR and persistence DTOs.
package portfolio

// This file defines broker-independent portfolio, position, instrument, money, and exchange-rate data.

import "time"

// Money represents a monetary amount in a specific currency.
type Money struct {
	Amount    float64
	Currency  string
	Available bool
}

// ExchangeRate converts one unit of Currency into BaseCurrency.
type ExchangeRate struct {
	Currency          string
	BaseCurrency      string
	BaseAmountPerUnit float64
	Available         bool
	Source            string
	AsOf              time.Time
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

// FundHolding is one issuer-reported constituent normalized independently from broker DTOs.
type FundHolding struct {
	SecurityID string
	Symbol     string
	Name       string
	AssetClass string
	Sector     string
	Currency   string
	Weight     float64
}

// Percentage records an optional percentage without treating zero as unavailable.
type Percentage struct {
	Value     float64
	Available bool
}

// FundHoldings records normalized holdings and the provenance needed to assess their quality.
type FundHoldings struct {
	Fund                      Instrument
	Available                 bool
	Complete                  bool
	SourceURL                 string
	SourceType                string
	RetrievedAt               time.Time
	ExpiresAt                 time.Time
	EffectiveDate             time.Time
	CoveragePercentage        Percentage
	TopHoldingsOnly           bool
	CurrencyAssumptions       []string
	ClassificationAssumptions []string
	Warnings                  []string
	Confidence                string
	Holdings                  []FundHolding
}

// InstrumentSearchResult identifies a stock returned by a broker instrument search.
type InstrumentSearchResult struct {
	ContractID int64
	Symbol     string
	Name       string
	Exchange   string
	AssetClass string
}

// OptionalNumber represents a numeric market value that may be unavailable from the broker.
type OptionalNumber struct {
	Value             float64
	Available         bool
	UnavailableReason string
}

// StockSnapshot contains current quote and reference values for one stock contract.
type StockSnapshot struct {
	Instrument       Instrument
	Currency         string
	OpenPrice        OptionalNumber
	ClosePrice       OptionalNumber
	BidPrice         OptionalNumber
	AskPrice         OptionalNumber
	Volume           OptionalNumber
	High52Week       OptionalNumber
	Low52Week        OptionalNumber
	MarketDataStatus string
	AsOf             time.Time
}

// StockHistoryPoint contains one broker-provided OHLCV bar.
type StockHistoryPoint struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// StockHistory contains the normalized bars returned for a selected range.
type StockHistory struct {
	Range             string
	Points            []StockHistoryPoint
	MarketDataStatus  string
	DelaySeconds      int
	UnavailableReason string
}

// Position represents the portfolio's holding and valuation data for one instrument.
type Position struct {
	Instrument        Instrument
	Quantity          float64
	AverageCost       Money
	MarketPrice       Money
	MarketValue       Money
	BaseMarketValue   Money
	UnrealizedPnL     Money
	BaseUnrealizedPnL Money
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
	ExchangeRates  []ExchangeRate
}
