package evaluation

// This file verifies suite immutability, strict response parsing, scoring, ordering, storage, and environment capture.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wails/ai"
	"wails/analytics"
)

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
