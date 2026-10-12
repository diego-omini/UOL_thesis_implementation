package evaluation

// This file loads and verifies the three-model version 5 evaluation protocol.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

var v5DefinitionPaths = []string{
	"v5/manifest.yaml",
	"v4/conditions.yaml",
	"v4/response_schema.json",
	"v2/scoring_rules.yaml",
	"v2/manual_rubric.yaml",
	"scenarios/01_concentrated_multicurrency.json",
	"scenarios/02_diversified_mixed_exposure.json",
	"scenarios/03_incomplete_data.json",
}

// V5Suite contains the fixed model matrix and the corrected version 4 comparison definitions.
type V5Suite struct {
	Manifest       V5Manifest
	Conditions     []Condition
	Scenarios      []FrozenScenario
	ResponseSchema map[string]any
	SchemaJSON     json.RawMessage
	ScoringRules   V2ScoringRules
	ManualRubric   V2ManualRubric
	ProtocolHashes ProtocolHashes
}

// LoadV5Suite verifies the cross-model protocol lock and all inherited fixed definitions.
func LoadV5Suite() (V5Suite, error) {
	base, err := LoadV4Suite()
	if err != nil {
		return V5Suite{}, err
	}
	hashes, err := verifyV5ProtocolLock()
	if err != nil {
		return V5Suite{}, err
	}
	var manifest V5Manifest
	if err := decodeV2YAML("v5/manifest.yaml", &manifest); err != nil {
		return V5Suite{}, err
	}
	suite := V5Suite{
		Manifest: manifest, Conditions: base.Conditions, Scenarios: base.Scenarios,
		ResponseSchema: base.ResponseSchema, SchemaJSON: append([]byte(nil), base.SchemaJSON...),
		ScoringRules: base.ScoringRules, ManualRubric: base.ManualRubric, ProtocolHashes: hashes,
	}
	if err := validateV5Suite(suite); err != nil {
		return V5Suite{}, err
	}
	return suite, nil
}

func verifyV5ProtocolLock() (ProtocolHashes, error) {
	contents, err := v2Files.ReadFile("v5/protocol-lock.json")
	if err != nil {
		return ProtocolHashes{}, fmt.Errorf("read version 5 protocol lock: %w", err)
	}
	var lock v2ProtocolLock
	if err := decodeStrictJSON(contents, &lock); err != nil {
		return ProtocolHashes{}, fmt.Errorf("decode version 5 protocol lock: %w", err)
	}
	if lock.Algorithm != "sha256" || len(lock.Definitions) != len(v5DefinitionPaths) {
		return ProtocolHashes{}, fmt.Errorf("version 5 protocol lock is incomplete")
	}
	for _, path := range v5DefinitionPaths {
		definition, readErr := v2Files.ReadFile(path)
		if readErr != nil {
			return ProtocolHashes{}, fmt.Errorf("read version 5 definition %s: %w", path, readErr)
		}
		digest := sha256.Sum256(definition)
		actual := hex.EncodeToString(digest[:])
		if !strings.EqualFold(lock.Definitions[path], actual) {
			return ProtocolHashes{}, fmt.Errorf("version 5 definition %s has digest %s, expected %s", path, actual, lock.Definitions[path])
		}
	}
	lockDigest := sha256.Sum256(contents)
	return ProtocolHashes{Algorithm: "sha256", Definitions: lock.Definitions, LockFile: hex.EncodeToString(lockDigest[:])}, nil
}

func validateV5Suite(suite V5Suite) error {
	manifest := suite.Manifest
	if manifest.SuiteID != "portfolio-prompt-comparison" || manifest.Version != 5 {
		return fmt.Errorf("version 5 manifest has an unexpected identity")
	}
	if manifest.Repetitions != 5 || manifest.OrderStrategy != "alternating_within_configuration" {
		return fmt.Errorf("version 5 run controls are incomplete")
	}
	expected := []struct{ id, provider, model string }{
		{"ollama-qwen2-5-3b", "ollama", "qwen2.5:3b"},
		{"openai-gpt-6-luna", "openai", "gpt-6-luna"},
		{"openai-gpt-6-1-sol", "openai", "gpt-6.1-sol"},
	}
	if len(manifest.Configurations) != len(expected) {
		return fmt.Errorf("version 5 requires exactly three model configurations")
	}
	for index, want := range expected {
		configuration := manifest.Configurations[index]
		if configuration.ID != want.id || configuration.Provider != want.provider || configuration.Model != want.model {
			return fmt.Errorf("version 5 configuration %d does not match the fixed model matrix", index+1)
		}
		if configuration.RequestTimeoutSeconds < 1 || configuration.Settings.MaxTokens != 3072 || strings.TrimSpace(configuration.SamplingPolicy) == "" {
			return fmt.Errorf("version 5 configuration %q has incomplete controls", configuration.ID)
		}
		if configuration.Provider == "ollama" {
			if len(configuration.ExpectedModelDigest) != 64 || configuration.Settings.Seed != 42 || configuration.Settings.ContextSize != 8192 || configuration.Settings.ReasoningEffort != "" {
				return fmt.Errorf("version 5 local-model controls are incomplete")
			}
		} else if configuration.ExpectedModelDigest != "" || configuration.Settings.Seed != 0 || configuration.Settings.ContextSize != 0 || configuration.Settings.ReasoningEffort != "low" {
			return fmt.Errorf("version 5 hosted-model controls are incomplete for %q", configuration.ID)
		}
	}
	if len(suite.Conditions) != 2 || suite.Conditions[0].Version != 4 || suite.Conditions[1].Version != 4 {
		return fmt.Errorf("version 5 must reuse the corrected version 4 conditions")
	}
	if strings.TrimSpace(manifest.Motivation) == "" || !strings.Contains(strings.ToLower(manifest.RelationshipToEarlier), "does not replace") {
		return fmt.Errorf("version 5 must record its motivation and separation from earlier experiments")
	}
	return nil
}

// NewV5Environment records local runner identity before provider-specific inspection is attached.
func NewV5Environment() V5Environment {
	return V5Environment{
		OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH,
		LogicalCPUs: runtime.NumCPU(), GoVersion: runtime.Version(),
	}
}
