//go:build showcase

package main

// showcase_enabled.go selects one controlled portfolio fixture for UI capture builds.

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"wails/evaluation"
)

const defaultShowcaseScenario = "concentrated-multicurrency"

// loadShowcaseScenario selects one embedded portfolio fixture for a tagged build.
func loadShowcaseScenario() (*showcaseScenario, error) {
	suite, err := evaluation.LoadV4Suite()
	if err != nil {
		return nil, err
	}
	scenarios := append([]evaluation.FrozenScenario(nil), suite.Scenarios...)
	additional, err := loadShowcaseScenarios()
	if err != nil {
		return nil, err
	}
	scenarios = append(scenarios, additional...)
	seenScenarios := make(map[string]struct{}, len(scenarios))
	for _, scenario := range scenarios {
		if _, exists := seenScenarios[scenario.ID]; exists {
			return nil, fmt.Errorf("duplicate showcase scenario ID %q", scenario.ID)
		}
		seenScenarios[scenario.ID] = struct{}{}
	}
	scenarioID := strings.TrimSpace(os.Getenv("SHOWCASE_SCENARIO"))
	if scenarioID == "" {
		scenarioID = defaultShowcaseScenario
	}
	for _, scenario := range scenarios {
		if scenario.ID == scenarioID {
			return &showcaseScenario{
				ID:          scenario.ID,
				Description: scenario.Description,
				SnapshotAt:  scenario.SnapshotAt,
				Portfolio:   scenario.Portfolio,
			}, nil
		}
	}
	available := make([]string, 0, len(scenarios))
	for _, scenario := range scenarios {
		available = append(available, scenario.ID)
	}
	sort.Strings(available)
	return nil, fmt.Errorf("unknown SHOWCASE_SCENARIO %q; choose one of %s", scenarioID, strings.Join(available, ", "))
}
