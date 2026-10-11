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

func TestCounselorResponseNormalizesFactReferencesAndKnownUnitAliases(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations[0].SupportingFactIDs = []string{"id=portfolio.value | kind=deterministic_derived_fact"}
	response.Observations[0].ReportedValues[0].Unit = "%"
	facts := counselorTestFacts()
	facts[0].Unit = "percent"
	response.Limitations[0].FactIDs = []string{"id=warning.missing_sector | available=false"}
	parsed, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), facts, nil)
	if len(errors) != 0 || parsed == nil {
		t.Fatalf("representation-only mistakes were not normalized: parsed=%+v errors=%v", parsed, errors)
	}
	if parsed.Observations[0].SupportingFactIDs[0] != "portfolio.value" || parsed.Observations[0].ReportedValues[0].Unit != "percent" {
		t.Fatalf("unexpected normalized response: %+v", parsed)
	}
}

func TestCounselorResponseRejectsReportedNumericFactWithoutSupport(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations[0].SupportingFactIDs = nil
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "without listing it as support") {
		t.Fatalf("unsupported reported value was not rejected: %v", errors)
	}
}

func TestCounselorResponseRejectsDirectNumericFactWithoutReportedValue(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations[0].ReportedValues = nil
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "without reporting its value and unit") {
		t.Fatalf("unreported direct numeric fact was not rejected: %v", errors)
	}
}

func TestCounselorResponseRejectsUnavailableFactReportedAsNumeric(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations[0].SupportingFactIDs = append(response.Observations[0].SupportingFactIDs, "warning.missing_sector")
	response.Observations[0].ReportedValues = append(response.Observations[0].ReportedValues, ReportedValue{FactID: "warning.missing_sector", Value: 0, Unit: "count"})
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "unavailable or non-numeric") {
		t.Fatalf("unavailable numeric value was not rejected: %v", errors)
	}
}

func TestCounselorResponseRejectsUnknownLimitationFactID(t *testing.T) {
	response := validCounselorTestResponse()
	response.Limitations[0].FactIDs = []string{"warning.invented"}
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "references unknown fact") {
		t.Fatalf("invented limitation fact was not rejected: %v", errors)
	}
}

func TestCounselorResponseRejectsUnknownFields(t *testing.T) {
	raw := strings.TrimSuffix(encodeCounselorTestResponse(t, validCounselorTestResponse()), "}") + `,"unexpected":true}`
	if _, errors := decodeAndValidateCounselorResponse(raw, counselorTestFacts(), nil); len(errors) == 0 || !strings.Contains(errors[0], "unknown field") {
		t.Fatalf("unknown field was not rejected: %v", errors)
	}
}

func TestCounselorResponseRejectsInvalidEnums(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CounselorResponse)
		want   string
	}{
		{name: "category", mutate: func(response *CounselorResponse) { response.Observations[0].Category = "fact|interpretation" }, want: "invalid category"},
		{name: "severity", mutate: func(response *CounselorResponse) { response.Observations[0].Severity = "medium" }, want: "invalid severity"},
		{name: "confidence", mutate: func(response *CounselorResponse) { response.Observations[0].Confidence = "certain" }, want: "invalid confidence"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := validCounselorTestResponse()
			test.mutate(&response)
			if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, test.want) {
				t.Fatalf("invalid enum was not rejected: %v", errors)
			}
		})
	}
}

func TestCounselorResponseRejectsExcessObservationCount(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations = make([]CounselorObservation, maxCounselorObservations+1)
	for index := range response.Observations {
		response.Observations[index] = validCounselorTestResponse().Observations[0]
	}
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "observations exceed") {
		t.Fatalf("oversized observation set was not rejected: %v", errors)
	}
}

func TestCounselorResponseEnforcesWorkflowObservationLimit(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations = append(response.Observations, response.Observations[0])
	if _, errors := decodeAndValidateCounselorResponseWithLimit(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil, 1); !containsValidationError(errors, "maximum of 1") {
		t.Fatalf("workflow observation limit was not enforced: %v", errors)
	}
}

func TestCounselorResponseRejectsUnknownFactID(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations[0].SupportingFactIDs = []string{"portfolio.unknown"}
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "unknown fact") {
		t.Fatalf("unknown fact was not rejected: %v", errors)
	}
}

