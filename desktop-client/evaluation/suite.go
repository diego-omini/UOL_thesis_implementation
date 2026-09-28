package evaluation

// This file embeds and validates the fixed evaluation manifest, condition definitions, and scenarios.

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"wails/analytics"
)

//go:embed manifest.yaml conditions.yaml manual_rubric.yaml scenarios/*.json
var suiteFiles embed.FS

// LoadSuite returns the versioned evaluation definition embedded in the executable.
func LoadSuite() (Manifest, []Condition, []FrozenScenario, ManualRubric, error) {
	manifestBytes, err := fs.ReadFile(suiteFiles, "manifest.yaml")
	if err != nil {
		return Manifest{}, nil, nil, ManualRubric{}, fmt.Errorf("read evaluation manifest: %w", err)
	}
	var manifest Manifest
	if err := decodeYAML(manifestBytes, &manifest); err != nil {
		return Manifest{}, nil, nil, ManualRubric{}, fmt.Errorf("decode evaluation manifest: %w", err)
	}
	conditionBytes, err := fs.ReadFile(suiteFiles, "conditions.yaml")
	if err != nil {
		return Manifest{}, nil, nil, ManualRubric{}, fmt.Errorf("read evaluation conditions: %w", err)
	}
	var definitions conditionFile
	if err := decodeYAML(conditionBytes, &definitions); err != nil {
		return Manifest{}, nil, nil, ManualRubric{}, fmt.Errorf("decode evaluation conditions: %w", err)
	}
	rubricBytes, err := fs.ReadFile(suiteFiles, "manual_rubric.yaml")
	if err != nil {
		return Manifest{}, nil, nil, ManualRubric{}, fmt.Errorf("read manual rubric: %w", err)
	}
	var rubric ManualRubric
	if err := decodeYAML(rubricBytes, &rubric); err != nil {
		return Manifest{}, nil, nil, ManualRubric{}, fmt.Errorf("decode manual rubric: %w", err)
	}

	paths, err := fs.Glob(suiteFiles, "scenarios/*.json")
	if err != nil {
		return Manifest{}, nil, nil, ManualRubric{}, fmt.Errorf("find evaluation scenarios: %w", err)
	}
	sort.Strings(paths)
	scenarios := make([]FrozenScenario, 0, len(paths))
	for _, path := range paths {
		contents, readErr := fs.ReadFile(suiteFiles, path)
		if readErr != nil {
			return Manifest{}, nil, nil, ManualRubric{}, fmt.Errorf("read %s: %w", path, readErr)
		}
		var scenario FrozenScenario
		if decodeErr := json.Unmarshal(contents, &scenario); decodeErr != nil {
			return Manifest{}, nil, nil, ManualRubric{}, fmt.Errorf("decode %s: %w", path, decodeErr)
		}
		scenarios = append(scenarios, scenario)
	}
	if err := validateSuite(manifest, definitions.Conditions, scenarios, rubric); err != nil {
		return Manifest{}, nil, nil, ManualRubric{}, err
	}
	return manifest, definitions.Conditions, scenarios, rubric, nil
}

func decodeYAML(contents []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	return decoder.Decode(target)
}

func validateSuite(manifest Manifest, conditions []Condition, scenarios []FrozenScenario, rubric ManualRubric) error {
	if strings.TrimSpace(manifest.SuiteID) == "" || manifest.Version < 1 || strings.TrimSpace(manifest.Model) == "" {
		return fmt.Errorf("evaluation manifest requires a suite ID, version, and model")
	}
	if manifest.Repetitions < 2 {
		return fmt.Errorf("evaluation repetitions must be at least 2")
	}
	if manifest.OrderStrategy != "alternating" {
		return fmt.Errorf("unsupported evaluation order strategy %q", manifest.OrderStrategy)
	}
	if manifest.Settings.MaxTokens < 1 || manifest.Settings.ContextSize < 1 || manifest.Settings.Seed == 0 {
		return fmt.Errorf("evaluation generation settings require max tokens, context size, and a non-zero seed")
	}
	if len(conditions) != 2 || len(scenarios) < 3 {
		return fmt.Errorf("evaluation suite requires exactly two conditions and at least three scenarios")
	}
	conditionIDs := map[string]bool{}
	for _, condition := range conditions {
		if condition.ID == "" || condition.Version < 1 || condition.SystemPrompt == "" || condition.UserPrompt == "" {
			return fmt.Errorf("evaluation condition is incomplete")
		}
		if condition.Context != "raw" && condition.Context != "analytics" {
			return fmt.Errorf("condition %q has unsupported context %q", condition.ID, condition.Context)
		}
		if conditionIDs[condition.ID] {
			return fmt.Errorf("duplicate evaluation condition %q", condition.ID)
		}
		conditionIDs[condition.ID] = true
	}
	for _, required := range []string{"baseline", "proposed"} {
		if !conditionIDs[required] {
			return fmt.Errorf("evaluation condition %q is required", required)
		}
	}
	if rubric.Version < 1 || strings.TrimSpace(rubric.Instructions) == "" || len(rubric.Criteria) == 0 {
		return fmt.Errorf("manual evaluation rubric is incomplete")
	}
	criterionIDs := map[string]bool{}
	for _, criterion := range rubric.Criteria {
		if criterion.ID == "" || criterion.Description == "" || len(criterion.Anchors) < 2 || criterionIDs[criterion.ID] {
			return fmt.Errorf("manual evaluation rubric contains an invalid criterion")
		}
		criterionIDs[criterion.ID] = true
	}

	scenarioIDs := map[string]bool{}
	for _, scenario := range scenarios {
		if scenario.ID == "" || scenario.Version < 1 || scenario.SnapshotAt.IsZero() || len(scenario.Portfolio.Positions) == 0 {
			return fmt.Errorf("evaluation scenario is incomplete")
		}
		if scenario.Provenance != "controlled_sanitized_scenario" {
			return fmt.Errorf("scenario %q must state its controlled sanitized provenance", scenario.ID)
		}
		if scenarioIDs[scenario.ID] {
			return fmt.Errorf("duplicate evaluation scenario %q", scenario.ID)
		}
		scenarioIDs[scenario.ID] = true
		metrics := metricLookup(buildMetrics(scenario.Portfolio, analytics.Calculate(scenario.Portfolio, scenario.SnapshotAt)))
		for _, fact := range scenario.RequiredFacts {
			metric, ok := metrics[fact.MetricID]
			if !ok || !metric.Available {
				return fmt.Errorf("scenario %q requires unavailable metric %q", scenario.ID, fact.MetricID)
			}
			if math.Abs(metric.Value-fact.Value) > fact.Tolerance {
				return fmt.Errorf("scenario %q expected %s=%g but calculated %g", scenario.ID, fact.MetricID, fact.Value, metric.Value)
			}
		}
	}
	return nil
}
