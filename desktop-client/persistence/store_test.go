package persistence

// This file verifies migrations and sanitized storage for every focused persistence use case.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wails/ai"
	"wails/analytics"
	"wails/broker"
	"wails/evaluation"
	"wails/portfolio"
	"wails/trading"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "application.db"))
	if err != nil {
		t.Fatalf("Open returned an error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestOpenMigrationIsIdempotentAndPreservesExistingRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "application.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record := PortfolioSnapshotRecord{
		SchemaVersion: SchemaVersion, CapturedAt: time.Now().UTC(), PositionCount: 0,
		SnapshotHash: "existing", SnapshotJSON: `{}`, AnalyticsJSON: `{}`,
	}
	if err := store.db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	defer reopened.Close()
	var count int64
	if err := reopened.db.Model(&PortfolioSnapshotRecord{}).Where("snapshot_hash = ?", "existing").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("existing rows after repeated migration = %d, want 1", count)
	}
}

func TestSavePortfolioSnapshotRemovesAccountIdentity(t *testing.T) {
	store := openTestStore(t)
	capturedAt := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	snapshot := portfolio.Portfolio{
		AccountSummary: portfolio.AccountSummary{
			AccountID:      "DU-SENSITIVE-123",
			NetLiquidation: portfolio.Money{Amount: 1000, Currency: "CHF", Available: true},
			CashBalance:    portfolio.Money{Amount: 200, Currency: "CHF", Available: true},
		},
		Positions: []portfolio.Position{{
			Instrument:    portfolio.Instrument{ContractID: 42, Symbol: "TEST", AssetClass: "STK", Currency: "CHF", Sector: "Technology"},
			MarketPrice:   portfolio.Money{Amount: 80, Currency: "CHF", Available: true},
			MarketValue:   portfolio.Money{Amount: 800, Currency: "CHF", Available: true},
			UnrealizedPnL: portfolio.Money{Amount: 50, Currency: "CHF", Available: true},
		}},
	}
	result := analytics.Calculate(snapshot, capturedAt)
	if err := store.SavePortfolioSnapshot(context.Background(), snapshot, result); err != nil {
		t.Fatalf("SavePortfolioSnapshot returned an error: %v", err)
	}
	var record PortfolioSnapshotRecord
	if err := store.db.First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if record.AccountFingerprint == "" || strings.Contains(record.AccountFingerprint, snapshot.AccountSummary.AccountID) {
		t.Fatalf("account fingerprint was not sanitized: %q", record.AccountFingerprint)
	}
	if strings.Contains(record.SnapshotJSON, snapshot.AccountSummary.AccountID) || strings.Contains(record.AnalyticsJSON, snapshot.AccountSummary.AccountID) {
		t.Fatal("raw account identifier was persisted")
	}
	if record.PositionCount != 1 || record.BaseCurrency != "CHF" || record.SnapshotHash == "" {
		t.Fatalf("unexpected snapshot record: %+v", record)
	}
}

func TestRecordPromptExecutionRemovesSecretsAndTokens(t *testing.T) {
	store := openTestStore(t)
	execution := ai.PromptExecution{
		StartedAt: time.Now(), CompletedAt: time.Now(), Provider: "ollama", Model: "test",
		PromptID: "portfolio_resume", PromptVersion: 3,
		Parameters: map[string]any{
			"account_id": "DU-SECRET", "analysis_timestamp": "2026-09-26T12:00:00Z",
			"nested": map[string]any{"preview_token": "opaque-secret", "value": 4},
		},
		ModelSettings: ai.GenerationSettings{Temperature: 0, MaxTokens: 2800},
		RawResponse:   `{"invalid":true}`, RepairResponse: `{"schema_version":1}`,
		ResponseText:     `{"schema_version":1}`,
		ParsedResponse:   &ai.CounselorResponse{SchemaVersion: 1, Summary: "summary"},
		ValidationResult: ai.CounselorValidationResult{Valid: true, InitialErrors: []string{"unknown field"}},
		RepairStatus:     "succeeded",
	}
	if err := store.RecordPromptExecution(context.Background(), execution); err != nil {
		t.Fatalf("RecordPromptExecution returned an error: %v", err)
	}
	var record PromptExecutionRecord
	if err := store.db.First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(record.ParametersJSON, "DU-SECRET") || strings.Contains(record.ParametersJSON, "opaque-secret") {
		t.Fatalf("sensitive prompt fields were persisted: %s", record.ParametersJSON)
	}
	var parameters map[string]any
	if err := json.Unmarshal([]byte(record.ParametersJSON), &parameters); err != nil || parameters["analysis_timestamp"] == nil {
		t.Fatalf("sanitized parameters are invalid: %v %v", parameters, err)
	}
	if record.PromptVersion != 3 || record.RawResponseText == "" || record.RepairResponseText == "" || record.ParsedResponseJSON == "" || record.ValidationResultJSON == "" || record.ModelSettingsJSON == "" || record.RepairStatus != "succeeded" {
		t.Fatalf("prompt execution audit fields were not persisted: %+v", record)
	}
}

