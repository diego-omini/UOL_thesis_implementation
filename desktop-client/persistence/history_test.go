package persistence

// This file verifies bounded snapshot-history filtering, ordering, currency separation, and reduction.

import (
	"context"
	"strings"
	"testing"
	"time"

	"wails/analytics"
	"wails/portfolio"
	"wails/portfoliohistory"
)

func saveHistorySnapshot(t *testing.T, store *Store, accountID, currency string, capturedAt time.Time, amount float64, available bool) {
	t.Helper()
	snapshot := portfolio.Portfolio{
		AccountSummary: portfolio.AccountSummary{
			AccountID:      accountID,
			NetLiquidation: portfolio.Money{Amount: amount, Currency: currency, Available: available},
			CashBalance:    portfolio.Money{Amount: amount / 4, Currency: currency, Available: available},
		},
		Positions: []portfolio.Position{{
			Instrument:    portfolio.Instrument{ContractID: 1, Symbol: "TEST", AssetClass: "STK", Currency: currency, Sector: "Technology"},
			MarketPrice:   portfolio.Money{Amount: amount, Currency: currency, Available: available},
			MarketValue:   portfolio.Money{Amount: amount, Currency: currency, Available: available},
			UnrealizedPnL: portfolio.Money{Amount: amount / 10, Currency: currency, Available: available},
		}},
	}
	result := analytics.Calculate(snapshot, capturedAt)
	if err := store.SavePortfolioSnapshot(context.Background(), snapshot, result); err != nil {
		t.Fatalf("SavePortfolioSnapshot returned an error: %v", err)
	}
}