func TestCounselorResponseRejectsMissingRequiredObservationCoverage(t *testing.T) {
	response := validCounselorTestResponse()
	facts := counselorTestFacts()
	facts = append(facts, CounselorFact{ID: "instrument.example.symbol", Kind: string(CategorySuppliedFact), Label: "Ticker", TextValue: "EX", Available: true, RequiredObservation: true})
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), facts, nil); !containsValidationError(errors, "required observation coverage") {
		t.Fatalf("missing required observation coverage was not rejected: %v", errors)
	}
}

func TestCounselorResponseEnforcesOneOrderedObservationPerRequiredFact(t *testing.T) {
	facts := append(counselorTestFacts(),
		CounselorFact{ID: "instrument.first.symbol", Kind: string(CategorySuppliedFact), Label: "First ticker", TextValue: "ONE", Available: true, RequiredObservation: true, ObservationOrder: 1},
		CounselorFact{ID: "instrument.second.symbol", Kind: string(CategorySuppliedFact), Label: "Second ticker", TextValue: "TWO", Available: true, RequiredObservation: true, ObservationOrder: 2},
	)
	observation := func(id string) CounselorObservation {
		return CounselorObservation{Category: CategorySuppliedFact, Statement: "Identity coverage.", SupportingFactIDs: []string{id}, ReportedValues: []ReportedValue{}, Severity: SeverityInformational, Confidence: ConfidenceHigh, Limitations: []string{}, Explanation: "Covers the supplied public identity.", Citations: []CounselorCitation{}}
	}

	t.Run("combined", func(t *testing.T) {
		response := validCounselorTestResponse()
		combined := observation("instrument.first.symbol")
		combined.SupportingFactIDs = append(combined.SupportingFactIDs, "instrument.second.symbol")
		response.Observations = append(response.Observations, combined)
		if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), facts, nil); !containsValidationError(errors, "combines multiple required-observation facts") {
			t.Fatalf("combined required coverage was not rejected: %v", errors)
		}
	})

	t.Run("out of order", func(t *testing.T) {
		response := validCounselorTestResponse()
		response.Observations = append(response.Observations, observation("instrument.second.symbol"), observation("instrument.first.symbol"))
		if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), facts, nil); !containsValidationError(errors, "not covered in supplied order") {
			t.Fatalf("out-of-order required coverage was not rejected: %v", errors)
		}
	})

	t.Run("duplicated", func(t *testing.T) {
		response := validCounselorTestResponse()
		response.Observations = append(response.Observations, observation("instrument.first.symbol"), observation("instrument.first.symbol"), observation("instrument.second.symbol"))
		if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), facts, nil); !containsValidationError(errors, "covered more than once") {
			t.Fatalf("duplicate required coverage was not rejected: %v", errors)
		}
	})
}

func TestCounselorResponseRejectsNumericalContradiction(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations[0].ReportedValues[0].Value = 43
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "supplied value") {
		t.Fatalf("numeric contradiction was not rejected: %v", errors)
	}
}

func TestCounselorResponseAddsMissingRequiredLimitationFromBackendFact(t *testing.T) {
	response := validCounselorTestResponse()
	response.Limitations[0].FactIDs = []string{}
	parsed, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil)
	if len(errors) != 0 || parsed == nil || len(parsed.Limitations) != 2 || parsed.Limitations[1].FactIDs[0] != "warning.missing_sector" {
		t.Fatalf("backend-required limitation was not appended: parsed=%+v errors=%v", parsed, errors)
	}
}

func TestCounselorResponseGroupsEqualRequiredLimitations(t *testing.T) {
	response := validCounselorTestResponse()
	response.Limitations = []CounselorLimitation{{Statement: "Scope limitation.", FactIDs: []string{}}}
	facts := counselorTestFacts()
	facts = append(facts, CounselorFact{ID: "warning.missing_industry", Kind: string(CategoryUnavailableInformation), Label: "Missing industry", Available: false, Limitation: "Sector data is incomplete.", RequiredLimitation: true})
	parsed, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), facts, nil)
	if len(errors) != 0 || parsed == nil || len(parsed.Limitations) != 2 || len(parsed.Limitations[1].FactIDs) != 2 {
		t.Fatalf("equal required limitations were not grouped: parsed=%+v errors=%v", parsed, errors)
	}
}

