package ai

// This file maps deterministic stock and proposed-order metrics into the order-analysis prompt.

import (
	"context"
	"fmt"
	"strings"

	"wails/portfolio"
	"wails/stockanalysis"
)

const OrderAnalysisPromptAlias = "order_analysis"

type promptOrderContext struct {
	Side                string
	Quantity            string
	EstimatedPrice      string
	EstimatedOrderValue string
	BaseOrderValue      string
	CashBalance         string
	CashUsage           string
	CurrentQuantity     string
	ResultingQuantity   string
	CurrentWeight       string
	ResultingWeight     string
	WeightChange        string
}

type promptMarketMetrics struct {
	Period                string
	Observations          int
	TotalReturn           string
	AnnualizedVolatility  string
	MaximumDrawdown       string
	PositionIn52WeekRange string
	DistanceFromSMA50     string
	DistanceFromSMA200    string
	MarketDataStatus      string
}

// OrderAnalysis explains deterministic market and portfolio metrics with bounded public research.
func (c *Counselor) OrderAnalysis(ctx context.Context, result stockanalysis.Result) (Response, error) {
	publicInstrument := PublicInstrument{
		Name: strings.TrimSpace(result.Instrument.Name), Ticker: strings.TrimSpace(result.Instrument.Symbol),
	}
	if publicInstrument.Name == "" && publicInstrument.Ticker == "" {
		return Response{}, fmt.Errorf("the stock has no public name or ticker to research")
	}
	parameters := map[string]any{
		"analysis_timestamp": result.CalculatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"symbol":             valueOrUnavailable(result.Instrument.Symbol),
		"name":               valueOrUnavailable(result.Instrument.Name),
		"exchange":           valueOrUnavailable(result.Instrument.Exchange),
		"currency":           valueOrUnavailable(result.Instrument.Currency),
		"order":              toPromptOrderContext(result),
		"market_metrics":     toPromptMarketMetrics(result),
		"data_warnings":      result.Warnings,
	}
	var tools []Tool
	if c.researchFactory != nil {
		tools = c.researchFactory.ForInstruments([]PublicInstrument{publicInstrument})
	}
	return c.executor.Execute(ctx, OrderAnalysisPromptAlias, parameters, tools)
}

func toPromptOrderContext(result stockanalysis.Result) promptOrderContext {
	cashUsage := "not applicable for a sell order"
	if result.Side == "BUY" {
		cashUsage = formatAnalysisMetric(result.CashUsage, true)
	}
	return promptOrderContext{
		Side: result.Side, Quantity: fmt.Sprintf("%.0f shares", result.Quantity),
		EstimatedPrice:      formatAnalysisMoney(result.EstimatedPrice),
		EstimatedOrderValue: formatAnalysisMoney(result.EstimatedOrderValue),
		BaseOrderValue:      formatAnalysisMoney(result.BaseOrderValue),
		CashBalance:         formatAnalysisMoney(result.CashBalance), CashUsage: cashUsage,
		CurrentQuantity:   fmt.Sprintf("%.0f shares", result.CurrentQuantity),
		ResultingQuantity: fmt.Sprintf("%.0f shares", result.ResultingQuantity),
		CurrentWeight:     formatAnalysisMetric(result.CurrentWeight, true),
		ResultingWeight:   formatAnalysisMetric(result.ResultingWeight, true),
		WeightChange:      formatSignedAnalysisMetric(result.WeightChange),
	}
}

func toPromptMarketMetrics(result stockanalysis.Result) promptMarketMetrics {
	period := "unavailable"
	if !result.HistoryStart.IsZero() && !result.HistoryEnd.IsZero() {
		period = result.HistoryStart.Format("2006-01-02") + " to " + result.HistoryEnd.Format("2006-01-02")
	}
	return promptMarketMetrics{
		Period: period, Observations: result.HistoryObservations,
		TotalReturn:           formatAnalysisMetric(result.TotalReturn, true),
		AnnualizedVolatility:  formatAnalysisMetric(result.AnnualizedVolatility, true),
		MaximumDrawdown:       formatAnalysisMetric(result.MaximumDrawdown, true),
		PositionIn52WeekRange: formatAnalysisMetric(result.PositionIn52WeekRange, true),
		DistanceFromSMA50:     formatSignedPercent(result.DistanceFromSMA50),
		DistanceFromSMA200:    formatSignedPercent(result.DistanceFromSMA200),
		MarketDataStatus:      valueOrUnavailable(result.MarketDataStatus),
	}
}

func formatAnalysisMetric(metric stockanalysis.Metric, percent bool) string {
	if !metric.Available {
		return "unavailable"
	}
	if percent {
		return fmt.Sprintf("%.1f%%", metric.Value*100)
	}
	return fmt.Sprintf("%.4f", metric.Value)
}

func formatSignedAnalysisMetric(metric stockanalysis.Metric) string {
	if !metric.Available {
		return "unavailable"
	}
	return fmt.Sprintf("%+.1f percentage points", metric.Value*100)
}

func formatSignedPercent(metric stockanalysis.Metric) string {
	if !metric.Available {
		return "unavailable"
	}
	return fmt.Sprintf("%+.1f%%", metric.Value*100)
}

func formatAnalysisMoney(value portfolio.Money) string {
	if !value.Available || strings.TrimSpace(value.Currency) == "" {
		return "unavailable"
	}
	return fmt.Sprintf("%.2f %s", value.Amount, value.Currency)
}
