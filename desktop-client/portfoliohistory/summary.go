package portfoliohistory

// This file computes descriptive history summaries without treating account-value changes as returns.

import "time"

// SeriesSummary contains deterministic descriptive values for one compatible history series.
type SeriesSummary struct {
	Product          string
	BaseCurrency     string
	ObservationCount int
	FirstAt          time.Time
	LastAt           time.Time
	FirstValue       Metric
	LastValue        Metric
	AbsoluteChange   Metric
	Minimum          Metric
	Maximum          Metric
	Sources          []string
}

// Summary contains bounded history coverage and descriptive account-value/tracked-analytics changes.
type Summary struct {
	From              time.Time
	To                time.Time
	TrackingStartedAt time.Time
	Series            []SeriesSummary
	Warnings          []string
}

// Summarize derives descriptive endpoints and ranges; it does not calculate investment returns.
func Summarize(result Result) Summary {
	summary := Summary{From: result.From, To: result.To, TrackingStartedAt: result.TrackingStartedAt}
	for _, series := range result.AccountValueSeries {
		summary.Series = append(summary.Series, summarizeSeries("account_value", series, func(point Point) Metric { return point.NetLiquidation }))
	}
	for _, series := range result.TrackingSeries {
		summary.Series = append(summary.Series, summarizeSeries("tracked_net_liquidation", series, func(point Point) Metric { return point.NetLiquidation }))
	}
	if result.AccountValueDownsampled || result.TrackingDownsampled {
		summary.Warnings = append(summary.Warnings, "The returned history was deterministically downsampled and preserves endpoints rather than every stored capture.")
	}
	if result.AccountValueDuplicatePointCount > 0 || result.TrackingDuplicatePointCount > 0 {
		summary.Warnings = append(summary.Warnings, "Near-duplicate captures were collapsed within the configured 90-second window.")
	}
	if result.ImportWarning != "" {
		summary.Warnings = append(summary.Warnings, result.ImportWarning)
	}
	summary.Warnings = append(summary.Warnings, "Account-value changes are descriptive and are not investment returns because deposits and withdrawals are unavailable.")
	return summary
}

func summarizeSeries(product string, series Series, selectMetric func(Point) Metric) SeriesSummary {
	value := SeriesSummary{Product: product, BaseCurrency: series.BaseCurrency}
	seenSources := map[string]bool{}
	for _, point := range series.Points {
		metric := selectMetric(point)
		if point.Source != "" && !seenSources[point.Source] {
			seenSources[point.Source] = true
			value.Sources = append(value.Sources, point.Source)
		}
		if !metric.Available {
			continue
		}
		value.ObservationCount++
		if value.FirstAt.IsZero() {
			value.FirstAt, value.FirstValue = point.CapturedAt, metric
			value.Minimum, value.Maximum = metric, metric
		}
		value.LastAt, value.LastValue = point.CapturedAt, metric
		if metric.Value < value.Minimum.Value {
			value.Minimum = metric
		}
		if metric.Value > value.Maximum.Value {
			value.Maximum = metric
		}
	}
	if value.ObservationCount > 0 {
		value.AbsoluteChange = Metric{Value: value.LastValue.Value - value.FirstValue.Value, Currency: value.BaseCurrency, Available: true}
	}
	return value
}
