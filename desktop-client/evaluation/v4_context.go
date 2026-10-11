package evaluation

// This file builds version 4 contexts with normalized base-currency position facts and explicit provenance.

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"wails/analytics"
	"wails/portfolio"
)

func v4ConditionContext(scenario FrozenScenario, condition Condition) (string, map[string]v2Evidence, []MetricRecord, error) {
	result := analytics.Calculate(scenario.Portfolio, scenario.SnapshotAt)
	metrics := buildMetrics(scenario.Portfolio, result)
	facts := buildV4Facts(scenario, result)
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
		return "", nil, nil, fmt.Errorf("encode version 4 %s context: %w", condition.ID, err)
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

func buildV4Facts(scenario FrozenScenario, result analytics.Result) []FactRecord {
	rates := make(map[string]portfolio.ExchangeRate, len(scenario.Portfolio.ExchangeRates))
	for _, rate := range scenario.Portfolio.ExchangeRates {
		if rate.Available && rate.BaseAmountPerUnit > 0 && strings.EqualFold(strings.TrimSpace(rate.BaseCurrency), result.BaseCurrency) {
			rates[strings.ToUpper(strings.TrimSpace(rate.Currency))] = rate
		}
	}
	facts := []FactRecord{
		v4TextFact("portfolio.snapshot_at", "timestamp", scenario.SnapshotAt.UTC().Format("2006-01-02T15:04:05Z"), true, "evaluation_fixture"),
		v4TextFact("portfolio.base_currency", "currency", result.BaseCurrency, result.BaseCurrency != "", "normalized_account"),
		v4MoneyFact("portfolio.net_liquidation", scenario.Portfolio.AccountSummary.NetLiquidation, "broker_account_summary"),
		v4MoneyFact("portfolio.cash_balance", scenario.Portfolio.AccountSummary.CashBalance, "broker_account_summary"),
	}
	for _, position := range scenario.Portfolio.Positions {
		prefix := fmt.Sprintf("position.%d.", position.Instrument.ContractID)
		baseMarketValue, marketSource := normalizeV4Money(position.MarketValue, position.BaseMarketValue, result.BaseCurrency, rates)
		baseUnrealizedPnL, pnlSource := normalizeV4Money(position.UnrealizedPnL, position.BaseUnrealizedPnL, result.BaseCurrency, rates)
		facts = append(facts,
			v4TextFact(prefix+"symbol", "symbol", position.Instrument.Symbol, position.Instrument.Symbol != "", "broker_position"),
			v4TextFact(prefix+"name", "name", position.Instrument.Name, position.Instrument.Name != "", "broker_position"),
			v4TextFact(prefix+"asset_class", "classification", position.Instrument.AssetClass, position.Instrument.AssetClass != "", "broker_position"),
			v4TextFact(prefix+"currency", "currency", position.Instrument.Currency, position.Instrument.Currency != "", "broker_position"),
			v4TextFact(prefix+"sector", "classification", position.Instrument.Sector, position.Instrument.Sector != "", "broker_position"),
			v4NumberFact(prefix+"quantity", "shares", "", position.Quantity, true, "broker_position"),
			v4MoneyFact(prefix+"market_price", position.MarketPrice, "broker_position"),
			v4MoneyFact(prefix+"market_value", position.MarketValue, "broker_position"),
			v4MoneyFact(prefix+"base_market_value", baseMarketValue, marketSource),
			v4MoneyFact(prefix+"unrealized_pnl", position.UnrealizedPnL, "broker_position"),
			v4MoneyFact(prefix+"base_unrealized_pnl", baseUnrealizedPnL, pnlSource),
		)
	}
	for _, rate := range scenario.Portfolio.ExchangeRates {
		id := metricID("exchange_rate", rate.Currency, rate.BaseCurrency)
		facts = append(facts, v4NumberFact(id, "rate", "", rate.BaseAmountPerUnit, rate.Available, "broker_ledger"))
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].ID < facts[j].ID })
	return facts
}

func normalizeV4Money(local, base portfolio.Money, baseCurrency string, rates map[string]portfolio.ExchangeRate) (portfolio.Money, string) {
	if base.Available {
		return portfolio.Money{Amount: base.Amount, Currency: baseCurrency, Available: true}, "broker_base_value"
	}
	if !local.Available || strings.TrimSpace(baseCurrency) == "" {
		return portfolio.Money{Currency: baseCurrency}, "unavailable"
	}
	if strings.EqualFold(strings.TrimSpace(local.Currency), baseCurrency) {
		return portfolio.Money{Amount: local.Amount, Currency: baseCurrency, Available: true}, "same_currency_normalization"
	}
	rate, ok := rates[strings.ToUpper(strings.TrimSpace(local.Currency))]
	if !ok {
		return portfolio.Money{Currency: baseCurrency}, "unavailable"
	}
	return portfolio.Money{Amount: local.Amount * rate.BaseAmountPerUnit, Currency: baseCurrency, Available: true}, "broker_ledger_conversion"
}

func v4TextFact(id, kind, value string, available bool, source string) FactRecord {
	return FactRecord{ID: id, Kind: kind, Text: strings.TrimSpace(value), Source: source, Available: available}
}

func v4MoneyFact(id string, value portfolio.Money, source string) FactRecord {
	return v4NumberFact(id, "money", value.Currency, value.Amount, value.Available, source)
}

func v4NumberFact(id, unit, currency string, value float64, available bool, source string) FactRecord {
	fact := FactRecord{ID: id, Kind: "number", Unit: unit, Currency: currency, Source: source, Available: available}
	if available && !math.IsNaN(value) && !math.IsInf(value, 0) {
		fact.Value = &value
	} else {
		fact.Available = false
	}
	return fact
}
