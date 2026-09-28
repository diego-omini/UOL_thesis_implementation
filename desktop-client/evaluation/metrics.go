package evaluation

// This file creates stable metric identifiers and condition-specific portfolio contexts.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"wails/analytics"
	"wails/portfolio"
)

var metricLabelPattern = regexp.MustCompile(`[^a-z0-9]+`)

type rawContext struct {
	SnapshotAt    string                   `json:"snapshot_at"`
	BaseCurrency  string                   `json:"base_currency"`
	CashBalance   portfolio.Money          `json:"cash_balance"`
	Positions     []portfolio.Position     `json:"positions"`
	ExchangeRates []portfolio.ExchangeRate `json:"exchange_rates"`
}

type analyticsContext struct {
	Raw      rawContext              `json:"portfolio"`
	Metrics  []MetricRecord          `json:"deterministic_metrics"`
	Warnings []analytics.DataWarning `json:"data_warnings"`
}

type baselineContext struct {
	Raw rawContext `json:"portfolio"`
}

func buildRawContext(scenario FrozenScenario, result analytics.Result) rawContext {
	return rawContext{
		SnapshotAt:    scenario.SnapshotAt.UTC().Format("2006-01-02T15:04:05Z"),
		BaseCurrency:  result.BaseCurrency,
		CashBalance:   scenario.Portfolio.AccountSummary.CashBalance,
		Positions:     scenario.Portfolio.Positions,
		ExchangeRates: scenario.Portfolio.ExchangeRates,
	}
}

func conditionContext(scenario FrozenScenario, condition Condition) (string, []MetricRecord, error) {
	result := analytics.Calculate(scenario.Portfolio, scenario.SnapshotAt)
	metrics := buildMetrics(scenario.Portfolio, result)
	raw := buildRawContext(scenario, result)
	var value any = baselineContext{Raw: raw}
	if condition.Context == "analytics" {
		value = analyticsContext{Raw: raw, Metrics: metrics, Warnings: result.Warnings}
	}
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("encode %s context: %w", condition.ID, err)
	}
	return string(contents), metrics, nil
}

func buildMetrics(snapshot portfolio.Portfolio, result analytics.Result) []MetricRecord {
	metrics := []MetricRecord{
		moneyMetric(analytics.MetricNetMarketValue, result.NetMarketValue),
		moneyMetric(analytics.MetricGrossExposure, result.GrossExposure),
		moneyMetric(analytics.MetricTotalUnrealizedPnL, result.TotalUnrealizedPnL),
		ratioMetric(analytics.MetricTopThreeConcentration, result.TopThreeConcentration),
		ratioMetric(analytics.MetricHHI, result.HHI),
	}
	for _, position := range result.PositionWeights {
		metrics = append(metrics, MetricRecord{
			ID:    metricID("position", fmt.Sprintf("%d", position.ContractID), "weight"),
			Value: position.Weight.Value, Unit: "ratio", Available: position.Weight.Available,
		})
	}
	metrics = append(metrics, allocationMetrics("currency", result.CurrencyAllocation)...)
	metrics = append(metrics, allocationMetrics("asset_class", result.AssetClassAllocation)...)
	metrics = append(metrics, allocationMetrics("sector", result.SectorAllocation)...)
	for _, warning := range result.Warnings {
		metrics = append(metrics, MetricRecord{
			ID: metricID("warning", warning.Code), Value: float64(warning.Count), Unit: "count", Available: true,
		})
	}
	sort.Slice(metrics, func(i, j int) bool { return metrics[i].ID < metrics[j].ID })
	return metrics
}

func moneyMetric(id string, value portfolio.Money) MetricRecord {
	return MetricRecord{ID: id, Value: value.Amount, Unit: "money", Currency: value.Currency, Available: value.Available}
}

func ratioMetric(id string, value analytics.Ratio) MetricRecord {
	return MetricRecord{ID: id, Value: value.Value, Unit: "ratio", Available: value.Available}
}

func allocationMetrics(kind string, allocations []analytics.Allocation) []MetricRecord {
	metrics := make([]MetricRecord, 0, len(allocations))
	for _, allocation := range allocations {
		metrics = append(metrics, MetricRecord{
			ID:    metricID("allocation", kind, allocation.Label),
			Value: allocation.Weight.Value, Unit: "ratio", Available: allocation.Weight.Available,
		})
	}
	return metrics
}

func metricID(parts ...string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.ToLower(strings.TrimSpace(part))
		value = strings.Trim(metricLabelPattern.ReplaceAllString(value, "_"), "_")
		normalized = append(normalized, value)
	}
	return strings.Join(normalized, ".")
}

func metricLookup(metrics []MetricRecord) map[string]MetricRecord {
	lookup := make(map[string]MetricRecord, len(metrics))
	for _, metric := range metrics {
		lookup[metric.ID] = metric
	}
	return lookup
}
