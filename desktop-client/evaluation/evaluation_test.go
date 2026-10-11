package evaluation

// This file verifies suite immutability, strict response parsing, scoring, ordering, storage, and environment capture.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"wails/ai"
	"wails/analytics"
)

func TestLoadV2SuiteVerifiesFrozenProtocolAndFreshAnalytics(t *testing.T) {
	suite, err := LoadV2Suite()
	if err != nil {
		t.Fatalf("LoadV2Suite returned an error: %v", err)
	}
	if suite.Manifest.Version != 2 || suite.Manifest.Repetitions != 5 || suite.Manifest.Settings.MaxTokens != 3072 {
		t.Fatalf("unexpected version 2 manifest: %+v", suite.Manifest)
	}
	if len(suite.Conditions) != 2 || len(suite.Scenarios) != 3 || len(suite.ProtocolHashes.Definitions) != 8 {
		t.Fatalf("unexpected version 2 dimensions or hashes")
	}
	for _, scenario := range suite.Scenarios {
		metrics := metricLookup(buildMetrics(scenario.Portfolio, analytics.Calculate(scenario.Portfolio, scenario.SnapshotAt)))
		for _, expected := range scenario.RequiredFacts {
			if metric := metrics[expected.MetricID]; !metric.Available || math.Abs(metric.Value-expected.Value) > expected.Tolerance {
				t.Fatalf("scenario %s no longer matches %s", scenario.ID, expected.MetricID)
			}
		}
	}
}

func TestLoadV3SuiteReusesFrozenTreatmentsWithOpenAISettings(t *testing.T) {
	suite, err := LoadV3Suite()
	if err != nil {
		t.Fatalf("LoadV3Suite returned an error: %v", err)
	}
	if suite.Manifest.Version != 3 || suite.Manifest.Provider != "openai" || suite.Manifest.Model != "gpt-6-luna" || suite.Manifest.Settings.ReasoningEffort != "low" {
		t.Fatalf("unexpected version 3 manifest: %+v", suite.Manifest)
	}
	if len(suite.Conditions) != 2 || suite.Conditions[0].Version != 2 || len(suite.Scenarios) != 3 || len(suite.ProtocolHashes.Definitions) != 8 {
		t.Fatal("version 3 did not retain the frozen treatment and scenario definitions")
	}
}

func TestLoadV4SuiteLocksCorrectedSemanticContract(t *testing.T) {
	suite, err := LoadV4Suite()
	if err != nil {
		t.Fatalf("LoadV4Suite returned an error: %v", err)
	}
	if suite.Manifest.Version != 4 || suite.Manifest.Provider != "openai" || suite.Manifest.Model != "gpt-6-luna" || suite.Manifest.Settings.ReasoningEffort != "low" {
		t.Fatalf("unexpected version 4 manifest: %+v", suite.Manifest)
	}
	if len(suite.Conditions) != 2 || suite.Conditions[0].Version != 4 || len(suite.Scenarios) != 3 || len(suite.ProtocolHashes.Definitions) != 8 {
		t.Fatal("version 4 did not retain the comparison and frozen scenarios")
	}
}

func TestV4ContextNormalizesDerivableBaseValuesAndKeepsTrueGapsUnavailable(t *testing.T) {
	suite, err := LoadV4Suite()
	if err != nil {
		t.Fatal(err)
	}
	concentratedJSON, _, _, err := v4ConditionContext(suite.Scenarios[0], suite.Conditions[0])
	if err != nil {
		t.Fatal(err)
	}
	var concentrated v2RawContext
	if err := json.Unmarshal([]byte(concentratedJSON), &concentrated); err != nil {
		t.Fatal(err)
	}
	assertV4Fact(t, concentrated.Facts, "position.1002.base_market_value", 2375, true, "broker_ledger_conversion")
	assertV4Fact(t, concentrated.Facts, "position.1001.base_market_value", 6000, true, "same_currency_normalization")

	incompleteJSON, _, _, err := v4ConditionContext(suite.Scenarios[2], suite.Conditions[0])
	if err != nil {
		t.Fatal(err)
	}
	var incomplete v2RawContext
	if err := json.Unmarshal([]byte(incompleteJSON), &incomplete); err != nil {
		t.Fatal(err)
	}
	assertV4Fact(t, incomplete.Facts, "position.3002.base_market_value", 0, false, "unavailable")
}

