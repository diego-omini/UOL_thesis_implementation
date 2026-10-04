package persistence

// This file queries sanitized portfolio snapshots into bounded currency-compatible history series.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"wails/analytics"
	"wails/portfoliohistory"

	"gorm.io/gorm"
)

const maxHistoryRecords = 20000

// QueryPortfolioHistory reads one account's sanitized captures without exposing persisted JSON or its fingerprint.
func (s *Store) QueryPortfolioHistory(ctx context.Context, accountID string, query portfoliohistory.Query) (portfoliohistory.Result, error) {
	if err := query.Validate(); err != nil {
		return portfoliohistory.Result{}, err
	}
	if strings.TrimSpace(accountID) == "" {
		return portfoliohistory.Result{}, fmt.Errorf("portfolio history requires an active account")
	}

	var records []PortfolioSnapshotRecord
	err := s.db.WithContext(ctx).
		Where("account_fingerprint = ? AND captured_at >= ? AND captured_at <= ?", accountFingerprint(accountID), query.From.UTC(), query.To.UTC()).
		Order("captured_at ASC").
		Order("id ASC").
		Limit(maxHistoryRecords + 1).
		Find(&records).Error
	if err != nil {
		return portfoliohistory.Result{}, fmt.Errorf("query portfolio history: %w", err)
	}
	if len(records) > maxHistoryRecords {
		return portfoliohistory.Result{}, fmt.Errorf("portfolio history contains more than %d captures; select a shorter range", maxHistoryRecords)
	}
	fingerprint := accountFingerprint(accountID)
	var latestExternalImport PortfolioNAVImportRecord
	importResult := s.db.WithContext(ctx).
		Where("account_fingerprint = ? AND source = ?", fingerprint, portfoliohistory.SourceSnapTradeBalanceHistory).
		Order("imported_at DESC").Order("id DESC").First(&latestExternalImport)
	if importResult.Error != nil && !errors.Is(importResult.Error, gorm.ErrRecordNotFound) {
		return portfoliohistory.Result{}, fmt.Errorf("query latest portfolio NAV import: %w", importResult.Error)
	}
	var navRecords []PortfolioNAVHistoryRecord
	navQuery := s.db.WithContext(ctx).
		Where("account_fingerprint = ? AND observed_date >= ? AND observed_date <= ?",
			fingerprint, query.From.UTC().Format("2006-01-02"), query.To.UTC().Format("2006-01-02"))
	if importResult.Error == nil {
		navQuery = navQuery.Where("source <> ? OR imported_at = ?", portfoliohistory.SourceSnapTradeBalanceHistory, latestExternalImport.ImportedAt)
	} else {
		legacyLatestImport := s.db.Model(&PortfolioNAVHistoryRecord{}).
			Select("MAX(imported_at)").
			Where("account_fingerprint = ? AND source = ?", fingerprint, portfoliohistory.SourceSnapTradeBalanceHistory)
		navQuery = navQuery.Where("source <> ? OR imported_at = (?)", portfoliohistory.SourceSnapTradeBalanceHistory, legacyLatestImport)
	}
	err = navQuery.
		Order("observed_date ASC").
		Order("id ASC").
		Limit(maxHistoryRecords + 1).
		Find(&navRecords).Error
	if err != nil {
		return portfoliohistory.Result{}, fmt.Errorf("query imported portfolio NAV history: %w", err)
	}
	if len(navRecords) > maxHistoryRecords {
		return portfoliohistory.Result{}, fmt.Errorf("imported portfolio history contains more than %d observations; select a shorter range", maxHistoryRecords)
	}

	result := portfoliohistory.Result{From: query.From.UTC(), To: query.To.UTC(), StoredPointCount: len(records) + len(navRecords)}
	result.TrackingStartedAt, err = s.trackingStartedAt(ctx, fingerprint)
	if err != nil {
		return portfoliohistory.Result{}, err
	}
	accountValues := make(map[string][]portfoliohistory.Point)
	trackedAnalytics := make(map[string][]portfoliohistory.Point)
	officialDates := make(map[string]bool)
	for _, record := range navRecords {
		if record.Source == portfoliohistory.SourceIBKRPortfolioAnalyst {
			officialDates[strings.ToUpper(strings.TrimSpace(record.BaseCurrency))+"\x00"+record.ObservedDate] = true
		}
	}
	for _, record := range records {
		currency, point, err := historyPoint(record)
		if err != nil {
			return portfoliohistory.Result{}, err
		}
		trackedAnalytics[currency] = append(trackedAnalytics[currency], point)
		accountValues[currency] = append(accountValues[currency], accountValuePoint(point))
	}
	for _, record := range navRecords {
		currency := strings.ToUpper(strings.TrimSpace(record.BaseCurrency))
		if record.Source == portfoliohistory.SourceSnapTradeBalanceHistory && officialDates[currency+"\x00"+record.ObservedDate] {
			continue
		}
		observedAt, err := time.Parse("2006-01-02", record.ObservedDate)
		if err != nil {
			return portfoliohistory.Result{}, fmt.Errorf("decode imported portfolio NAV date %q: %w", record.ObservedDate, err)
		}
		observedAt = observedAt.UTC().Add(12 * time.Hour)
		if observedAt.Before(query.From.UTC()) || observedAt.After(query.To.UTC()) {
			continue
		}
		var warnings []portfoliohistory.DataWarning
		if record.Source == portfoliohistory.SourceSnapTradeBalanceHistory {
			warnings = []portfoliohistory.DataWarning{{
				Code: "estimated_external_history", Message: "SnapTrade dynamically estimates total account value from historical positions and cash; it is not official broker NAV.", Count: 1,
			}}
			result.EstimatedPointCount++
		} else if record.Source == portfoliohistory.SourceIBKRPortfolioAnalyst {
			result.OfficialPointCount++
		}
		accountValues[currency] = append(accountValues[currency], portfoliohistory.Point{
			CapturedAt: observedAt, Source: record.Source, EndOfDay: true,
			NetLiquidation: metric(record.NAV, currency, true),
			Warnings:       warnings,
		})
	}

	result.AccountValueSeries, result.AccountValueDuplicatePointCount, result.AccountValuePointCount, result.AccountValueDownsampled = buildHistorySeries(accountValues, query.MaxPoints)
	result.TrackingSeries, result.TrackingDuplicatePointCount, result.TrackingPointCount, result.TrackingDownsampled = buildHistorySeries(trackedAnalytics, query.MaxPoints)
	return result, nil
}