func TestRecordFocusedWorkflowExecutionMetadata(t *testing.T) {
	store := openTestStore(t)
	promptIDs := []string{"currency_exposure", "effective_diversification", "portfolio_history_summary", "position_risk", "portfolio_data_quality"}
	for _, promptID := range promptIDs {
		execution := ai.PromptExecution{
			StartedAt: time.Now(), CompletedAt: time.Now(), Provider: "ollama", Model: "test-model",
			PromptID: promptID, PromptVersion: 1, Parameters: map[string]any{"facts": []any{}},
			ModelSettings: ai.GenerationSettings{Temperature: 0, MaxTokens: 2400},
			ResponseText:  `{"schema_version":1}`, RepairStatus: "not_attempted",
			ValidationResult: ai.CounselorValidationResult{Valid: true},
		}
		if err := store.RecordPromptExecution(context.Background(), execution); err != nil {
			t.Fatalf("RecordPromptExecution(%s) error: %v", promptID, err)
		}
	}
	var records []PromptExecutionRecord
	if err := store.db.Order("id").Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	if len(records) != len(promptIDs) {
		t.Fatalf("prompt execution record count = %d, want %d", len(records), len(promptIDs))
	}
	for index, record := range records {
		if record.PromptID != promptIDs[index] || record.PromptVersion != 1 || record.Provider != "ollama" || record.Model != "test-model" || record.Status != "success" || record.ModelSettingsJSON == "" {
			t.Fatalf("unexpected focused workflow audit record: %+v", record)
		}
	}
}

func TestSaveEvaluationBundleAndOrderAudit(t *testing.T) {
	store := openTestStore(t)
	createdAt := time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)
	bundle := evaluation.Bundle{
		CreatedAt:   createdAt,
		Manifest:    evaluation.Manifest{SuiteID: "suite", Version: 1, Model: "llama3.2"},
		Environment: evaluation.Environment{Model: "llama3.2", ModelDigest: "sha256:model"},
		Runs:        []evaluation.RunResult{{Sequence: 1}},
	}
	if err := store.SaveEvaluationBundle(context.Background(), bundle); err != nil {
		t.Fatalf("SaveEvaluationBundle returned an error: %v", err)
	}
	event := trading.AuditEvent{
		OccurredAt: createdAt, EventType: trading.AuditSubmissionAttempted,
		AccountID: "DU-PRIVATE", Instrument: portfolio.Instrument{ContractID: 42, Symbol: "TEST", Name: "Test Corp"},
		Side: broker.OrderSideBuy, Quantity: 2,
		Total: portfolio.Money{Amount: 20, Currency: "USD", Available: true},
	}
	if err := store.RecordOrderAudit(context.Background(), event); err != nil {
		t.Fatalf("RecordOrderAudit returned an error: %v", err)
	}
	var evaluationRecord EvaluationOutputRecord
	if err := store.db.First(&evaluationRecord).Error; err != nil {
		t.Fatal(err)
	}
	if evaluationRecord.RunCount != 1 || evaluationRecord.ModelDigest != "sha256:model" {
		t.Fatalf("unexpected evaluation record: %+v", evaluationRecord)
	}
	var orderRecord OrderAuditRecord
	if err := store.db.First(&orderRecord).Error; err != nil {
		t.Fatal(err)
	}
	if orderRecord.AccountFingerprint == "" || strings.Contains(orderRecord.AccountFingerprint, "DU-PRIVATE") || orderRecord.EventType != trading.AuditSubmissionAttempted {
		t.Fatalf("unexpected order audit record: %+v", orderRecord)
	}
}
