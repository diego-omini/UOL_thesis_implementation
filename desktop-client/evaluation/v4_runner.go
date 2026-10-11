package evaluation

// This file executes the corrected OpenAI version 4 experiment with normalized facts and strict validation.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"wails/ai"
)

const v4ResponseInstructions = `Return one concise JSON response matching the supplied schema. Use empty arrays rather than null. Use decimal ratios rather than percentages. supporting_ids may cite exact supplied available or unavailable identifiers; available=false identifiers support only missingness or limitations. reported_values may contain only available numeric evidence with its exact identifier, unit, currency, and value. A baseline self-derived reported value must use an exact evaluation_targets ID and cite the available raw facts used. Never put text, currency-label, timestamp, classification, or unavailable identifiers in reported_values. Do not add fields.`

// V4Runner owns one corrected OpenAI constrained-output evaluation execution.
type V4Runner struct {
	provider ai.Provider
	now      func() time.Time
}

// NewV4Runner creates an evaluator without broker, research, persistence, file, or trading tools.
func NewV4Runner(provider ai.Provider) *V4Runner {
	return &V4Runner{provider: provider, now: time.Now}
}

// VerifySchemaAcceptance performs a non-dataset hosted call before the version 4 pilot begins.
func (r *V4Runner) VerifySchemaAcceptance(ctx context.Context, suite V4Suite) (SchemaPreflight, error) {
	settings := suite.Manifest.Settings
	settings.MaxTokens = 512
	response, err := r.provider.Chat(ctx, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: "system", Content: v4ResponseInstructions},
			{Role: "user", Content: `Return a minimal general-information response. State that this is a schema preflight and no trade was executed.`},
		},
		ResponseSchema: suite.ResponseSchema,
		Settings:       settings,
	})
	preflight := SchemaPreflight{CompletedAt: r.now().UTC()}
	if err != nil {
		return preflight, fmt.Errorf("version 4 schema preflight failed: %w", err)
	}
	preflight.ProviderModel = response.Model
	preflight.FinishReason = response.FinishReason
	preflight.ResponseID = response.ResponseID
	preflight.ServiceTier = response.ServiceTier
	preflight.ReasoningEffort = response.EffectiveReasoningEffort
	preflight.Usage = response.Usage
	if _, err := parseV2Response(response.Message.Content); err != nil {
		return preflight, fmt.Errorf("version 4 schema preflight returned invalid structured output: %w", err)
	}
	preflight.Passed = true
	return preflight, nil
}

// Run executes a bounded version 4 pilot or the complete final experiment.
func (r *V4Runner) Run(ctx context.Context, suite V4Suite, environment V3Environment, runKind string, repetitions int, preflight SchemaPreflight) V4Bundle {
	bundle := V4Bundle{
		CreatedAt: r.now().UTC(), RunKind: runKind, ExecutedRepetitions: repetitions,
		Manifest: suite.Manifest, Conditions: suite.Conditions, Scenarios: suite.Scenarios,
		ResponseSchema: suite.SchemaJSON, ScoringRules: suite.ScoringRules, ManualRubric: suite.ManualRubric,
		ProtocolHashes: suite.ProtocolHashes, Environment: environment, Preflight: preflight,
	}
	sequence := 0
	for _, scenario := range suite.Scenarios {
		for repetition := 1; repetition <= repetitions; repetition++ {
			for _, condition := range orderedConditions(suite.Conditions, repetition) {
				sequence++
				bundle.Runs = append(bundle.Runs, r.runOne(ctx, suite, scenario, condition, repetition, sequence))
			}
		}
	}
	bundle.Summaries = summarizeV2Runs(bundle.Runs, suite.Conditions)
	bundle.Usage = totalV3Usage(bundle.Runs)
	return bundle
}

func (r *V4Runner) runOne(ctx context.Context, suite V4Suite, scenario FrozenScenario, condition Condition, repetition, sequence int) V2RunResult {
	started := r.now().UTC()
	result := V2RunResult{
		Sequence: sequence, Repetition: repetition, ScenarioID: scenario.ID,
		ConditionID: condition.ID, ConditionVersion: condition.Version, StartedAt: started,
	}
	contextJSON, evidence, _, err := v4ConditionContext(scenario, condition)
	if err != nil {
		result.FailureKind = "protocol_failure"
		result.Error = err.Error()
		result.Score = invalidScore()
		return result
	}
	callContext, cancel := context.WithTimeout(ctx, time.Duration(suite.Manifest.RequestTimeoutSeconds)*time.Second)
	response, err := r.provider.Chat(callContext, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: "system", Content: condition.SystemPrompt + "\n\n" + v4ResponseInstructions},
			{Role: "user", Content: condition.UserPrompt + "\n\nFrozen input:\n" + contextJSON},
		},
		ResponseSchema: suite.ResponseSchema,
		Settings:       suite.Manifest.Settings,
	})
	cancel()
	result.DurationMillis = r.now().UTC().Sub(started).Milliseconds()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(callContext.Err(), context.DeadlineExceeded) {
			result.FailureKind = "timeout"
		} else {
			result.FailureKind = "provider_failure"
		}
		result.Error = fmt.Sprintf("provider execution: %v", err)
		result.Score = invalidScore()
		return result
	}
	result.ProviderModel = response.Model
	result.ResponseID = response.ResponseID
	result.ServiceTier = response.ServiceTier
	result.ReasoningEffort = response.EffectiveReasoningEffort
	result.Usage = response.Usage
	result.FinishReason = response.FinishReason
	result.RawResponse = response.Message.Content
	parsed, parseErr := parseV2Response(response.Message.Content)
	if parseErr != nil {
		result.FailureKind = classifyV4ParseFailure(response.FinishReason, parseErr)
		result.Error = parseErr.Error()
		result.Score = invalidScore()
		return result
	}
	if validationErr := validateV4Response(parsed, condition, evidence); validationErr != nil {
		result.FailureKind = "semantic_validation"
		result.Error = validationErr.Error()
		result.ParsedResponse = parsed
		result.Score = invalidScore()
		return result
	}
	result.ParsedResponse = parsed
	result.Score = scoreV2Response(parsed, scenario, evidence)
	return result
}

func classifyV4ParseFailure(finishReason string, err error) string {
	if strings.EqualFold(strings.TrimSpace(finishReason), "max_output_tokens") || strings.EqualFold(strings.TrimSpace(finishReason), "length") {
		return "truncation"
	}
	return classifyV2ParseFailure(finishReason, err)
}
