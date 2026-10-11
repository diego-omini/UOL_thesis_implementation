//go:build !showcase

package main

// showcase_disabled_test.go verifies normal builds cannot activate controlled fixtures.

import "testing"

func TestNormalBuildCannotLoadShowcaseScenario(t *testing.T) {
	t.Setenv("SHOWCASE_SCENARIO", "concentrated-multicurrency")

	scenario, err := loadShowcaseScenario()
	if err != nil {
		t.Fatal(err)
	}
	if scenario != nil {
		t.Fatal("normal build unexpectedly loaded a showcase scenario")
	}
}
