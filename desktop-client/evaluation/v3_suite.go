package evaluation

// This file loads the OpenAI version 3 manifest while reusing the unchanged version 2 treatment definitions.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

var v3DefinitionPaths = []string{
	"v3/manifest.yaml",
	"v2/conditions.yaml",
	"v2/response_schema.json",
	"v2/scoring_rules.yaml",
	"v2/manual_rubric.yaml",
	"scenarios/01_concentrated_multicurrency.json",
	"scenarios/02_diversified_mixed_exposure.json",
	"scenarios/03_incomplete_data.json",
}

// V3Suite contains the hosted-model manifest and the unchanged version 2 comparison definitions.
type V3Suite struct {
	Manifest       V3Manifest
	Conditions     []Condition
	Scenarios      []FrozenScenario
	ResponseSchema map[string]any
	SchemaJSON     json.RawMessage
	ScoringRules   V2ScoringRules
	ManualRubric   V2ManualRubric
	ProtocolHashes ProtocolHashes
}

// LoadV3Suite verifies both the reused version 2 definitions and the version 3 protocol lock.
func LoadV3Suite() (V3Suite, error) {
	base, err := LoadV2Suite()
	if err != nil {
		return V3Suite{}, err
	}
	hashes, err := verifyV3ProtocolLock()
	if err != nil {
		return V3Suite{}, err
	}
	var manifest V3Manifest
	if err := decodeV2YAML("v3/manifest.yaml", &manifest); err != nil {
		return V3Suite{}, err
	}
	suite := V3Suite{
		Manifest: manifest, Conditions: base.Conditions, Scenarios: base.Scenarios,
		ResponseSchema: base.ResponseSchema, SchemaJSON: append([]byte(nil), base.SchemaJSON...),
		ScoringRules: base.ScoringRules, ManualRubric: base.ManualRubric, ProtocolHashes: hashes,
	}
	if err := validateV3Suite(suite); err != nil {
		return V3Suite{}, err
	}
	return suite, nil
}

func verifyV3ProtocolLock() (ProtocolHashes, error) {
	contents, err := v2Files.ReadFile("v3/protocol-lock.json")
	if err != nil {
		return ProtocolHashes{}, fmt.Errorf("read version 3 protocol lock: %w", err)
	}
	var lock v2ProtocolLock
	if err := decodeStrictJSON(contents, &lock); err != nil {
		return ProtocolHashes{}, fmt.Errorf("decode version 3 protocol lock: %w", err)
	}
	if lock.Algorithm != "sha256" || len(lock.Definitions) != len(v3DefinitionPaths) {
		return ProtocolHashes{}, fmt.Errorf("version 3 protocol lock is incomplete")
	}
	for _, path := range v3DefinitionPaths {
		definition, readErr := v2Files.ReadFile(path)
		if readErr != nil {
			return ProtocolHashes{}, fmt.Errorf("read version 3 definition %s: %w", path, readErr)
		}
		digest := sha256.Sum256(definition)
		actual := hex.EncodeToString(digest[:])
		if !strings.EqualFold(lock.Definitions[path], actual) {
			return ProtocolHashes{}, fmt.Errorf("version 3 definition %s has digest %s, expected %s", path, actual, lock.Definitions[path])
		}
	}
	lockDigest := sha256.Sum256(contents)
	return ProtocolHashes{Algorithm: "sha256", Definitions: lock.Definitions, LockFile: hex.EncodeToString(lockDigest[:])}, nil
}

func validateV3Suite(suite V3Suite) error {
	manifest := suite.Manifest
	if manifest.SuiteID != "portfolio-prompt-comparison" || manifest.Version != 3 {
		return fmt.Errorf("version 3 manifest has an unexpected identity")
	}
	if manifest.Provider != "openai" || manifest.Model != "gpt-6-luna" {
		return fmt.Errorf("version 3 requires the frozen OpenAI gpt-6-luna configuration")
	}
	if manifest.Repetitions != 5 || manifest.OrderStrategy != "alternating" || manifest.RequestTimeoutSeconds < 1 {
		return fmt.Errorf("version 3 run controls are incomplete")
	}
	if manifest.Settings.MaxTokens != 3072 || manifest.Settings.ReasoningEffort != "low" || manifest.Settings.Seed != 0 || manifest.Settings.ContextSize != 0 {
		return fmt.Errorf("version 3 generation settings do not match the frozen protocol")
	}
	if strings.TrimSpace(manifest.Motivation) == "" || !strings.Contains(strings.ToLower(manifest.RelationshipToEarlier), "does not replace") {
		return fmt.Errorf("version 3 must record its motivation and separation from earlier experiments")
	}
	if strings.TrimSpace(manifest.SamplingPolicy) == "" || strings.TrimSpace(manifest.OutputLimitRationale) == "" {
		return fmt.Errorf("version 3 sampling and output policies are required")
	}
	return nil
}

// InspectV3Environment records only non-secret provider and local runtime identity.
func InspectV3Environment(manifest V3Manifest) V3Environment {
	return V3Environment{
		OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH,
		LogicalCPUs: runtime.NumCPU(), GoVersion: runtime.Version(),
		Provider: manifest.Provider, ConfiguredModel: manifest.Model,
	}
}
