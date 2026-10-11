// Command evaluate runs a frozen portfolio-prompt comparison.
package main

// This file enforces provider and schema gates before storing a pilot or final evaluation bundle.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wails/ai"
	"wails/config"
	"wails/evaluation"
	"wails/persistence"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "evaluation failed:", err)
		os.Exit(1)
	}
}

func run() error {
	dataDirectory, err := config.DataDirectory()
	if err != nil {
		return err
	}
	suiteVersion := flag.Int("suite-version", 4, "evaluation protocol version: 2, 3, or 4")
	baseURL := flag.String("base-url", strings.TrimSpace(os.Getenv("AI_BASE_URL")), "provider API base URL")
	output := flag.String("output", filepath.Join("evaluation", "results"), "result bundle directory")
	databasePath := flag.String("database", filepath.Join(dataDirectory, "portfolio-tracker.db"), "SQLite database path")
	runKind := flag.String("run-kind", "pilot", "recorded run kind: pilot or final")
	pilotRepetitions := flag.Int("pilot-repetitions", 1, "repetitions per scenario for a version 3 pilot")
	preflightOnly := flag.Bool("preflight-only", false, "verify environment, model digest, and schema acceptance without starting an experiment")
	expectedDigest := flag.String("expected-model-digest", "", "must match the digest frozen in the version 2 manifest when supplied")
	timeout := flag.Duration("timeout", 2*time.Hour, "maximum duration for the complete suite")
	flag.Parse()

	kind := strings.ToLower(strings.TrimSpace(*runKind))
	if kind != "pilot" && kind != "final" {
		return fmt.Errorf("run-kind must be pilot or final")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	switch *suiteVersion {
	case 2:
		return runV2(ctx, *baseURL, *output, *databasePath, kind, *preflightOnly, *expectedDigest)
	case 3:
		return runV3(ctx, *baseURL, *output, *databasePath, kind, *preflightOnly, *pilotRepetitions)
	case 4:
		return runV4(ctx, *baseURL, *output, *databasePath, kind, *preflightOnly, *pilotRepetitions)
	default:
		return fmt.Errorf("suite-version must be 2, 3, or 4")
	}
}

func runV2(ctx context.Context, baseURL, output, databasePath, kind string, preflightOnly bool, expectedDigest string) error {
	if baseURL == "" {
		baseURL = "http://localhost:11434/api"
	}
	suite, err := evaluation.LoadV2Suite()
	if err != nil {
		return err
	}
	if supplied := strings.TrimSpace(expectedDigest); supplied != "" && !strings.EqualFold(supplied, suite.Manifest.ExpectedModelDigest) {
		return fmt.Errorf("supplied model digest differs from the frozen version 2 digest")
	}
	environment, err := evaluation.InspectEnvironment(ctx, baseURL, suite.Manifest.Model)
	if err != nil {
		return err
	}
	if !strings.EqualFold(suite.Manifest.ExpectedModelDigest, environment.ModelDigest) {
		return fmt.Errorf("model digest mismatch: expected %s, found %s", suite.Manifest.ExpectedModelDigest, environment.ModelDigest)
	}
	provider := ai.NewOllamaProvider(baseURL, suite.Manifest.Model)
	runner := evaluation.NewV2Runner(provider)
	preflight, err := runner.VerifySchemaAcceptance(ctx, suite)
	if err != nil {
		return err
	}
	fmt.Printf("Schema preflight passed with %s on Ollama %s.\n", environment.Model, environment.OllamaVersion)
	if preflightOnly {
		return nil
	}

	bundle := runner.Run(ctx, suite, environment, kind, preflight)
	paths, err := evaluation.SaveV2Bundle(output, bundle)
	if err != nil {
		return err
	}
	store, err := persistence.Open(databasePath)
	if err != nil {
		return fmt.Errorf("bundle stored at %s but database open failed: %w", paths.BundlePath, err)
	}
	defer store.Close()
	if err := store.SaveV2EvaluationBundle(context.WithoutCancel(ctx), bundle); err != nil {
		return fmt.Errorf("bundle stored at %s but database storage failed: %w", paths.BundlePath, err)
	}
	fmt.Printf("Stored %d version 2 %s runs in %s\n", len(bundle.Runs), kind, paths.BundlePath)
	fmt.Printf("Blind review: %s\nBlind mapping: %s\n", paths.ReviewPath, paths.MapPath)
	for _, summary := range bundle.Summaries {
		fmt.Printf("%s: schema %.3f (%d/%d), coverage %s, accuracy %s, grounding %s, repeatability %s\n",
			summary.ConditionID, summary.SchemaValidity, summary.SchemaValidRuns, summary.PlannedRuns,
			formatMetric(summary.RequiredFactCoverage), formatMetric(summary.NumericalAccuracy),
			formatMetric(summary.Grounding), formatMetric(summary.Repeatability),
		)
	}
	return nil
}

func runV3(ctx context.Context, baseURL, output, databasePath, kind string, preflightOnly bool, pilotRepetitions int) error {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required for version 3")
	}
	suite, err := evaluation.LoadV3Suite()
	if err != nil {
		return err
	}
	repetitions := suite.Manifest.Repetitions
	if kind == "pilot" {
		repetitions = pilotRepetitions
		if repetitions < 1 || repetitions > suite.Manifest.Repetitions {
			return fmt.Errorf("pilot-repetitions must be between 1 and %d", suite.Manifest.Repetitions)
		}
	}
	provider := ai.NewOpenAIProvider(baseURL, apiKey, suite.Manifest.Model)
	runner := evaluation.NewV3Runner(provider)
	preflight, err := runner.VerifySchemaAcceptance(ctx, suite)
	if err != nil {
		return err
	}
	fmt.Printf("Schema preflight passed with %s through OpenAI.\n", preflight.ProviderModel)
	if preflightOnly {
		return nil
	}

	bundle := runner.Run(ctx, suite, evaluation.InspectV3Environment(suite.Manifest), kind, repetitions, preflight)
	paths, err := evaluation.SaveV3Bundle(output, bundle)
	if err != nil {
		return err
	}
	store, err := persistence.Open(databasePath)
	if err != nil {
		return fmt.Errorf("bundle stored at %s but database open failed: %w", paths.BundlePath, err)
	}
	defer store.Close()
	if err := store.SaveV3EvaluationBundle(context.WithoutCancel(ctx), bundle); err != nil {
		return fmt.Errorf("bundle stored at %s but database storage failed: %w", paths.BundlePath, err)
	}
	fmt.Printf("Stored %d version 3 %s runs in %s\n", len(bundle.Runs), kind, paths.BundlePath)
	fmt.Printf("Blind review: %s\nBlind mapping: %s\n", paths.ReviewPath, paths.MapPath)
	fmt.Printf("Usage: %d input tokens (%d cached, %d cache writes), %d output tokens (%d reasoning), %d total.\n",
		bundle.Usage.InputTokens, bundle.Usage.CachedInputTokens, bundle.Usage.CacheWriteTokens, bundle.Usage.OutputTokens,
		bundle.Usage.ReasoningTokens, bundle.Usage.TotalTokens,
	)
	for _, summary := range bundle.Summaries {
		fmt.Printf("%s: schema %.3f (%d/%d), coverage %s, accuracy %s, grounding %s, repeatability %s\n",
			summary.ConditionID, summary.SchemaValidity, summary.SchemaValidRuns, summary.PlannedRuns,
			formatMetric(summary.RequiredFactCoverage), formatMetric(summary.NumericalAccuracy),
			formatMetric(summary.Grounding), formatMetric(summary.Repeatability),
		)
	}
	return nil
}

