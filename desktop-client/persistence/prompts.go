package persistence

// This file persists sanitized prompt parameters, model output, metadata, and failures.

import (
	"context"
	"fmt"

	"wails/ai"
)

// RecordPromptExecution implements the AI executor's narrow execution-recorder boundary.
func (s *Store) RecordPromptExecution(ctx context.Context, execution ai.PromptExecution) error {
	parameters, err := sanitizedJSON(execution.Parameters)
	if err != nil {
		return fmt.Errorf("sanitize prompt parameters: %w", err)
	}
	transcript, err := sanitizedJSON(execution.Transcript)
	if err != nil {
		return fmt.Errorf("sanitize prompt transcript: %w", err)
	}
	modelSettings, err := sanitizedJSON(execution.ModelSettings)
	if err != nil {
		return fmt.Errorf("encode prompt model settings: %w", err)
	}
	parsedResponse, err := sanitizedJSON(execution.ParsedResponse)
	if err != nil {
		return fmt.Errorf("encode parsed prompt response: %w", err)
	}
	validationResult, err := sanitizedJSON(execution.ValidationResult)
	if err != nil {
		return fmt.Errorf("encode prompt validation result: %w", err)
	}
	status := "success"
	if execution.ErrorText != "" {
		status = "error"
	}
	record := PromptExecutionRecord{
		SchemaVersion: SchemaVersion,
		StartedAt:     execution.StartedAt.UTC(), CompletedAt: execution.CompletedAt.UTC(), ContextAsOf: execution.ContextAsOf.UTC(),
		Provider: execution.Provider, Model: execution.Model,
		PromptID: execution.PromptID, PromptVersion: execution.PromptVersion,
		Status: status, ParametersJSON: string(parameters),
		SystemPrompt: execution.SystemPrompt, UserPrompt: execution.UserPrompt,
		TranscriptJSON:    string(transcript),
		ModelSettingsJSON: string(modelSettings), RawResponseText: execution.RawResponse,
		RepairResponseText: execution.RepairResponse, ResponseText: execution.ResponseText,
		ParsedResponseJSON: string(parsedResponse), ValidationResultJSON: string(validationResult),
		RepairStatus: execution.RepairStatus, ErrorText: execution.ErrorText,
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("store prompt execution: %w", err)
	}
	return nil
}
