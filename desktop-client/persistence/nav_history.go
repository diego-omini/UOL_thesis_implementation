package persistence

// This file stores idempotent broker-reported daily portfolio values without retaining account identifiers.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wails/portfoliohistory"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SavePortfolioNAVHistory upserts official daily NAV observations by account, date, currency, and source.
func (s *Store) SavePortfolioNAVHistory(ctx context.Context, history portfoliohistory.NAVHistory) error {
	if err := history.Validate(); err != nil {
		return err
	}
	records := make([]PortfolioNAVHistoryRecord, 0, len(history.Points))
	fingerprint := accountFingerprint(history.AccountID)
	source := strings.TrimSpace(history.Source)
	for _, point := range history.Points {
		pointSource := strings.TrimSpace(point.Source)
		if source == "" {
			source = pointSource
		} else if pointSource != source {
			return fmt.Errorf("portfolio NAV history contains inconsistent sources")
		}
		records = append(records, PortfolioNAVHistoryRecord{
			SchemaVersion: SchemaVersion, AccountFingerprint: fingerprint,
			ObservedDate: point.Date.UTC().Format("2006-01-02"),
			BaseCurrency: strings.ToUpper(strings.TrimSpace(point.BaseCurrency)),
			Source:       strings.TrimSpace(point.Source), NAV: point.Value,
			Frequency:  strings.ToUpper(strings.TrimSpace(history.Frequency)),
			ImportedAt: history.RetrievedAt.UTC(),
		})
	}
	if source == "" {
		return fmt.Errorf("portfolio NAV history requires a source even when the import is empty")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(records) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "account_fingerprint"}, {Name: "observed_date"},
					{Name: "base_currency"}, {Name: "source"},
				},
				DoUpdates: clause.AssignmentColumns([]string{"schema_version", "nav", "frequency", "imported_at", "updated_at"}),
			}).Create(&records).Error; err != nil {
				return err
			}
		}
		return tx.Create(&PortfolioNAVImportRecord{
			SchemaVersion: SchemaVersion, AccountFingerprint: fingerprint,
			Source: source, ImportedAt: history.RetrievedAt.UTC(),
		}).Error
	})
	if err != nil {
		return fmt.Errorf("store portfolio NAV history: %w", err)
	}
	return nil
}

// PortfolioNAVHistoryImportedRecently reports whether one source was persisted inside the pacing window.
func (s *Store) PortfolioNAVHistoryImportedRecently(ctx context.Context, accountID, source string, since time.Time) (bool, error) {
	if strings.TrimSpace(accountID) == "" {
		return false, fmt.Errorf("portfolio NAV history requires an active account")
	}
	if strings.TrimSpace(source) == "" {
		return false, fmt.Errorf("portfolio NAV history requires a source")
	}
	var count int64
	err := s.db.WithContext(ctx).Model(&PortfolioNAVImportRecord{}).
		Where("account_fingerprint = ? AND source = ? AND imported_at >= ?",
			accountFingerprint(accountID), strings.TrimSpace(source), since.UTC()).
		Limit(1).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check portfolio NAV import freshness: %w", err)
	}
	if count > 0 {
		return true, nil
	}
	err = s.db.WithContext(ctx).Model(&PortfolioNAVHistoryRecord{}).
		Where("account_fingerprint = ? AND source = ? AND imported_at >= ?",
			accountFingerprint(accountID), strings.TrimSpace(source), since.UTC()).
		Limit(1).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check legacy portfolio NAV import freshness: %w", err)
	}
	return count > 0, nil
}
