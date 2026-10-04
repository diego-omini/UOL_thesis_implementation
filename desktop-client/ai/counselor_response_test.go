package ai

// This file verifies strict counselor response decoding, grounding, safety, citations, and repair.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type citationTestTool struct {
	accepted map[string]bool
}

func (t citationTestTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "research", Description: "Research fixture.", Parameters: map[string]any{"type": "object"}}
}

func (citationTestTool) Execute(context.Context, map[string]any) (string, error) {
	return "", nil
}

func (t citationTestTool) AcceptsCitation(rawURL string) bool {
	return t.accepted[rawURL]
}

func counselorTestFacts() []CounselorFact {
	value := 42.0
	return []CounselorFact{
		{ID: "portfolio.value", Kind: string(CategoryDeterministicDerivedFact), Label: "Portfolio value", NumericValue: &value, Unit: "USD", Available: true, AbsoluteTolerance: .01},
		{ID: "warning.missing_sector", Kind: string(CategoryUnavailableInformation), Label: "Missing sector", Available: false, Limitation: "Sector data is incomplete.", RequiredLimitation: true},
	}
}

func validCounselorTestResponse() CounselorResponse {
	return CounselorResponse{
		SchemaVersion: 1,
		Summary:       "The deterministic value is reported with incomplete sector coverage.",
		Observations: []CounselorObservation{{
			Category: CategoryDeterministicDerivedFact, Statement: "Portfolio value is 42 USD.",
			SupportingFactIDs: []string{"portfolio.value"},
			ReportedValues:    []ReportedValue{{FactID: "portfolio.value", Value: 42, Unit: "USD"}},
			Severity:          SeverityInformational, Confidence: ConfidenceHigh,
			Limitations: []string{"Sector coverage is incomplete."}, Explanation: "This repeats the application-supplied deterministic metric.", Citations: []CounselorCitation{},
		}},
		Limitations: []CounselorLimitation{{Statement: "Sector data is incomplete.", FactIDs: []string{"warning.missing_sector"}}},
	}
}

func encodeCounselorTestResponse(t *testing.T, response CounselorResponse) string {
	t.Helper()
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestCounselorResponseAcceptsValidGroundedOutput(t *testing.T) {
	parsed, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, validCounselorTestResponse()), counselorTestFacts(), nil)
	if len(errors) != 0 || parsed == nil || parsed.Observations[0].ReportedValues[0].Value != 42 {
		t.Fatalf("valid response was rejected: parsed=%+v errors=%v", parsed, errors)
	}
}

func TestCounselorResponseRejectsUnknownFields(t *testing.T) {
	raw := strings.TrimSuffix(encodeCounselorTestResponse(t, validCounselorTestResponse()), "}") + `,"unexpected":true}`
	if _, errors := decodeAndValidateCounselorResponse(raw, counselorTestFacts(), nil); len(errors) == 0 || !strings.Contains(errors[0], "unknown field") {
		t.Fatalf("unknown field was not rejected: %v", errors)
	}
}

func TestCounselorResponseRejectsUnknownFactID(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations[0].SupportingFactIDs = []string{"portfolio.unknown"}
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "unknown fact") {
		t.Fatalf("unknown fact was not rejected: %v", errors)
	}
}

func TestCounselorResponseRejectsNumericalContradiction(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations[0].ReportedValues[0].Value = 43
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "supplied value") {
		t.Fatalf("numeric contradiction was not rejected: %v", errors)
	}
}

func TestCounselorResponseRejectsMissingRequiredLimitation(t *testing.T) {
	response := validCounselorTestResponse()
	response.Limitations[0].FactIDs = []string{}
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "required limitation") {
		t.Fatalf("missing required limitation was not rejected: %v", errors)
	}
}

func TestCounselorResponseRejectsUnsafeAuthorityClaim(t *testing.T) {
	response := validCounselorTestResponse()
	response.Summary = "I submitted the order for the user."
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "trading authority") {
		t.Fatalf("unsafe authority claim was not rejected: %v", errors)
	}
}

func TestCounselorResponseRejectsPrescriptiveAndOrderLikeGuidance(t *testing.T) {
	for _, statement := range []string{
		"You should buy more shares.",
		"Use a target allocation for this holding.",
		"Replace Fund A with Fund B.",
		"Next, place an order after reviewing the risk.",
	} {
		response := validCounselorTestResponse()
		response.Summary = statement
		if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "prohibited prescriptive") {
			t.Fatalf("prescriptive guidance %q was not rejected: %v", statement, errors)
		}
	}
}

func TestCounselorResponseAllowsSafetyLimitations(t *testing.T) {
	response := validCounselorTestResponse()
	response.Summary = "This analysis does not set a target allocation and cannot place an order."
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); len(errors) != 0 {
		t.Fatalf("safety limitation was rejected: %v", errors)
	}
}

