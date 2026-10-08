// Package stockanalysis calculates deterministic market and proposed-order metrics for one stock.
package stockanalysis

// This file derives price behavior and portfolio impact without relying on model arithmetic.

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"wails/analytics"
	"wails/portfolio"
)

// Metric is a numeric result that may be unavailable when its required inputs are missing.
type Metric struct {
	Value     float64
	Available bool
}

// Result contains reproducible stock and proposed-order metrics for counselor interpretation.
type Result struct {
	CalculatedAt          time.Time
	Instrument            portfolio.Instrument
	Side                  string
	Quantity              float64
	EstimatedPrice        portfolio.Money
	EstimatedOrderValue   portfolio.Money
	BaseOrderValue        portfolio.Money
	CashBalance           portfolio.Money
	CashUsage             Metric
	CurrentQuantity       float64
	ResultingQuantity     float64
	CurrentWeight         Metric
	ResultingWeight       Metric
	WeightChange          Metric
	HistoryStart          time.Time
	HistoryEnd            time.Time
	HistoryObservations   int
	TotalReturn           Metric
	AnnualizedVolatility  Metric
	MaximumDrawdown       Metric
	PositionIn52WeekRange Metric
	DistanceFromSMA50     Metric
	DistanceFromSMA200    Metric
	MarketDataStatus      string
	Warnings              []string
}

// Calculate validates the proposed order and derives its market and concentration context.
func Calculate(
	snapshot portfolio.StockSnapshot,
	history portfolio.StockHistory,
	account portfolio.Portfolio,
	side string,
	quantity float64,
	calculatedAt time.Time,
) (Result, error) {
	side = strings.ToUpper(strings.TrimSpace(side))
	if side != "BUY" && side != "SELL" {
		return Result{}, errors.New("order side must be BUY or SELL")
	}
	if math.IsNaN(quantity) || math.IsInf(quantity, 0) || quantity <= 0 || math.Round(quantity) != quantity {
		return Result{}, errors.New("quantity must be a positive whole number of shares")
	}
	if snapshot.Instrument.ContractID <= 0 || !strings.EqualFold(snapshot.Instrument.AssetClass, "STK") {
		return Result{}, errors.New("order analysis requires a valid stock contract")
	}

	price := snapshot.AskPrice
	if side == "SELL" {
		price = snapshot.BidPrice
	}
	if !price.Available || price.Value <= 0 {
		return Result{}, errors.New("a current executable quote is required for order analysis")
	}

	currentQuantity := currentPositionQuantity(account.Positions, snapshot.Instrument.ContractID)
	if side == "SELL" && quantity > math.Max(currentQuantity, 0) {
		return Result{}, fmt.Errorf("cannot analyze a sale of %.0f shares; only %.0f shares are available", quantity, math.Max(currentQuantity, 0))
	}
	direction := 1.0
	if side == "SELL" {
		direction = -1
	}
	resultingQuantity := currentQuantity + direction*quantity
	localOrderValue := portfolio.Money{
		Amount: quantity * price.Value, Currency: snapshot.Currency, Available: true,
	}

	portfolioResult := analytics.Calculate(account, calculatedAt)
	baseCurrency := portfolioResult.BaseCurrency
	baseOrderValue, baseAvailable := convertMoney(localOrderValue, baseCurrency, account.ExchangeRates)
	currentBaseValue, currentValueAvailable := currentPositionValue(portfolioResult, snapshot.Instrument.ContractID)

	result := Result{
		CalculatedAt: calculatedAt.UTC(), Instrument: snapshot.Instrument,
		Side: side, Quantity: quantity,
		EstimatedPrice:      portfolio.Money{Amount: price.Value, Currency: snapshot.Currency, Available: true},
		EstimatedOrderValue: localOrderValue,
		BaseOrderValue:      baseOrderValue,
		CashBalance:         account.AccountSummary.CashBalance,
		CurrentQuantity:     currentQuantity, ResultingQuantity: resultingQuantity,
		MarketDataStatus: snapshot.MarketDataStatus,
		Warnings:         append([]string(nil), historyWarnings(history)...),
	}

	if side == "BUY" && baseAvailable && result.CashBalance.Available && result.CashBalance.Amount > 0 && strings.EqualFold(result.CashBalance.Currency, baseCurrency) {
		result.CashUsage = Metric{Value: baseOrderValue.Amount / result.CashBalance.Amount, Available: true}
	}
	if portfolioResult.GrossExposure.Available && currentValueAvailable {
		result.CurrentWeight = Metric{Available: true}
		if portfolioResult.GrossExposure.Amount > 0 {
			result.CurrentWeight.Value = math.Abs(currentBaseValue) / portfolioResult.GrossExposure.Amount
		}
		if baseAvailable {
			resultingBaseValue := currentBaseValue + direction*baseOrderValue.Amount
			resultingGross := portfolioResult.GrossExposure.Amount - math.Abs(currentBaseValue) + math.Abs(resultingBaseValue)
			if resultingGross > 0 {
				result.ResultingWeight = Metric{Value: math.Abs(resultingBaseValue) / resultingGross, Available: true}
				result.WeightChange = Metric{Value: result.ResultingWeight.Value - result.CurrentWeight.Value, Available: true}
			}
		}
	}
	if !baseAvailable {
		result.Warnings = append(result.Warnings, "The estimated order value could not be converted into the account base currency.")
	}
	if side == "BUY" && !result.CashUsage.Available {
		result.Warnings = append(result.Warnings, "Cash usage could not be calculated from the available account data.")
	}
	if !result.ResultingWeight.Available {
		result.Warnings = append(result.Warnings, "Resulting portfolio concentration could not be calculated from the available account data.")
	}

	calculateMarketMetrics(&result, history, snapshot)
	return result, nil
}

