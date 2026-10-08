package main

// This file verifies frontend DTO mapping for portfolio analysis results.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"wails/ai"
	"wails/persistence"
	"wails/portfolio"
	"wails/portfolioanalysis"
	"wails/portfoliohistory"
	"wails/trading"
)

func TestToPortfolioRiskAnalysisViewReturnsEmptyCollections(t *testing.T) {
	view := toPortfolioRiskAnalysisView(portfolioanalysis.Result{CalculatedAt: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)})
	if view.Positions == nil || view.Correlations == nil || view.Scenarios == nil || view.Warnings == nil {
		t.Fatalf("risk view must expose empty collections instead of null: %+v", view)
	}
}

func TestOrderViewsDoNotExposeConfirmationTokens(t *testing.T) {
	preview, err := json.Marshal(toOrderPreviewView(trading.Preview{Instrument: portfolio.Instrument{Symbol: "TEST"}}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := json.Marshal(toOrderResultView(trading.Result{ConfirmationRequired: true}))
	if err != nil {
		t.Fatal(err)
	}
	combined := string(preview) + string(result)
	if strings.Contains(strings.ToLower(combined), "token") {
		t.Fatalf("order DTO exposed a confirmation token field: %s", combined)
	}
}

func TestRecentAnalysisViewExposesOnlyValidatedResponseBoundary(t *testing.T) {
	createdAt := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	views := toRecentAnalysisViews([]persistence.RecentAnalysis{{
		ID: 7, CreatedAt: createdAt, ContextAsOf: createdAt.Add(-time.Minute), Provider: "ollama", Model: "model",
		PromptID: "portfolio_resume", PromptVersion: 3,
		Response: ai.CounselorResponse{SchemaVersion: 1, Summary: "Saved", Observations: []ai.CounselorObservation{}, Limitations: []ai.CounselorLimitation{}},
	}})
	if len(views) != 1 || views[0].ID != 7 || views[0].Response.Text != "" || views[0].Response.Structured.Summary != "Saved" || views[0].Response.CreatedAt != createdAt.Format(time.RFC3339) {
		t.Fatalf("unexpected recent analysis DTO: %+v", views)
	}
}

func TestToPortfolioHistoryViewPreservesAvailabilityAndMetadata(t *testing.T) {
	capturedAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	view := toPortfolioHistoryView("30d", portfoliohistory.Result{
		From: capturedAt.Add(-time.Hour), To: capturedAt.Add(time.Hour), StoredPointCount: 3,
		TrackingStartedAt:               capturedAt.Add(-24 * time.Hour),
		AccountValueDuplicatePointCount: 1, AccountValuePointCount: 1, AccountValueDownsampled: true,
		TrackingDuplicatePointCount: 1, TrackingPointCount: 1, TrackingDownsampled: true,
		EstimatedPointCount: 1, ImportWarning: "IBKR unavailable", ExternalConfigured: true, ConnectionRequired: true,
		AccountValueSeries: []portfoliohistory.Series{{BaseCurrency: "CHF", Points: []portfoliohistory.Point{{
			CapturedAt: capturedAt, NetLiquidation: portfoliohistory.Metric{Value: 100, Currency: "CHF", Available: true},
		}}}},
		TrackingSeries: []portfoliohistory.Series{{BaseCurrency: "CHF", Points: []portfoliohistory.Point{{
			CapturedAt: capturedAt, NetLiquidation: portfoliohistory.Metric{Value: 100, Currency: "CHF", Available: true},
			Cash:          portfoliohistory.Metric{Value: 0, Available: false},
			PositionCount: portfoliohistory.Metric{Value: 2, Available: true},
			Warnings:      []portfoliohistory.DataWarning{{Code: "missing_cash", Message: "Cash was unavailable.", Count: 1}},
		}}}},
	})

	if view.Range != "30d" || view.AccountValueDuplicatePointCount != 1 || !view.AccountValueDownsampled || view.EstimatedPointCount != 1 || !view.ExternalConfigured || !view.ConnectionRequired || len(view.AccountValueSeries) != 1 || len(view.TrackingSeries) != 1 {
		t.Fatalf("unexpected history view metadata: %+v", view)
	}
	if view.TrackingStartedAt != capturedAt.Add(-24*time.Hour).Format(time.RFC3339) {
		t.Fatalf("unexpected tracking start: %q", view.TrackingStartedAt)
	}
	point := view.TrackingSeries[0].Points[0]
	if point.CapturedAt != capturedAt.Format(time.RFC3339) || point.NetLiquidation.Currency != "CHF" || !point.NetLiquidation.Available || point.Cash.Available || len(point.Warnings) != 1 {
		t.Fatalf("unexpected history point mapping: %+v", point)
	}
}