func TestV4ValidationAllowsUnavailableSupportButNotReportedValues(t *testing.T) {
	evidence := map[string]v2Evidence{
		"position.missing.market_value": {ID: "position.missing.market_value", Unit: "money", Currency: "USD", Supplied: true, Available: false},
	}
	response := &V2EvaluationResponse{
		Summary: "Missing value.",
		Observations: []V2Observation{{
			Category: "limitation", Statement: "The position market value is unavailable.",
			SupportingIDs: []string{"position.missing.market_value"}, Severity: "limited",
		}},
		Safety: SafetyDeclaration{Disclaimer: "General information only."},
	}
	if err := validateV4Response(response, Condition{ID: "baseline"}, evidence); err != nil {
		t.Fatalf("version 4 rejected supplied missingness evidence: %v", err)
	}
	response.Observations[0].ReportedValues = []V2ReportedValue{{
		ReferenceID: "position.missing.market_value", Value: 0, Unit: "money", Currency: "USD", Source: "supplied",
	}}
	if err := validateV4Response(response, Condition{ID: "baseline"}, evidence); err == nil || !strings.Contains(err.Error(), "unavailable or mismatched") {
		t.Fatalf("version 4 accepted an unavailable numeric value: %v", err)
	}
}

func TestVersion1DefinitionsAndResultsRemainUnchanged(t *testing.T) {
	expected := map[string]string{
		"manifest.yaml":      "1d6ba8daf00df2f6b9c6dbeffaa769632bf05cd594d201e4fe6df2e5bf18489d",
		"conditions.yaml":    "11175aafe0bfeb8ebccd746f3e39e110b19f983f38d388f06c155e614e79b0f7",
		"manual_rubric.yaml": "e48345fd06411a8cfc352f44b95b6fa83d39ea5158179f8e86abcb7e663d52ff",
		"scenarios/01_concentrated_multicurrency.json":                        "5e1ef100ce7b49b372a1e1b61c3c0ad6c084a5e4c5ec9a7b0f8ad9ef7281a8ab",
		"scenarios/02_diversified_mixed_exposure.json":                        "0a6d4f0b1571498189d9303f1a241cc9fa4f5689077ab89558707e65fdc1f836",
		"scenarios/03_incomplete_data.json":                                   "09578c37eb32b5fe54271db0acaa6c56a6955d6d3262b4189ea67d5082b74f14",
		"results/portfolio-prompt-comparison-v1-20260928T075931Z.json":        "35444f619e968a93b8ecf891981c97a1086dd049f6f4608beb0111428b72ce96",
		"results/portfolio-prompt-comparison-v1-20260928T075931Z-analysis.md": "bd13c6b7e970b924bcdb16384578cfd675dde1562aef418b3a5b28daebea7649",
	}
	for path, digest := range expected {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read frozen version 1 artifact %s: %v", path, err)
		}
		actual := fmt.Sprintf("%x", sha256.Sum256(contents))
		if actual != digest {
			t.Fatalf("version 1 artifact %s changed: %s", path, actual)
		}
	}
}

func TestV2StrictDecodingAndSemanticValidation(t *testing.T) {
	valid := `{
		"summary":"Concise summary.",
		"observations":[{"category":"general_information","statement":"General context.","supporting_ids":[],"reported_values":[],"severity":"informational","limitations":[]}],
		"limitations":[],
		"safety":{"claims_trade_execution":false,"contains_trade_instruction":false,"disclaimer":"General information; no trade was executed."}
	}`
	response, err := parseV2Response(valid)
	if err != nil {
		t.Fatalf("parse valid response: %v", err)
	}
	if err := validateV2Response(response, Condition{ID: "baseline"}, map[string]v2Evidence{}); err != nil {
		t.Fatalf("validate response: %v", err)
	}

	unknownField := strings.Replace(valid, `"summary":"Concise summary."`, `"summary":"Concise summary.","extra":true`, 1)
	if _, err := parseV2Response(unknownField); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown-field rejection, got %v", err)
	}
	invalidEnum := strings.Replace(valid, `"severity":"informational"`, `"severity":"urgent"`, 1)
	if _, err := parseV2Response(invalidEnum); err == nil || !strings.Contains(err.Error(), "invalid severity") {
		t.Fatalf("expected enum rejection, got %v", err)
	}
	numericString := strings.Replace(valid, `"reported_values":[]`, `"reported_values":[{"reference_id":"portfolio.gross_exposure","value":"10","unit":"money","currency":"USD","source":"self_derived"}]`, 1)
	if _, err := parseV2Response(numericString); err == nil || !strings.Contains(err.Error(), "cannot unmarshal string") {
		t.Fatalf("expected numeric-string rejection, got %v", err)
	}
	if _, err := parseV2Response(valid + `{}`); err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("expected trailing-data rejection, got %v", err)
	}
}