// trackingStartedAt finds the first local snapshot independently from the selected display range.
func (s *Store) trackingStartedAt(ctx context.Context, fingerprint string) (time.Time, error) {
	var record PortfolioSnapshotRecord
	result := s.db.WithContext(ctx).
		Where("account_fingerprint = ?", fingerprint).
		Order("captured_at ASC").Order("id ASC").First(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return time.Time{}, nil
	}
	if result.Error != nil {
		return time.Time{}, fmt.Errorf("query portfolio tracking start: %w", result.Error)
	}
	return record.CapturedAt.UTC(), nil
}

// buildHistorySeries applies reductions independently so value and tracking coverage cannot be conflated.
func buildHistorySeries(grouped map[string][]portfoliohistory.Point, limit int) ([]portfoliohistory.Series, int, int, bool) {
	currencies := make([]string, 0, len(grouped))
	for currency := range grouped {
		currencies = append(currencies, currency)
	}
	sort.Slice(currencies, func(i, j int) bool {
		if currencies[i] == "" {
			return false
		}
		if currencies[j] == "" {
			return true
		}
		return currencies[i] < currencies[j]
	})

	series := make([]portfoliohistory.Series, 0, len(currencies))
	duplicates := 0
	returned := 0
	downsampled := false
	for _, currency := range currencies {
		points := grouped[currency]
		sort.SliceStable(points, func(i, j int) bool {
			return points[i].CapturedAt.Before(points[j].CapturedAt)
		})
		points, collapsed := collapseNearDuplicates(points)
		duplicates += collapsed
		if len(points) > limit {
			points = evenlySample(points, limit)
			downsampled = true
		}
		series = append(series, portfoliohistory.Series{BaseCurrency: currency, Points: points})
		returned += len(points)
	}
	return series, duplicates, returned, downsampled
}

