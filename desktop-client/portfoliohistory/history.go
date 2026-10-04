// Package portfoliohistory defines validated read-only portfolio snapshot history queries and results.
package portfoliohistory

// This file owns persistence-independent portfolio history ranges, metrics, points, and series.

import (
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	MaxRange                      = 366 * 24 * time.Hour
	MaxPoints                     = 366
	DuplicateWindow               = 90 * time.Second
	SourceLocalSnapshot           = "local_snapshot"
	SourceIBKRPortfolioAnalyst    = "ibkr_portfolio_analyst"
	SourceSnapTradeBalanceHistory = "snaptrade_balance_history"
)

// NAVObservation is one broker-reported end-of-day net asset value.
type NAVObservation struct {
	Date         time.Time
	Value        float64
	BaseCurrency string
	Source       string
}

// NAVHistory contains broker-reported portfolio value observations for one account.
type NAVHistory struct {
	AccountID   string
	Frequency   string
	Source      string
	RetrievedAt time.Time
	Points      []NAVObservation
}

// Validate rejects incomplete, incompatible, or unordered broker history.
func (h NAVHistory) Validate() error {
	if strings.TrimSpace(h.AccountID) == "" {
		return fmt.Errorf("portfolio NAV history requires an account ID")
	}
	if h.RetrievedAt.IsZero() {
		return fmt.Errorf("portfolio NAV history requires a retrieval timestamp")
	}
	if !strings.EqualFold(strings.TrimSpace(h.Frequency), "D") {
		return fmt.Errorf("portfolio NAV history requires daily observations")
	}
	previous := time.Time{}
	for index, point := range h.Points {
		if point.Date.IsZero() || strings.TrimSpace(point.BaseCurrency) == "" || strings.TrimSpace(point.Source) == "" {
			return fmt.Errorf("portfolio NAV observation %d is incomplete", index)
		}
		if math.IsNaN(point.Value) || math.IsInf(point.Value, 0) {
			return fmt.Errorf("portfolio NAV observation %d has an invalid value", index)
		}
		if !previous.IsZero() && !point.Date.After(previous) {
			return fmt.Errorf("portfolio NAV observations must be strictly ordered")
		}
		previous = point.Date
	}
	return nil
}

// Query bounds a history lookup and the number of returned points in each currency series.
type Query struct {
	From      time.Time
	To        time.Time
	MaxPoints int
}

// Validate rejects unbounded, inverted, or overly large history requests.
func (q Query) Validate() error {
	if q.From.IsZero() || q.To.IsZero() {
		return fmt.Errorf("portfolio history requires both start and end timestamps")
	}
	if !q.From.Before(q.To) {
		return fmt.Errorf("portfolio history start must be before end")
	}
	if q.To.Sub(q.From) > MaxRange {
		return fmt.Errorf("portfolio history range cannot exceed %d days", int(MaxRange/(24*time.Hour)))
	}
	if q.MaxPoints < 2 || q.MaxPoints > MaxPoints {
		return fmt.Errorf("portfolio history point limit must be between 2 and %d", MaxPoints)
	}
	return nil
}

// Metric retains an observed numeric value without treating zero as available data.
type Metric struct {
	Value     float64
	Currency  string
	Available bool
}

// DataWarning retains deterministic analytics quality metadata for one capture.
type DataWarning struct {
	Code    string
	Message string
	Count   int
}

// Point contains the historical values derived from one sanitized snapshot.
type Point struct {
	CapturedAt            time.Time
	Source                string
	EndOfDay              bool
	NetLiquidation        Metric
	Cash                  Metric
	GrossExposure         Metric
	NetMarketValue        Metric
	UnrealizedPnL         Metric
	TopThreeConcentration Metric
	HHI                   Metric
	PositionCount         Metric
	Warnings              []DataWarning
}

// Series contains only points with one compatible base currency.
type Series struct {
	BaseCurrency string
	Points       []Point
}

// Result separates account-value observations from locally tracked analytical snapshots.
type Result struct {
	From                            time.Time
	To                              time.Time
	AccountValueSeries              []Series
	TrackingSeries                  []Series
	TrackingStartedAt               time.Time
	StoredPointCount                int
	AccountValuePointCount          int
	TrackingPointCount              int
	AccountValueDuplicatePointCount int
	TrackingDuplicatePointCount     int
	AccountValueDownsampled         bool
	TrackingDownsampled             bool
	OfficialPointCount              int
	EstimatedPointCount             int
	ImportWarning                   string
	ExternalConfigured              bool
	ConnectionRequired              bool
}
