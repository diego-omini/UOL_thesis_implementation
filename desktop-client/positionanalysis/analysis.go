// Package positionanalysis calculates deterministic current-position and market-risk metrics.
package positionanalysis

// This file derives selected-holding risk context without model arithmetic or order assumptions.

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

// Metric is a numeric result that may be unavailable when required inputs are missing.
type Metric struct {
	Value     float64
	Available bool
}

// Result contains reproducible current-holding and historical market-risk facts.
type Result struct {
	CalculatedAt          time.Time
	QuoteAsOf             time.Time
	Instrument            portfolio.Instrument
	Quantity              float64
	AverageCost           portfolio.Money
	MarketPrice           portfolio.Money
	MarketValue           portfolio.Money
	BaseMarketValue       portfolio.Money
	UnrealizedPnL         portfolio.Money
	BaseUnrealizedPnL     portfolio.Money
	PortfolioWeight       Metric
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
	HistoryDelaySeconds   int
	Warnings              []string
}

// Calculate derives current-position and one-year market behavior from normalized broker data.
func Calculate(snapshot portfolio.StockSnapshot, history portfolio.StockHistory, position portfolio.Position, weight analytics.Ratio, calculatedAt time.Time) (Result, error) {
	if snapshot.Instrument.ContractID <= 0 || !strings.EqualFold(snapshot.Instrument.AssetClass, "STK") {
		return Result{}, errors.New("position risk requires a valid stock contract")
	}
	if position.Instrument.ContractID != snapshot.Instrument.ContractID {
		return Result{}, errors.New("position risk requires the selected current holding")
	}
	result := Result{
		CalculatedAt: calculatedAt.UTC(), QuoteAsOf: snapshot.AsOf.UTC(), Instrument: snapshot.Instrument,
		Quantity: position.Quantity, AverageCost: position.AverageCost, MarketPrice: position.MarketPrice,
		MarketValue: position.MarketValue, BaseMarketValue: position.BaseMarketValue,
		UnrealizedPnL: position.UnrealizedPnL, BaseUnrealizedPnL: position.BaseUnrealizedPnL,
		PortfolioWeight:  Metric{Value: weight.Value, Available: weight.Available},
		MarketDataStatus: snapshot.MarketDataStatus, HistoryDelaySeconds: history.DelaySeconds,
	}
	if strings.TrimSpace(history.UnavailableReason) != "" {
		result.Warnings = append(result.Warnings, history.UnavailableReason)
	}
	if history.DelaySeconds > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Historical market data is delayed by %d seconds.", history.DelaySeconds))
	}
	calculateMarketMetrics(&result, snapshot, history)
	return result, nil
}

// calculateMarketMetrics derives return behavior from valid closes and preserves unavailable metrics.
func calculateMarketMetrics(result *Result, snapshot portfolio.StockSnapshot, history portfolio.StockHistory) {
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
	low, high := rangeBounds(snapshot, closes)
	if high > low {
		result.PositionIn52WeekRange = Metric{Value: math.Max(0, math.Min(1, (last-low)/(high-low))), Available: true}
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
	return math.Sqrt(variance * 252), true
}

func maximumDrawdown(closes []closeObservation) float64 {
	peak, worst := closes[0].Close, 0.0
	for _, observation := range closes[1:] {
		if observation.Close > peak {
			peak = observation.Close
			continue
		}
		worst = math.Min(worst, observation.Close/peak-1)
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

func rangeBounds(snapshot portfolio.StockSnapshot, closes []closeObservation) (float64, float64) {
	if snapshot.Low52Week.Available && snapshot.High52Week.Available && snapshot.High52Week.Value > snapshot.Low52Week.Value {
		return snapshot.Low52Week.Value, snapshot.High52Week.Value
	}
	low, high := closes[0].Close, closes[0].Close
	for _, observation := range closes[1:] {
		low = math.Min(low, observation.Close)
		high = math.Max(high, observation.Close)
	}
	return low, high
}
