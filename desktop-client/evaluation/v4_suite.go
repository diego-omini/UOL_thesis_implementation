package evaluation

// This file loads the corrected OpenAI version 4 protocol and verifies every fixed definition.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

var v4DefinitionPaths = []string{
	"v4/manifest.yaml",
	"v4/conditions.yaml",
	"v4/response_schema.json",
	"v2/scoring_rules.yaml",
	"v2/manual_rubric.yaml",
	"scenarios/01_concentrated_multicurrency.json",
	"scenarios/02_diversified_mixed_exposure.json",
	"scenarios/03_incomplete_data.json",
}

// V4Suite contains the corrected prompt conditions, response contract, and unchanged scoring fixtures.
type V4Suite struct {
	Manifest       V4Manifest
	Conditions     []Condition
	Scenarios      []FrozenScenario
	ResponseSchema map[string]any
	SchemaJSON     json.RawMessage
	ScoringRules   V2ScoringRules
	ManualRubric   V2ManualRubric
	ProtocolHashes ProtocolHashes
}

// LoadV4Suite verifies the corrected protocol lock and recalculates all frozen scenario expectations.
func LoadV4Suite() (V4Suite, error) {
	base, err := LoadV2Suite()
	if err != nil {
		return V4Suite{}, err
	}
	hashes, err := verifyV4ProtocolLock()
	if err != nil {
		return V4Suite{}, err
	}
	var manifest V4Manifest
	if err := decodeV2YAML("v4/manifest.yaml", &manifest); err != nil {
		return V4Suite{}, err
	}
	var conditionFile conditionFile
	if err := decodeV2YAML("v4/conditions.yaml", &conditionFile); err != nil {
		return V4Suite{}, err
	}
	schemaJSON, err := v2Files.ReadFile("v4/response_schema.json")
	if err != nil {
		return V4Suite{}, fmt.Errorf("read version 4 response schema: %w", err)
	}
	var responseSchema map[string]any
	if err := decodeStrictJSON(schemaJSON, &responseSchema); err != nil {
		return V4Suite{}, fmt.Errorf("decode version 4 response schema: %w", err)
	}
	suite := V4Suite{
		Manifest: manifest, Conditions: conditionFile.Conditions, Scenarios: base.Scenarios,
		ResponseSchema: responseSchema, SchemaJSON: append([]byte(nil), schemaJSON...),
		ScoringRules: base.ScoringRules, ManualRubric: base.ManualRubric, ProtocolHashes: hashes,
	}
	if err := validateV4Suite(suite); err != nil {
		return V4Suite{}, err
	}
	return suite, nil
}

func verifyV4ProtocolLock() (ProtocolHashes, error) {
	contents, err := v2Files.ReadFile("v4/protocol-lock.json")
	if err != nil {
		return ProtocolHashes{}, fmt.Errorf("read version 4 protocol lock: %w", err)
	}
	var lock v2ProtocolLock
	if err := decodeStrictJSON(contents, &lock); err != nil {
		return ProtocolHashes{}, fmt.Errorf("decode version 4 protocol lock: %w", err)
	}
	if lock.Algorithm != "sha256" || len(lock.Definitions) != len(v4DefinitionPaths) {
		return ProtocolHashes{}, fmt.Errorf("version 4 protocol lock is incomplete")
	}
	for _, path := range v4DefinitionPaths {
		definition, readErr := v2Files.ReadFile(path)
		if readErr != nil {
			return ProtocolHashes{}, fmt.Errorf("read version 4 definition %s: %w", path, readErr)
		}
		digest := sha256.Sum256(definition)
		actual := hex.EncodeToString(digest[:])
		if !strings.EqualFold(lock.Definitions[path], actual) {
			return ProtocolHashes{}, fmt.Errorf("version 4 definition %s has digest %s, expected %s", path, actual, lock.Definitions[path])
		}
	}
	lockDigest := sha256.Sum256(contents)
	return ProtocolHashes{Algorithm: "sha256", Definitions: lock.Definitions, LockFile: hex.EncodeToString(lockDigest[:])}, nil
}

func validateV4Suite(suite V4Suite) error {
	manifest := suite.Manifest
	if manifest.SuiteID != "portfolio-prompt-comparison" || manifest.Version != 4 {
		return fmt.Errorf("version 4 manifest has an unexpected identity")
	}
	if manifest.Provider != "openai" || manifest.Model != "gpt-6-luna" {
		return fmt.Errorf("version 4 requires the frozen OpenAI gpt-6-luna configuration")
	}
	if manifest.Repetitions != 5 || manifest.OrderStrategy != "alternating" || manifest.RequestTimeoutSeconds < 1 {
		return fmt.Errorf("version 4 run controls are incomplete")
	}
	if manifest.Settings.MaxTokens != 3072 || manifest.Settings.ReasoningEffort != "low" || manifest.Settings.Seed != 0 || manifest.Settings.ContextSize != 0 {
		return fmt.Errorf("version 4 generation settings do not match the frozen protocol")
	}
	if len(suite.Conditions) != 2 || suite.Conditions[0].ID != "baseline" || suite.Conditions[1].ID != "proposed" {
		return fmt.Errorf("version 4 requires baseline and proposed conditions")
	}
	for _, condition := range suite.Conditions {
		if condition.Version != 4 || !strings.Contains(condition.SystemPrompt, "available=false") || !strings.Contains(condition.SystemPrompt, "reported_values") {
			return fmt.Errorf("version 4 condition %q does not declare the corrected evidence contract", condition.ID)
		}
	}
	if strings.TrimSpace(manifest.Motivation) == "" || !strings.Contains(strings.ToLower(manifest.RelationshipToEarlier), "does not replace") {
		return fmt.Errorf("version 4 must record its motivation and separation from earlier experiments")
	}
	return nil
}

// InspectV4Environment records only non-secret provider and local runtime identity.
func InspectV4Environment(manifest V4Manifest) V3Environment {
	return V3Environment{
		OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH,
		LogicalCPUs: runtime.NumCPU(), GoVersion: runtime.Version(),
		Provider: manifest.Provider, ConfiguredModel: manifest.Model,
	}
}
