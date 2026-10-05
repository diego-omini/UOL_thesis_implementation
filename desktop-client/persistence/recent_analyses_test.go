package persistence

// This file verifies bounded recent-analysis queries and their sensitive-field boundary.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"wails/ai"
)

func TestQueryRecentAnalysesReturnsOnlyValidatedSafeProjection(t *testing.T) {
	store := openTestStore(t)
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	response := ai.CounselorResponse{
		SchemaVersion: 1, Summary: "Validated summary",
		Observations: []ai.CounselorObservation{{
			Category: ai.CategoryInterpretation, Statement: "Measured observation", Explanation: "Grounded explanation",
			Severity: ai.SeverityInformational, Confidence: ai.ConfidenceHigh,
			SupportingFactIDs: []string{}, ReportedValues: []ai.ReportedValue{}, Limitations: []string{}, Citations: []ai.CounselorCitation{},
		}},
		Limitations: []ai.CounselorLimitation{{Statement: "Saved data may no longer be current.", FactIDs: []string{}}},
	}
	parsed, _ := json.Marshal(response)
	validation, _ := json.Marshal(ai.CounselorValidationResult{Valid: true})
	record := PromptExecutionRecord{
		SchemaVersion: SchemaVersion, StartedAt: now.Add(-time.Second), CompletedAt: now, ContextAsOf: now.Add(-time.Minute),
		Provider: "ollama", Model: "local-model", PromptID: "portfolio_resume", PromptVersion: 3, Status: "success",
		ParametersJSON: `{"account_id":"DU-PRIVATE","token":"secret-token"}`,
		SystemPrompt:   "private rendered system prompt", UserPrompt: "private rendered user prompt",
		TranscriptJSON: `[{"content":"private tool transcript"}]`, RawResponseText: "private raw output",
		ParsedResponseJSON: string(parsed), ValidationResultJSON: string(validation),
	}
	if err := store.db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}

	analyses, err := store.QueryRecentAnalyses(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(analyses) != 1 || analyses[0].PromptID != "portfolio_resume" || analyses[0].Response.Summary != "Validated summary" || !analyses[0].ContextAsOf.Equal(now.Add(-time.Minute)) {
		t.Fatalf("unexpected recent analysis: %+v", analyses)
	}
	exposed, _ := json.Marshal(analyses)
	for _, secret := range []string{"DU-PRIVATE", "secret-token", "rendered system", "rendered user", "tool transcript", "raw output"} {
		if strings.Contains(string(exposed), secret) {
			t.Fatalf("recent analysis exposed sensitive execution material %q: %s", secret, exposed)
		}
	}
}

func TestQueryRecentAnalysesIsBoundedAndSkipsInvalidRows(t *testing.T) {
	store := openTestStore(t)
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	validation, _ := json.Marshal(ai.CounselorValidationResult{Valid: true})
	for index := 0; index < maxRecentAnalyses+3; index++ {
		response := validStoredResponse(t, index)
		if err := store.db.Create(&PromptExecutionRecord{
			SchemaVersion: SchemaVersion, StartedAt: now, CompletedAt: now.Add(time.Duration(index) * time.Minute),
			PromptID: "portfolio_resume", PromptVersion: 3, Status: "success",
			ParsedResponseJSON: response, ValidationResultJSON: string(validation),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := store.db.Create(&PromptExecutionRecord{
		SchemaVersion: SchemaVersion, StartedAt: now, CompletedAt: now.Add(24 * time.Hour),
		PromptID: "portfolio_resume", PromptVersion: 3, Status: "success",
		ParsedResponseJSON: `{"schema_version":1}`, ValidationResultJSON: string(validation),
	}).Error; err != nil {
		t.Fatal(err)
	}

	analyses, err := store.QueryRecentAnalyses(context.Background(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(analyses) != maxRecentAnalyses {
		t.Fatalf("analysis count = %d, want %d", len(analyses), maxRecentAnalyses)
	}
}

func validStoredResponse(t *testing.T, index int) string {
	t.Helper()
	contents, err := json.Marshal(ai.CounselorResponse{
		SchemaVersion: 1, Summary: "Summary",
		Observations: []ai.CounselorObservation{{
			Category: ai.CategoryGeneralInformation, Statement: "Observation", Explanation: "Explanation",
			Severity: ai.SeverityInformational, Confidence: ai.ConfidenceModerate,
			SupportingFactIDs: []string{}, ReportedValues: []ai.ReportedValue{}, Limitations: []string{}, Citations: []ai.CounselorCitation{},
		}},
		Limitations: []ai.CounselorLimitation{{Statement: "Limitation", FactIDs: []string{}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
