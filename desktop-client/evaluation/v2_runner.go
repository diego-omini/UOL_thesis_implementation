package evaluation

// This file executes version 2 with one shared schema, no repair pass, and complete failure retention.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"wails/ai"
)

const v2ResponseInstructions = `Return one concise JSON response matching the supplied schema. Use empty arrays rather than null. Use decimal ratios rather than percentages. Use only identifiers present in the frozen input. Do not add fields.`

// V2Runner owns one constrained-output evaluation execution.
type V2Runner struct {
	provider ai.Provider
	now      func() time.Time
}

// NewV2Runner creates an evaluator that has no broker, research, persistence, file, or trading tools.
func NewV2Runner(provider ai.Provider) *V2Runner {
	return &V2Runner{provider: provider, now: time.Now}
}

// VerifySchemaAcceptance performs a non-dataset call before a final experiment begins.
func (r *V2Runner) VerifySchemaAcceptance(ctx context.Context, suite V2Suite) (SchemaPreflight, error) {
	settings := suite.Manifest.Settings
	settings.MaxTokens = 512
	response, err := r.provider.Chat(ctx, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: "system", Content: v2ResponseInstructions},
			{Role: "user", Content: `Return a minimal general-information response. State that this is a schema preflight and no trade was executed.`},
		},
		ResponseSchema: suite.ResponseSchema,
		Settings:       settings,
	})
	preflight := SchemaPreflight{CompletedAt: r.now().UTC()}
	if err != nil {
		return preflight, fmt.Errorf("version 2 schema preflight failed: %w", err)
	}
	preflight.ProviderModel = response.Model
	preflight.FinishReason = response.FinishReason
	if _, err := parseV2Response(response.Message.Content); err != nil {
		return preflight, fmt.Errorf("version 2 schema preflight returned invalid structured output: %w", err)
	}
	preflight.Passed = true
	return preflight, nil
}

// Run executes all frozen calls after environment and schema preflight gates have passed.
func (r *V2Runner) Run(ctx context.Context, suite V2Suite, environment Environment, runKind string, preflight SchemaPreflight) V2Bundle {
	bundle := V2Bundle{
		CreatedAt: r.now().UTC(), RunKind: runKind, Manifest: suite.Manifest,
		Conditions: suite.Conditions, Scenarios: suite.Scenarios, ResponseSchema: suite.SchemaJSON,
		ScoringRules: suite.ScoringRules, ManualRubric: suite.ManualRubric,
		ProtocolHashes: suite.ProtocolHashes, Environment: environment, Preflight: preflight,
	}
	sequence := 0
	for _, scenario := range suite.Scenarios {
		for repetition := 1; repetition <= suite.Manifest.Repetitions; repetition++ {
			for _, condition := range orderedConditions(suite.Conditions, repetition) {
				sequence++
				bundle.Runs = append(bundle.Runs, r.runOne(ctx, suite, scenario, condition, repetition, sequence))
			}
		}
	}
	bundle.Summaries = summarizeV2Runs(bundle.Runs, suite.Conditions)
	return bundle
}

func (r *V2Runner) runOne(ctx context.Context, suite V2Suite, scenario FrozenScenario, condition Condition, repetition, sequence int) V2RunResult {
	started := r.now().UTC()
	result := V2RunResult{
		Sequence: sequence, Repetition: repetition, ScenarioID: scenario.ID,
		ConditionID: condition.ID, ConditionVersion: condition.Version, StartedAt: started,
	}
	contextJSON, evidence, _, err := v2ConditionContext(scenario, condition)
	if err != nil {
		result.FailureKind = "protocol_failure"
		result.Error = err.Error()
		result.Score = invalidScore()
		return result
	}
	callContext, cancel := context.WithTimeout(ctx, time.Duration(suite.Manifest.RequestTimeoutSeconds)*time.Second)
	response, err := r.provider.Chat(callContext, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: "system", Content: condition.SystemPrompt + "\n\n" + v2ResponseInstructions},
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
	result.FinishReason = response.FinishReason
	result.RawResponse = response.Message.Content
	parsed, parseErr := parseV2Response(response.Message.Content)
	if parseErr != nil {
		result.FailureKind = classifyV2ParseFailure(response.FinishReason, parseErr)
		result.Error = parseErr.Error()
		result.Score = invalidScore()
		return result
	}
	if validationErr := validateV2Response(parsed, condition, evidence); validationErr != nil {
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

func classifyV2ParseFailure(finishReason string, err error) string {
	if strings.EqualFold(strings.TrimSpace(finishReason), "length") {
		return "truncation"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unexpected end") || strings.Contains(message, "unexpected eof") || strings.Contains(message, "invalid character") {
		return "malformed_json"
	}
	return "structural_validation"
}
