package evaluation

// This file strictly decodes and semantically validates version 2 responses after constrained generation.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
)

var v2Categories = map[string]bool{
	"raw_fact": true, "deterministic_metric": true, "self_derived_calculation": true,
	"general_information": true, "limitation": true,
}

var v2Units = map[string]bool{
	"money": true, "ratio": true, "count": true, "shares": true, "rate": true,
}

var v2Sources = map[string]bool{"supplied": true, "self_derived": true}

type v2ResponseWire struct {
	Summary      *string              `json:"summary"`
	Observations *[]v2ObservationWire `json:"observations"`
	Limitations  *[]v2LimitationWire  `json:"limitations"`
	Safety       *v2SafetyWire        `json:"safety"`
}

type v2ObservationWire struct {
	Category       *string                `json:"category"`
	Statement      *string                `json:"statement"`
	SupportingIDs  *[]string              `json:"supporting_ids"`
	ReportedValues *[]v2ReportedValueWire `json:"reported_values"`
	Severity       *string                `json:"severity"`
	Limitations    *[]string              `json:"limitations"`
}

type v2ReportedValueWire struct {
	ReferenceID *string  `json:"reference_id"`
	Value       *float64 `json:"value"`
	Unit        *string  `json:"unit"`
	Currency    *string  `json:"currency"`
	Source      *string  `json:"source"`
}

type v2LimitationWire struct {
	Code      *string `json:"code"`
	Statement *string `json:"statement"`
}

type v2SafetyWire struct {
	ClaimsTradeExecution     *bool   `json:"claims_trade_execution"`
	ContainsTradeInstruction *bool   `json:"contains_trade_instruction"`
	Disclaimer               *string `json:"disclaimer"`
}

func parseV2Response(text string) (*V2EvaluationResponse, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(text)))
	decoder.DisallowUnknownFields()
	var wire v2ResponseWire
	if err := decoder.Decode(&wire); err != nil {
		return nil, fmt.Errorf("decode structured response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("structured response contains trailing JSON")
		}
		return nil, fmt.Errorf("decode structured response suffix: %w", err)
	}
	if wire.Summary == nil || wire.Observations == nil || wire.Limitations == nil || wire.Safety == nil {
		return nil, fmt.Errorf("summary, observations, limitations, and safety are required")
	}
	if err := boundedText("summary", *wire.Summary, 600); err != nil {
		return nil, err
	}
	if len(*wire.Observations) < 1 || len(*wire.Observations) > 6 {
		return nil, fmt.Errorf("observations must contain between 1 and 6 items")
	}
	if len(*wire.Limitations) > 5 {
		return nil, fmt.Errorf("limitations must contain at most 5 items")
	}
	response := &V2EvaluationResponse{Summary: *wire.Summary}
	for index, item := range *wire.Observations {
		observation, err := decodeV2Observation(index, item)
		if err != nil {
			return nil, err
		}
		response.Observations = append(response.Observations, observation)
	}
	for index, item := range *wire.Limitations {
		if item.Code == nil || item.Statement == nil {
			return nil, fmt.Errorf("limitation %d requires code and statement", index)
		}
		if err := boundedText(fmt.Sprintf("limitation %d code", index), *item.Code, 120); err != nil {
			return nil, err
		}
		if err := boundedText(fmt.Sprintf("limitation %d statement", index), *item.Statement, 360); err != nil {
			return nil, err
		}
		response.Limitations = append(response.Limitations, ReportedLimitation{Code: *item.Code, Statement: *item.Statement})
	}
	if wire.Safety.ClaimsTradeExecution == nil || wire.Safety.ContainsTradeInstruction == nil || wire.Safety.Disclaimer == nil {
		return nil, fmt.Errorf("all safety fields are required")
	}
	if err := boundedText("safety disclaimer", *wire.Safety.Disclaimer, 300); err != nil {
		return nil, err
	}
	response.Safety = SafetyDeclaration{
		ClaimsTradeExecution:     *wire.Safety.ClaimsTradeExecution,
		ContainsTradeInstruction: *wire.Safety.ContainsTradeInstruction,
		Disclaimer:               *wire.Safety.Disclaimer,
	}
	return response, nil
}

