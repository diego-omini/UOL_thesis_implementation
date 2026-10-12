package evaluation

// This file executes the version 5 comparison across fixed local and hosted model configurations.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"wails/ai"
)

// V5Provider binds a fixed configuration to its provider adapter.
type V5Provider struct {
	Configuration V5ModelConfiguration
	Provider      ai.Provider
}

// V5Runner owns one complete cross-model evaluation execution.
type V5Runner struct {
	providers []V5Provider
	now       func() time.Time
	progress  func(V2RunResult)
}

// NewV5Runner creates an evaluator without broker, research, persistence, file, or trading tools.
func NewV5Runner(providers []V5Provider) *V5Runner {
	return &V5Runner{providers: providers, now: time.Now}
}

// SetProgress registers an observer for completed calls without changing evaluation behavior.
func (r *V5Runner) SetProgress(progress func(V2RunResult)) {
	r.progress = progress
}

// VerifySchemaAcceptance checks the shared schema once with every fixed model before dataset calls begin.
func (r *V5Runner) VerifySchemaAcceptance(ctx context.Context, suite V5Suite) ([]V5Preflight, error) {
	if err := r.validateProviders(suite); err != nil {
		return nil, err
	}
	preflights := make([]V5Preflight, 0, len(r.providers))
	for _, configured := range r.providers {
		settings := configured.Configuration.Settings
		settings.MaxTokens = 512
		response, err := configured.Provider.Chat(ctx, ai.ChatRequest{
			Messages: []ai.Message{
				{Role: "system", Content: v4ResponseInstructions},
				{Role: "user", Content: `Return a minimal general-information response. State that this is a schema preflight and no trade was executed.`},
			},
			ResponseSchema: suite.ResponseSchema,
			Settings:       settings,
		})
		preflight := SchemaPreflight{CompletedAt: r.now().UTC()}
		if err != nil {
			return preflights, fmt.Errorf("version 5 schema preflight for %s failed: %w", configured.Configuration.ID, err)
		}
		preflight.ProviderModel = response.Model
		preflight.FinishReason = response.FinishReason
		preflight.ResponseID = response.ResponseID
		preflight.ServiceTier = response.ServiceTier
		preflight.ReasoningEffort = response.EffectiveReasoningEffort
		preflight.Usage = response.Usage
		if _, err := parseV2Response(response.Message.Content); err != nil {
			return preflights, fmt.Errorf("version 5 schema preflight for %s returned invalid structured output: %w", configured.Configuration.ID, err)
		}
		preflight.Passed = true
		preflights = append(preflights, V5Preflight{ConfigurationID: configured.Configuration.ID, Result: preflight})
	}
	return preflights, nil
}

// Run executes every model configuration while keeping scores and usage separated.
func (r *V5Runner) Run(ctx context.Context, suite V5Suite, environment V5Environment, runKind string, repetitions int, preflights []V5Preflight) V5Bundle {
	bundle := V5Bundle{
		CreatedAt: r.now().UTC(), RunKind: runKind, ExecutedRepetitions: repetitions,
		Manifest: suite.Manifest, Conditions: suite.Conditions, Scenarios: suite.Scenarios,
		ResponseSchema: suite.SchemaJSON, ScoringRules: suite.ScoringRules, ManualRubric: suite.ManualRubric,
		ProtocolHashes: suite.ProtocolHashes, Environment: environment, Preflights: preflights,
	}
	for _, configured := range r.providers {
		bundle.ExecutedConfigurations = append(bundle.ExecutedConfigurations, configured.Configuration.ID)
	}
	sequence := 0
	for _, configured := range r.providers {
		configurationRuns := make([]V2RunResult, 0, len(suite.Scenarios)*len(suite.Conditions)*repetitions)
		for _, scenario := range suite.Scenarios {
			for repetition := 1; repetition <= repetitions; repetition++ {
				for _, condition := range orderedConditions(suite.Conditions, repetition) {
					sequence++
					run := r.runOne(ctx, suite, configured, scenario, condition, repetition, sequence)
					configurationRuns = append(configurationRuns, run)
					bundle.Runs = append(bundle.Runs, run)
					if r.progress != nil {
						r.progress(run)
					}
				}
			}
		}
		for _, summary := range summarizeV2Runs(configurationRuns, suite.Conditions) {
			bundle.Summaries = append(bundle.Summaries, V5ConditionSummary{
				ConfigurationID: configured.Configuration.ID,
				Provider:        configured.Configuration.Provider,
				ConfiguredModel: configured.Configuration.Model,
				Scores:          summary,
			})
		}
		bundle.Usage = append(bundle.Usage, V5UsageSummary{
			ConfigurationID: configured.Configuration.ID,
			Usage:           totalV3Usage(configurationRuns),
		})
	}
	return bundle
}

func (r *V5Runner) validateProviders(suite V5Suite) error {
	if len(r.providers) == 0 {
		return fmt.Errorf("version 5 requires at least one fixed provider configuration")
	}
	seen := make(map[string]bool, len(r.providers))
	for index, actual := range r.providers {
		if actual.Provider == nil || seen[actual.Configuration.ID] {
			return fmt.Errorf("version 5 provider %d is missing or duplicated", index+1)
		}
		seen[actual.Configuration.ID] = true
		matched := false
		for _, expected := range suite.Manifest.Configurations {
			if actual.Configuration.ID == expected.ID && actual.Configuration.Provider == expected.Provider && actual.Configuration.Model == expected.Model {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("version 5 provider %d is not part of the fixed model matrix", index+1)
		}
	}
	return nil
}

func (r *V5Runner) runOne(ctx context.Context, suite V5Suite, configured V5Provider, scenario FrozenScenario, condition Condition, repetition, sequence int) V2RunResult {
	started := r.now().UTC()
	result := V2RunResult{
		Sequence: sequence, ConfigurationID: configured.Configuration.ID, Repetition: repetition,
		ScenarioID: scenario.ID, ConditionID: condition.ID, ConditionVersion: condition.Version, StartedAt: started,
	}
	contextJSON, evidence, _, err := v4ConditionContext(scenario, condition)
	if err != nil {
		result.FailureKind = "protocol_failure"
		result.Error = err.Error()
		result.Score = invalidScore()
		return result
	}
	callContext, cancel := context.WithTimeout(ctx, time.Duration(configured.Configuration.RequestTimeoutSeconds)*time.Second)
	response, err := configured.Provider.Chat(callContext, ai.ChatRequest{
		Messages: []ai.Message{
			{Role: "system", Content: condition.SystemPrompt + "\n\n" + v4ResponseInstructions},
			{Role: "user", Content: condition.UserPrompt + "\n\nFrozen input:\n" + contextJSON},
		},
		ResponseSchema: suite.ResponseSchema,
		Settings:       configured.Configuration.Settings,
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
		result.FailureKind = classifyV5ParseFailure(response.FinishReason, parseErr)
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

func classifyV5ParseFailure(finishReason string, err error) string {
	if strings.EqualFold(strings.TrimSpace(finishReason), "max_output_tokens") || strings.EqualFold(strings.TrimSpace(finishReason), "length") {
		return "truncation"
	}
	return classifyV2ParseFailure(finishReason, err)
}
