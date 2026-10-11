package ai

// This file assigns stable identifiers and explicit tolerances to counselor input facts.

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"wails/analytics"
	"wails/portfolio"
	"wails/portfolioanalysis"
	"wails/stockanalysis"
)

const (
	moneyAbsoluteTolerance      = 0.01
	percentAbsoluteTolerance    = 0.05
	ratioAbsoluteTolerance      = 0.0005
	quantityTolerance           = 0.000000001
	defaultRelativeTolerance    = 0.000001
	maxPortfolioResumePositions = 12
	maxPortfolioNewsPositions   = 12
)

var factIDCharacters = regexp.MustCompile(`[^a-z0-9]+`)

// portfolioResumeFacts maps the complete normalized portfolio context into stable validation facts.
func portfolioResumeFacts(snapshot portfolio.Portfolio, result analytics.Result) []CounselorFact {
	facts := []CounselorFact{
		textFact("portfolio.analysis_timestamp", CategorySuppliedFact, "Analysis timestamp", result.CalculatedAt.UTC().Format(time.RFC3339)),
		textFact("portfolio.base_currency", CategorySuppliedFact, "Account base currency", result.BaseCurrency),
		moneyFact(analytics.MetricNetMarketValue, "Net position market value", result.NetMarketValue),
		moneyFact(analytics.MetricGrossExposure, "Gross position exposure", result.GrossExposure),
		moneyFact(analytics.MetricTotalUnrealizedPnL, "Total unrealized profit or loss", result.TotalUnrealizedPnL),
		ratioFact(analytics.MetricTopThreeConcentration, "Top-three position concentration", result.TopThreeConcentration, "percent"),
		ratioFact(analytics.MetricHHI, "Herfindahl-Hirschman concentration index", result.HHI, "hhi_index"),
	}
	positions, indices, truncated := boundedPortfolioPositions(snapshot, result, maxPortfolioResumePositions)
	for selectedIndex, position := range positions {
		index := indices[selectedIndex]
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
	if truncated {
		facts = append(facts, CounselorFact{
			ID: "warning.portfolio_resume.position_coverage", Kind: string(CategoryUnavailableInformation),
			Label: "Position detail supplied to the counselor", Available: false,
			TextValue:          fmt.Sprintf("largest %d of %d open positions by absolute gross-exposure weight", len(positions), len(snapshot.Positions)),
			Limitation:         fmt.Sprintf("Detailed position facts were limited to the %d largest of %d open positions by absolute gross-exposure weight.", len(positions), len(snapshot.Positions)),
			RequiredLimitation: true,
		})
	}
	facts = append(facts, boundedAllocationFacts("allocation.sector", "Sector allocation", result.SectorAllocation, 12)...)
	facts = append(facts, boundedAllocationFacts("allocation.asset_class", "Asset-class allocation", result.AssetClassAllocation, 12)...)
	facts = append(facts, boundedAllocationFacts("allocation.currency", "Currency allocation", result.CurrencyAllocation, 12)...)
	facts = append(facts, focusedEffectiveExposureFacts(result, 12)...)
	facts = append(facts, fundLookThroughFacts(result.FundLookThrough, 12)...)
	for _, warning := range result.Warnings {
		facts = append(facts, CounselorFact{
			ID: "warning." + stableFactSegment(warning.Code), Kind: string(CategoryUnavailableInformation),
			Label: "Data-quality warning", TextValue: warning.Message + warningCountSuffix(warning.Count),
			Available: false, Limitation: warning.Message, RequiredLimitation: true,
		})
	}
	return facts
}

func boundedPortfolioPositions(snapshot portfolio.Portfolio, result analytics.Result, limit int) ([]portfolio.Position, []int, bool) {
	indices := make([]int, len(snapshot.Positions))
	for index := range indices {
		indices[index] = index
	}
	sort.SliceStable(indices, func(left, right int) bool {
		leftWeight := 0.0
		rightWeight := 0.0
		if left < len(result.PositionWeights) && result.PositionWeights[left].Weight.Available {
			leftWeight = math.Abs(result.PositionWeights[left].Weight.Value)
		}
		if right < len(result.PositionWeights) && result.PositionWeights[right].Weight.Available {
			rightWeight = math.Abs(result.PositionWeights[right].Weight.Value)
		}
		return leftWeight > rightWeight
	})
	truncated := limit > 0 && len(indices) > limit
	if truncated {
		indices = indices[:limit]
	}
	positions := make([]portfolio.Position, 0, len(indices))
	for _, index := range indices {
		positions = append(positions, snapshot.Positions[index])
	}
	return positions, indices, truncated
}

// portfolioRiskFacts exposes calculated risk, contribution, correlation, and scenario values verbatim.
func portfolioRiskFacts(result portfolioanalysis.Result) []CounselorFact {
	const maxCorrelations = 12
	const maxPositions = 12

	facts := []CounselorFact{
		textFact("risk.analysis_timestamp", CategorySuppliedFact, "Risk calculation timestamp", result.CalculatedAt.UTC().Format(time.RFC3339)),
		textFact("risk.base_currency", CategorySuppliedFact, "Account base currency", result.BaseCurrency),
		numberFact("risk.history_observations", CategoryDeterministicDerivedFact, "Common daily return observations", float64(result.Observations), "count", 0, 0),
		numberFact("risk.included_positions", CategoryDeterministicDerivedFact, "Positions included in modeled risk", float64(result.IncludedPositionCount), "count", 0, 0),
		numberFact("risk.total_positions", CategorySuppliedFact, "Total current positions", float64(result.TotalPositionCount), "count", 0, 0),
		portfolioRiskMetricFact("risk.modeled_total_return", "Modeled cumulative return", result.ModeledTotalReturn, "percent"),
		portfolioRiskMetricFact("risk.annualized_volatility", "Annualized volatility", result.AnnualizedVolatility, "percent"),
		portfolioRiskMetricFact("risk.downside_deviation", "Annualized downside deviation", result.DownsideDeviation, "percent"),
		portfolioRiskMetricFact("risk.maximum_drawdown", "Maximum drawdown", result.MaximumDrawdown, "percent"),
		numberFact("risk.correlation_pairs_available", CategoryDeterministicDerivedFact, "Calculated correlation pairs", float64(len(result.Correlations)), "count", 0, 0),
	}
	if !result.PeriodStart.IsZero() && !result.PeriodEnd.IsZero() {
		facts = append(facts,
			textFact("risk.period_start", CategoryDeterministicDerivedFact, "Modeled period start", result.PeriodStart.UTC().Format(time.RFC3339)),
			textFact("risk.period_end", CategoryDeterministicDerivedFact, "Modeled period end", result.PeriodEnd.UTC().Format(time.RFC3339)),
		)
	}
	positionIndices := make([]int, len(result.Positions))
	for index := range positionIndices {
		positionIndices[index] = index
	}
	sort.SliceStable(positionIndices, func(left, right int) bool {
		return math.Abs(result.Positions[positionIndices[left]].SignedWeight) > math.Abs(result.Positions[positionIndices[right]].SignedWeight)
	})
	if len(positionIndices) > maxPositions {
		positionIndices = positionIndices[:maxPositions]
		facts = append(facts, CounselorFact{
			ID: "warning.portfolio_risk.position_coverage", Kind: string(CategoryUnavailableInformation),
			Label: "Modeled-risk position facts supplied to the counselor", Available: false,
			TextValue:          fmt.Sprintf("largest %d of %d included modeled-risk positions by absolute signed weight", maxPositions, len(result.Positions)),
			Limitation:         fmt.Sprintf("The explanation received only the %d largest of %d included modeled-risk positions by absolute signed weight.", maxPositions, len(result.Positions)),
			RequiredLimitation: true,
		})
	}
	for _, index := range positionIndices {
		position := result.Positions[index]
		prefix := positionFactPrefix(portfolio.Instrument{ContractID: position.ContractID, Symbol: position.Symbol}, index)
		facts = append(facts,
			textFact(prefix+".symbol", CategorySuppliedFact, "Position ticker", position.Symbol),
			textFact(prefix+".name", CategorySuppliedFact, "Position name", position.Name),
			numberFact(prefix+".signed_weight", CategoryDeterministicDerivedFact, "Current signed gross-exposure weight", position.SignedWeight*100, "percent", percentAbsoluteTolerance, 0),
			portfolioRiskMetricFact(prefix+".modeled_total_return", "Position cumulative return", position.TotalReturn, "percent"),
			portfolioRiskMetricFact(prefix+".modeled_annualized_volatility", "Position annualized volatility", position.AnnualizedVolatility, "percent"),
			portfolioRiskMetricFact(prefix+".return_contribution", "Arithmetic modeled return contribution", position.ReturnContribution, "percentage_points"),
			portfolioRiskMetricFact(prefix+".risk_contribution", "Variance-based portfolio risk contribution", position.RiskContribution, "percent"),
		)
	}
	correlations := result.Correlations
	if len(correlations) > maxCorrelations {
		correlations = correlations[:maxCorrelations]
		facts = append(facts, CounselorFact{
			ID: "warning.portfolio_risk.correlation_coverage", Kind: string(CategoryUnavailableInformation),
			Label: "Correlation coverage", TextValue: "The counselor received only the 12 strongest absolute correlations.", Available: false,
			Limitation: "The counselor explanation covers only the 12 strongest absolute correlations.", RequiredLimitation: true,
		})
	}
	for _, correlation := range correlations {
		id := fmt.Sprintf("correlation.contract_%d.contract_%d", correlation.LeftContractID, correlation.RightContractID)
		facts = append(facts, numberFact(id, CategoryDeterministicDerivedFact, correlation.LeftSymbol+" and "+correlation.RightSymbol+" return correlation", correlation.Value, "correlation", ratioAbsoluteTolerance, 0))
	}
	for _, scenario := range result.Scenarios {
		prefix := "scenario." + stableFactSegment(scenario.ID)
		facts = append(facts,
			textFact(prefix+".description", CategoryDeterministicDerivedFact, scenario.Name, scenario.Description),
			moneyFact(prefix+".impact", scenario.Name+" estimated monetary impact", scenario.Impact),
			portfolioRiskMetricFact(prefix+".impact_ratio", scenario.Name+" impact relative to net liquidation", scenario.ImpactRatio, "percent"),
		)
	}
	for index, warning := range result.Warnings {
		facts = append(facts, CounselorFact{
			ID: fmt.Sprintf("warning.portfolio_risk.%d", index+1), Kind: string(CategoryUnavailableInformation),
			Label: "Portfolio-risk limitation", TextValue: warning, Available: false,
			Limitation: warning, RequiredLimitation: true,
		})
	}
	return facts
}

func fundLookThroughFacts(results []portfolio.FundHoldings, limit int) []CounselorFact {
	facts := make([]CounselorFact, 0)
	bounded := results
	if limit > 0 && len(bounded) > limit {
		bounded = bounded[:limit]
		facts = append(facts, CounselorFact{
			ID: "warning.fund_look_through.coverage", Kind: string(CategoryUnavailableInformation),
			Label: "Fund look-through sources supplied to the counselor", Available: false,
			TextValue:          fmt.Sprintf("first %d of %d fund look-through results", len(bounded), len(results)),
			Limitation:         fmt.Sprintf("The explanation received only %d of %d fund look-through source records.", len(bounded), len(results)),
			RequiredLimitation: true,
		})
	}
	for index, result := range bounded {
		prefix := positionFactPrefix(result.Fund, index) + ".look_through"
		if strings.TrimSpace(result.SourceType) != "" {
			facts = append(facts, textFact(prefix+".source_type", CategorySuppliedFact, "Fund holdings source type", result.SourceType))
		}
		if strings.TrimSpace(result.SourceURL) != "" {
			facts = append(facts, textFact(prefix+".source_url", CategorySuppliedFact, "Fund holdings source URL", result.SourceURL))
		}
		if strings.TrimSpace(result.Confidence) != "" {
			facts = append(facts, textFact(prefix+".confidence", CategorySuppliedFact, "Fund holdings confidence", result.Confidence))
		}
		if !result.RetrievedAt.IsZero() {
			facts = append(facts, textFact(prefix+".retrieved_at", CategorySuppliedFact, "Fund holdings retrieval time", result.RetrievedAt.UTC().Format(time.RFC3339)))
		}
		if !result.EffectiveDate.IsZero() {
			facts = append(facts, textFact(prefix+".effective_date", CategorySuppliedFact, "Fund holdings effective date", result.EffectiveDate.UTC().Format("2006-01-02")))
		}
		if result.CoveragePercentage.Available {
			facts = append(facts, numberFact(prefix+".coverage", CategorySuppliedFact, "Fund holdings coverage", result.CoveragePercentage.Value, "percent", percentAbsoluteTolerance, 0))
		}
		for warningIndex, warning := range result.Warnings {
			facts = append(facts, CounselorFact{
				ID: fmt.Sprintf("%s.warning.%d", prefix, warningIndex+1), Kind: string(CategoryUnavailableInformation),
				Label: "Fund holdings warning", TextValue: warning, Available: false, Limitation: warning, RequiredLimitation: true,
			})
		}
	}
	return facts
}

func portfolioNewsFacts(snapshot portfolio.Portfolio, researchTime time.Time) []CounselorFact {
	facts := []CounselorFact{textFact("research.timestamp", CategorySuppliedFact, "Research timestamp", researchTime.UTC().Format(time.RFC3339))}
	for index, position := range snapshot.Positions {
		prefix := positionFactPrefix(position.Instrument, index)
		symbol := textFact(prefix+".symbol", CategorySuppliedFact, "Position ticker", position.Instrument.Symbol)
		name := textFact(prefix+".name", CategorySuppliedFact, "Position name", position.Instrument.Name)
		if symbol.Available {
			symbol.RequiredObservation = true
			symbol.ObservationOrder = index + 1
		} else {
			name.RequiredObservation = true
			name.ObservationOrder = index + 1
		}
		facts = append(facts,
			symbol,
			name,
			textFact(prefix+".asset_class", CategorySuppliedFact, "Position asset class", position.Instrument.AssetClass),
		)
	}
	return facts
}

func stockNewsFacts(instrument portfolio.Instrument, researchTime time.Time) []CounselorFact {
	prefix := instrumentFactPrefix(instrument)
	symbol := textFact(prefix+".symbol", CategorySuppliedFact, "Stock ticker", instrument.Symbol)
	name := textFact(prefix+".name", CategorySuppliedFact, "Stock name", instrument.Name)
	if symbol.Available {
		symbol.RequiredObservation = true
	} else {
		name.RequiredObservation = true
	}
	return []CounselorFact{
		textFact("research.timestamp", CategorySuppliedFact, "Research timestamp", researchTime.UTC().Format(time.RFC3339)),
		symbol,
		name,
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
		moneyFactWithKind("order.estimated_price", CategorySuppliedFact, "Current executable quote used for the estimate", result.EstimatedPrice),
		moneyFact("order.estimated_value", "Estimated order value excluding commission", result.EstimatedOrderValue),
		moneyFact("order.base_value", "Estimated order value in account base currency", result.BaseOrderValue),
		moneyFactWithKind("portfolio.cash_balance", CategorySuppliedFact, "Broker-reported available cash", result.CashBalance),
		stockMetricFact("order.cash_usage", "Estimated cash usage", result.CashUsage, "percent"),
		numberFact("position.current_quantity", CategorySuppliedFact, "Current position quantity", result.CurrentQuantity, "shares", quantityTolerance, 0),
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

func boundedAllocationFacts(prefix, label string, allocations []analytics.Allocation, limit int) []CounselorFact {
	bounded := append([]analytics.Allocation(nil), allocations...)
	sort.SliceStable(bounded, func(left, right int) bool {
		if bounded[left].Weight.Available != bounded[right].Weight.Available {
			return bounded[left].Weight.Available
		}
		return math.Abs(bounded[left].Weight.Value) > math.Abs(bounded[right].Weight.Value)
	})
	truncated := limit > 0 && len(bounded) > limit
	if truncated {
		bounded = bounded[:limit]
	}
	facts := allocationFacts(prefix, label, bounded)
	if truncated {
		facts = append(facts, CounselorFact{
			ID: "warning." + stableFactSegment(prefix) + ".coverage", Kind: string(CategoryUnavailableInformation),
			Label: label + " facts supplied to the counselor", Available: false,
			TextValue:          fmt.Sprintf("largest %d of %d allocation entries", len(bounded), len(allocations)),
			Limitation:         fmt.Sprintf("The explanation received only the %d largest of %d %s entries.", len(bounded), len(allocations), strings.ToLower(label)),
			RequiredLimitation: true,
		})
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

// PromptNumericValue renders a fact without scientific notation and with at most two decimal places.
func (fact CounselorFact) PromptNumericValue() string {
	if fact.NumericValue == nil {
		return ""
	}
	value := *fact.NumericValue
	if math.Abs(value) < 0.005 {
		value = 0
	}
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', 2, 64), "0"), ".")
}

func moneyFact(id, label string, value portfolio.Money) CounselorFact {
	return moneyFactWithKind(id, CategoryDeterministicDerivedFact, label, value)
}

func moneyFactWithKind(id string, kind ObservationCategory, label string, value portfolio.Money) CounselorFact {
	if !value.Available || strings.TrimSpace(value.Currency) == "" {
		return unavailableFact(id, label, strings.TrimSpace(value.Currency), label+" is unavailable.")
	}
	return numberFact(id, kind, label, value.Amount, value.Currency, moneyAbsoluteTolerance, defaultRelativeTolerance)
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

func portfolioRiskMetricFact(id, label string, value portfolioanalysis.Metric, unit string) CounselorFact {
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
