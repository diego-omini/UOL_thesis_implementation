// Package portfolioanalysis calculates modeled portfolio risk from current exposures and historical prices.
package portfolioanalysis

// This file derives reproducible return, covariance, contribution, and stress-scenario results.

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"wails/analytics"
	"wails/portfolio"
)

const tradingDaysPerYear = 252.0

type Metric struct {
	Value     float64
	Available bool
}

type PositionResult struct {
	ContractID           int64
	Symbol               string
	Name                 string
	SignedWeight         float64
	TotalReturn          Metric
	AnnualizedVolatility Metric
	ReturnContribution   Metric
	RiskContribution     Metric
}

type Correlation struct {
	LeftContractID  int64
	LeftSymbol      string
	RightContractID int64
	RightSymbol     string
	Value           float64
}

type Scenario struct {
	ID          string
	Name        string
	Description string
	Impact      portfolio.Money
	ImpactRatio Metric
}

type Result struct {
	CalculatedAt          time.Time
	BaseCurrency          string
	PeriodStart           time.Time
	PeriodEnd             time.Time
	Observations          int
	IncludedPositionCount int
	TotalPositionCount    int
	ModeledTotalReturn    Metric
	AnnualizedVolatility  Metric
	DownsideDeviation     Metric
	MaximumDrawdown       Metric
	Positions             []PositionResult
	Correlations          []Correlation
	Scenarios             []Scenario
	Warnings              []string
}

type positionSeries struct {
	position portfolio.Position
	weight   float64
	returns  map[string]float64
	dates    map[string]time.Time
}

// Calculate models a daily-rebalanced portfolio using current signed gross-exposure weights.
func Calculate(snapshot portfolio.Portfolio, current analytics.Result, histories map[int64]portfolio.StockHistory, calculatedAt time.Time) Result {
	result := Result{CalculatedAt: calculatedAt.UTC(), BaseCurrency: current.BaseCurrency, TotalPositionCount: len(snapshot.Positions)}
	if !current.GrossExposure.Available || current.GrossExposure.Amount <= 0 {
		result.Warnings = append(result.Warnings, "Portfolio risk requires complete positive gross exposure in the account base currency.")
		result.Scenarios = calculateScenarios(snapshot, current)
		return result
	}

	marketValues := make(map[int64]float64, len(current.PositionWeights))
	for _, value := range current.PositionWeights {
		if value.MarketValue.Available {
			marketValues[value.ContractID] = value.MarketValue.Amount
		}
	}

	series := make([]positionSeries, 0, len(snapshot.Positions))
	for _, position := range snapshot.Positions {
		value, valueOK := marketValues[position.Instrument.ContractID]
		history, historyOK := histories[position.Instrument.ContractID]
		returns, dates := dailyReturns(history.Points)
		if !valueOK || !historyOK || len(returns) < 20 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s was excluded because complete base-currency exposure and at least 20 daily returns were not available.", instrumentLabel(position.Instrument)))
			continue
		}
		if !strings.EqualFold(position.Instrument.Currency, current.BaseCurrency) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s price returns exclude currency movements because historical FX rates are unavailable.", instrumentLabel(position.Instrument)))
		}
		series = append(series, positionSeries{position: position, weight: value / current.GrossExposure.Amount, returns: returns, dates: dates})
	}
	result.IncludedPositionCount = len(series)
	result.Scenarios = calculateScenarios(snapshot, current)
	if len(series) == 0 {
		result.Warnings = append(result.Warnings, "No positions had sufficient common market history for modeled portfolio risk.")
		return result
	}

	common := commonDates(series)
	if len(common) < 20 {
		result.Warnings = append(result.Warnings, "Fewer than 20 common daily return observations were available across included positions.")
		return result
	}
	result.Observations = len(common)
	result.PeriodStart = series[0].dates[common[0]]
	result.PeriodEnd = series[0].dates[common[len(common)-1]]

	positionReturns := make([][]float64, len(series))
	portfolioReturns := make([]float64, len(common))
	for i, item := range series {
		positionReturns[i] = make([]float64, len(common))
		for day, date := range common {
			value := item.returns[date]
			positionReturns[i][day] = value
			portfolioReturns[day] += item.weight * value
		}
	}
	result.ModeledTotalReturn = Metric{Value: compoundedReturn(portfolioReturns), Available: true}
	result.AnnualizedVolatility = metricFrom(annualizedVolatility(portfolioReturns))
	result.DownsideDeviation = Metric{Value: downsideDeviation(portfolioReturns), Available: true}
	result.MaximumDrawdown = Metric{Value: maximumDrawdown(portfolioReturns), Available: true}

	covariance := covarianceMatrix(positionReturns)
	variance := portfolioVariance(series, covariance)
	for i, item := range series {
		positionResult := PositionResult{
			ContractID: item.position.Instrument.ContractID, Symbol: item.position.Instrument.Symbol,
			Name: item.position.Instrument.Name, SignedWeight: item.weight,
			TotalReturn:          Metric{Value: compoundedReturn(positionReturns[i]), Available: true},
			AnnualizedVolatility: metricFrom(annualizedVolatility(positionReturns[i])),
			ReturnContribution:   Metric{Value: item.weight * sum(positionReturns[i]), Available: true},
		}
		if variance > 0 {
			marginal := 0.0
			for j := range series {
				marginal += covariance[i][j] * series[j].weight
			}
			positionResult.RiskContribution = Metric{Value: item.weight * marginal / variance, Available: true}
		}
		result.Positions = append(result.Positions, positionResult)
	}
	for i := range series {
		for j := i + 1; j < len(series); j++ {
			if value, ok := correlation(positionReturns[i], positionReturns[j]); ok {
				result.Correlations = append(result.Correlations, Correlation{
					LeftContractID: series[i].position.Instrument.ContractID, LeftSymbol: series[i].position.Instrument.Symbol,
					RightContractID: series[j].position.Instrument.ContractID, RightSymbol: series[j].position.Instrument.Symbol, Value: value,
				})
			}
		}
	}
	sort.Slice(result.Positions, func(i, j int) bool {
		return math.Abs(result.Positions[i].RiskContribution.Value) > math.Abs(result.Positions[j].RiskContribution.Value)
	})
	sort.Slice(result.Correlations, func(i, j int) bool {
		return math.Abs(result.Correlations[i].Value) > math.Abs(result.Correlations[j].Value)
	})
	result.Warnings = append(result.Warnings, "Historical metrics model the current portfolio as daily rebalanced; they are not realized account performance or attribution.")
	return result
}

