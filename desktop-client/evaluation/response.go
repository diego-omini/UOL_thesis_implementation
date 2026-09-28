package evaluation

// This file strictly decodes the shared JSON response contract before any scoring occurs.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

var allowedObservationTypes = map[string]bool{
	"deterministic_metric": true,
	"portfolio_fact":       true,
	"general_information":  true,
	"limitation":           true,
}

var allowedSeverities = map[string]bool{
	"informational": true,
	"limited":       true,
	"moderate":      true,
	"elevated":      true,
	"high":          true,
}

func parseResponse(text string) (*EvaluationResponse, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(text)))
	decoder.DisallowUnknownFields()
	var response EvaluationResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("decode structured response: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.Summary) == "" || len(response.Observations) == 0 {
		return nil, fmt.Errorf("summary and at least one observation are required")
	}
	for index, observation := range response.Observations {
		if !allowedObservationTypes[observation.Type] {
			return nil, fmt.Errorf("observation %d has unsupported type %q", index, observation.Type)
		}
		if !allowedSeverities[observation.Severity] || strings.TrimSpace(observation.Statement) == "" {
			return nil, fmt.Errorf("observation %d has an invalid severity or empty statement", index)
		}
		for _, value := range observation.ReportedValues {
			if strings.TrimSpace(value.MetricID) == "" || strings.TrimSpace(value.Unit) == "" {
				return nil, fmt.Errorf("observation %d has an incomplete reported value", index)
			}
		}
	}
	for index, limitation := range response.Limitations {
		if strings.TrimSpace(limitation.Code) == "" || strings.TrimSpace(limitation.Statement) == "" {
			return nil, fmt.Errorf("limitation %d requires a code and statement", index)
		}
	}
	if strings.TrimSpace(response.Safety.Disclaimer) == "" {
		return nil, fmt.Errorf("safety disclaimer is required")
	}
	return &response, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("structured response contains trailing JSON")
		}
		return fmt.Errorf("decode structured response suffix: %w", err)
	}
	return nil
}