func TestV2RejectsUnknownEvidenceIdentifiers(t *testing.T) {
	text := `{
		"summary":"Summary.",
		"observations":[{"category":"raw_fact","statement":"Claim.","supporting_ids":["unknown.fact"],"reported_values":[],"severity":"informational","limitations":[]}],
		"limitations":[],
		"safety":{"claims_trade_execution":false,"contains_trade_instruction":false,"disclaimer":"No trade was executed."}
	}`
	response, err := parseV2Response(text)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateV2Response(response, Condition{ID: "baseline"}, map[string]v2Evidence{}); err == nil || !strings.Contains(err.Error(), "unknown fact or metric") {
		t.Fatalf("expected unknown-evidence rejection, got %v", err)
	}
}

func TestV2RunnerSuppliesExactSameSchemaAndSettings(t *testing.T) {
	suite, err := LoadV2Suite()
	if err != nil {
		t.Fatal(err)
	}
	suite.Manifest.Repetitions = 1
	suite.Scenarios = suite.Scenarios[:1]
	provider := &capturingProvider{response: validV2Response()}
	bundle := NewV2Runner(provider).Run(context.Background(), suite, Environment{}, "pilot", SchemaPreflight{Passed: true})
	if len(bundle.Runs) != 2 || len(provider.requests) != 2 {
		t.Fatalf("expected two calls, got %d", len(provider.requests))
	}
	if !reflect.DeepEqual(provider.requests[0].ResponseSchema, suite.ResponseSchema) || !reflect.DeepEqual(provider.requests[1].ResponseSchema, suite.ResponseSchema) {
		t.Fatal("provider did not receive the exact frozen response schema")
	}
	if provider.requests[0].Settings != provider.requests[1].Settings || len(provider.requests[0].Tools) != 0 || len(provider.requests[1].Tools) != 0 {
		t.Fatal("conditions did not receive equal settings and empty tool sets")
	}
}

func TestV2RunnerAlternatesAndRetainsEveryFailure(t *testing.T) {
	suite, err := LoadV2Suite()
	if err != nil {
		t.Fatal(err)
	}
	suite.Manifest.Repetitions = 2
	suite.Scenarios = suite.Scenarios[:1]
	provider := &capturingProvider{err: errors.New("provider unavailable")}
	bundle := NewV2Runner(provider).Run(context.Background(), suite, Environment{}, "pilot", SchemaPreflight{Passed: true})
	if len(bundle.Runs) != 4 {
		t.Fatalf("expected all four failures retained, got %d", len(bundle.Runs))
	}
	order := []string{bundle.Runs[0].ConditionID, bundle.Runs[1].ConditionID, bundle.Runs[2].ConditionID, bundle.Runs[3].ConditionID}
	if strings.Join(order, ",") != "baseline,proposed,proposed,baseline" {
		t.Fatalf("unexpected alternating order: %v", order)
	}
	for _, run := range bundle.Runs {
		if run.FailureKind != "provider_failure" || run.Score.SchemaValid {
			t.Fatalf("failure was not retained correctly: %+v", run)
		}
	}
}

func TestV2TruncationAndMalformedJSONRemainFailures(t *testing.T) {
	suite, err := LoadV2Suite()
	if err != nil {
		t.Fatal(err)
	}
	suite.Manifest.Repetitions = 1
	suite.Scenarios = suite.Scenarios[:1]
	provider := &capturingProvider{response: `{"summary":`, finishReason: "length"}
	bundle := NewV2Runner(provider).Run(context.Background(), suite, Environment{}, "pilot", SchemaPreflight{Passed: true})
	for _, run := range bundle.Runs {
		if run.FailureKind != "truncation" || run.RawResponse == "" {
			t.Fatalf("truncation was not retained: %+v", run)
		}
	}
	if got := classifyV2ParseFailure("stop", errors.New("invalid character")); got != "malformed_json" {
		t.Fatalf("unexpected malformed JSON classification: %s", got)
	}
}

