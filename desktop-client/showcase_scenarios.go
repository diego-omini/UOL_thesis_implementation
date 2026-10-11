//go:build showcase

package main

// showcase_scenarios.go strictly loads additional portfolio fixtures used only by showcase builds.

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"wails/evaluation"
)

//go:embed showcase/scenarios/*.json
var showcaseScenarioFiles embed.FS

func loadShowcaseScenarios() ([]evaluation.FrozenScenario, error) {
	paths, err := fs.Glob(showcaseScenarioFiles, "showcase/scenarios/*.json")
	if err != nil {
		return nil, fmt.Errorf("find showcase scenarios: %w", err)
	}
	scenarios := make([]evaluation.FrozenScenario, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		contents, readErr := showcaseScenarioFiles.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", path, readErr)
		}
		var scenario evaluation.FrozenScenario
		decoder := json.NewDecoder(strings.NewReader(string(contents)))
		decoder.DisallowUnknownFields()
		if decodeErr := decoder.Decode(&scenario); decodeErr != nil {
			return nil, fmt.Errorf("decode %s: %w", path, decodeErr)
		}
		if decodeErr := decoder.Decode(&struct{}{}); decodeErr != io.EOF {
			return nil, fmt.Errorf("decode %s: expected one JSON object", path)
		}
		if validationErr := validateShowcaseScenario(scenario); validationErr != nil {
			return nil, fmt.Errorf("validate %s: %w", path, validationErr)
		}
		if _, exists := seen[scenario.ID]; exists {
			return nil, fmt.Errorf("duplicate showcase scenario ID %q", scenario.ID)
		}
		seen[scenario.ID] = struct{}{}
		scenarios = append(scenarios, scenario)
	}
	return scenarios, nil
}

func validateShowcaseScenario(scenario evaluation.FrozenScenario) error {
	if strings.TrimSpace(scenario.ID) == "" || strings.TrimSpace(scenario.Description) == "" {
		return fmt.Errorf("scenario ID and description are required")
	}
	if scenario.Version != 1 || scenario.Provenance != "controlled_showcase_scenario" {
		return fmt.Errorf("scenario %q has an unsupported version or provenance", scenario.ID)
	}
	if scenario.SnapshotAt.IsZero() || len(scenario.Portfolio.Positions) == 0 {
		return fmt.Errorf("scenario %q requires a timestamp and positions", scenario.ID)
	}
	return nil
}