func runV4(ctx context.Context, baseURL, output, databasePath, kind string, preflightOnly bool, pilotRepetitions int) error {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required for version 4")
	}
	suite, err := evaluation.LoadV4Suite()
	if err != nil {
		return err
	}
	repetitions := suite.Manifest.Repetitions
	if kind == "pilot" {
		repetitions = pilotRepetitions
		if repetitions < 1 || repetitions > suite.Manifest.Repetitions {
			return fmt.Errorf("pilot-repetitions must be between 1 and %d", suite.Manifest.Repetitions)
		}
	}
	provider := ai.NewOpenAIProvider(baseURL, apiKey, suite.Manifest.Model)
	runner := evaluation.NewV4Runner(provider)
	preflight, err := runner.VerifySchemaAcceptance(ctx, suite)
	if err != nil {
		return err
	}
	fmt.Printf("Schema preflight passed with %s through OpenAI.\n", preflight.ProviderModel)
	if preflightOnly {
		return nil
	}

	bundle := runner.Run(ctx, suite, evaluation.InspectV4Environment(suite.Manifest), kind, repetitions, preflight)
	paths, err := evaluation.SaveV4Bundle(output, bundle)
	if err != nil {
		return err
	}
	store, err := persistence.Open(databasePath)
	if err != nil {
		return fmt.Errorf("bundle stored at %s but database open failed: %w", paths.BundlePath, err)
	}
	defer store.Close()
	if err := store.SaveV4EvaluationBundle(context.WithoutCancel(ctx), bundle); err != nil {
		return fmt.Errorf("bundle stored at %s but database storage failed: %w", paths.BundlePath, err)
	}
	fmt.Printf("Stored %d version 4 %s runs in %s\n", len(bundle.Runs), kind, paths.BundlePath)
	fmt.Printf("Blind review: %s\nBlind mapping: %s\n", paths.ReviewPath, paths.MapPath)
	fmt.Printf("Usage: %d input tokens (%d cached, %d cache writes), %d output tokens (%d reasoning), %d total.\n",
		bundle.Usage.InputTokens, bundle.Usage.CachedInputTokens, bundle.Usage.CacheWriteTokens, bundle.Usage.OutputTokens,
		bundle.Usage.ReasoningTokens, bundle.Usage.TotalTokens,
	)
	for _, summary := range bundle.Summaries {
		fmt.Printf("%s: schema %.3f (%d/%d), coverage %s, accuracy %s, grounding %s, repeatability %s\n",
			summary.ConditionID, summary.SchemaValidity, summary.SchemaValidRuns, summary.PlannedRuns,
			formatMetric(summary.RequiredFactCoverage), formatMetric(summary.NumericalAccuracy),
			formatMetric(summary.Grounding), formatMetric(summary.Repeatability),
		)
	}
	return nil
}

func formatMetric(metric evaluation.MetricMean) string {
	if metric.Value == nil {
		return fmt.Sprintf("undefined (n=%d)", metric.Denominator)
	}
	return fmt.Sprintf("%.3f (n=%d)", *metric.Value, metric.Denominator)
}
