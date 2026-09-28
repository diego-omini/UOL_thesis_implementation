package ai

// This file assigns stable identifiers and explicit tolerances to counselor input facts.

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"wails/analytics"
	"wails/portfolio"
	"wails/stockanalysis"
)

const (
	moneyAbsoluteTolerance   = 0.01
	percentAbsoluteTolerance = 0.05
	ratioAbsoluteTolerance   = 0.0005
	quantityTolerance        = 0.000000001
	defaultRelativeTolerance = 0.000001
)

var factIDCharacters = regexp.MustCompile(`[^a-z0-9]+`)

// portfolioResumeFacts maps the complete normalized portfolio context into stable validation facts.
func portfolioResumeFacts(snapshot portfolio.Portfolio, result analytics.Result) []CounselorFact {
	facts := []CounselorFact{
		textFact("portfolio.analysis_timestamp", CategorySuppliedFact, "Analysis timestamp", result.CalculatedAt.UTC().Format(time.RFC3339)),
		textFact("portfolio.base_currency", CategoryDeterministicDerivedFact, "Account base currency", result.BaseCurrency),
		moneyFact(analytics.MetricNetMarketValue, "Net position market value", result.NetMarketValue),
		moneyFact(analytics.MetricGrossExposure, "Gross position exposure", result.GrossExposure),
		moneyFact(analytics.MetricTotalUnrealizedPnL, "Total unrealized profit or loss", result.TotalUnrealizedPnL),
		ratioFact(analytics.MetricTopThreeConcentration, "Top-three position concentration", result.TopThreeConcentration, "percent"),
		ratioFact(analytics.MetricHHI, "Herfindahl-Hirschman concentration index", result.HHI, "hhi_index"),
	}
	for index, position := range snapshot.Positions {
		prefix := positionFactPrefix(position.Instrument, index)
		facts = append(facts,
			textFact(prefix+".symbol", CategorySuppliedFact, "Position ticker", position.Instrument.Symbol),
			textFact(prefix+".name", CategorySuppliedFact, "Position name", position.Instrument.Name),
			textFact(prefix+".asset_class", CategorySuppliedFact, "Position asset class", position.Instrument.AssetClass),
			textFact(prefix+".sector", CategorySuppliedFact, "Position sector", position.Instrument.Sector),
			textFact(prefix+".currency", CategorySuppliedFact, "Position currency", position.Instrument.Currency),
			numberFact(prefix+".quantity", CategorySuppliedFact, "Position quantity", position.Quantity, "shares", quantityTolerance, 0),
		)
		if index < len(result.PositionWeights) {
			facts = append(facts, ratioFact(prefix+".weight", "Position gross-exposure weight", result.PositionWeights[index].Weight, "percent"))
		} else {
			facts = append(facts, unavailableFact(prefix+".weight", "Position gross-exposure weight", "percent", "The deterministic position weight was not supplied."))
		}
	}
	facts = append(facts, allocationFacts("allocation.sector", "Sector allocation", result.SectorAllocation)...)
	facts = append(facts, allocationFacts("allocation.asset_class", "Asset-class allocation", result.AssetClassAllocation)...)
	facts = append(facts, allocationFacts("allocation.currency", "Currency allocation", result.CurrencyAllocation)...)
	for _, warning := range result.Warnings {
		facts = append(facts, CounselorFact{
			ID: "warning." + stableFactSegment(warning.Code), Kind: string(CategoryUnavailableInformation),
			Label: "Data-quality warning", TextValue: warning.Message + warningCountSuffix(warning.Count),
			Available: false, Limitation: warning.Message, RequiredLimitation: true,
		})
	}
	return facts
}

func portfolioNewsFacts(snapshot portfolio.Portfolio, researchTime time.Time) []CounselorFact {
	facts := []CounselorFact{textFact("research.timestamp", CategorySuppliedFact, "Research timestamp", researchTime.UTC().Format(time.RFC3339))}
	for index, position := range snapshot.Positions {
		prefix := positionFactPrefix(position.Instrument, index)
		facts = append(facts,
			textFact(prefix+".symbol", CategorySuppliedFact, "Position ticker", position.Instrument.Symbol),
			textFact(prefix+".name", CategorySuppliedFact, "Position name", position.Instrument.Name),
			textFact(prefix+".asset_class", CategorySuppliedFact, "Position asset class", position.Instrument.AssetClass),
		)
	}
	return facts
}