// calculateMarketMetrics computes return, volatility, drawdown, range, and moving-average distance.
func calculateMarketMetrics(result *Result, history portfolio.StockHistory, snapshot portfolio.StockSnapshot) {
	closes := validCloses(history.Points)
	if len(closes) == 0 {
		result.Warnings = append(result.Warnings, "No usable one-year closing-price observations were available.")
		return
	}
	result.HistoryStart = closes[0].Time
	result.HistoryEnd = closes[len(closes)-1].Time
	result.HistoryObservations = len(closes)
	if len(closes) >= 2 {
		result.TotalReturn = Metric{Value: closes[len(closes)-1].Close/closes[0].Close - 1, Available: true}
		result.MaximumDrawdown = Metric{Value: maximumDrawdown(closes), Available: true}
		if volatility, ok := annualizedVolatility(closes); ok {
			result.AnnualizedVolatility = Metric{Value: volatility, Available: true}
		}
	}
	last := closes[len(closes)-1].Close
	if len(closes) >= 50 {
		result.DistanceFromSMA50 = Metric{Value: last/simpleMovingAverage(closes, 50) - 1, Available: true}
	}
	if len(closes) >= 200 {
		result.DistanceFromSMA200 = Metric{Value: last/simpleMovingAverage(closes, 200) - 1, Available: true}
	}
	low, high, available := rangeBounds(snapshot, closes)
	if available && high > low {
		position := (last - low) / (high - low)
		result.PositionIn52WeekRange = Metric{Value: math.Max(0, math.Min(1, position)), Available: true}
	}
}

type closeObservation struct {
	Time  time.Time
	Close float64
}

