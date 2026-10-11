package evaluation

// This file executes the hosted OpenAI version 3 experiment and retains provider usage metadata.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"wails/ai"
)

// V3Runner owns one OpenAI constrained-output evaluation execution.
type V3Runner struct {
	provider ai.Provider
	now      func() time.Time
}

// NewV3Runner creates an evaluator without broker, research, persistence, file, or trading tools.
func NewV3Runner(provider ai.Provider) *V3Runner {
	return &V3Runner{provider: provider, now: time.Now}
}

// VerifySchemaAcceptance performs a non-dataset hosted call before the pilot begins.
func (r *V3Runner) VerifySchemaAcceptance(ctx context.Context, suite V3Suite) (SchemaPreflight, error) {
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
		return preflight, fmt.Errorf("version 3 schema preflight failed: %w", err)
	}
	preflight.ProviderModel = response.Model
	preflight.FinishReason = response.FinishReason
	preflight.ResponseID = response.ResponseID
	preflight.ServiceTier = response.ServiceTier
	preflight.ReasoningEffort = response.EffectiveReasoningEffort
	preflight.Usage = response.Usage
	if _, err := parseV2Response(response.Message.Content); err != nil {
		return preflight, fmt.Errorf("version 3 schema preflight returned invalid structured output: %w", err)
	}
	preflight.Passed = true
	return preflight, nil
}

// Run executes a bounded pilot or the complete frozen experiment.
func (r *V3Runner) Run(ctx context.Context, suite V3Suite, environment V3Environment, runKind string, repetitions int, preflight SchemaPreflight) V3Bundle {
	bundle := V3Bundle{
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

func (r *V3Runner) runOne(ctx context.Context, suite V3Suite, scenario FrozenScenario, condition Condition, repetition, sequence int) V2RunResult {
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
	result.ResponseID = response.ResponseID
	result.ServiceTier = response.ServiceTier
	result.ReasoningEffort = response.EffectiveReasoningEffort
	result.Usage = response.Usage
	result.FinishReason = response.FinishReason
	result.RawResponse = response.Message.Content
	parsed, parseErr := parseV2Response(response.Message.Content)
	if parseErr != nil {
		result.FailureKind = classifyV3ParseFailure(response.FinishReason, parseErr)
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

func classifyV3ParseFailure(finishReason string, err error) string {
	if strings.EqualFold(strings.TrimSpace(finishReason), "max_output_tokens") || strings.EqualFold(strings.TrimSpace(finishReason), "length") {
		return "truncation"
	}
	return classifyV2ParseFailure(finishReason, err)
}

func totalV3Usage(runs []V2RunResult) UsageTotals {
	var total UsageTotals
	for _, run := range runs {
		if run.ResponseID != "" || run.Usage.TotalTokens > 0 {
			total.CompletedCalls++
		}
		total.InputTokens += run.Usage.InputTokens
		total.CachedInputTokens += run.Usage.CachedInputTokens
		total.CacheWriteTokens += run.Usage.CacheWriteTokens
		total.OutputTokens += run.Usage.OutputTokens
		total.ReasoningTokens += run.Usage.ReasoningTokens
		total.TotalTokens += run.Usage.TotalTokens
	}
	return total
}