func TestCounselorResponseRejectsCitationOutsideResearchBoundary(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations[0].Category = CategorySourcedExternalEvidence
	response.Observations[0].Citations = []CounselorCitation{{Title: "Unaccepted", URL: "https://example.com/report"}}
	tools := map[string]Tool{"research": citationTestTool{accepted: map[string]bool{"https://sec.gov/accepted": true}}}
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), tools); !containsValidationError(errors, "outside the accepted research boundary") {
		t.Fatalf("invalid citation was not rejected: %v", errors)
	}
}

func TestCounselorExecutorRepairsOnceAndPreservesBothResponses(t *testing.T) {
	valid := encodeCounselorTestResponse(t, validCounselorTestResponse())
	provider := &recordingProvider{responses: []ChatResponse{
		{Message: Message{Role: "assistant", Content: `{"schema_version":1,"unknown":true}`}, Model: "test-model"},
		{Message: Message{Role: "assistant", Content: valid}, Model: "test-model"},
	}}
	definition := PromptDefinition{
		ID: "structured_test", Version: 1, Description: "Structured test.",
		Parameters:     []ParameterDefinition{{Name: "facts", Type: "list", Required: true, Description: "Facts."}},
		SystemTemplate: "System", UserTemplate: "{{range .facts}}{{.ID}}{{end}}",
		Response: ResponseSettings{Format: "json", Schema: CounselorResponseSchema, MaxCharacters: 5000, MaxTokens: 100},
	}
	catalog := &Catalog{
		prompts: map[promptKey]PromptDefinition{{id: definition.ID, version: definition.Version}: definition},
		aliases: map[string]PromptReference{"structured_test": {ID: definition.ID, Version: definition.Version}},
	}
	recorder := &promptExecutionRecorder{}
	executor := NewExecutor(catalog, provider)
	executor.SetExecutionRecorder("ollama", recorder)
	facts := counselorTestFacts()
	response, err := executor.ExecuteCounselor(context.Background(), "structured_test", map[string]any{"facts": facts}, nil, facts)
	if err != nil {
		t.Fatalf("ExecuteCounselor returned an error: %v", err)
	}
	if response.Structured == nil || len(provider.requests) != 2 || provider.requests[1].ResponseSchema == nil {
		t.Fatalf("structured repair did not complete: response=%+v requests=%+v", response, provider.requests)
	}
	if len(recorder.records) != 1 {
		t.Fatalf("record count = %d, want 1", len(recorder.records))
	}
	record := recorder.records[0]
	if record.RawResponse == "" || record.RepairResponse != valid || record.RepairStatus != "succeeded" || !record.ValidationResult.Valid || len(record.ValidationResult.InitialErrors) == 0 {
		t.Fatalf("repair audit fields were not preserved: %+v", record)
	}
}

func TestCounselorExecutorRejectsStillInvalidRepairWithoutThirdAttempt(t *testing.T) {
	provider := &recordingProvider{responses: []ChatResponse{
		{Message: Message{Role: "assistant", Content: `{"schema_version":1,"unknown":true}`}, Model: "test-model"},
		{Message: Message{Role: "assistant", Content: `{"schema_version":1,"still_unknown":true}`}, Model: "test-model"},
	}}
	definition := PromptDefinition{
		ID: "structured_failure", Version: 1, Description: "Structured failure test.",
		Parameters:     []ParameterDefinition{{Name: "facts", Type: "list", Required: true, Description: "Facts."}},
		SystemTemplate: "System", UserTemplate: "{{range .facts}}{{.ID}}{{end}}",
		Response: ResponseSettings{Format: "json", Schema: CounselorResponseSchema, MaxCharacters: 5000, MaxTokens: 100},
	}
	catalog := &Catalog{
		prompts: map[promptKey]PromptDefinition{{id: definition.ID, version: definition.Version}: definition},
		aliases: map[string]PromptReference{"structured_failure": {ID: definition.ID, Version: definition.Version}},
	}
	recorder := &promptExecutionRecorder{}
	executor := NewExecutor(catalog, provider)
	executor.SetExecutionRecorder("ollama", recorder)
	facts := counselorTestFacts()
	_, err := executor.ExecuteCounselor(context.Background(), "structured_failure", map[string]any{"facts": facts}, nil, facts)
	if err == nil || !strings.Contains(err.Error(), "after one repair attempt") || len(provider.requests) != 2 {
		t.Fatalf("invalid repair was not rejected after one attempt: err=%v requests=%d", err, len(provider.requests))
	}
	if len(recorder.records) != 1 || recorder.records[0].RepairStatus != "failed" || len(recorder.records[0].ValidationResult.FinalErrors) == 0 {
		t.Fatalf("failed repair audit was not retained: %+v", recorder.records)
	}
}

func containsValidationError(errors []string, fragment string) bool {
	for _, value := range errors {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}