func TestQueryPortfolioHistoryFiltersRangeAndOrdersCaptures(t *testing.T) {
	store := openTestStore(t)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	saveHistorySnapshot(t, store, "DU-1", "CHF", base.Add(3*time.Hour), 300, true)
	saveHistorySnapshot(t, store, "DU-1", "CHF", base.Add(-time.Hour), 50, true)
	saveHistorySnapshot(t, store, "DU-1", "CHF", base.Add(time.Hour), 100, true)
	saveHistorySnapshot(t, store, "DU-OTHER", "CHF", base.Add(2*time.Hour), 999, true)

	result, err := store.QueryPortfolioHistory(context.Background(), "DU-1", portfoliohistory.Query{
		From: base, To: base.Add(4 * time.Hour), MaxPoints: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StoredPointCount != 2 || len(result.AccountValueSeries) != 1 || len(result.TrackingSeries) != 1 || len(result.TrackingSeries[0].Points) != 2 {
		t.Fatalf("unexpected range result: %+v", result)
	}
	points := result.TrackingSeries[0].Points
	if !points[0].CapturedAt.Before(points[1].CapturedAt) || points[0].NetLiquidation.Value != 100 || points[1].NetLiquidation.Value != 300 {
		t.Fatalf("captures were not ordered: %+v", points)
	}
	if !result.TrackingStartedAt.Equal(base.Add(-time.Hour)) {
		t.Fatalf("tracking start = %v, want %v", result.TrackingStartedAt, base.Add(-time.Hour))
	}
}

func TestQueryPortfolioHistoryCollapsesNearDuplicatesAndDownsamplesDeterministically(t *testing.T) {
	store := openTestStore(t)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for index, offset := range []time.Duration{0, time.Minute, 3 * time.Minute, 5 * time.Minute, 7 * time.Minute, 9 * time.Minute} {
		saveHistorySnapshot(t, store, "DU-1", "CHF", base.Add(offset), float64(index+1), true)
	}

	result, err := store.QueryPortfolioHistory(context.Background(), "DU-1", portfoliohistory.Query{
		From: base.Add(-time.Second), To: base.Add(10 * time.Minute), MaxPoints: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountValueDuplicatePointCount != 1 || result.TrackingDuplicatePointCount != 1 || !result.AccountValueDownsampled || !result.TrackingDownsampled {
		t.Fatalf("expected explicit duplicate collapse and downsampling: %+v", result)
	}
	points := result.TrackingSeries[0].Points
	if len(points) != 3 || points[0].NetLiquidation.Value != 2 || points[1].NetLiquidation.Value != 4 || points[2].NetLiquidation.Value != 6 {
		t.Fatalf("unexpected deterministic sample: %+v", points)
	}
}

func TestQueryPortfolioHistoryKeepsIncompatibleCurrenciesSeparate(t *testing.T) {
	store := openTestStore(t)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	saveHistorySnapshot(t, store, "DU-1", "CHF", base, 100, true)
	saveHistorySnapshot(t, store, "DU-1", "USD", base.Add(2*time.Minute), 200, true)

	result, err := store.QueryPortfolioHistory(context.Background(), "DU-1", portfoliohistory.Query{
		From: base.Add(-time.Second), To: base.Add(3 * time.Minute), MaxPoints: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AccountValueSeries) != 2 || len(result.TrackingSeries) != 2 || result.TrackingSeries[0].BaseCurrency != "CHF" || result.TrackingSeries[1].BaseCurrency != "USD" {
		t.Fatalf("incompatible currencies were not separated: %+v", result.TrackingSeries)
	}
}

func TestQueryPortfolioHistoryPreservesUnavailableMetricsAndWarnings(t *testing.T) {
	store := openTestStore(t)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	saveHistorySnapshot(t, store, "DU-1", "CHF", base, 100, false)

	result, err := store.QueryPortfolioHistory(context.Background(), "DU-1", portfoliohistory.Query{
		From: base.Add(-time.Second), To: base.Add(time.Minute), MaxPoints: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	point := result.TrackingSeries[0].Points[0]
	if point.NetLiquidation.Available || point.GrossExposure.Available || point.UnrealizedPnL.Available {
		t.Fatalf("unavailable values became available: %+v", point)
	}
	if len(point.Warnings) == 0 {
		t.Fatal("expected data-quality warnings")
	}
}

func TestQueryPortfolioHistoryRejectsInvalidBounds(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UTC()
	_, err := store.QueryPortfolioHistory(context.Background(), "DU-1", portfoliohistory.Query{
		From: now.Add(-2 * portfoliohistory.MaxRange), To: now, MaxPoints: 20,
	})
	if err == nil {
		t.Fatal("expected an oversized range to fail")
	}
}

func TestPortfolioNAVHistoryUpsertSanitizesAccountAndRefreshesValue(t *testing.T) {
	store := openTestStore(t)
	date := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	history := portfoliohistory.NAVHistory{
		AccountID: "DU-PRIVATE", Frequency: "D", RetrievedAt: date.Add(time.Hour),
		Points: []portfoliohistory.NAVObservation{{
			Date: date, Value: 1000, BaseCurrency: "CHF", Source: portfoliohistory.SourceIBKRPortfolioAnalyst,
		}},
	}
	if err := store.SavePortfolioNAVHistory(context.Background(), history); err != nil {
		t.Fatal(err)
	}
	history.Points[0].Value = 1005
	history.RetrievedAt = history.RetrievedAt.Add(time.Hour)
	if err := store.SavePortfolioNAVHistory(context.Background(), history); err != nil {
		t.Fatal(err)
	}

	var records []PortfolioNAVHistoryRecord
	if err := store.db.Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].NAV != 1005 || records[0].AccountFingerprint == "" || strings.Contains(records[0].AccountFingerprint, "DU-PRIVATE") {
		t.Fatalf("unexpected sanitized upsert: %+v", records)
	}
	fresh, err := store.PortfolioNAVHistoryImportedRecently(context.Background(), "DU-PRIVATE", portfoliohistory.SourceIBKRPortfolioAnalyst, date)
	if err != nil || !fresh {
		t.Fatalf("expected recent import, fresh=%v err=%v", fresh, err)
	}
}

func TestQueryPortfolioHistoryKeepsOfficialNAVAndLocalCaptureOnSameDate(t *testing.T) {
	store := openTestStore(t)
	base := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	history := portfoliohistory.NAVHistory{
		AccountID: "DU-1", Frequency: "D", RetrievedAt: base.Add(time.Hour),
		Points: []portfoliohistory.NAVObservation{
			{Date: base.AddDate(0, 0, -1), Value: 900, BaseCurrency: "CHF", Source: portfoliohistory.SourceIBKRPortfolioAnalyst},
			{Date: base, Value: 950, BaseCurrency: "CHF", Source: portfoliohistory.SourceIBKRPortfolioAnalyst},
		},
	}
	if err := store.SavePortfolioNAVHistory(context.Background(), history); err != nil {
		t.Fatal(err)
	}
	saveHistorySnapshot(t, store, "DU-1", "CHF", base.Add(15*time.Hour), 1000, true)

	result, err := store.QueryPortfolioHistory(context.Background(), "DU-1", portfoliohistory.Query{
		From: base.AddDate(0, 0, -2), To: base.AddDate(0, 0, 1), MaxPoints: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OfficialPointCount != 2 || len(result.AccountValueSeries) != 1 || len(result.AccountValueSeries[0].Points) != 3 || len(result.TrackingSeries[0].Points) != 1 {
		t.Fatalf("unexpected merged result: %+v", result)
	}
	points := result.AccountValueSeries[0].Points
	if points[0].Source != portfoliohistory.SourceIBKRPortfolioAnalyst || points[0].NetLiquidation.Value != 900 || points[0].PositionCount.Available {
		t.Fatalf("unexpected official point: %+v", points[0])
	}
	if points[1].Source != portfoliohistory.SourceIBKRPortfolioAnalyst || points[1].NetLiquidation.Value != 950 {
		t.Fatalf("same-date official point was lost: %+v", points[1])
	}
	if points[2].Source != portfoliohistory.SourceLocalSnapshot || points[2].NetLiquidation.Value != 1000 || points[2].PositionCount.Available {
		t.Fatalf("account-value point exposed local analytics: %+v", points[2])
	}
	tracked := result.TrackingSeries[0].Points[0]
	if tracked.Source != portfoliohistory.SourceLocalSnapshot || tracked.NetLiquidation.Value != 1000 || !tracked.PositionCount.Available {
		t.Fatalf("unexpected tracked analytics point: %+v", tracked)
	}
}

func TestQueryPortfolioHistoryPrefersOfficialNAVToEstimatedSameDayValue(t *testing.T) {
	store := openTestStore(t)
	date := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	for source, value := range map[string]float64{
		portfoliohistory.SourceSnapTradeBalanceHistory: 995,
		portfoliohistory.SourceIBKRPortfolioAnalyst:    1000,
	} {
		history := portfoliohistory.NAVHistory{
			AccountID: "DU-1", Frequency: "D", RetrievedAt: date.Add(time.Hour),
			Points: []portfoliohistory.NAVObservation{{Date: date, Value: value, BaseCurrency: "CHF", Source: source}},
		}
		if err := store.SavePortfolioNAVHistory(context.Background(), history); err != nil {
			t.Fatal(err)
		}
	}
	result, err := store.QueryPortfolioHistory(context.Background(), "DU-1", portfoliohistory.Query{
		From: date.AddDate(0, 0, -1), To: date.AddDate(0, 0, 1), MaxPoints: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OfficialPointCount != 1 || result.EstimatedPointCount != 0 || len(result.AccountValueSeries[0].Points) != 1 {
		t.Fatalf("unexpected source precedence: %+v", result)
	}
	if got := result.AccountValueSeries[0].Points[0]; got.Source != portfoliohistory.SourceIBKRPortfolioAnalyst || got.NetLiquidation.Value != 1000 {
		t.Fatalf("official observation was not preferred: %+v", got)
	}
}

func TestQueryPortfolioHistoryUsesOnlyLatestEstimatedImportBatch(t *testing.T) {
	store := openTestStore(t)
	date := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	first := portfoliohistory.NAVHistory{
		AccountID: "DU-1", Frequency: "D", RetrievedAt: date.Add(time.Hour),
		Points: []portfoliohistory.NAVObservation{
			{Date: date.AddDate(0, 0, -1), Value: 900, BaseCurrency: "CHF", Source: portfoliohistory.SourceSnapTradeBalanceHistory},
			{Date: date, Value: 1000, BaseCurrency: "CHF", Source: portfoliohistory.SourceSnapTradeBalanceHistory},
		},
	}
	latest := portfoliohistory.NAVHistory{
		AccountID: "DU-1", Frequency: "D", RetrievedAt: date.Add(2 * time.Hour),
		Points: []portfoliohistory.NAVObservation{
			{Date: date, Value: 1005, BaseCurrency: "CHF", Source: portfoliohistory.SourceSnapTradeBalanceHistory},
		},
	}
	if err := store.SavePortfolioNAVHistory(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.SavePortfolioNAVHistory(context.Background(), latest); err != nil {
		t.Fatal(err)
	}
	result, err := store.QueryPortfolioHistory(context.Background(), "DU-1", portfoliohistory.Query{
		From: date.AddDate(0, 0, -2), To: date.AddDate(0, 0, 1), MaxPoints: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.EstimatedPointCount != 1 || len(result.AccountValueSeries[0].Points) != 1 || result.AccountValueSeries[0].Points[0].NetLiquidation.Value != 1005 {
		t.Fatalf("stale estimated observations were returned: %+v", result)
	}
}

func TestQueryPortfolioHistoryTreatsEmptyEstimatedImportAsNewestBatch(t *testing.T) {
	store := openTestStore(t)
	date := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	first := portfoliohistory.NAVHistory{
		AccountID: "DU-1", Frequency: "D", Source: portfoliohistory.SourceSnapTradeBalanceHistory,
		RetrievedAt: date.Add(time.Hour),
		Points: []portfoliohistory.NAVObservation{{
			Date: date, Value: 1000, BaseCurrency: "CHF", Source: portfoliohistory.SourceSnapTradeBalanceHistory,
		}},
	}
	empty := portfoliohistory.NAVHistory{
		AccountID: "DU-1", Frequency: "D", Source: portfoliohistory.SourceSnapTradeBalanceHistory,
		RetrievedAt: date.Add(2 * time.Hour),
	}
	if err := store.SavePortfolioNAVHistory(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.SavePortfolioNAVHistory(context.Background(), empty); err != nil {
		t.Fatal(err)
	}
	result, err := store.QueryPortfolioHistory(context.Background(), "DU-1", portfoliohistory.Query{
		From: date.AddDate(0, 0, -1), To: date.AddDate(0, 0, 1), MaxPoints: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.EstimatedPointCount != 0 || len(result.AccountValueSeries) != 0 || len(result.TrackingSeries) != 0 {
		t.Fatalf("stale estimates remained visible after an empty import: %+v", result)
	}
	fresh, err := store.PortfolioNAVHistoryImportedRecently(context.Background(), "DU-1", portfoliohistory.SourceSnapTradeBalanceHistory, date.Add(90*time.Minute))
	if err != nil || !fresh {
		t.Fatalf("empty successful import was not recorded: fresh=%v err=%v", fresh, err)
	}
}
