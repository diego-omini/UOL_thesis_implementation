package main

// This file verifies application-level gateway mapping and paced portfolio-history synchronization.

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wails/persistence"
	"wails/portfoliohistory"
)

type stubPortfolioHistoryProvider struct {
	history portfoliohistory.NAVHistory
	err     error
	calls   int
	period  string
}

type stubSnapTradeHistoryProvider struct {
	history portfoliohistory.NAVHistory
	err     error
	calls   int
}

func (s *stubSnapTradeHistoryProvider) Configured() bool { return true }
func (s *stubSnapTradeHistoryProvider) ConnectionPortalURL(context.Context) (string, error) {
	return "https://connect.snaptrade.test", nil
}
func (s *stubSnapTradeHistoryProvider) FetchPortfolioNAVHistory(context.Context, string) (portfoliohistory.NAVHistory, error) {
	s.calls++
	return s.history, s.err
}

func (s *stubPortfolioHistoryProvider) FetchPortfolioNAVHistory(_ context.Context, _, period string) (portfoliohistory.NAVHistory, error) {
	s.calls++
	s.period = period
	return s.history, s.err
}

func TestGatewayLoginURL(t *testing.T) {
	got, err := gatewayLoginURL("https://localhost:5000/v1/api")
	if err != nil {
		t.Fatalf("gatewayLoginURL() returned an error: %v", err)
	}
	if got != "https://localhost:5000/" {
		t.Fatalf("gatewayLoginURL() = %q", got)
	}
}

func TestSyncPortfolioNAVHistoryPersistsOnceInsidePacingWindow(t *testing.T) {
	store, err := persistence.Open(filepath.Join(t.TempDir(), "application.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	provider := &stubPortfolioHistoryProvider{history: portfoliohistory.NAVHistory{
		AccountID: "DU-1", Frequency: "D", RetrievedAt: now,
		Points: []portfoliohistory.NAVObservation{{
			Date: now.AddDate(0, 0, -1), Value: 1000, BaseCurrency: "CHF", Source: portfoliohistory.SourceIBKRPortfolioAnalyst,
		}},
	}}
	app := &App{
		ctx: context.Background(), store: store, portfolioHistory: provider,
		historyImports: make(map[string]historyImportState),
	}
	if state := app.syncPortfolioNAVHistory(context.Background(), "DU-1", now); state.warning != "" {
		t.Fatalf("unexpected import warning: %s", state.warning)
	}
	if state := app.syncPortfolioNAVHistory(context.Background(), "DU-1", now.Add(time.Minute)); state.warning != "" {
		t.Fatalf("unexpected cached import warning: %s", state.warning)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if provider.period != "1Y" {
		t.Fatalf("provider period = %q, want 1Y", provider.period)
	}
	result, err := store.QueryPortfolioHistory(context.Background(), "DU-1", portfoliohistory.Query{
		From: now.AddDate(0, 0, -2), To: now, MaxPoints: 20,
	})
	if err != nil || result.OfficialPointCount != 1 {
		t.Fatalf("official history was not persisted: result=%+v err=%v", result, err)
	}
}

func TestPortfolioHistoryStartUsesCalendarYear(t *testing.T) {
	to := time.Date(2025, 3, 1, 12, 30, 0, 0, time.UTC)
	from, err := portfolioHistoryStart("1y", to)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2024, 3, 1, 12, 30, 0, 0, time.UTC)
	if !from.Equal(want) {
		t.Fatalf("portfolioHistoryStart() = %s, want %s", from, want)
	}
}

func TestSafeHistoryErrorRedactsAccountID(t *testing.T) {
	got := safeHistoryError(errors.New("account DU-PRIVATE could not be loaded"), "du-private")
	if strings.Contains(strings.ToLower(got), "du-private") {
		t.Fatalf("account ID reached the safe error: %q", got)
	}
}

func TestSyncPortfolioNAVHistoryFallsBackToSnapTrade(t *testing.T) {
	store, err := persistence.Open(filepath.Join(t.TempDir(), "application.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	external := &stubSnapTradeHistoryProvider{history: portfoliohistory.NAVHistory{
		AccountID: "DU-1", Frequency: "D", RetrievedAt: now,
		Points: []portfoliohistory.NAVObservation{{
			Date: now.AddDate(0, 0, -1), Value: 1200, BaseCurrency: "CHF", Source: portfoliohistory.SourceSnapTradeBalanceHistory,
		}},
	}}
	app := &App{
		ctx: context.Background(), store: store,
		portfolioHistory: &stubPortfolioHistoryProvider{err: errors.New("PortfolioAnalyst unavailable")},
		snapTradeHistory: external, historyImports: make(map[string]historyImportState),
	}
	state := app.syncPortfolioNAVHistory(context.Background(), "DU-1", now)
	if state.connectionRequired || external.calls != 1 || state.warning == "" {
		t.Fatalf("unexpected fallback state: %+v calls=%d", state, external.calls)
	}
	result, err := store.QueryPortfolioHistory(context.Background(), "DU-1", portfoliohistory.Query{
		From: now.AddDate(0, 0, -2), To: now, MaxPoints: 20,
	})
	if err != nil || result.EstimatedPointCount != 1 {
		t.Fatalf("estimated history was not persisted: result=%+v err=%v", result, err)
	}
}

func TestBeginOperationCancelsSupersededRequest(t *testing.T) {
	app := &App{ctx: context.Background(), operations: make(map[string]operationState)}
	first, finishFirst := app.beginOperation("counselor", time.Minute)
	defer finishFirst()
	second, finishSecond := app.beginOperation("counselor", time.Minute)
	defer finishSecond()
	select {
	case <-first.Done():
	default:
		t.Fatal("superseded operation was not cancelled")
	}
	if second.Err() != nil {
		t.Fatalf("replacement operation was cancelled: %v", second.Err())
	}
}
