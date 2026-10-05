package ai

// This file assigns stable facts to the focused currency, diversification, history, position, and quality workflows.

import (
	"fmt"
	"strings"
	"time"

	"wails/analytics"
	"wails/portfolio"
	"wails/portfoliohistory"
	"wails/positionanalysis"
)

func currencyExposureFacts(_ portfolio.Portfolio, result analytics.Result) []CounselorFact {
	facts := []CounselorFact{
		textFact("currency.analysis_timestamp", CategorySuppliedFact, "Analysis timestamp", result.CalculatedAt.UTC().Format(time.RFC3339)),
		textFact("currency.base_currency", CategoryDeterministicDerivedFact, "Account base currency", result.BaseCurrency),
		textFact("currency.portfolio_source", CategorySuppliedFact, "Portfolio source", "IBKR Client Portal Gateway"),
	}
	facts = append(facts, allocationFacts("currency.allocation", "Gross exposure by instrument currency", result.CurrencyAllocation)...)
	for _, rate := range result.ExchangeRates {
		prefix := "currency.rate." + stableFactSegment(rate.Currency)
		facts = append(facts,
			textFact(prefix+".base_currency", CategorySuppliedFact, "Conversion base currency", rate.BaseCurrency),
			textFact(prefix+".source", CategorySuppliedFact, "Conversion-rate source", rate.Source),
		)
		if rate.Available {
			facts = append(facts, numberFact(prefix+".base_amount_per_unit", CategorySuppliedFact, "Base-currency amount per unit", rate.BaseAmountPerUnit, rate.BaseCurrency+"_per_"+rate.Currency, ratioAbsoluteTolerance, defaultRelativeTolerance))
		} else {
			facts = append(facts, unavailableFact(prefix+".base_amount_per_unit", "Currency conversion rate", rate.BaseCurrency+"_per_"+rate.Currency, "The broker did not supply a usable conversion rate."))
		}
		if !rate.AsOf.IsZero() {
			facts = append(facts, textFact(prefix+".as_of", CategorySuppliedFact, "Conversion-rate timestamp", rate.AsOf.UTC().Format(time.RFC3339)))
		}
	}
	for _, warning := range result.Warnings {
		switch warning.Code {
		case "missing_base_currency", "missing_currency", "missing_market_value", "missing_base_market_value":
			facts = appendAnalyticsWarnings(facts, "currency", []analytics.DataWarning{warning})
		}
	}
	return facts
}

func diversificationFacts(snapshot portfolio.Portfolio, result analytics.Result) []CounselorFact {
	facts := []CounselorFact{
		textFact("diversification.analysis_timestamp", CategorySuppliedFact, "Analysis timestamp", result.CalculatedAt.UTC().Format(time.RFC3339)),
		textFact("diversification.portfolio_source", CategorySuppliedFact, "Portfolio source", "IBKR Client Portal Gateway"),
		numberFact("diversification.position_count", CategorySuppliedFact, "Open position count", float64(len(snapshot.Positions)), "count", 0, 0),
		ratioFact(analytics.MetricTopThreeConcentration, "Top-three direct-position concentration", result.TopThreeConcentration, "percent"),
		ratioFact(analytics.MetricHHI, "Direct-position Herfindahl-Hirschman index", result.HHI, "hhi_index"),
	}
	facts = append(facts, allocationFacts("diversification.direct_sector", "Direct sector exposure", result.SectorAllocation)...)
	facts = append(facts, focusedEffectiveExposureFacts(result, 12)...)
	facts = append(facts, fundLookThroughFacts(result.FundLookThrough)...)
	return appendAnalyticsWarnings(facts, "diversification", result.Warnings)
}

