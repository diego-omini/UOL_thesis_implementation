// Package analytics calculates deterministic portfolio metrics from domain models.
package analytics

import (
	"math"
	"sort"
	"strings"
	"time"

	"wails/portfolio"
)

const (
	MetricNetMarketValue        = "portfolio.net_market_value"
	MetricGrossExposure         = "portfolio.gross_exposure"
	MetricTotalUnrealizedPnL    = "portfolio.total_unrealized_pnl"
	MetricTopThreeConcentration = "portfolio.top_three_concentration"
	MetricHHI                   = "portfolio.hhi"
)

type Ratio struct {
	Value     float64
	Available bool
}

type PositionWeight struct {
	ContractID  int64
	Symbol      string
	Name        string
	MarketValue portfolio.Money
	Weight      Ratio
}

type Allocation struct {
	Label       string
	MarketValue portfolio.Money
	Exposure    portfolio.Money
	Weight      Ratio
}

type DataWarning struct {
	Code    string
	Message string
	Count   int
}

type Result struct {
	CalculatedAt          time.Time
	BaseCurrency          string
	NetMarketValue        portfolio.Money
	GrossExposure         portfolio.Money
	TotalUnrealizedPnL    portfolio.Money
	PositionWeights       []PositionWeight
	LargestPositions      []PositionWeight
	CurrencyAllocation    []Allocation
	AssetClassAllocation  []Allocation
	SectorAllocation      []Allocation
	TopThreeConcentration Ratio
	HHI                   Ratio
	ExchangeRates         []portfolio.ExchangeRate
	Warnings              []DataWarning
}

type allocationValue struct {
	marketValue float64
	exposure    float64
}

var warningMessages = map[string]string{
	"missing_base_currency":       "The account base currency is unavailable.",
	"missing_currency":            "One or more positions have no currency.",
	"missing_market_price":        "One or more positions have no market price.",
	"missing_market_value":        "One or more positions have no market value.",
	"missing_base_market_value":   "One or more foreign-currency positions have no base-currency market value.",
	"missing_unrealized_pnl":      "One or more positions have no unrealized profit or loss value.",
	"missing_base_unrealized_pnl": "One or more foreign-currency positions have no base-currency unrealized profit or loss.",
	"missing_asset_class":         "One or more positions have no asset-class classification.",
	"missing_sector":              "One or more positions have no sector classification.",
}

var warningOrder = []string{
	"missing_base_currency",
	"missing_currency",
	"missing_market_price",
	"missing_market_value",
	"missing_base_market_value",
	"missing_unrealized_pnl",
	"missing_base_unrealized_pnl",
	"missing_asset_class",
	"missing_sector",
}