func TestV2AggregatesUseSchemaValidDenominators(t *testing.T) {
	validScore := RunScore{SchemaValid: true, RequiredFactCoverage: 0.5, NumericalAccuracy: 1, Grounding: 1, SafetyCompliance: 1}
	runs := []V2RunResult{
		{ConditionID: "baseline", ScenarioID: "one", Score: validScore},
		{ConditionID: "baseline", ScenarioID: "one", FailureKind: "structural_validation", Score: invalidScore()},
	}
	summary := summarizeV2Runs(runs, []Condition{{ID: "baseline"}})[0]
	if summary.SchemaValidity != 0.5 || summary.RequiredFactCoverage.Denominator != 1 || summary.RequiredFactCoverage.Value == nil || *summary.RequiredFactCoverage.Value != 0.5 {
		t.Fatalf("unexpected explicit denominators: %+v", summary)
	}
}

func TestSaveV2BundleUsesSeparateAtomicVersionedArtifacts(t *testing.T) {
	suite, err := LoadV2Suite()
	if err != nil {
		t.Fatal(err)
	}
	bundle := V2Bundle{
		CreatedAt: time.Date(2026, 10, 7, 8, 30, 0, 0, time.UTC), RunKind: "final",
		Manifest: suite.Manifest,
		Runs:     []V2RunResult{{Sequence: 1, ScenarioID: "scenario", ConditionID: "baseline", ConditionVersion: 2, RawResponse: "response"}},
	}
	directory := t.TempDir()
	paths, err := SaveV2Bundle(directory, bundle)
	if err != nil {
		t.Fatalf("SaveV2Bundle returned an error: %v", err)
	}
	if !strings.Contains(filepath.Base(paths.BundlePath), "-v2-final-") {
		t.Fatalf("unexpected versioned name: %s", paths.BundlePath)
	}
	review, err := os.ReadFile(paths.ReviewPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(review), "condition_id") {
		t.Fatal("blind review file disclosed condition identity")
	}
	if _, err := SaveV2Bundle(directory, bundle); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected overwrite protection, got %v", err)
	}
}

func TestV3RunnerRetainsUsageAndSaveUsesVersionedArtifacts(t *testing.T) {
	suite, err := LoadV3Suite()
	if err != nil {
		t.Fatal(err)
	}
	provider := &v3CapturingProvider{response: validV2Response()}
	bundle := NewV3Runner(provider).Run(context.Background(), suite, InspectV3Environment(suite.Manifest), "pilot", 1, SchemaPreflight{Passed: true})
	if len(bundle.Runs) != 6 || bundle.ExecutedRepetitions != 1 || bundle.Usage.CompletedCalls != 6 || bundle.Usage.TotalTokens != 900 {
		t.Fatalf("unexpected version 3 bundle: runs=%d usage=%+v", len(bundle.Runs), bundle.Usage)
	}
	for _, request := range provider.requests {
		if request.Settings.ReasoningEffort != "low" || len(request.Tools) != 0 {
			t.Fatalf("unexpected version 3 request: %+v", request)
		}
	}
	directory := t.TempDir()
	paths, err := SaveV3Bundle(directory, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.Base(paths.BundlePath), "-v3-pilot-") {
		t.Fatalf("unexpected version 3 artifact name: %s", paths.BundlePath)
	}
}

func TestV4RunnerRetainsUsageAndSaveUsesVersionedArtifacts(t *testing.T) {
	suite, err := LoadV4Suite()
	if err != nil {
		t.Fatal(err)
	}
	provider := &v3CapturingProvider{response: validV2Response()}
	bundle := NewV4Runner(provider).Run(context.Background(), suite, InspectV4Environment(suite.Manifest), "pilot", 1, SchemaPreflight{Passed: true})
	if len(bundle.Runs) != 6 || bundle.ExecutedRepetitions != 1 || bundle.Usage.CompletedCalls != 6 || bundle.Usage.TotalTokens != 900 {
		t.Fatalf("unexpected version 4 bundle: runs=%d usage=%+v", len(bundle.Runs), bundle.Usage)
	}
	for _, request := range provider.requests {
		if request.Settings.ReasoningEffort != "low" || len(request.Tools) != 0 {
			t.Fatalf("unexpected version 4 request: %+v", request)
		}
	}
	directory := t.TempDir()
	paths, err := SaveV4Bundle(directory, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.Base(paths.BundlePath), "-v4-pilot-") {
		t.Fatalf("unexpected version 4 artifact name: %s", paths.BundlePath)
	}
}

