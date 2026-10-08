// Command evaluate runs the frozen version 2 portfolio-prompt comparison.
package main

// This file enforces environment and schema gates before storing a pilot or final version 2 bundle.

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
	defaultBaseURL := strings.TrimSpace(os.Getenv("AI_BASE_URL"))
	if defaultBaseURL == "" {
		defaultBaseURL = "http://localhost:11434/api"
	}
	dataDirectory, err := config.DataDirectory()
	if err != nil {
		return err
	}
	baseURL := flag.String("base-url", defaultBaseURL, "Ollama API base URL")
	output := flag.String("output", filepath.Join("evaluation", "results"), "result bundle directory")
	databasePath := flag.String("database", filepath.Join(dataDirectory, "portfolio-tracker.db"), "SQLite database path")
	runKind := flag.String("run-kind", "pilot", "recorded run kind: pilot or final")
	preflightOnly := flag.Bool("preflight-only", false, "verify environment, model digest, and schema acceptance without starting an experiment")
	expectedDigest := flag.String("expected-model-digest", "", "must match the digest frozen in the version 2 manifest when supplied")
	timeout := flag.Duration("timeout", 2*time.Hour, "maximum duration for the complete suite")
	flag.Parse()

	suite, err := evaluation.LoadV2Suite()
	if err != nil {
		return err
	}
	kind := strings.ToLower(strings.TrimSpace(*runKind))
	if kind != "pilot" && kind != "final" {
		return fmt.Errorf("run-kind must be pilot or final")
	}
	if supplied := strings.TrimSpace(*expectedDigest); supplied != "" && !strings.EqualFold(supplied, suite.Manifest.ExpectedModelDigest) {
		return fmt.Errorf("supplied model digest differs from the frozen version 2 digest")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	environment, err := evaluation.InspectEnvironment(ctx, *baseURL, suite.Manifest.Model)
	if err != nil {
		return err
	}
	if !strings.EqualFold(suite.Manifest.ExpectedModelDigest, environment.ModelDigest) {
		return fmt.Errorf("model digest mismatch: expected %s, found %s", suite.Manifest.ExpectedModelDigest, environment.ModelDigest)
	}
	provider := ai.NewOllamaProvider(*baseURL, suite.Manifest.Model)
	runner := evaluation.NewV2Runner(provider)
	preflight, err := runner.VerifySchemaAcceptance(ctx, suite)
	if err != nil {
		return err
	}
	fmt.Printf("Schema preflight passed with %s on Ollama %s.\n", environment.Model, environment.OllamaVersion)
	if *preflightOnly {
		return nil
	}

	bundle := runner.Run(ctx, suite, environment, kind, preflight)
	paths, err := evaluation.SaveV2Bundle(*output, bundle)
	if err != nil {
		return err
	}
	store, err := persistence.Open(*databasePath)
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

func formatMetric(metric evaluation.MetricMean) string {
	if metric.Value == nil {
		return fmt.Sprintf("undefined (n=%d)", metric.Denominator)
	}
	return fmt.Sprintf("%.3f (n=%d)", *metric.Value, metric.Denominator)
}