func stockNewsFacts(instrument portfolio.Instrument, researchTime time.Time) []CounselorFact {
	prefix := instrumentFactPrefix(instrument)
	return []CounselorFact{
		textFact("research.timestamp", CategorySuppliedFact, "Research timestamp", researchTime.UTC().Format(time.RFC3339)),
		textFact(prefix+".symbol", CategorySuppliedFact, "Stock ticker", instrument.Symbol),
		textFact(prefix+".name", CategorySuppliedFact, "Stock name", instrument.Name),
		textFact(prefix+".asset_class", CategorySuppliedFact, "Stock asset class", instrument.AssetClass),
	}
}

// orderAnalysisFacts maps proposed-order and historical metrics without recalculating model-facing values.
func orderAnalysisFacts(result stockanalysis.Result) []CounselorFact {
	prefix := instrumentFactPrefix(result.Instrument)
	facts := []CounselorFact{
		textFact("analysis.timestamp", CategorySuppliedFact, "Analysis timestamp", result.CalculatedAt.UTC().Format(time.RFC3339)),
		textFact(prefix+".symbol", CategorySuppliedFact, "Stock ticker", result.Instrument.Symbol),
		textFact(prefix+".name", CategorySuppliedFact, "Stock name", result.Instrument.Name),
		textFact(prefix+".exchange", CategorySuppliedFact, "Stock exchange", result.Instrument.Exchange),
		textFact(prefix+".currency", CategorySuppliedFact, "Trading currency", result.Instrument.Currency),
		textFact("order.side", CategorySuppliedFact, "Proposed order side", result.Side),
		numberFact("order.quantity", CategorySuppliedFact, "Proposed order quantity", result.Quantity, "shares", quantityTolerance, 0),
		moneyFact("order.estimated_price", "Estimated executable price", result.EstimatedPrice),
		moneyFact("order.estimated_value", "Estimated order value excluding commission", result.EstimatedOrderValue),
		moneyFact("order.base_value", "Estimated order value in account base currency", result.BaseOrderValue),
		moneyFact("portfolio.cash_balance", "Available cash", result.CashBalance),
		stockMetricFact("order.cash_usage", "Estimated cash usage", result.CashUsage, "percent"),
		numberFact("position.current_quantity", CategoryDeterministicDerivedFact, "Current position quantity", result.CurrentQuantity, "shares", quantityTolerance, 0),
		numberFact("position.resulting_quantity", CategoryDeterministicDerivedFact, "Resulting position quantity", result.ResultingQuantity, "shares", quantityTolerance, 0),
		stockMetricFact("position.current_weight", "Current portfolio weight", result.CurrentWeight, "percent"),
		stockMetricFact("position.resulting_weight", "Resulting portfolio weight", result.ResultingWeight, "percent"),
		stockMetricFact("position.weight_change", "Portfolio-weight change", result.WeightChange, "percentage_points"),
		numberFact("market.history_observations", CategoryDeterministicDerivedFact, "Closing-price observations", float64(result.HistoryObservations), "count", 0, 0),
		stockMetricFact("market.total_return", "Historical total return", result.TotalReturn, "percent"),
		stockMetricFact("market.annualized_volatility", "Annualized daily-return volatility", result.AnnualizedVolatility, "percent"),
		stockMetricFact("market.maximum_drawdown", "Historical maximum drawdown", result.MaximumDrawdown, "percent"),
		stockMetricFact("market.position_52_week_range", "Position in 52-week range", result.PositionIn52WeekRange, "percent"),
		stockMetricFact("market.distance_sma_50", "Distance from 50-observation moving average", result.DistanceFromSMA50, "percent"),
		stockMetricFact("market.distance_sma_200", "Distance from 200-observation moving average", result.DistanceFromSMA200, "percent"),
		textFact("market.data_status", CategorySuppliedFact, "IBKR market-data status", result.MarketDataStatus),
	}
	if !result.HistoryStart.IsZero() && !result.HistoryEnd.IsZero() {
		facts = append(facts,
			textFact("market.history_start", CategoryDeterministicDerivedFact, "History start", result.HistoryStart.UTC().Format(time.RFC3339)),
			textFact("market.history_end", CategoryDeterministicDerivedFact, "History end", result.HistoryEnd.UTC().Format(time.RFC3339)),
		)
	} else {
		facts = append(facts, unavailableFact("market.history_period", "Historical period", "timestamp", "The historical period is unavailable."))
	}
	for index, warning := range result.Warnings {
		facts = append(facts, CounselorFact{
			ID: fmt.Sprintf("warning.order_analysis.%d", index+1), Kind: string(CategoryUnavailableInformation),
			Label: "Order-analysis data warning", TextValue: warning, Available: false,
			Limitation: warning, RequiredLimitation: true,
		})
	}
	return facts
}