func TestLoadSuiteValidatesFrozenFacts(t *testing.T) {
	manifest, conditions, scenarios, rubric, err := LoadSuite()
	if err != nil {
		t.Fatalf("LoadSuite returned an error: %v", err)
	}
	if manifest.Repetitions != 5 || manifest.Settings.Seed != 42 || manifest.Settings.ContextSize != 8192 {
		t.Fatalf("unexpected fixed manifest: %+v", manifest)
	}
	if len(conditions) != 2 || len(scenarios) != 3 {
		t.Fatalf("unexpected suite dimensions: %d conditions, %d scenarios", len(conditions), len(scenarios))
	}
	if rubric.Version != 1 || len(rubric.Criteria) != 4 {
		t.Fatalf("unexpected manual rubric: %+v", rubric)
	}
}

func TestParseResponseRejectsUnknownFields(t *testing.T) {
	_, err := parseResponse(`{
		"summary":"summary",
		"observations":[{"type":"general_information","statement":"text","supporting_metric_ids":[],"reported_values":[],"severity":"informational","limitations":[]}],
		"limitations":[],
		"safety":{"claims_trade_execution":false,"contains_trade_instruction":false,"disclaimer":"general information"},
		"unexpected":true
	}`)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected an unknown-field error, got %v", err)
	}
}

func TestScoreResponseCoversFixedFacts(t *testing.T) {
	_, _, scenarios, _, err := LoadSuite()
	if err != nil {
		t.Fatal(err)
	}
	scenario := scenarios[0]
	metrics := buildMetrics(scenario.Portfolio, analytics.Calculate(scenario.Portfolio, scenario.SnapshotAt))
	observations := make([]Observation, 0, len(scenario.RequiredFacts))
	for _, fact := range scenario.RequiredFacts {
		observations = append(observations, Observation{
			Type: "deterministic_metric", Statement: "supported fact",
			SupportingMetricIDs: []string{fact.MetricID},
			ReportedValues:      []ReportedValue{{MetricID: fact.MetricID, Value: fact.Value, Unit: fact.Unit, Currency: fact.Currency}},
			Severity:            "informational", Limitations: []string{},
		})
	}
	response := &EvaluationResponse{
		Summary: "summary", Observations: observations, Limitations: []ReportedLimitation{},
		Safety: SafetyDeclaration{Disclaimer: "General information; no trade was executed."},
	}
	score := scoreResponse(response, scenario, metrics)
	if !score.SchemaValid || score.RequiredFactCoverage != 1 || score.NumericalAccuracy != 1 || score.Grounding != 1 || score.UnsupportedClaimRate != 0 || score.ContradictionRate != 0 || score.SafetyCompliance != 1 {
		t.Fatalf("unexpected perfect-response score: %+v", score)
	}
}

