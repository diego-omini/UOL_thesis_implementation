// Package evaluation runs reproducible comparisons of portfolio-counseling prompt conditions.
package evaluation

// This file defines evaluation manifests, frozen scenarios, structured responses, scores, and result bundles.

import (
	"time"

	"wails/ai"
	"wails/portfolio"
)

// Manifest fixes the model, generation settings, repetitions, and execution order for one evaluation suite.
type Manifest struct {
	SuiteID             string                `yaml:"suite_id" json:"suite_id"`
	Version             int                   `yaml:"version" json:"version"`
	Model               string                `yaml:"model" json:"model"`
	ExpectedModelDigest string                `yaml:"expected_model_digest" json:"expected_model_digest,omitempty"`
	Repetitions         int                   `yaml:"repetitions" json:"repetitions"`
	OrderStrategy       string                `yaml:"order_strategy" json:"order_strategy"`
	Settings            ai.GenerationSettings `yaml:"settings" json:"settings"`
}

// Condition is one fixed prompt treatment in the comparison.
type Condition struct {
	ID           string `yaml:"id" json:"id"`
	Version      int    `yaml:"version" json:"version"`
	Description  string `yaml:"description" json:"description"`
	Context      string `yaml:"context" json:"context"`
	SystemPrompt string `yaml:"system_prompt" json:"system_prompt"`
	UserPrompt   string `yaml:"user_prompt" json:"user_prompt"`
}

type conditionFile struct {
	Conditions []Condition `yaml:"conditions"`
}

// ManualRubric fixes semantic judgments that cannot be inferred reliably from the JSON structure.
type ManualRubric struct {
	Version      int               `yaml:"version" json:"version"`
	Instructions string            `yaml:"instructions" json:"instructions"`
	Criteria     []ManualCriterion `yaml:"criteria" json:"criteria"`
}

// ManualCriterion defines one author-applied score with fixed anchors.
type ManualCriterion struct {
	ID          string        `yaml:"id" json:"id"`
	Description string        `yaml:"description" json:"description"`
	Anchors     []ScoreAnchor `yaml:"anchors" json:"anchors"`
}

// ScoreAnchor defines the meaning of one allowed manual score.
type ScoreAnchor struct {
	Score int    `yaml:"score" json:"score"`
	Label string `yaml:"label" json:"label"`
}

// FrozenScenario is an immutable, sanitized portfolio input with facts fixed before model execution.
type FrozenScenario struct {
	ID                  string              `json:"id"`
	Version             int                 `json:"version"`
	Description         string              `json:"description"`
	Provenance          string              `json:"provenance"`
	SnapshotAt          time.Time           `json:"snapshot_at"`
	Portfolio           portfolio.Portfolio `json:"portfolio"`
	RequiredFacts       []ExpectedFact      `json:"required_facts"`
	RequiredLimitations []string            `json:"required_limitations"`
}

// ExpectedFact fixes one scored deterministic metric and its accepted numerical tolerance.
type ExpectedFact struct {
	MetricID  string  `json:"metric_id"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Currency  string  `json:"currency,omitempty"`
	Tolerance float64 `json:"tolerance"`
}

// MetricRecord is one deterministic fact made available to the proposed condition and scorer.
type MetricRecord struct {
	ID        string  `json:"id"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Currency  string  `json:"currency,omitempty"`
	Available bool    `json:"available"`
}

// EvaluationResponse is the strict machine-readable output contract shared by both conditions.
type EvaluationResponse struct {
	Summary      string               `json:"summary"`
	Observations []Observation        `json:"observations"`
	Limitations  []ReportedLimitation `json:"limitations"`
	Safety       SafetyDeclaration    `json:"safety"`
}

// Observation links a portfolio statement to deterministic evidence when such evidence is supplied.
type Observation struct {
	Type                string          `json:"type"`
	Statement           string          `json:"statement"`
	SupportingMetricIDs []string        `json:"supporting_metric_ids"`
	ReportedValues      []ReportedValue `json:"reported_values"`
	Severity            string          `json:"severity"`
	Limitations         []string        `json:"limitations"`
}

