// Package evaluation runs reproducible comparisons of portfolio-counseling prompt conditions.
package evaluation

// This file defines evaluation manifests, frozen scenarios, structured responses, scores, and result bundles.

import (
	"encoding/json"
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

// V2Manifest fixes the separately versioned constrained-output experiment.
type V2Manifest struct {
	SuiteID               string                `yaml:"suite_id" json:"suite_id"`
	Version               int                   `yaml:"version" json:"version"`
	Motivation            string                `yaml:"motivation" json:"motivation"`
	RelationshipToV1      string                `yaml:"relationship_to_v1" json:"relationship_to_v1"`
	Model                 string                `yaml:"model" json:"model"`
	ExpectedModelDigest   string                `yaml:"expected_model_digest" json:"expected_model_digest"`
	Repetitions           int                   `yaml:"repetitions" json:"repetitions"`
	OrderStrategy         string                `yaml:"order_strategy" json:"order_strategy"`
	RequestTimeoutSeconds int                   `yaml:"request_timeout_seconds" json:"request_timeout_seconds"`
	Settings              ai.GenerationSettings `yaml:"settings" json:"settings"`
	OutputLimitRationale  string                `yaml:"output_limit_rationale" json:"output_limit_rationale"`
}

// V2ManualRubric fixes the condition-blind semantic review procedure.
type V2ManualRubric struct {
	Version      int               `yaml:"version" json:"version"`
	BlindReview  bool              `yaml:"blind_review" json:"blind_review"`
	Instructions string            `yaml:"instructions" json:"instructions"`
	Criteria     []ManualCriterion `yaml:"criteria" json:"criteria"`
}

// V2ScoringRules records metric denominators and the boundary between automated and manual assessment.
type V2ScoringRules struct {
	Version                   int               `yaml:"version" json:"version"`
	SchemaValidityDenominator string            `yaml:"schema_validity_denominator" json:"schema_validity_denominator"`
	DownstreamDenominator     string            `yaml:"downstream_metric_denominator" json:"downstream_metric_denominator"`
	InvalidResponseHandling   string            `yaml:"invalid_response_handling" json:"invalid_response_handling"`
	ProviderFailureHandling   string            `yaml:"provider_failure_handling" json:"provider_failure_handling"`
	RepeatabilityDenominator  string            `yaml:"repeatability_denominator" json:"repeatability_denominator"`
	UnsupportedClaimScope     string            `yaml:"unsupported_claim_scope" json:"unsupported_claim_scope"`
	ContradictionScope        string            `yaml:"contradiction_scope" json:"contradiction_scope"`
	Tolerances                map[string]string `yaml:"tolerances" json:"tolerances"`
}

// FactRecord is one stable normalized fact shared by both version 2 conditions.
type FactRecord struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Text      string   `json:"text,omitempty"`
	Value     *float64 `json:"value,omitempty"`
	Unit      string   `json:"unit,omitempty"`
	Currency  string   `json:"currency,omitempty"`
	Source    string   `json:"source,omitempty"`
	Available bool     `json:"available"`
}

// V2EvaluationResponse is the bounded structured contract shared by both version 2 conditions.
type V2EvaluationResponse struct {
	Summary      string               `json:"summary"`
	Observations []V2Observation      `json:"observations"`
	Limitations  []ReportedLimitation `json:"limitations"`
	Safety       SafetyDeclaration    `json:"safety"`
}

// V2Observation links one concise statement to supplied evidence or an explicit baseline calculation.
type V2Observation struct {
	Category       string            `json:"category"`
	Statement      string            `json:"statement"`
	SupportingIDs  []string          `json:"supporting_ids"`
	ReportedValues []V2ReportedValue `json:"reported_values"`
	Severity       string            `json:"severity"`
	Limitations    []string          `json:"limitations"`
}

