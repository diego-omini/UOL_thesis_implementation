package evaluation

// This file writes complete evaluation bundles atomically as human-readable JSON artifacts.

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

// V2StoredPaths identifies the final bundle and the separated blind-review exports.
type V2StoredPaths struct {
	BundlePath string
	ReviewPath string
	MapPath    string
}

type blindReviewEntry struct {
	ReviewID       string                `json:"review_id"`
	ScenarioID     string                `json:"scenario_id"`
	Repetition     int                   `json:"repetition"`
	RawResponse    string                `json:"raw_response"`
	ParsedResponse *V2EvaluationResponse `json:"parsed_response,omitempty"`
	FailureKind    string                `json:"failure_kind,omitempty"`
	Error          string                `json:"error,omitempty"`
}

type blindMapEntry struct {
	ReviewID         string `json:"review_id"`
	Sequence         int    `json:"sequence"`
	ConditionID      string `json:"condition_id"`
	ConditionVersion int    `json:"condition_version"`
}

// SaveV2Bundle atomically stores the versioned bundle and separate condition-blind review files.
func SaveV2Bundle(directory string, bundle V2Bundle) (V2StoredPaths, error) {
	if strings.TrimSpace(directory) == "" {
		return V2StoredPaths{}, fmt.Errorf("evaluation result directory is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return V2StoredPaths{}, fmt.Errorf("resolve evaluation result directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0750); err != nil {
		return V2StoredPaths{}, fmt.Errorf("create evaluation result directory: %w", err)
	}
	suiteID := filenamePattern.ReplaceAllString(bundle.Manifest.SuiteID, "-")
	runKind := filenamePattern.ReplaceAllString(bundle.RunKind, "-")
	base := fmt.Sprintf("%s-v%d-%s-%s", suiteID, bundle.Manifest.Version, runKind, bundle.CreatedAt.UTC().Format("20060102T150405Z"))
	paths := V2StoredPaths{
		BundlePath: filepath.Join(absolute, base+".json"),
		ReviewPath: filepath.Join(absolute, base+"-blind-review.json"),
		MapPath:    filepath.Join(absolute, base+"-blind-map.json"),
	}
	for _, path := range []string{paths.BundlePath, paths.ReviewPath, paths.MapPath} {
		if _, statErr := os.Stat(path); statErr == nil {
			return V2StoredPaths{}, fmt.Errorf("refusing to overwrite existing evaluation artifact %s", path)
		} else if !os.IsNotExist(statErr) {
			return V2StoredPaths{}, fmt.Errorf("inspect evaluation destination %s: %w", path, statErr)
		}
	}
	review, mapping, err := buildBlindExports(bundle.Runs)
	if err != nil {
		return V2StoredPaths{}, err
	}
	artifacts := []struct {
		path  string
		value any
	}{
		{paths.BundlePath, bundle},
		{paths.ReviewPath, review},
		{paths.MapPath, mapping},
	}
	for _, artifact := range artifacts {
		if err := writeAtomicJSON(absolute, artifact.path, artifact.value); err != nil {
			return V2StoredPaths{}, err
		}
	}
	return paths, nil
}

// SaveV3Bundle atomically stores the OpenAI bundle and separate condition-blind review files.
func SaveV3Bundle(directory string, bundle V3Bundle) (V2StoredPaths, error) {
	if strings.TrimSpace(directory) == "" {
		return V2StoredPaths{}, fmt.Errorf("evaluation result directory is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return V2StoredPaths{}, fmt.Errorf("resolve evaluation result directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0750); err != nil {
		return V2StoredPaths{}, fmt.Errorf("create evaluation result directory: %w", err)
	}
	suiteID := filenamePattern.ReplaceAllString(bundle.Manifest.SuiteID, "-")
	runKind := filenamePattern.ReplaceAllString(bundle.RunKind, "-")
	base := fmt.Sprintf("%s-v%d-%s-%s", suiteID, bundle.Manifest.Version, runKind, bundle.CreatedAt.UTC().Format("20060102T150405Z"))
	paths := V2StoredPaths{
		BundlePath: filepath.Join(absolute, base+".json"),
		ReviewPath: filepath.Join(absolute, base+"-blind-review.json"),
		MapPath:    filepath.Join(absolute, base+"-blind-map.json"),
	}
	for _, path := range []string{paths.BundlePath, paths.ReviewPath, paths.MapPath} {
		if _, statErr := os.Stat(path); statErr == nil {
			return V2StoredPaths{}, fmt.Errorf("refusing to overwrite existing evaluation artifact %s", path)
		} else if !os.IsNotExist(statErr) {
			return V2StoredPaths{}, fmt.Errorf("inspect evaluation destination %s: %w", path, statErr)
		}
	}
	review, mapping, err := buildBlindExports(bundle.Runs)
	if err != nil {
		return V2StoredPaths{}, err
	}
	artifacts := []struct {
		path  string
		value any
	}{
		{paths.BundlePath, bundle},
		{paths.ReviewPath, review},
		{paths.MapPath, mapping},
	}
	for _, artifact := range artifacts {
		if err := writeAtomicJSON(absolute, artifact.path, artifact.value); err != nil {
			return V2StoredPaths{}, err
		}
	}
	return paths, nil
}

// SaveV4Bundle atomically stores the corrected bundle and separate condition-blind review files.
func SaveV4Bundle(directory string, bundle V4Bundle) (V2StoredPaths, error) {
	if strings.TrimSpace(directory) == "" {
		return V2StoredPaths{}, fmt.Errorf("evaluation result directory is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return V2StoredPaths{}, fmt.Errorf("resolve evaluation result directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0750); err != nil {
		return V2StoredPaths{}, fmt.Errorf("create evaluation result directory: %w", err)
	}
	suiteID := filenamePattern.ReplaceAllString(bundle.Manifest.SuiteID, "-")
	runKind := filenamePattern.ReplaceAllString(bundle.RunKind, "-")
	base := fmt.Sprintf("%s-v%d-%s-%s", suiteID, bundle.Manifest.Version, runKind, bundle.CreatedAt.UTC().Format("20060102T150405Z"))
	paths := V2StoredPaths{
		BundlePath: filepath.Join(absolute, base+".json"),
		ReviewPath: filepath.Join(absolute, base+"-blind-review.json"),
		MapPath:    filepath.Join(absolute, base+"-blind-map.json"),
	}
	for _, path := range []string{paths.BundlePath, paths.ReviewPath, paths.MapPath} {
		if _, statErr := os.Stat(path); statErr == nil {
			return V2StoredPaths{}, fmt.Errorf("refusing to overwrite existing evaluation artifact %s", path)
		} else if !os.IsNotExist(statErr) {
			return V2StoredPaths{}, fmt.Errorf("inspect evaluation destination %s: %w", path, statErr)
		}
	}
	review, mapping, err := buildBlindExports(bundle.Runs)
	if err != nil {
		return V2StoredPaths{}, err
	}
	artifacts := []struct {
		path  string
		value any
	}{
		{paths.BundlePath, bundle},
		{paths.ReviewPath, review},
		{paths.MapPath, mapping},
	}
	for _, artifact := range artifacts {
		if err := writeAtomicJSON(absolute, artifact.path, artifact.value); err != nil {
			return V2StoredPaths{}, err
		}
	}
	return paths, nil
}

func buildBlindExports(runs []V2RunResult) ([]blindReviewEntry, []blindMapEntry, error) {
	review := make([]blindReviewEntry, 0, len(runs))
	mapping := make([]blindMapEntry, 0, len(runs))
	for _, run := range runs {
		identifierBytes := make([]byte, 8)
		if _, err := rand.Read(identifierBytes); err != nil {
			return nil, nil, fmt.Errorf("create blind review identifier: %w", err)
		}
		identifier := fmt.Sprintf("R-%x", identifierBytes)
		review = append(review, blindReviewEntry{
			ReviewID: identifier, ScenarioID: run.ScenarioID, Repetition: run.Repetition,
			RawResponse: run.RawResponse, ParsedResponse: run.ParsedResponse,
			FailureKind: run.FailureKind, Error: run.Error,
		})
		mapping = append(mapping, blindMapEntry{
			ReviewID: identifier, Sequence: run.Sequence, ConditionID: run.ConditionID,
			ConditionVersion: run.ConditionVersion,
		})
	}
	sort.Slice(review, func(i, j int) bool { return review[i].ReviewID < review[j].ReviewID })
	return review, mapping, nil
}

func writeAtomicJSON(directory, destination string, value any) error {
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode evaluation artifact: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".evaluation-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary evaluation artifact: %w", err)
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
		return fmt.Errorf("write evaluation artifact: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync evaluation artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close evaluation artifact: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("publish evaluation artifact: %w", err)
	}
	cleanup = false
	return nil
}