func validCloses(points []portfolio.StockHistoryPoint) []closeObservation {
	byDate := make(map[string]closeObservation, len(points))
	for _, point := range points {
		if !point.Time.IsZero() && point.Close > 0 && !math.IsNaN(point.Close) && !math.IsInf(point.Close, 0) {
			key := point.Time.UTC().Format("2006-01-02")
			current, exists := byDate[key]
			if !exists || !point.Time.Before(current.Time) {
				byDate[key] = closeObservation{Time: point.Time.UTC(), Close: point.Close}
			}
		}
	}
	values := make([]closeObservation, 0, len(byDate))
	for _, value := range byDate {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Time.Before(values[j].Time) })
	return values
}

func annualizedVolatility(closes []closeObservation) (float64, bool) {
	if len(closes) < 3 {
		return 0, false
	}
	returns := make([]float64, 0, len(closes)-1)
	mean := 0.0
	for index := 1; index < len(closes); index++ {
		value := math.Log(closes[index].Close / closes[index-1].Close)
		returns = append(returns, value)
		mean += value
	}
	mean /= float64(len(returns))
	variance := 0.0
	for _, value := range returns {
		difference := value - mean
		variance += difference * difference
	}
	variance /= float64(len(returns) - 1)
	return math.Sqrt(variance) * math.Sqrt(252), true
}

func maximumDrawdown(closes []closeObservation) float64 {
	peak := closes[0].Close
	worst := 0.0
	for _, observation := range closes[1:] {
		if observation.Close > peak {
			peak = observation.Close
			continue
		}
		drawdown := observation.Close/peak - 1
		if drawdown < worst {
			worst = drawdown
		}
	}
	return worst
}

func simpleMovingAverage(closes []closeObservation, periods int) float64 {
	total := 0.0
	for _, observation := range closes[len(closes)-periods:] {
		total += observation.Close
	}
	return total / float64(periods)
}

func rangeBounds(snapshot portfolio.StockSnapshot, closes []closeObservation) (float64, float64, bool) {
	if snapshot.Low52Week.Available && snapshot.High52Week.Available && snapshot.High52Week.Value > snapshot.Low52Week.Value {
		return snapshot.Low52Week.Value, snapshot.High52Week.Value, true
	}
	low, high := closes[0].Close, closes[0].Close
	for _, observation := range closes[1:] {
		low = math.Min(low, observation.Close)
		high = math.Max(high, observation.Close)
	}
	return low, high, high > low
}

func currentPositionQuantity(positions []portfolio.Position, contractID int64) float64 {
	for _, position := range positions {
		if position.Instrument.ContractID == contractID {
			return position.Quantity
		}
	}
	return 0
}

func currentPositionValue(result analytics.Result, contractID int64) (float64, bool) {
	for _, position := range result.PositionWeights {
		if position.ContractID == contractID {
			if position.MarketValue.Available {
				return position.MarketValue.Amount, true
			}
			return 0, false
		}
	}
	return 0, true
}

func convertMoney(value portfolio.Money, baseCurrency string, rates []portfolio.ExchangeRate) (portfolio.Money, bool) {
	if !value.Available || strings.TrimSpace(baseCurrency) == "" {
		return portfolio.Money{}, false
	}
	if strings.EqualFold(value.Currency, baseCurrency) {
		return portfolio.Money{Amount: value.Amount, Currency: baseCurrency, Available: true}, true
	}
	for _, rate := range rates {
		if rate.Available && rate.BaseAmountPerUnit > 0 && strings.EqualFold(rate.Currency, value.Currency) && strings.EqualFold(rate.BaseCurrency, baseCurrency) {
			return portfolio.Money{Amount: value.Amount * rate.BaseAmountPerUnit, Currency: baseCurrency, Available: true}, true
		}
	}
	return portfolio.Money{}, false
}

func historyWarnings(history portfolio.StockHistory) []string {
	warnings := []string{}
	if strings.TrimSpace(history.UnavailableReason) != "" {
		warnings = append(warnings, history.UnavailableReason)
	}
	if history.DelaySeconds > 0 {
		warnings = append(warnings, fmt.Sprintf("Historical market data is delayed by %d seconds.", history.DelaySeconds))
	}
	return warnings
}