// V2ReportedValue identifies whether a number was supplied or calculated by the baseline model.
type V2ReportedValue struct {
	ReferenceID string  `json:"reference_id"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	Currency    string  `json:"currency"`
	Source      string  `json:"source"`
}

// V2RunResult retains one planned call regardless of provider or validation outcome.
type V2RunResult struct {
	Sequence         int                   `json:"sequence"`
	Repetition       int                   `json:"repetition"`
	ScenarioID       string                `json:"scenario_id"`
	ConditionID      string                `json:"condition_id"`
	ConditionVersion int                   `json:"condition_version"`
	StartedAt        time.Time             `json:"started_at"`
	DurationMillis   int64                 `json:"duration_millis"`
	ProviderModel    string                `json:"provider_model,omitempty"`
	ResponseID       string                `json:"response_id,omitempty"`
	ServiceTier      string                `json:"service_tier,omitempty"`
	ReasoningEffort  string                `json:"reasoning_effort,omitempty"`
	Usage            ai.TokenUsage         `json:"usage,omitempty"`
	FinishReason     string                `json:"finish_reason,omitempty"`
	RawResponse      string                `json:"raw_response,omitempty"`
	ParsedResponse   *V2EvaluationResponse `json:"parsed_response,omitempty"`
	Score            RunScore              `json:"score"`
	FailureKind      string                `json:"failure_kind,omitempty"`
	Error            string                `json:"error,omitempty"`
}

// MetricMean reports a downstream mean and its schema-valid response denominator.
type MetricMean struct {
	Value       *float64 `json:"value"`
	Denominator int      `json:"denominator"`
}

// V2ConditionSummary separates planned-run validity from valid-response downstream metrics.
type V2ConditionSummary struct {
	ConditionID          string     `json:"condition_id"`
	PlannedRuns          int        `json:"planned_runs"`
	SchemaValidRuns      int        `json:"schema_valid_runs"`
	SchemaValidity       float64    `json:"schema_validity"`
	ProviderFailures     int        `json:"provider_failures"`
	Timeouts             int        `json:"timeouts"`
	Truncations          int        `json:"truncations"`
	ValidationFailures   int        `json:"validation_failures"`
	RequiredFactCoverage MetricMean `json:"required_fact_coverage"`
	NumericalAccuracy    MetricMean `json:"numerical_accuracy"`
	Grounding            MetricMean `json:"grounding"`
	UnsupportedClaimRate MetricMean `json:"unsupported_claim_rate"`
	ContradictionRate    MetricMean `json:"contradiction_rate"`
	LimitationCompliance MetricMean `json:"limitation_compliance"`
	SafetyCompliance     MetricMean `json:"safety_compliance"`
	Repeatability        MetricMean `json:"repeatability"`
}

// SchemaPreflight records the non-dataset call that verifies the selected provider accepts the frozen schema.
type SchemaPreflight struct {
	CompletedAt     time.Time     `json:"completed_at"`
	ProviderModel   string        `json:"provider_model"`
	FinishReason    string        `json:"finish_reason,omitempty"`
	ResponseID      string        `json:"response_id,omitempty"`
	ServiceTier     string        `json:"service_tier,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
	Usage           ai.TokenUsage `json:"usage,omitempty"`
	Passed          bool          `json:"passed"`
}

// ProtocolHashes binds every file-based fixed definition to the final experiment.
type ProtocolHashes struct {
	Algorithm   string            `json:"algorithm"`
	Definitions map[string]string `json:"definitions"`
	LockFile    string            `json:"lock_file"`
}

// V2Bundle is the complete immutable artifact for one version 2 execution.
type V2Bundle struct {
	CreatedAt      time.Time            `json:"created_at"`
	RunKind        string               `json:"run_kind"`
	Manifest       V2Manifest           `json:"manifest"`
	Conditions     []Condition          `json:"conditions"`
	Scenarios      []FrozenScenario     `json:"scenarios"`
	ResponseSchema json.RawMessage      `json:"response_schema"`
	ScoringRules   V2ScoringRules       `json:"scoring_rules"`
	ManualRubric   V2ManualRubric       `json:"manual_rubric"`
	ProtocolHashes ProtocolHashes       `json:"protocol_hashes"`
	Environment    Environment          `json:"environment"`
	Preflight      SchemaPreflight      `json:"schema_preflight"`
	Runs           []V2RunResult        `json:"runs"`
	Summaries      []V2ConditionSummary `json:"summaries"`
}

// V3Manifest fixes the OpenAI constrained-output experiment without changing version 2.
type V3Manifest struct {
	SuiteID               string                `yaml:"suite_id" json:"suite_id"`
	Version               int                   `yaml:"version" json:"version"`
	Motivation            string                `yaml:"motivation" json:"motivation"`
	RelationshipToEarlier string                `yaml:"relationship_to_earlier" json:"relationship_to_earlier"`
	Provider              string                `yaml:"provider" json:"provider"`
	Model                 string                `yaml:"model" json:"model"`
	Repetitions           int                   `yaml:"repetitions" json:"repetitions"`
	OrderStrategy         string                `yaml:"order_strategy" json:"order_strategy"`
	RequestTimeoutSeconds int                   `yaml:"request_timeout_seconds" json:"request_timeout_seconds"`
	Settings              ai.GenerationSettings `yaml:"settings" json:"settings"`
	SamplingPolicy        string                `yaml:"sampling_policy" json:"sampling_policy"`
	OutputLimitRationale  string                `yaml:"output_limit_rationale" json:"output_limit_rationale"`
}