func TestCounselorResponseAllowsMissingResearchForKnownIdentity(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations[0] = CounselorObservation{
		Category: CategoryUnavailableInformation, Statement: "No accepted recent evidence was found.",
		SupportingFactIDs: []string{"instrument.test.symbol"}, ReportedValues: []ReportedValue{},
		Severity: SeverityLimited, Confidence: ConfidenceLow,
		Limitations: []string{"The bounded search returned no accepted source."}, Explanation: "The identity was researched without substituting a weak source.", Citations: []CounselorCitation{},
	}
	facts := counselorTestFacts()
	facts = append(facts, CounselorFact{ID: "instrument.test.symbol", Kind: string(CategorySuppliedFact), Label: "Ticker", TextValue: "TEST", Available: true})
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), facts, nil); len(errors) != 0 {
		t.Fatalf("disclosed unavailable research was rejected: %v", errors)
	}
}

func TestCounselorResponseRejectsCategoryMixing(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CounselorObservation)
		want   string
	}{
		{name: "general facts", mutate: func(observation *CounselorObservation) { observation.Category = CategoryGeneralInformation }, want: "general information"},
		{name: "application fact citation", mutate: func(observation *CounselorObservation) {
			observation.Citations = []CounselorCitation{{Title: "Accepted", URL: "https://sec.gov/accepted"}}
		}, want: "mixes an application fact category"},
		{name: "external reported value", mutate: func(observation *CounselorObservation) {
			observation.Category = CategorySourcedExternalEvidence
			observation.Citations = []CounselorCitation{{Title: "Accepted", URL: "https://sec.gov/accepted"}}
		}, want: "external or unavailable category"},
	}
	tools := map[string]Tool{"research": citationTestTool{accepted: map[string]bool{"https://sec.gov/accepted": true}}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := validCounselorTestResponse()
			test.mutate(&response.Observations[0])
			if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), tools); !containsValidationError(errors, test.want) {
				t.Fatalf("category mixing was not rejected: %v", errors)
			}
		})
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
		"You should not sell this position.",
		"It would be prudent to trim this holding.",
		"This stock looks like a buy.",
		"Use a target allocation for this holding.",
		"Replace Fund A with Fund B.",
		"Next, place an order after reviewing the risk.",
		"Buy AAPL after the next earnings release.",
		"Allocate this position to 10% of the portfolio.",
	} {
		response := validCounselorTestResponse()
		response.Summary = statement
		if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "prohibited prescriptive") {
			t.Fatalf("prescriptive guidance %q was not rejected: %v", statement, errors)
		}
	}
}

func TestCounselorResponseRejectsBroaderTradingAuthorityClaim(t *testing.T) {
	response := validCounselorTestResponse()
	response.Summary = "I can trade and rebalance the portfolio."
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "trading authority") {
		t.Fatalf("broader trading-authority claim was not rejected: %v", errors)
	}
}

func TestCounselorFactsRejectAmbiguousOrDuplicateInputs(t *testing.T) {
	value := 2.0
	for _, test := range []struct {
		name  string
		facts []CounselorFact
		want  string
	}{
		{name: "duplicate", facts: []CounselorFact{textFact("test.id", CategorySuppliedFact, "Test", "one"), textFact("test.id", CategorySuppliedFact, "Test", "two")}, want: "duplicated"},
		{name: "missing unit", facts: []CounselorFact{{ID: "test.number", Kind: string(CategorySuppliedFact), Label: "Number", NumericValue: &value, Available: true}}, want: "finite value and unit"},
		{name: "unavailable numeric", facts: []CounselorFact{{ID: "test.unavailable", Kind: string(CategoryUnavailableInformation), Label: "Unavailable", NumericValue: &value, Unit: "count", Available: false, Limitation: "Missing."}}, want: "no numeric value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateCounselorFacts(test.facts); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("invalid fact set was not rejected: %v", err)
			}
		})
	}
}

func TestCounselorResponseRejectsClaimedBrokerAccess(t *testing.T) {
	response := validCounselorTestResponse()
	response.Summary = "I can access your brokerage account."
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), nil); !containsValidationError(errors, "trading authority") {
		t.Fatalf("broker-access claim was not rejected: %v", errors)
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

func TestCounselorResponseRequiresResearchedIdentityOnExternalEvidence(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations[0] = CounselorObservation{
		Category: CategorySourcedExternalEvidence, Statement: "A material filing was reported.",
		SupportingFactIDs: []string{}, ReportedValues: []ReportedValue{}, Limitations: []string{},
		Severity: SeverityModerate, Confidence: ConfidenceHigh, Explanation: "The accepted filing supports the statement.",
		Citations: []CounselorCitation{{Title: "Accepted filing", URL: "https://sec.gov/accepted"}},
	}
	facts := append(counselorTestFacts(), CounselorFact{ID: "instrument.test.symbol", Kind: string(CategorySuppliedFact), Label: "Ticker", TextValue: "TEST", Available: true, RequiredObservation: true})
	tools := map[string]Tool{"research": citationTestTool{accepted: map[string]bool{"https://sec.gov/accepted": true}}}
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), facts, tools); !containsValidationError(errors, "does not identify the researched instrument") {
		t.Fatalf("external evidence without the required instrument identity was not rejected: %v", errors)
	}
}