// Calculate produces portfolio metrics using values expressed in the account base currency.
func Calculate(snapshot portfolio.Portfolio, calculatedAt time.Time) Result {
	baseCurrency := determineBaseCurrency(snapshot)
	result := Result{
		CalculatedAt:    calculatedAt.UTC(),
		BaseCurrency:    baseCurrency,
		PositionWeights: make([]PositionWeight, len(snapshot.Positions)),
		ExchangeRates:   append([]portfolio.ExchangeRate(nil), snapshot.ExchangeRates...),
	}
	exchangeRates := exchangeRateLookup(snapshot.ExchangeRates, baseCurrency)

	warningCounts := make(map[string]int)
	if baseCurrency == "" {
		warningCounts["missing_base_currency"] = 1
	}

	marketValues := make([]float64, len(snapshot.Positions))
	marketValuesComplete := baseCurrency != ""
	pnlComplete := baseCurrency != ""
	currencyValues := make(map[string]allocationValue)
	assetClassValues := make(map[string]allocationValue)
	sectorValues := make(map[string]allocationValue)

	for index, position := range snapshot.Positions {
		result.PositionWeights[index] = PositionWeight{
			ContractID: position.Instrument.ContractID,
			Symbol:     position.Instrument.Symbol,
			Name:       position.Instrument.Name,
		}

		if position.Instrument.AssetClass == "" {
			warningCounts["missing_asset_class"]++
		}
		if position.Instrument.Sector == "" {
			warningCounts["missing_sector"]++
		}
		if strings.TrimSpace(position.MarketValue.Currency) == "" {
			warningCounts["missing_currency"]++
		}
		if !position.MarketPrice.Available {
			warningCounts["missing_market_price"]++
		}

		marketValue, available, warningCode := baseMoney(
			position.MarketValue,
			position.BaseMarketValue,
			baseCurrency,
			exchangeRates,
			"missing_market_value",
			"missing_base_market_value",
		)
		if !available {
			marketValuesComplete = false
			recordWarning(warningCounts, warningCode)
		} else {
			marketValues[index] = marketValue.Amount
			result.PositionWeights[index].MarketValue = marketValue
			addAllocation(currencyValues, labelOrDefault(position.MarketValue.Currency, "Unknown currency"), marketValue.Amount)

			assetClass := labelOrDefault(position.Instrument.AssetClass, "Unclassified")
			addAllocation(assetClassValues, assetClass, marketValue.Amount)

			sector := labelOrDefault(position.Instrument.Sector, "Unclassified")
			addAllocation(sectorValues, sector, marketValue.Amount)
		}

		pnl, available, warningCode := baseMoney(
			position.UnrealizedPnL,
			position.BaseUnrealizedPnL,
			baseCurrency,
			exchangeRates,
			"missing_unrealized_pnl",
			"missing_base_unrealized_pnl",
		)
		if !available {
			pnlComplete = false
			recordWarning(warningCounts, warningCode)
		} else {
			result.TotalUnrealizedPnL.Amount += pnl.Amount
		}
	}

	result.TotalUnrealizedPnL.Currency = baseCurrency
	result.TotalUnrealizedPnL.Available = pnlComplete

	if marketValuesComplete {
		completeMarketAnalytics(&result, marketValues, currencyValues, assetClassValues, sectorValues)
	}

	result.Warnings = buildWarnings(warningCounts)
	return result
}

func completeMarketAnalytics(
	result *Result,
	marketValues []float64,
	currencyValues map[string]allocationValue,
	assetClassValues map[string]allocationValue,
	sectorValues map[string]allocationValue,
) {
	result.NetMarketValue = portfolio.Money{Currency: result.BaseCurrency, Available: true}
	result.GrossExposure = portfolio.Money{Currency: result.BaseCurrency, Available: true}

	for _, value := range marketValues {
		result.NetMarketValue.Amount += value
		result.GrossExposure.Amount += math.Abs(value)
	}

	if result.GrossExposure.Amount == 0 {
		return
	}

	for index, value := range marketValues {
		result.PositionWeights[index].Weight = Ratio{
			Value:     math.Abs(value) / result.GrossExposure.Amount,
			Available: true,
		}
	}

	result.LargestPositions = append([]PositionWeight(nil), result.PositionWeights...)
	sort.SliceStable(result.LargestPositions, func(i, j int) bool {
		return result.LargestPositions[i].Weight.Value > result.LargestPositions[j].Weight.Value
	})

	for _, position := range result.PositionWeights {
		result.HHI.Value += position.Weight.Value * position.Weight.Value
	}
	result.HHI.Available = true

	topCount := 3
	if len(result.LargestPositions) < topCount {
		topCount = len(result.LargestPositions)
	}
	for index := 0; index < topCount; index++ {
		result.TopThreeConcentration.Value += result.LargestPositions[index].Weight.Value
	}
	result.TopThreeConcentration.Available = true

	result.CurrencyAllocation = buildAllocations(currencyValues, result.BaseCurrency, result.GrossExposure.Amount)
	result.AssetClassAllocation = buildAllocations(assetClassValues, result.BaseCurrency, result.GrossExposure.Amount)
	result.SectorAllocation = buildAllocations(sectorValues, result.BaseCurrency, result.GrossExposure.Amount)
}