func dailyReturns(points []portfolio.StockHistoryPoint) (map[string]float64, map[string]time.Time) {
	byDate := make(map[string]portfolio.StockHistoryPoint, len(points))
	for _, point := range points {
		if point.Time.IsZero() || point.Close <= 0 || math.IsNaN(point.Close) || math.IsInf(point.Close, 0) {
			continue
		}
		key := point.Time.UTC().Format("2006-01-02")
		current, exists := byDate[key]
		if !exists || !point.Time.Before(current.Time) {
			point.Time = point.Time.UTC()
			byDate[key] = point
		}
	}
	valid := make([]portfolio.StockHistoryPoint, 0, len(byDate))
	for _, point := range byDate {
		valid = append(valid, point)
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].Time.Before(valid[j].Time) })
	returns := map[string]float64{}
	dates := map[string]time.Time{}
	for i := 1; i < len(valid); i++ {
		key := valid[i].Time.UTC().Format("2006-01-02")
		returns[key] = valid[i].Close/valid[i-1].Close - 1
		dates[key] = valid[i].Time.UTC()
	}
	return returns, dates
}

func commonDates(series []positionSeries) []string {
	counts := map[string]int{}
	for _, item := range series {
		for date := range item.returns {
			counts[date]++
		}
	}
	var dates []string
	for date, count := range counts {
		if count == len(series) {
			dates = append(dates, date)
		}
	}
	sort.Strings(dates)
	return dates
}

func compoundedReturn(values []float64) float64 {
	total := 1.0
	for _, v := range values {
		total *= 1 + v
	}
	return total - 1
}
func sum(values []float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}
func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return sum(values) / float64(len(values))
}

func annualizedVolatility(values []float64) (float64, bool) {
	if len(values) < 2 {
		return 0, false
	}
	m := mean(values)
	variance := 0.0
	for _, v := range values {
		d := v - m
		variance += d * d
	}
	return math.Sqrt(variance/float64(len(values)-1)) * math.Sqrt(tradingDaysPerYear), true
}
func metricFrom(value float64, ok bool) Metric { return Metric{Value: value, Available: ok} }
func downsideDeviation(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, v := range values {
		if v < 0 {
			total += v * v
		}
	}
	return math.Sqrt(total/float64(len(values))) * math.Sqrt(tradingDaysPerYear)
}
func maximumDrawdown(values []float64) float64 {
	wealth, peak, worst := 1.0, 1.0, 0.0
	for _, v := range values {
		wealth *= 1 + v
		if wealth > peak {
			peak = wealth
		}
		d := wealth/peak - 1
		if d < worst {
			worst = d
		}
	}
	return worst
}