func accountValuePoint(point portfoliohistory.Point) portfoliohistory.Point {
	return portfoliohistory.Point{
		CapturedAt:     point.CapturedAt,
		Source:         point.Source,
		EndOfDay:       point.EndOfDay,
		NetLiquidation: point.NetLiquidation,
	}
}

// collapseNearDuplicates keeps the latest point in each fixed duplicate window.
func collapseNearDuplicates(points []portfoliohistory.Point) ([]portfoliohistory.Point, int) {
	collapsed := make([]portfoliohistory.Point, 0, len(points))
	windowStart := time.Time{}
	removed := 0
	for _, point := range points {
		if len(collapsed) > 0 && point.Source == collapsed[len(collapsed)-1].Source && point.CapturedAt.Sub(windowStart) <= portfoliohistory.DuplicateWindow {
			collapsed[len(collapsed)-1] = point
			removed++
			continue
		}
		collapsed = append(collapsed, point)
		windowStart = point.CapturedAt
	}
	return collapsed, removed
}

// historyPoint decodes only the sanitized domain values needed by the history feature.
func historyPoint(record PortfolioSnapshotRecord) (string, portfoliohistory.Point, error) {
	var snapshot sanitizedPortfolioSnapshot
	if err := json.Unmarshal([]byte(record.SnapshotJSON), &snapshot); err != nil {
		return "", portfoliohistory.Point{}, fmt.Errorf("decode portfolio snapshot %d: %w", record.ID, err)
	}
	var result analytics.Result
	if err := json.Unmarshal([]byte(record.AnalyticsJSON), &result); err != nil {
		return "", portfoliohistory.Point{}, fmt.Errorf("decode portfolio analytics %d: %w", record.ID, err)
	}
	currency := strings.ToUpper(strings.TrimSpace(record.BaseCurrency))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(result.BaseCurrency))
	}
	warnings := make([]portfoliohistory.DataWarning, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		warnings = append(warnings, portfoliohistory.DataWarning(warning))
	}
	return currency, portfoliohistory.Point{
		CapturedAt:            record.CapturedAt.UTC(),
		Source:                portfoliohistory.SourceLocalSnapshot,
		NetLiquidation:        metric(snapshot.AccountSummary.NetLiquidation.Amount, snapshot.AccountSummary.NetLiquidation.Currency, snapshot.AccountSummary.NetLiquidation.Available),
		Cash:                  metric(snapshot.AccountSummary.CashBalance.Amount, snapshot.AccountSummary.CashBalance.Currency, snapshot.AccountSummary.CashBalance.Available),
		GrossExposure:         metric(result.GrossExposure.Amount, result.GrossExposure.Currency, result.GrossExposure.Available),
		NetMarketValue:        metric(result.NetMarketValue.Amount, result.NetMarketValue.Currency, result.NetMarketValue.Available),
		UnrealizedPnL:         metric(result.TotalUnrealizedPnL.Amount, result.TotalUnrealizedPnL.Currency, result.TotalUnrealizedPnL.Available),
		TopThreeConcentration: metric(result.TopThreeConcentration.Value, "", result.TopThreeConcentration.Available),
		HHI:                   metric(result.HHI.Value, "", result.HHI.Available),
		PositionCount:         metric(float64(record.PositionCount), "", true),
		Warnings:              warnings,
	}, nil
}

func metric(value float64, currency string, available bool) portfoliohistory.Metric {
	return portfoliohistory.Metric{Value: value, Currency: strings.ToUpper(strings.TrimSpace(currency)), Available: available}
}

// evenlySample preserves the first and last captures and chooses stable evenly spaced indexes between them.
func evenlySample(points []portfoliohistory.Point, limit int) []portfoliohistory.Point {
	if len(points) <= limit {
		return append([]portfoliohistory.Point(nil), points...)
	}
	sampled := make([]portfoliohistory.Point, limit)
	for index := range sampled {
		pointIndex := index * (len(points) - 1) / (limit - 1)
		sampled[index] = points[pointIndex]
	}
	return sampled
}