func dataQualityFacts(snapshot portfolio.Portfolio, result analytics.Result) []CounselorFact {
	facts := []CounselorFact{
		textFact("quality.analysis_timestamp", CategorySuppliedFact, "Audit timestamp", result.CalculatedAt.UTC().Format(time.RFC3339)),
		textFact("quality.portfolio_source", CategorySuppliedFact, "Portfolio source", "IBKR Client Portal Gateway"),
		numberFact("quality.position_count", CategorySuppliedFact, "Open position count", float64(len(snapshot.Positions)), "count", 0, 0),
		textFact("quality.base_currency", CategoryDeterministicDerivedFact, "Account base currency", result.BaseCurrency),
		availabilityFact("quality.net_market_value", "Net market value availability", result.NetMarketValue.Available),
		availabilityFact("quality.gross_exposure", "Gross exposure availability", result.GrossExposure.Available),
		availabilityFact("quality.unrealized_pnl", "Unrealized profit or loss availability", result.TotalUnrealizedPnL.Available),
		availabilityFact("quality.effective_exposure", "Effective look-through exposure completeness", result.EffectiveExposureComplete),
		numberFact("quality.warning_count", CategoryDeterministicDerivedFact, "Distinct data-warning count", float64(len(result.Warnings)), "count", 0, 0),
	}
	for index, fund := range result.FundLookThrough {
		prefix := fmt.Sprintf("quality.fund.%d", index+1)
		facts = append(facts,
			textFact(prefix+".symbol", CategorySuppliedFact, "Fund ticker", fund.Fund.Symbol),
			availabilityFact(prefix+".available", "Structured holdings availability", fund.Available),
			availabilityFact(prefix+".complete", "Structured holdings completeness", fund.Complete),
		)
		if strings.TrimSpace(fund.SourceType) != "" {
			facts = append(facts, textFact(prefix+".source_type", CategorySuppliedFact, "Fund holdings source type", fund.SourceType))
		}
		if strings.TrimSpace(fund.SourceURL) != "" {
			facts = append(facts, textFact(prefix+".source_url", CategorySuppliedFact, "Fund holdings source URL", fund.SourceURL))
		}
		if strings.TrimSpace(fund.Confidence) != "" {
			facts = append(facts, textFact(prefix+".confidence", CategorySuppliedFact, "Fund holdings confidence", fund.Confidence))
		}
		if !fund.RetrievedAt.IsZero() {
			facts = append(facts, textFact(prefix+".retrieved_at", CategorySuppliedFact, "Fund holdings retrieval time", fund.RetrievedAt.UTC().Format(time.RFC3339)))
		}
		if !fund.EffectiveDate.IsZero() {
			facts = append(facts, textFact(prefix+".effective_date", CategorySuppliedFact, "Fund holdings effective date", fund.EffectiveDate.UTC().Format("2006-01-02")))
		}
		if fund.CoveragePercentage.Available {
			facts = append(facts, numberFact(prefix+".coverage", CategorySuppliedFact, "Fund holdings coverage", fund.CoveragePercentage.Value, "percent", percentAbsoluteTolerance, 0))
		}
		for warningIndex, warning := range fund.Warnings {
			facts = append(facts, CounselorFact{ID: fmt.Sprintf("%s.warning.%d", prefix, warningIndex+1), Kind: string(CategoryUnavailableInformation), Label: "Fund holdings warning", TextValue: warning, Available: false, Limitation: warning, RequiredLimitation: true})
		}
	}
	return appendAnalyticsWarnings(facts, "quality", result.Warnings)
}

func focusedEffectiveExposureFacts(result analytics.Result, limit int) []CounselorFact {
	facts := make([]CounselorFact, 0, limit+len(result.EffectiveSectorAllocation)+3)
	total := len(result.EffectiveSecurityExposure)
	facts = append(facts, numberFact("effective.security_count_total", CategoryDeterministicDerivedFact, "Calculated effective securities", float64(total), "count", 0, 0))
	securityExposures := result.EffectiveSecurityExposure
	if limit > 0 && len(securityExposures) > limit {
		securityExposures = securityExposures[:limit]
		facts = append(facts, CounselorFact{
			ID: "effective.security_coverage", Kind: string(CategoryUnavailableInformation), Label: "Effective security facts supplied to the counselor",
			TextValue: fmt.Sprintf("largest %d of %d calculated effective security exposures", limit, total), Available: false,
			Limitation: fmt.Sprintf("The explanation received only the %d largest of %d calculated effective security exposures.", limit, total), RequiredLimitation: true,
		})
	}
	facts = append(facts, numberFact("effective.security_count_supplied", CategoryDeterministicDerivedFact, "Effective securities supplied to the counselor", float64(len(securityExposures)), "count", 0, 0))
	for _, exposure := range securityExposures {
		facts = append(facts, ratioFact(exposure.MetricID, "Effective security exposure for "+valueOrUnavailable(exposure.Name), exposure.Weight, "percent"))
	}
	facts = append(facts, allocationFacts(strings.TrimSuffix(analytics.MetricEffectiveSectorPrefix, "."), "Effective sector exposure", result.EffectiveSectorAllocation)...)
	if result.EffectiveExposureComplete {
		facts = append(facts, textFact("effective.data_quality", CategoryDeterministicDerivedFact, "Effective exposure data quality", "complete"))
	} else {
		facts = append(facts, CounselorFact{ID: "effective.data_quality", Kind: string(CategoryUnavailableInformation), Label: "Effective exposure data quality", TextValue: "incomplete", Available: false, Limitation: "Effective look-through exposure is incomplete.", RequiredLimitation: true})
	}
	return facts
}