// V3Environment records the hosted provider and local runner identity without persisting credentials.
type V3Environment struct {
	OperatingSystem string `json:"operating_system"`
	Architecture    string `json:"architecture"`
	LogicalCPUs     int    `json:"logical_cpus"`
	GoVersion       string `json:"go_version"`
	Provider        string `json:"provider"`
	ConfiguredModel string `json:"configured_model"`
}

// UsageTotals aggregates provider-reported token usage across completed calls.
type UsageTotals struct {
	CompletedCalls    int `json:"completed_calls"`
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	CacheWriteTokens  int `json:"cache_write_tokens"`
	OutputTokens      int `json:"output_tokens"`
	ReasoningTokens   int `json:"reasoning_tokens"`
	TotalTokens       int `json:"total_tokens"`
}

// V3Bundle is the complete immutable artifact for one OpenAI version 3 execution.
type V3Bundle struct {
	CreatedAt           time.Time            `json:"created_at"`
	RunKind             string               `json:"run_kind"`
	ExecutedRepetitions int                  `json:"executed_repetitions"`
	Manifest            V3Manifest           `json:"manifest"`
	Conditions          []Condition          `json:"conditions"`
	Scenarios           []FrozenScenario     `json:"scenarios"`
	ResponseSchema      json.RawMessage      `json:"response_schema"`
	ScoringRules        V2ScoringRules       `json:"scoring_rules"`
	ManualRubric        V2ManualRubric       `json:"manual_rubric"`
	ProtocolHashes      ProtocolHashes       `json:"protocol_hashes"`
	Environment         V3Environment        `json:"environment"`
	Preflight           SchemaPreflight      `json:"schema_preflight"`
	Runs                []V2RunResult        `json:"runs"`
	Summaries           []V2ConditionSummary `json:"summaries"`
	Usage               UsageTotals          `json:"usage"`
}

// V4Manifest fixes the corrected OpenAI semantic-contract experiment.
type V4Manifest struct {
	SuiteID               string                `yaml:"suite_id" json:"suite_id"`
	Version               int                   `yaml:"version" json:"version"`
	Motivation            string                `yaml:"motivation" json:"motivation"`
	RelationshipToEarlier string                `yaml:"relationship_to_earlier" json:"relationship_to_earlier"`
	Provider              string                `yaml:"provider" json:"provider"`
	Model                 string                `yaml:"model" json:"model"`
	Repetitions           int                   `yaml:"repetitions" json:"repetitions"`
	OrderStrategy         string                `yaml:"order_strategy" json:"order_strategy"`
	RequestTimeoutSeconds int                   `yaml:"request_timeout_seconds" json:"request_timeout_seconds"`
	Settings              ai.GenerationSettings `yaml:"settings" json:"settings"`
	SamplingPolicy        string                `yaml:"sampling_policy" json:"sampling_policy"`
	OutputLimitRationale  string                `yaml:"output_limit_rationale" json:"output_limit_rationale"`
}

// V4Bundle retains one corrected semantic-contract execution separately from version 3.
type V4Bundle struct {
	CreatedAt           time.Time            `json:"created_at"`
	RunKind             string               `json:"run_kind"`
	ExecutedRepetitions int                  `json:"executed_repetitions"`
	Manifest            V4Manifest           `json:"manifest"`
	Conditions          []Condition          `json:"conditions"`
	Scenarios           []FrozenScenario     `json:"scenarios"`
	ResponseSchema      json.RawMessage      `json:"response_schema"`
	ScoringRules        V2ScoringRules       `json:"scoring_rules"`
	ManualRubric        V2ManualRubric       `json:"manual_rubric"`
	ProtocolHashes      ProtocolHashes       `json:"protocol_hashes"`
	Environment         V3Environment        `json:"environment"`
	Preflight           SchemaPreflight      `json:"schema_preflight"`
	Runs                []V2RunResult        `json:"runs"`
	Summaries           []V2ConditionSummary `json:"summaries"`
	Usage               UsageTotals          `json:"usage"`
}