func determineBaseCurrency(snapshot portfolio.Portfolio) string {
	if currency := strings.TrimSpace(snapshot.AccountSummary.NetLiquidation.Currency); currency != "" {
		return currency
	}
	if currency := strings.TrimSpace(snapshot.AccountSummary.CashBalance.Currency); currency != "" {
		return currency
	}

	currency := ""
	for _, position := range snapshot.Positions {
		positionCurrency := strings.TrimSpace(position.MarketValue.Currency)
		if positionCurrency == "" {
			continue
		}
		if currency != "" && !strings.EqualFold(currency, positionCurrency) {
			return ""
		}
		currency = positionCurrency
	}
	return currency
}

func baseMoney(
	local portfolio.Money,
	base portfolio.Money,
	baseCurrency string,
	exchangeRates map[string]portfolio.ExchangeRate,
	missingLocalCode string,
	missingBaseCode string,
) (portfolio.Money, bool, string) {
	if baseCurrency == "" {
		return portfolio.Money{}, false, "missing_base_currency"
	}
	if base.Available {
		return portfolio.Money{Amount: base.Amount, Currency: baseCurrency, Available: true}, true, ""
	}
	if !local.Available {
		return portfolio.Money{}, false, missingLocalCode
	}
	if strings.EqualFold(strings.TrimSpace(local.Currency), baseCurrency) {
		return portfolio.Money{Amount: local.Amount, Currency: baseCurrency, Available: true}, true, ""
	}
	rate, ok := exchangeRates[strings.ToUpper(strings.TrimSpace(local.Currency))]
	if ok {
		return portfolio.Money{
			Amount:    local.Amount * rate.BaseAmountPerUnit,
			Currency:  baseCurrency,
			Available: true,
		}, true, ""
	}
	return portfolio.Money{}, false, missingBaseCode
}

func exchangeRateLookup(rates []portfolio.ExchangeRate, baseCurrency string) map[string]portfolio.ExchangeRate {
	lookup := make(map[string]portfolio.ExchangeRate)
	for _, rate := range rates {
		currency := strings.ToUpper(strings.TrimSpace(rate.Currency))
		if currency == "" || !strings.EqualFold(strings.TrimSpace(rate.BaseCurrency), baseCurrency) {
			continue
		}
		if !rate.Available || rate.BaseAmountPerUnit <= 0 || math.IsNaN(rate.BaseAmountPerUnit) || math.IsInf(rate.BaseAmountPerUnit, 0) {
			continue
		}
		lookup[currency] = rate
	}
	return lookup
}

func recordWarning(counts map[string]int, code string) {
	if code == "" {
		return
	}
	if code == "missing_base_currency" {
		counts[code] = 1
		return
	}
	counts[code]++
}

func addAllocation(values map[string]allocationValue, label string, marketValue float64) {
	value := values[label]
	value.marketValue += marketValue
	value.exposure += math.Abs(marketValue)
	values[label] = value
}

func buildAllocations(values map[string]allocationValue, currency string, grossExposure float64) []Allocation {
	allocations := make([]Allocation, 0, len(values))
	for label, value := range values {
		allocations = append(allocations, Allocation{
			Label:       label,
			MarketValue: portfolio.Money{Amount: value.marketValue, Currency: currency, Available: true},
			Exposure:    portfolio.Money{Amount: value.exposure, Currency: currency, Available: true},
			Weight:      Ratio{Value: value.exposure / grossExposure, Available: true},
		})
	}

	sort.Slice(allocations, func(i, j int) bool {
		if allocations[i].Weight.Value == allocations[j].Weight.Value {
			return allocations[i].Label < allocations[j].Label
		}
		return allocations[i].Weight.Value > allocations[j].Weight.Value
	})
	return allocations
}

func buildWarnings(counts map[string]int) []DataWarning {
	warnings := make([]DataWarning, 0, len(counts))
	for _, code := range warningOrder {
		if counts[code] == 0 {
			continue
		}
		warnings = append(warnings, DataWarning{
			Code:    code,
			Message: warningMessages[code],
			Count:   counts[code],
		})
	}
	return warnings
}

func labelOrDefault(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}
