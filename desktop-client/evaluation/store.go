package evaluation

// This file writes complete evaluation bundles atomically as human-readable JSON artifacts.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var filenamePattern = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// SaveBundle writes one timestamped result file and returns its absolute path.
func SaveBundle(directory string, bundle Bundle) (string, error) {
	if strings.TrimSpace(directory) == "" {
		return "", fmt.Errorf("evaluation result directory is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve evaluation result directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0750); err != nil {
		return "", fmt.Errorf("create evaluation result directory: %w", err)
	}
	suiteID := filenamePattern.ReplaceAllString(bundle.Manifest.SuiteID, "-")
	filename := fmt.Sprintf("%s-v%d-%s.json", suiteID, bundle.Manifest.Version, bundle.CreatedAt.UTC().Format("20060102T150405Z"))
	destination := filepath.Join(absolute, filename)
	contents, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode evaluation result: %w", err)
	}
	temporary, err := os.CreateTemp(absolute, ".evaluation-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary evaluation result: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write evaluation result: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close evaluation result: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return "", fmt.Errorf("publish evaluation result: %w", err)
	}
	cleanup = false
	return destination, nil
}