func portfolioHistoryFacts(summary portfoliohistory.Summary) []CounselorFact {
	facts := []CounselorFact{
		textFact("history.range_start", CategorySuppliedFact, "Requested history start", summary.From.UTC().Format(time.RFC3339)),
		textFact("history.range_end", CategorySuppliedFact, "Requested history end", summary.To.UTC().Format(time.RFC3339)),
	}
	if !summary.TrackingStartedAt.IsZero() {
		facts = append(facts, textFact("history.tracking_started_at", CategorySuppliedFact, "First local tracking timestamp", summary.TrackingStartedAt.UTC().Format(time.RFC3339)))
	} else {
		facts = append(facts, unavailableFact("history.tracking_started_at", "First local tracking timestamp", "timestamp", "No local tracking start is available."))
	}
	for index, series := range summary.Series {
		prefix := fmt.Sprintf("history.series.%d", index+1)
		facts = append(facts,
			textFact(prefix+".product", CategorySuppliedFact, "History product", series.Product),
			textFact(prefix+".currency", CategorySuppliedFact, "History base currency", series.BaseCurrency),
			numberFact(prefix+".observations", CategoryDeterministicDerivedFact, "Available observations", float64(series.ObservationCount), "count", 0, 0),
			textFact(prefix+".sources", CategorySuppliedFact, "History sources", strings.Join(series.Sources, ", ")),
		)
		if series.ObservationCount > 0 {
			facts = append(facts,
				textFact(prefix+".first_at", CategorySuppliedFact, "First observation timestamp", series.FirstAt.UTC().Format(time.RFC3339)),
				textFact(prefix+".last_at", CategorySuppliedFact, "Last observation timestamp", series.LastAt.UTC().Format(time.RFC3339)),
				historyMetricFact(prefix+".first_value", "First observed account value", series.FirstValue),
				historyMetricFact(prefix+".last_value", "Last observed account value", series.LastValue),
				historyMetricFact(prefix+".absolute_change", "Observed absolute account-value change", series.AbsoluteChange),
				historyMetricFact(prefix+".minimum", "Minimum observed account value", series.Minimum),
				historyMetricFact(prefix+".maximum", "Maximum observed account value", series.Maximum),
			)
		} else {
			facts = append(facts, unavailableFact(prefix+".values", "History observations", series.BaseCurrency, "No available numeric observations were present in this history series."))
		}
	}
	for index, warning := range summary.Warnings {
		facts = append(facts, CounselorFact{ID: fmt.Sprintf("warning.portfolio_history.%d", index+1), Kind: string(CategoryUnavailableInformation), Label: "Portfolio-history limitation", TextValue: warning, Available: false, Limitation: warning, RequiredLimitation: true})
	}
	return facts
}