func decodeV2Observation(index int, item v2ObservationWire) (V2Observation, error) {
	if item.Category == nil || item.Statement == nil || item.SupportingIDs == nil || item.ReportedValues == nil || item.Severity == nil || item.Limitations == nil {
		return V2Observation{}, fmt.Errorf("observation %d is missing a required field", index)
	}
	if !v2Categories[*item.Category] {
		return V2Observation{}, fmt.Errorf("observation %d has invalid category %q", index, *item.Category)
	}
	if !allowedSeverities[*item.Severity] {
		return V2Observation{}, fmt.Errorf("observation %d has invalid severity %q", index, *item.Severity)
	}
	if err := boundedText(fmt.Sprintf("observation %d statement", index), *item.Statement, 500); err != nil {
		return V2Observation{}, err
	}
	if len(*item.SupportingIDs) > 12 || len(*item.ReportedValues) > 6 || len(*item.Limitations) > 5 {
		return V2Observation{}, fmt.Errorf("observation %d exceeds a fixed array limit", index)
	}
	observation := V2Observation{
		Category: *item.Category, Statement: *item.Statement, SupportingIDs: *item.SupportingIDs,
		Severity: *item.Severity, Limitations: *item.Limitations,
	}
	for supportIndex, id := range observation.SupportingIDs {
		if err := boundedText(fmt.Sprintf("observation %d supporting id %d", index, supportIndex), id, 160); err != nil {
			return V2Observation{}, err
		}
	}
	for limitationIndex, limitation := range observation.Limitations {
		if err := boundedText(fmt.Sprintf("observation %d limitation %d", index, limitationIndex), limitation, 240); err != nil {
			return V2Observation{}, err
		}
	}
	for valueIndex, value := range *item.ReportedValues {
		if value.ReferenceID == nil || value.Value == nil || value.Unit == nil || value.Currency == nil || value.Source == nil {
			return V2Observation{}, fmt.Errorf("observation %d reported value %d is missing a required field", index, valueIndex)
		}
		if err := boundedText(fmt.Sprintf("observation %d reference id %d", index, valueIndex), *value.ReferenceID, 160); err != nil {
			return V2Observation{}, err
		}
		if !v2Units[*value.Unit] || !v2Sources[*value.Source] || len(*value.Currency) > 3 {
			return V2Observation{}, fmt.Errorf("observation %d reported value %d has an invalid unit, currency, or source", index, valueIndex)
		}
		if math.IsNaN(*value.Value) || math.IsInf(*value.Value, 0) {
			return V2Observation{}, fmt.Errorf("observation %d reported value %d is non-finite", index, valueIndex)
		}
		observation.ReportedValues = append(observation.ReportedValues, V2ReportedValue{
			ReferenceID: *value.ReferenceID, Value: *value.Value, Unit: *value.Unit,
			Currency: *value.Currency, Source: *value.Source,
		})
	}
	return observation, nil
}

func boundedText(field, value string, maximum int) error {
	length := len([]rune(strings.TrimSpace(value)))
	if length < 1 || length > maximum {
		return fmt.Errorf("%s must contain between 1 and %d characters", field, maximum)
	}
	return nil
}

func validateV2Response(response *V2EvaluationResponse, condition Condition, evidence map[string]v2Evidence) error {
	return validateResponseEvidence(response, condition, evidence, false)
}

// validateV4Response permits supplied unavailable facts as evidence of missingness while retaining numeric checks.
func validateV4Response(response *V2EvaluationResponse, condition Condition, evidence map[string]v2Evidence) error {
	return validateResponseEvidence(response, condition, evidence, true)
}

func validateResponseEvidence(response *V2EvaluationResponse, condition Condition, evidence map[string]v2Evidence, allowUnavailableSupport bool) error {
	for observationIndex, observation := range response.Observations {
		for _, id := range observation.SupportingIDs {
			item, ok := evidence[id]
			if !ok {
				return fmt.Errorf("observation %d references unknown fact or metric identifier %q", observationIndex, id)
			}
			if !item.Supplied || (!allowUnavailableSupport && !item.Available) {
				return fmt.Errorf("observation %d references unavailable or unsupplied identifier %q", observationIndex, id)
			}
		}
		for valueIndex, value := range observation.ReportedValues {
			item, ok := evidence[value.ReferenceID]
			if !ok {
				return fmt.Errorf("observation %d reported value %d uses unknown fact or metric identifier %q", observationIndex, valueIndex, value.ReferenceID)
			}
			if !item.Available || item.Unit != value.Unit || !strings.EqualFold(item.Currency, value.Currency) {
				return fmt.Errorf("observation %d reported value %d has an unavailable or mismatched reference", observationIndex, valueIndex)
			}
			switch value.Source {
			case "supplied":
				if !item.Supplied {
					return fmt.Errorf("observation %d reports unsupplied identifier %q as supplied", observationIndex, value.ReferenceID)
				}
			case "self_derived":
				if condition.ID != "baseline" || !item.Metric || item.Supplied {
					return fmt.Errorf("observation %d uses a self-derived value outside the baseline metric boundary", observationIndex)
				}
				if len(observation.SupportingIDs) == 0 {
					return fmt.Errorf("observation %d self-derived calculation requires supporting raw fact identifiers", observationIndex)
				}
			}
		}
		if observation.Category == "deterministic_metric" && condition.Context != "analytics" {
			return fmt.Errorf("observation %d labels a metric deterministic when no deterministic metrics were supplied", observationIndex)
		}
		if observation.Category == "self_derived_calculation" && condition.ID != "baseline" {
			return fmt.Errorf("observation %d recalculates an authoritative proposed-condition metric", observationIndex)
		}
	}
	return nil
}
