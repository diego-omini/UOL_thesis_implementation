package portfoliohistory

// This file verifies descriptive history summaries and their mandatory return limitation.

import (
	"strings"
	"testing"
	"time"
)

func TestSummarizeUsesObservedEndpointsWithoutReturnCalculation(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := Result{From: start, To: start.AddDate(0, 1, 0), AccountValueSeries: []Series{{BaseCurrency: "GBP", Points: []Point{
		{CapturedAt: start, Source: SourceIBKRPortfolioAnalyst, NetLiquidation: Metric{Value: 100, Currency: "GBP", Available: true}},
		{CapturedAt: start.AddDate(0, 0, 1), Source: SourceLocalSnapshot, NetLiquidation: Metric{Value: 125, Currency: "GBP", Available: true}},
	}}}}
	summary := Summarize(result)
	if len(summary.Series) != 1 || summary.Series[0].AbsoluteChange.Value != 25 || summary.Series[0].ObservationCount != 2 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if len(summary.Warnings) == 0 || !strings.Contains(summary.Warnings[len(summary.Warnings)-1], "not investment returns") {
		t.Fatalf("required limitation missing: %#v", summary.Warnings)
	}
}
