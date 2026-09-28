// Command evaluate runs the fixed portfolio-prompt comparison and stores one JSON result bundle.
package main

// This file wires the embedded evaluation suite to the local Ollama provider.

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
	expectedDigest := flag.String("expected-model-digest", "", "optional exact Ollama model digest required before execution")
	timeout := flag.Duration("timeout", 45*time.Minute, "maximum duration for the complete suite")
	flag.Parse()

	manifest, conditions, scenarios, rubric, err := evaluation.LoadSuite()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	environment, err := evaluation.InspectEnvironment(ctx, *baseURL, manifest.Model)
	if err != nil {
		return err
	}
	expected := strings.TrimSpace(*expectedDigest)
	if expected == "" {
		expected = strings.TrimSpace(manifest.ExpectedModelDigest)
	}
	if expected != "" && !strings.EqualFold(expected, environment.ModelDigest) {
		return fmt.Errorf("model digest mismatch: expected %s, found %s", expected, environment.ModelDigest)
	}
	manifest.ExpectedModelDigest = expected
	store, err := persistence.Open(*databasePath)
	if err != nil {
		return err
	}
	defer store.Close()

	provider := ai.NewOllamaProvider(*baseURL, manifest.Model)
	bundle := evaluation.NewRunner(provider).Run(ctx, manifest, conditions, scenarios, rubric, environment)
	path, err := evaluation.SaveBundle(*output, bundle)
	if err != nil {
		return err
	}
	if err := store.SaveEvaluationBundle(context.WithoutCancel(ctx), bundle); err != nil {
		return err
	}
	fmt.Printf("Stored %d evaluation runs in %s\n", len(bundle.Runs), path)
	for _, summary := range bundle.Summaries {
		fmt.Printf(
			"%s: schema %.2f, coverage %.2f, accuracy %.2f, grounding %.2f, repeatability %.2f\n",
			summary.ConditionID, summary.SchemaValidity, summary.RequiredFactCoverage,
			summary.NumericalAccuracy, summary.Grounding, summary.Repeatability,
		)
	}
	return nil
}