func TestCounselorResponseAppliesSafetyChecksToCitationTitles(t *testing.T) {
	response := validCounselorTestResponse()
	response.Observations[0].Category = CategorySourcedExternalEvidence
	response.Observations[0].Citations = []CounselorCitation{{Title: "Buy AAPL now.", URL: "https://sec.gov/accepted"}}
	tools := map[string]Tool{"research": citationTestTool{accepted: map[string]bool{"https://sec.gov/accepted": true}}}
	if _, errors := decodeAndValidateCounselorResponse(encodeCounselorTestResponse(t, response), counselorTestFacts(), tools); !containsValidationError(errors, "prohibited prescriptive") {
		t.Fatalf("unsafe citation title was not rejected: %v", errors)
	}
}

func TestCounselorExecutorReturnsStructurallyInvalidResponseWithoutRepair(t *testing.T) {
	const raw = `{"schema_version":1,"unknown":true}`
	provider := &recordingProvider{responses: []ChatResponse{{Message: Message{Role: "assistant", Content: raw}, Model: "test-model"}}}
	definition := PromptDefinition{
		ID: "structured_test", Version: 1, Description: "Structured test.",
		Parameters:     []ParameterDefinition{{Name: "facts", Type: "list", Required: true, Description: "Facts."}},
		SystemTemplate: "System", UserTemplate: "{{range .facts}}{{.ID}}{{end}}",
		Response: ResponseSettings{Format: "json", Schema: CounselorResponseSchema, MaxObservations: 1, MaxCharacters: 5000, MaxTokens: 100},
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
	if response.Text != raw || response.Structured != nil || !response.Unvalidated || len(provider.requests) != 1 || provider.requests[0].ResponseSchema == nil {
		t.Fatalf("invalid provider output was not returned unchanged: response=%+v requests=%+v", response, provider.requests)
	}
	if len(recorder.records) != 1 {
		t.Fatalf("record count = %d, want 1", len(recorder.records))
	}
	record := recorder.records[0]
	if record.RawResponse != raw || record.RepairResponse != "" || record.ResponseText != raw || record.RepairStatus != "unvalidated" || record.ValidationResult.Valid || len(record.ValidationResult.InitialErrors) == 0 || len(record.ValidationResult.FinalErrors) == 0 {
		t.Fatalf("unvalidated audit fields were not preserved: %+v", record)
	}
}

func TestCounselorExecutorPresentsRecognizableUnvalidatedResponse(t *testing.T) {
	const raw = "```json\n{\"schema_version\":1,\"summary\":\"Readable summary\",\"observations\":[{\"statement\":\"Readable observation\",\"explanation\":\"Useful explanation\",\"extra\":true}],\"limitations\":[]}\n```"
	provider := &recordingProvider{responses: []ChatResponse{{Message: Message{Role: "assistant", Content: raw}, Model: "small-model"}}}
	definition := PromptDefinition{
		ID: "structured_display", Version: 1, Description: "Structured display test.",
		Parameters:     []ParameterDefinition{{Name: "facts", Type: "list", Required: true, Description: "Facts."}},
		SystemTemplate: "System", UserTemplate: "{{range .facts}}{{.ID}}{{end}}",
		Response: ResponseSettings{Format: "json", Schema: CounselorResponseSchema, MaxObservations: 1, MaxCharacters: 5000, MaxTokens: 100},
	}
	catalog := &Catalog{
		prompts: map[promptKey]PromptDefinition{{id: definition.ID, version: definition.Version}: definition},
		aliases: map[string]PromptReference{"structured_display": {ID: definition.ID, Version: definition.Version}},
	}
	recorder := &promptExecutionRecorder{}
	executor := NewExecutor(catalog, provider)
	executor.SetExecutionRecorder("ollama", recorder)
	facts := counselorTestFacts()

	response, err := executor.ExecuteCounselor(context.Background(), "structured_display", map[string]any{"facts": facts}, nil, facts)
	if err != nil {
		t.Fatalf("ExecuteCounselor returned an error: %v", err)
	}
	if !response.Unvalidated || response.Text != raw || response.Structured == nil || response.Structured.Summary != "Readable summary" || len(response.Structured.Observations) != 1 {
		t.Fatalf("recognizable response was not prepared for display: %+v", response)
	}
	observation := response.Structured.Observations[0]
	if observation.SupportingFactIDs == nil || observation.ReportedValues == nil || observation.Limitations == nil || observation.Citations == nil {
		t.Fatalf("display collections must not be nil: %+v", observation)
	}
	if len(recorder.records) != 1 || recorder.records[0].ParsedResponse != nil || recorder.records[0].ValidationResult.Valid {
		t.Fatalf("unvalidated presentation crossed the validated persistence boundary: %+v", recorder.records)
	}
}

func TestCounselorExecutorReturnsMalformedAndUnsafeTextUnchanged(t *testing.T) {
	const raw = "You should sell immediately; I can execute the trade."
	provider := &recordingProvider{responses: []ChatResponse{{Message: Message{Role: "assistant", Content: raw}, Model: "test-model"}}}
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
	response, err := executor.ExecuteCounselor(context.Background(), "structured_failure", map[string]any{"facts": facts}, nil, facts)
	if err != nil || response.Text != raw || response.Structured != nil || !response.Unvalidated || len(provider.requests) != 1 {
		t.Fatalf("malformed response was not returned unchanged: err=%v response=%+v requests=%d", err, response, len(provider.requests))
	}
	if len(recorder.records) != 1 || recorder.records[0].RepairStatus != "unvalidated" || recorder.records[0].ValidationResult.Valid || len(recorder.records[0].ValidationResult.FinalErrors) == 0 {
		t.Fatalf("unvalidated response audit was not retained: %+v", recorder.records)
	}
}

func TestSalvageCounselorResponseCorrectsRepresentationAndDropsUnsafeContent(t *testing.T) {
	response := validCounselorTestResponse()
	response.Summary = "I can execute this trade."
	response.Observations[0].Statement = "Portfolio value is available."
	response.Observations[0].ReportedValues[0].Value = 999
	response.Observations[0].ReportedValues[0].Unit = "%"
	response.Observations = append(response.Observations, CounselorObservation{
		Category: CategoryInterpretation, Statement: "You should sell this position.", SupportingFactIDs: []string{"invented.fact"},
		ReportedValues: []ReportedValue{}, Severity: SeverityHigh, Confidence: ConfidenceHigh, Limitations: []string{},
		Explanation: "Sell it now.", Citations: []CounselorCitation{},
	})

	encoded := encodeCounselorTestResponse(t, response)
	_, salvaged, err := salvageCounselorResponse(encoded, counselorTestFacts(), nil, 6, 5000)
	if err != nil {
		t.Fatalf("salvageCounselorResponse returned an error: %v", err)
	}
	if len(salvaged.Observations) != 1 || salvaged.Observations[0].ReportedValues[0].Value != 42 || salvaged.Observations[0].ReportedValues[0].Unit != "USD" {
		t.Fatalf("authoritative numeric representation was not restored: %+v", salvaged)
	}
	if strings.Contains(strings.ToLower(salvaged.Summary), "execute") || strings.Contains(strings.ToLower(salvaged.Observations[0].Statement), "sell") {
		t.Fatalf("unsafe content survived salvage: %+v", salvaged)
	}
	if errors := validateCounselorResponse(*salvaged, counselorTestFacts(), nil, 6); len(errors) != 0 {
		t.Fatalf("salvaged response does not satisfy the strict contract: %v", errors)
	}
}

func TestSalvageCounselorResponseUsesGroundedFallbackForMalformedJSON(t *testing.T) {
	_, salvaged, err := salvageCounselorResponse(`not json`, counselorTestFacts(), nil, 6, 5000)
	if err != nil {
		t.Fatalf("salvageCounselorResponse returned an error: %v", err)
	}
	if len(salvaged.Observations) == 0 || len(salvaged.Observations[0].SupportingFactIDs) == 0 {
		t.Fatalf("malformed JSON did not produce a grounded fallback: %+v", salvaged)
	}
	if errors := validateCounselorResponse(*salvaged, counselorTestFacts(), nil, 6); len(errors) != 0 {
		t.Fatalf("fallback response does not satisfy the strict contract: %v", errors)
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