func allocationFacts(prefix, label string, allocations []analytics.Allocation) []CounselorFact {
	facts := make([]CounselorFact, 0, len(allocations))
	for _, allocation := range allocations {
		id := prefix + "." + stableFactSegment(allocation.Label) + ".weight"
		facts = append(facts, ratioFact(id, label+" for "+allocation.Label, allocation.Weight, "percent"))
	}
	return facts
}

func textFact(id string, kind ObservationCategory, label, value string) CounselorFact {
	value = strings.TrimSpace(value)
	if value == "" {
		return unavailableFact(id, label, "text", label+" is unavailable.")
	}
	return CounselorFact{ID: id, Kind: string(kind), Label: label, TextValue: value, Available: true}
}

func numberFact(id string, kind ObservationCategory, label string, value float64, unit string, absoluteTolerance, relativeTolerance float64) CounselorFact {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return unavailableFact(id, label, unit, label+" is unavailable.")
	}
	copy := value
	return CounselorFact{
		ID: id, Kind: string(kind), Label: label, NumericValue: &copy, Unit: unit, Available: true,
		AbsoluteTolerance: absoluteTolerance, RelativeTolerance: relativeTolerance,
	}
}

func moneyFact(id, label string, value portfolio.Money) CounselorFact {
	if !value.Available || strings.TrimSpace(value.Currency) == "" {
		return unavailableFact(id, label, strings.TrimSpace(value.Currency), label+" is unavailable.")
	}
	return numberFact(id, CategoryDeterministicDerivedFact, label, value.Amount, value.Currency, moneyAbsoluteTolerance, defaultRelativeTolerance)
}

func ratioFact(id, label string, value analytics.Ratio, unit string) CounselorFact {
	if !value.Available {
		return unavailableFact(id, label, unit, label+" is unavailable.")
	}
	if unit == "percent" {
		return numberFact(id, CategoryDeterministicDerivedFact, label, value.Value*100, unit, percentAbsoluteTolerance, 0)
	}
	return numberFact(id, CategoryDeterministicDerivedFact, label, value.Value, unit, ratioAbsoluteTolerance, 0)
}

func stockMetricFact(id, label string, value stockanalysis.Metric, unit string) CounselorFact {
	if !value.Available {
		return unavailableFact(id, label, unit, label+" is unavailable.")
	}
	if unit == "percent" || unit == "percentage_points" {
		return numberFact(id, CategoryDeterministicDerivedFact, label, value.Value*100, unit, percentAbsoluteTolerance, 0)
	}
	return numberFact(id, CategoryDeterministicDerivedFact, label, value.Value, unit, ratioAbsoluteTolerance, 0)
}

func unavailableFact(id, label, unit, limitation string) CounselorFact {
	return CounselorFact{
		ID: id, Kind: string(CategoryUnavailableInformation), Label: label, Unit: unit,
		Available: false, Limitation: limitation, RequiredLimitation: true,
	}
}

func positionFactPrefix(instrument portfolio.Instrument, index int) string {
	if instrument.ContractID > 0 {
		return "position.contract_" + strconv.FormatInt(instrument.ContractID, 10)
	}
	segment := stableFactSegment(instrument.Symbol)
	if segment == "unavailable" {
		segment = "index_" + strconv.Itoa(index+1)
	}
	return "position." + segment
}

func instrumentFactPrefix(instrument portfolio.Instrument) string {
	if instrument.ContractID > 0 {
		return "instrument.contract_" + strconv.FormatInt(instrument.ContractID, 10)
	}
	return "instrument." + stableFactSegment(instrument.Symbol)
}

func stableFactSegment(value string) string {
	segment := strings.Trim(factIDCharacters.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "_"), "_")
	if segment == "" {
		return "unavailable"
	}
	return segment
}

func warningCountSuffix(count int) string {
	if count > 1 {
		return fmt.Sprintf(" (%d affected positions)", count)
	}
	return ""
}