func TestRunnerAlternatesConditionOrderAndStoresBundle(t *testing.T) {
	manifest, conditions, scenarios, rubric, err := LoadSuite()
	if err != nil {
		t.Fatal(err)
	}
	manifest.Repetitions = 2
	provider := &staticProvider{response: `{
		"summary":"summary",
		"observations":[{"type":"general_information","statement":"general statement","supporting_metric_ids":[],"reported_values":[],"severity":"informational","limitations":[]}],
		"limitations":[],
		"safety":{"claims_trade_execution":false,"contains_trade_instruction":false,"disclaimer":"General information; no trade was executed."}
	}`}
	runner := NewRunner(provider)
	fixedTime := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	runner.now = func() time.Time { return fixedTime }
	bundle := runner.Run(context.Background(), manifest, conditions, scenarios[:1], rubric, Environment{Model: manifest.Model})
	if len(bundle.Runs) != 4 {
		t.Fatalf("expected four paired runs, got %d", len(bundle.Runs))
	}
	order := []string{bundle.Runs[0].ConditionID, bundle.Runs[1].ConditionID, bundle.Runs[2].ConditionID, bundle.Runs[3].ConditionID}
	if strings.Join(order, ",") != "baseline,proposed,proposed,baseline" {
		t.Fatalf("unexpected alternating order: %v", order)
	}
	directory := t.TempDir()
	path, err := SaveBundle(directory, bundle)
	if err != nil {
		t.Fatalf("SaveBundle returned an error: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored Bundle
	if err := json.Unmarshal(contents, &stored); err != nil {
		t.Fatalf("decode stored bundle: %v", err)
	}
	if stored.Manifest.SuiteID != manifest.SuiteID || filepath.Dir(path) != directory {
		t.Fatalf("unexpected stored bundle: %s %+v", path, stored.Manifest)
	}
}

func TestInspectEnvironmentRecordsModelDigest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/version":
			_, _ = writer.Write([]byte(`{"version":"0.11.0"}`))
		case "/api/tags":
			_, _ = writer.Write([]byte(`{"models":[{"name":"llama3.2:latest","model":"llama3.2:latest","digest":"sha256:test"}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	environment, err := InspectEnvironment(context.Background(), server.URL+"/api", "llama3.2")
	if err != nil {
		t.Fatalf("InspectEnvironment returned an error: %v", err)
	}
	if environment.OllamaVersion != "0.11.0" || environment.ConfiguredModel != "llama3.2" || environment.Model != "llama3.2:latest" || environment.ModelDigest != "sha256:test" || environment.LogicalCPUs < 1 {
		t.Fatalf("unexpected environment: %+v", environment)
	}
}

type staticProvider struct {
	response string
}

func (p *staticProvider) Chat(_ context.Context, _ ai.ChatRequest) (ai.ChatResponse, error) {
	return ai.ChatResponse{Model: "test-model", Message: ai.Message{Role: "assistant", Content: p.response}}, nil
}

type capturingProvider struct {
	requests     []ai.ChatRequest
	response     string
	finishReason string
	err          error
}

type v3CapturingProvider struct {
	requests []ai.ChatRequest
	response string
}

func assertV4Fact(t *testing.T, facts []FactRecord, id string, value float64, available bool, source string) {
	t.Helper()
	for _, fact := range facts {
		if fact.ID != id {
			continue
		}
		if fact.Available != available || fact.Source != source {
			t.Fatalf("unexpected fact %s: %+v", id, fact)
		}
		if available && (fact.Value == nil || math.Abs(*fact.Value-value) > 1e-9) {
			t.Fatalf("unexpected value for %s: %+v", id, fact)
		}
		return
	}
	t.Fatalf("fact %s was not found", id)
}

func (p *v3CapturingProvider) Chat(_ context.Context, request ai.ChatRequest) (ai.ChatResponse, error) {
	p.requests = append(p.requests, request)
	return ai.ChatResponse{
		Model: "gpt-6-luna", FinishReason: "completed", ResponseID: fmt.Sprintf("resp_%d", len(p.requests)),
		ServiceTier: "default", EffectiveReasoningEffort: "low",
		Usage:   ai.TokenUsage{InputTokens: 100, OutputTokens: 50, ReasoningTokens: 10, TotalTokens: 150},
		Message: ai.Message{Role: "assistant", Content: p.response},
	}, nil
}

func (p *capturingProvider) Chat(_ context.Context, request ai.ChatRequest) (ai.ChatResponse, error) {
	p.requests = append(p.requests, request)
	if p.err != nil {
		return ai.ChatResponse{}, p.err
	}
	return ai.ChatResponse{
		Model: "llama3.2:latest", FinishReason: p.finishReason,
		Message: ai.Message{Role: "assistant", Content: p.response},
	}, nil
}

func validV2Response() string {
	return `{
		"summary":"Concise summary.",
		"observations":[{"category":"general_information","statement":"General context.","supporting_ids":[],"reported_values":[],"severity":"informational","limitations":[]}],
		"limitations":[],
		"safety":{"claims_trade_execution":false,"contains_trade_instruction":false,"disclaimer":"General information; no trade was executed."}
	}`
}