// ReportedValue records a model-reported numeric metric for deterministic checking.
type ReportedValue struct {
	MetricID string  `json:"metric_id"`
	Value    float64 `json:"value"`
	Unit     string  `json:"unit"`
	Currency string  `json:"currency,omitempty"`
}

// ReportedLimitation records an expected or model-identified data limitation.
type ReportedLimitation struct {
	Code      string `json:"code"`
	Statement string `json:"statement"`
}

// SafetyDeclaration makes prohibited trading-authority claims machine-checkable.
type SafetyDeclaration struct {
	ClaimsTradeExecution     bool   `json:"claims_trade_execution"`
	ContainsTradeInstruction bool   `json:"contains_trade_instruction"`
	Disclaimer               string `json:"disclaimer"`
}

// RunScore contains only metrics that can be evaluated deterministically from structured output.
type RunScore struct {
	SchemaValid          bool     `json:"schema_valid"`
	RequiredFactCoverage float64  `json:"required_fact_coverage"`
	NumericalAccuracy    float64  `json:"numerical_accuracy"`
	Grounding            float64  `json:"grounding"`
	UnsupportedClaimRate float64  `json:"unsupported_claim_rate"`
	ContradictionRate    float64  `json:"contradiction_rate"`
	LimitationCompliance float64  `json:"limitation_compliance"`
	SafetyCompliance     float64  `json:"safety_compliance"`
	CoveredFactIDs       []string `json:"covered_fact_ids"`
	ManualReviewRequired []string `json:"manual_review_required"`
}

// RunResult stores one scenario, condition, and repetition result without discarding failures.
type RunResult struct {
	Sequence         int                 `json:"sequence"`
	Repetition       int                 `json:"repetition"`
	ScenarioID       string              `json:"scenario_id"`
	ConditionID      string              `json:"condition_id"`
	ConditionVersion int                 `json:"condition_version"`
	StartedAt        time.Time           `json:"started_at"`
	DurationMillis   int64               `json:"duration_millis"`
	ProviderModel    string              `json:"provider_model,omitempty"`
	RawResponse      string              `json:"raw_response,omitempty"`
	ParsedResponse   *EvaluationResponse `json:"parsed_response,omitempty"`
	Score            RunScore            `json:"score"`
	Error            string              `json:"error,omitempty"`
}

// ConditionSummary aggregates deterministic run metrics and fact-set repeatability.
type ConditionSummary struct {
	ConditionID          string  `json:"condition_id"`
	Runs                 int     `json:"runs"`
	SuccessfulRuns       int     `json:"successful_runs"`
	SchemaValidity       float64 `json:"schema_validity"`
	RequiredFactCoverage float64 `json:"required_fact_coverage"`
	NumericalAccuracy    float64 `json:"numerical_accuracy"`
	Grounding            float64 `json:"grounding"`
	UnsupportedClaimRate float64 `json:"unsupported_claim_rate"`
	ContradictionRate    float64 `json:"contradiction_rate"`
	LimitationCompliance float64 `json:"limitation_compliance"`
	SafetyCompliance     float64 `json:"safety_compliance"`
	Repeatability        float64 `json:"repeatability"`
}

// Environment records the local inference identity needed to reproduce a run.
type Environment struct {
	OperatingSystem string `json:"operating_system"`
	Architecture    string `json:"architecture"`
	LogicalCPUs     int    `json:"logical_cpus"`
	GoVersion       string `json:"go_version"`
	OllamaVersion   string `json:"ollama_version,omitempty"`
	ConfiguredModel string `json:"configured_model"`
	Model           string `json:"model"`
	ModelDigest     string `json:"model_digest,omitempty"`
}

// Bundle is the complete immutable JSON artifact written for one evaluation execution.
type Bundle struct {
	CreatedAt    time.Time          `json:"created_at"`
	Manifest     Manifest           `json:"manifest"`
	Conditions   []Condition        `json:"conditions"`
	Scenarios    []FrozenScenario   `json:"scenarios"`
	ManualRubric ManualRubric       `json:"manual_rubric"`
	Environment  Environment        `json:"environment"`
	Runs         []RunResult        `json:"runs"`
	Summaries    []ConditionSummary `json:"summaries"`
}
