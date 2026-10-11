//go:build !showcase

package main

// showcase_disabled.go keeps controlled scenarios out of normal application builds.

func loadShowcaseScenario() (*showcaseScenario, error) {
	return nil, nil
}
