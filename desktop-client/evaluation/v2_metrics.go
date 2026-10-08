package evaluation

// This file builds the stable raw-fact and deterministic-metric contexts used by version 2.

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"wails/analytics"
	"wails/portfolio"
)

type v2Target struct {
	ID       string `json:"id"`
	Unit     string `json:"unit"`
	Currency string `json:"currency"`
}

type v2RawContext struct {
	Facts             []FactRecord `json:"facts"`
	EvaluationTargets []v2Target   `json:"evaluation_targets"`
}

type v2AnalyticsContext struct {
	Facts                []FactRecord            `json:"facts"`
	EvaluationTargets    []v2Target              `json:"evaluation_targets"`
	DeterministicMetrics []MetricRecord          `json:"deterministic_metrics"`
	DataWarnings         []analytics.DataWarning `json:"data_warnings"`
}

type v2Evidence struct {
	ID        string
	Value     float64
	Unit      string
	Currency  string
	Available bool
	Supplied  bool
	Metric    bool
}

func v2ConditionContext(scenario FrozenScenario, condition Condition) (string, map[string]v2Evidence, []MetricRecord, error) {
	result := analytics.Calculate(scenario.Portfolio, scenario.SnapshotAt)
	metrics := buildMetrics(scenario.Portfolio, result)
	facts := buildV2Facts(scenario, result)
	targets := make([]v2Target, 0, len(scenario.RequiredFacts))
	for _, fact := range scenario.RequiredFacts {
		targets = append(targets, v2Target{ID: fact.MetricID, Unit: fact.Unit, Currency: fact.Currency})
	}
	var contextValue any = v2RawContext{Facts: facts, EvaluationTargets: targets}
	if condition.Context == "analytics" {
		contextValue = v2AnalyticsContext{
			Facts: facts, EvaluationTargets: targets, DeterministicMetrics: metrics, DataWarnings: result.Warnings,
		}
	}
	encoded, err := json.MarshalIndent(contextValue, "", "  ")
	if err != nil {
		return "", nil, nil, fmt.Errorf("encode version 2 %s context: %w", condition.ID, err)
	}
	evidence := make(map[string]v2Evidence, len(facts)+len(metrics))
	for _, fact := range facts {
		entry := v2Evidence{ID: fact.ID, Unit: fact.Unit, Currency: fact.Currency, Available: fact.Available, Supplied: true}
		if fact.Value != nil {
			entry.Value = *fact.Value
		}
		evidence[fact.ID] = entry
	}
	for _, metric := range metrics {
		evidence[metric.ID] = v2Evidence{
			ID: metric.ID, Value: metric.Value, Unit: metric.Unit, Currency: metric.Currency,
			Available: metric.Available, Supplied: condition.Context == "analytics", Metric: true,
		}
	}
	return string(encoded), evidence, metrics, nil
}

func buildV2Facts(scenario FrozenScenario, result analytics.Result) []FactRecord {
	facts := []FactRecord{
		textFact("portfolio.snapshot_at", "timestamp", scenario.SnapshotAt.UTC().Format("2006-01-02T15:04:05Z"), true),
		textFact("portfolio.base_currency", "currency", result.BaseCurrency, result.BaseCurrency != ""),
		moneyFact("portfolio.net_liquidation", scenario.Portfolio.AccountSummary.NetLiquidation),
		moneyFact("portfolio.cash_balance", scenario.Portfolio.AccountSummary.CashBalance),
	}
	for _, position := range scenario.Portfolio.Positions {
		prefix := fmt.Sprintf("position.%d.", position.Instrument.ContractID)
		facts = append(facts,
			textFact(prefix+"symbol", "symbol", position.Instrument.Symbol, position.Instrument.Symbol != ""),
			textFact(prefix+"name", "name", position.Instrument.Name, position.Instrument.Name != ""),
			textFact(prefix+"asset_class", "classification", position.Instrument.AssetClass, position.Instrument.AssetClass != ""),
			textFact(prefix+"currency", "currency", position.Instrument.Currency, position.Instrument.Currency != ""),
			textFact(prefix+"sector", "classification", position.Instrument.Sector, position.Instrument.Sector != ""),
			numberFact(prefix+"quantity", "shares", "", position.Quantity, true),
			moneyFact(prefix+"market_price", position.MarketPrice),
			moneyFact(prefix+"market_value", position.MarketValue),
			moneyFact(prefix+"base_market_value", position.BaseMarketValue),
			moneyFact(prefix+"unrealized_pnl", position.UnrealizedPnL),
			moneyFact(prefix+"base_unrealized_pnl", position.BaseUnrealizedPnL),
		)
	}
	for _, rate := range scenario.Portfolio.ExchangeRates {
		id := metricID("exchange_rate", rate.Currency, rate.BaseCurrency)
		facts = append(facts, numberFact(id, "rate", "", rate.BaseAmountPerUnit, rate.Available))
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].ID < facts[j].ID })
	return facts
}

func textFact(id, kind, value string, available bool) FactRecord {
	return FactRecord{ID: id, Kind: kind, Text: strings.TrimSpace(value), Available: available}
}

func moneyFact(id string, value portfolio.Money) FactRecord {
	return numberFact(id, "money", value.Currency, value.Amount, value.Available)
}

func numberFact(id, unit, currency string, value float64, available bool) FactRecord {
	fact := FactRecord{ID: id, Kind: "number", Unit: unit, Currency: currency, Available: available}
	if available && !math.IsNaN(value) && !math.IsInf(value, 0) {
		fact.Value = &value
	} else {
		fact.Available = false
	}
	return fact
}