func covarianceMatrix(values [][]float64) [][]float64 {
	result := make([][]float64, len(values))
	means := make([]float64, len(values))
	for i := range values {
		means[i] = mean(values[i])
		result[i] = make([]float64, len(values))
	}
	for i := range values {
		for j := i; j < len(values); j++ {
			c := 0.0
			for k := range values[i] {
				c += (values[i][k] - means[i]) * (values[j][k] - means[j])
			}
			c /= float64(len(values[i]) - 1)
			result[i][j] = c
			result[j][i] = c
		}
	}
	return result
}
func portfolioVariance(series []positionSeries, covariance [][]float64) float64 {
	total := 0.0
	for i := range series {
		for j := range series {
			total += series[i].weight * series[j].weight * covariance[i][j]
		}
	}
	return total
}
func correlation(a, b []float64) (float64, bool) {
	if len(a) != len(b) || len(a) < 2 {
		return 0, false
	}
	ma, mb := mean(a), mean(b)
	cov, va, vb := 0.0, 0.0, 0.0
	for i := range a {
		da, db := a[i]-ma, b[i]-mb
		cov += da * db
		va += da * da
		vb += db * db
	}
	if va == 0 || vb == 0 {
		return 0, false
	}
	return cov / math.Sqrt(va*vb), true
}

// calculateScenarios applies fixed shocks to signed current base-currency exposures.
func calculateScenarios(snapshot portfolio.Portfolio, current analytics.Result) []Scenario {
	if !current.GrossExposure.Available {
		return nil
	}
	values := map[int64]float64{}
	for _, v := range current.PositionWeights {
		if v.MarketValue.Available {
			values[v.ContractID] = v.MarketValue.Amount
		}
	}
	type shock struct {
		id, name, description string
		applies               func(portfolio.Position) bool
		rate                  float64
	}
	largest := int64(0)
	largestValue := 0.0
	sectors := map[string]float64{}
	for _, p := range snapshot.Positions {
		v := values[p.Instrument.ContractID]
		if math.Abs(v) > largestValue {
			largestValue = math.Abs(v)
			largest = p.Instrument.ContractID
		}
		sectors[p.Instrument.Sector] += math.Abs(v)
	}
	topSector := ""
	for s, v := range sectors {
		if v > sectors[topSector] {
			topSector = s
		}
	}
	shocks := []shock{
		{"market_down_10", "Broad market decline", "All current position values decline by 10%.", func(portfolio.Position) bool { return true }, -0.10},
		{"largest_down_20", "Largest position decline", "The largest current position declines by 20%.", func(p portfolio.Position) bool { return p.Instrument.ContractID == largest }, -0.20},
		{"top_sector_down_15", "Largest sector decline", fmt.Sprintf("Current positions in %s decline by 15%%.", labelOr(topSector, "the largest sector")), func(p portfolio.Position) bool { return p.Instrument.Sector == topSector }, -0.15},
		{"foreign_currency_down_5", "Foreign-currency decline", "Non-base-currency exposures decline by 5% from currency movement alone.", func(p portfolio.Position) bool {
			return !strings.EqualFold(p.Instrument.Currency, current.BaseCurrency)
		}, -0.05},
	}
	result := make([]Scenario, 0, len(shocks))
	for _, s := range shocks {
		impact := 0.0
		for _, p := range snapshot.Positions {
			if s.applies(p) {
				impact += values[p.Instrument.ContractID] * s.rate
			}
		}
		item := Scenario{ID: s.id, Name: s.name, Description: s.description, Impact: portfolio.Money{Amount: impact, Currency: current.BaseCurrency, Available: true}}
		if snapshot.AccountSummary.NetLiquidation.Available && snapshot.AccountSummary.NetLiquidation.Amount != 0 {
			item.ImpactRatio = Metric{Value: impact / snapshot.AccountSummary.NetLiquidation.Amount, Available: true}
		}
		result = append(result, item)
	}
	return result
}

func instrumentLabel(value portfolio.Instrument) string {
	if strings.TrimSpace(value.Symbol) != "" {
		return value.Symbol
	}
	return labelOr(value.Name, "A position")
}
func labelOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
