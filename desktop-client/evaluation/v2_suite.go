package evaluation

// This file loads and verifies the frozen version 2 protocol without changing version 1 definitions.

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"wails/analytics"
)

//go:embed v2/*.yaml v2/*.json scenarios/*.json
var v2Files embed.FS

var v2DefinitionPaths = []string{
	"v2/manifest.yaml",
	"v2/conditions.yaml",
	"v2/response_schema.json",
	"v2/scoring_rules.yaml",
	"v2/manual_rubric.yaml",
	"scenarios/01_concentrated_multicurrency.json",
	"scenarios/02_diversified_mixed_exposure.json",
	"scenarios/03_incomplete_data.json",
}

type v2ProtocolLock struct {
	Algorithm   string            `json:"algorithm"`
	Definitions map[string]string `json:"definitions"`
}

// V2Suite contains every definition frozen before a version 2 model call.
type V2Suite struct {
	Manifest       V2Manifest
	Conditions     []Condition
	Scenarios      []FrozenScenario
	ResponseSchema map[string]any
	SchemaJSON     json.RawMessage
	ScoringRules   V2ScoringRules
	ManualRubric   V2ManualRubric
	ProtocolHashes ProtocolHashes
}

// LoadV2Suite verifies the protocol lock and returns the separately versioned experiment.
func LoadV2Suite() (V2Suite, error) {
	hashes, err := verifyV2ProtocolLock()
	if err != nil {
		return V2Suite{}, err
	}
	var suite V2Suite
	if err := decodeV2YAML("v2/manifest.yaml", &suite.Manifest); err != nil {
		return V2Suite{}, err
	}
	var definitions conditionFile
	if err := decodeV2YAML("v2/conditions.yaml", &definitions); err != nil {
		return V2Suite{}, err
	}
	suite.Conditions = definitions.Conditions
	if err := decodeV2YAML("v2/scoring_rules.yaml", &suite.ScoringRules); err != nil {
		return V2Suite{}, err
	}
	if err := decodeV2YAML("v2/manual_rubric.yaml", &suite.ManualRubric); err != nil {
		return V2Suite{}, err
	}
	schema, err := fs.ReadFile(v2Files, "v2/response_schema.json")
	if err != nil {
		return V2Suite{}, fmt.Errorf("read version 2 response schema: %w", err)
	}
	if err := decodeStrictJSON(schema, &suite.ResponseSchema); err != nil {
		return V2Suite{}, fmt.Errorf("decode version 2 response schema: %w", err)
	}
	suite.SchemaJSON = append(json.RawMessage(nil), schema...)
	paths, err := fs.Glob(v2Files, "scenarios/*.json")
	if err != nil {
		return V2Suite{}, fmt.Errorf("find version 2 scenarios: %w", err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		contents, readErr := fs.ReadFile(v2Files, path)
		if readErr != nil {
			return V2Suite{}, fmt.Errorf("read %s: %w", path, readErr)
		}
		var scenario FrozenScenario
		if decodeErr := decodeStrictJSON(contents, &scenario); decodeErr != nil {
			return V2Suite{}, fmt.Errorf("decode %s: %w", path, decodeErr)
		}
		suite.Scenarios = append(suite.Scenarios, scenario)
	}
	suite.ProtocolHashes = hashes
	if err := validateV2Suite(suite); err != nil {
		return V2Suite{}, err
	}
	return suite, nil
}

func decodeV2YAML(path string, target any) error {
	contents, err := fs.ReadFile(v2Files, path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func decodeStrictJSON(contents []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func verifyV2ProtocolLock() (ProtocolHashes, error) {
	contents, err := fs.ReadFile(v2Files, "v2/protocol-lock.json")
	if err != nil {
		return ProtocolHashes{}, fmt.Errorf("read version 2 protocol lock: %w", err)
	}
	var lock v2ProtocolLock
	if err := decodeStrictJSON(contents, &lock); err != nil {
		return ProtocolHashes{}, fmt.Errorf("decode version 2 protocol lock: %w", err)
	}
	if lock.Algorithm != "sha256" || len(lock.Definitions) != len(v2DefinitionPaths) {
		return ProtocolHashes{}, fmt.Errorf("version 2 protocol lock is incomplete")
	}
	for _, path := range v2DefinitionPaths {
		definition, readErr := fs.ReadFile(v2Files, path)
		if readErr != nil {
			return ProtocolHashes{}, fmt.Errorf("read frozen definition %s: %w", path, readErr)
		}
		digest := sha256.Sum256(definition)
		actual := hex.EncodeToString(digest[:])
		if !strings.EqualFold(lock.Definitions[path], actual) {
			return ProtocolHashes{}, fmt.Errorf("frozen definition %s has digest %s, expected %s", path, actual, lock.Definitions[path])
		}
	}
	lockDigest := sha256.Sum256(contents)
	return ProtocolHashes{
		Algorithm: "sha256", Definitions: lock.Definitions, LockFile: hex.EncodeToString(lockDigest[:]),
	}, nil
}

func validateV2Suite(suite V2Suite) error {
	manifest := suite.Manifest
	if manifest.SuiteID != "portfolio-prompt-comparison" || manifest.Version != 2 {
		return fmt.Errorf("version 2 manifest has an unexpected identity")
	}
	if strings.TrimSpace(manifest.Motivation) == "" || !strings.Contains(strings.ToLower(manifest.RelationshipToV1), "does not replace") {
		return fmt.Errorf("version 2 manifest must record its motivation and separation from version 1")
	}
	if manifest.Model == "" || manifest.ExpectedModelDigest == "" || manifest.Repetitions != 5 || manifest.OrderStrategy != "alternating" {
		return fmt.Errorf("version 2 manifest has incomplete final-run controls")
	}
	if manifest.Settings.Temperature != 0 || manifest.Settings.MaxTokens != 3072 || manifest.Settings.Seed != 42 || manifest.Settings.ContextSize != 8192 {
		return fmt.Errorf("version 2 generation settings do not match the frozen protocol")
	}
	if manifest.RequestTimeoutSeconds < 1 || strings.TrimSpace(manifest.OutputLimitRationale) == "" {
		return fmt.Errorf("version 2 timeout and output-limit rationale are required")
	}
	if len(suite.Conditions) != 2 || suite.Conditions[0].ID != "baseline" || suite.Conditions[1].ID != "proposed" {
		return fmt.Errorf("version 2 requires baseline followed by proposed")
	}
	for _, condition := range suite.Conditions {
		if condition.Version != 2 || condition.SystemPrompt == "" || condition.UserPrompt == "" {
			return fmt.Errorf("version 2 condition %q is incomplete", condition.ID)
		}
	}
	if suite.Conditions[0].Context != "raw" || suite.Conditions[1].Context != "analytics" {
		return fmt.Errorf("version 2 condition contexts are invalid")
	}
	if len(suite.Scenarios) != 3 {
		return fmt.Errorf("version 2 must use the three frozen version 1 scenarios")
	}
	for _, scenario := range suite.Scenarios {
		if scenario.Version != 1 || scenario.Provenance != "controlled_sanitized_scenario" {
			return fmt.Errorf("scenario %q is not an unchanged version 1 controlled scenario", scenario.ID)
		}
		metrics := metricLookup(buildMetrics(scenario.Portfolio, analytics.Calculate(scenario.Portfolio, scenario.SnapshotAt)))
		for _, fact := range scenario.RequiredFacts {
			metric, ok := metrics[fact.MetricID]
			if !ok || !metric.Available || math.Abs(metric.Value-fact.Value) > fact.Tolerance {
				return fmt.Errorf("scenario %q no longer matches fresh analytics for %q", scenario.ID, fact.MetricID)
			}
		}
	}
	if suite.ScoringRules.Version != 2 || suite.ScoringRules.SchemaValidityDenominator != "all_planned_runs" || suite.ScoringRules.DownstreamDenominator != "schema_valid_runs" {
		return fmt.Errorf("version 2 scoring denominators are not frozen")
	}
	if suite.ManualRubric.Version != 2 || !suite.ManualRubric.BlindReview || len(suite.ManualRubric.Criteria) != 5 {
		return fmt.Errorf("version 2 manual rubric is incomplete")
	}
	return nil
}