func positionRiskFacts(result positionanalysis.Result) []CounselorFact {
	prefix := instrumentFactPrefix(result.Instrument)
	facts := []CounselorFact{
		textFact("position_risk.analysis_timestamp", CategorySuppliedFact, "Analysis timestamp", result.CalculatedAt.UTC().Format(time.RFC3339)),
		textFact("position_risk.source", CategorySuppliedFact, "Market and position source", "IBKR Client Portal Gateway"),
		textFact(prefix+".symbol", CategorySuppliedFact, "Stock ticker", result.Instrument.Symbol),
		textFact(prefix+".name", CategorySuppliedFact, "Stock name", result.Instrument.Name),
		numberFact("position_risk.quantity", CategorySuppliedFact, "Current position quantity", result.Quantity, "shares", quantityTolerance, 0),
		moneyFact("position_risk.average_cost", "Average cost", result.AverageCost),
		moneyFact("position_risk.market_price", "Current position market price", result.MarketPrice),
		moneyFact("position_risk.market_value", "Current position market value", result.MarketValue),
		moneyFact("position_risk.base_market_value", "Current position base-currency market value", result.BaseMarketValue),
		moneyFact("position_risk.unrealized_pnl", "Current unrealized profit or loss", result.UnrealizedPnL),
		moneyFact("position_risk.base_unrealized_pnl", "Current base-currency unrealized profit or loss", result.BaseUnrealizedPnL),
		positionMetricFact("position_risk.portfolio_weight", "Current gross-exposure portfolio weight", result.PortfolioWeight, "percent"),
		numberFact("position_risk.history_observations", CategoryDeterministicDerivedFact, "Closing-price observations", float64(result.HistoryObservations), "count", 0, 0),
		positionMetricFact("position_risk.total_return", "Historical total return", result.TotalReturn, "percent"),
		positionMetricFact("position_risk.annualized_volatility", "Annualized daily-return volatility", result.AnnualizedVolatility, "percent"),
		positionMetricFact("position_risk.maximum_drawdown", "Historical maximum drawdown", result.MaximumDrawdown, "percent"),
		positionMetricFact("position_risk.position_52_week_range", "Position in 52-week range", result.PositionIn52WeekRange, "percent"),
		positionMetricFact("position_risk.distance_sma_50", "Distance from 50-observation moving average", result.DistanceFromSMA50, "percent"),
		positionMetricFact("position_risk.distance_sma_200", "Distance from 200-observation moving average", result.DistanceFromSMA200, "percent"),
		textFact("position_risk.market_data_status", CategorySuppliedFact, "IBKR market-data status", result.MarketDataStatus),
	}
	if !result.QuoteAsOf.IsZero() {
		facts = append(facts, textFact("position_risk.quote_as_of", CategorySuppliedFact, "Quote timestamp", result.QuoteAsOf.UTC().Format(time.RFC3339)))
	}
	if !result.HistoryStart.IsZero() && !result.HistoryEnd.IsZero() {
		facts = append(facts, textFact("position_risk.history_start", CategoryDeterministicDerivedFact, "History start", result.HistoryStart.UTC().Format(time.RFC3339)), textFact("position_risk.history_end", CategoryDeterministicDerivedFact, "History end", result.HistoryEnd.UTC().Format(time.RFC3339)))
	}
	for index, warning := range result.Warnings {
		facts = append(facts, CounselorFact{ID: fmt.Sprintf("warning.position_risk.%d", index+1), Kind: string(CategoryUnavailableInformation), Label: "Position-risk limitation", TextValue: warning, Available: false, Limitation: warning, RequiredLimitation: true})
	}
	return facts
}

func appendAnalyticsWarnings(facts []CounselorFact, namespace string, warnings []analytics.DataWarning) []CounselorFact {
	for index, warning := range warnings {
		message := warning.Message + warningCountSuffix(warning.Count)
		facts = append(facts, CounselorFact{ID: fmt.Sprintf("warning.%s.%d", namespace, index+1), Kind: string(CategoryUnavailableInformation), Label: "Data-quality warning", TextValue: message, Available: false, Limitation: message, RequiredLimitation: true})
	}
	return facts
}

func availabilityFact(id, label string, available bool) CounselorFact {
	if !available {
		return unavailableFact(id, label, "availability", label+" is incomplete or unavailable.")
	}
	return textFact(id, CategoryDeterministicDerivedFact, label, "available")
}

func historyMetricFact(id, label string, metric portfoliohistory.Metric) CounselorFact {
	if !metric.Available || strings.TrimSpace(metric.Currency) == "" {
		return unavailableFact(id, label, metric.Currency, label+" is unavailable.")
	}
	return numberFact(id, CategoryDeterministicDerivedFact, label, metric.Value, metric.Currency, moneyAbsoluteTolerance, defaultRelativeTolerance)
}

func positionMetricFact(id, label string, metric positionanalysis.Metric, unit string) CounselorFact {
	if !metric.Available {
		return unavailableFact(id, label, unit, label+" is unavailable.")
	}
	value := metric.Value
	if unit == "percent" || unit == "percentage_points" {
		value *= 100
	}
	return numberFact(id, CategoryDeterministicDerivedFact, label, value, unit, percentAbsoluteTolerance, 0)
}
