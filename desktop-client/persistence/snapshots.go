package persistence

// This file persists account-sanitized portfolio snapshots and matching deterministic analytics.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"wails/analytics"
	"wails/portfolio"
)

type sanitizedAccountSummary struct {
	NetLiquidation portfolio.Money `json:"net_liquidation"`
	CashBalance    portfolio.Money `json:"cash_balance"`
}

type sanitizedPortfolioSnapshot struct {
	SchemaVersion  int                      `json:"schema_version"`
	CapturedAt     time.Time                `json:"captured_at"`
	AccountSummary sanitizedAccountSummary  `json:"account_summary"`
	Positions      []portfolio.Position     `json:"positions"`
	ExchangeRates  []portfolio.ExchangeRate `json:"exchange_rates"`
}

// SavePortfolioSnapshot stores one normalized snapshot without retaining the broker account identifier.
func (s *Store) SavePortfolioSnapshot(ctx context.Context, snapshot portfolio.Portfolio, result analytics.Result) error {
	payload := sanitizedPortfolioSnapshot{
		SchemaVersion: SchemaVersion,
		CapturedAt:    result.CalculatedAt.UTC(),
		AccountSummary: sanitizedAccountSummary{
			NetLiquidation: snapshot.AccountSummary.NetLiquidation,
			CashBalance:    snapshot.AccountSummary.CashBalance,
		},
		Positions:     append([]portfolio.Position(nil), snapshot.Positions...),
		ExchangeRates: append([]portfolio.ExchangeRate(nil), snapshot.ExchangeRates...),
	}
	snapshotJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode portfolio snapshot: %w", err)
	}
	analyticsJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode portfolio analytics: %w", err)
	}
	digest := sha256.Sum256(snapshotJSON)
	record := PortfolioSnapshotRecord{
		SchemaVersion: SchemaVersion, CapturedAt: result.CalculatedAt.UTC(),
		AccountFingerprint: accountFingerprint(snapshot.AccountSummary.AccountID),
		BaseCurrency:       result.BaseCurrency, PositionCount: len(snapshot.Positions),
		SnapshotHash: hex.EncodeToString(digest[:]), SnapshotJSON: string(snapshotJSON),
		AnalyticsJSON: string(analyticsJSON),
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("store portfolio snapshot: %w", err)
	}
	return nil
}
