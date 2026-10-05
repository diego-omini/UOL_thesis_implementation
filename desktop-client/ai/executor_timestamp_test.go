package ai

// This file verifies counselor provenance selects the newest authoritative context timestamp.

import (
	"testing"
	"time"
)

func TestCounselorContextAsOfSelectsNewestAuthoritativeTimestamp(t *testing.T) {
	older := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	result := counselorContextAsOf([]CounselorFact{
		{ID: "currency.rate.usd.as_of", TextValue: older.Format(time.RFC3339)},
		{ID: "portfolio.analysis_timestamp", TextValue: newer.Format(time.RFC3339)},
		{ID: "position.symbol", TextValue: "TEST"},
	})
	if !result.Equal(newer) {
		t.Fatalf("context timestamp = %s, want %s", result, newer)
	}
}
