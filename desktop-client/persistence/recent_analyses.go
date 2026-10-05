package persistence

// This file exposes bounded validated counselor results without private execution material.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"wails/ai"
)

const maxRecentAnalyses = 20

// RecentAnalysis is the safe subset of one successful prompt execution that may cross the frontend boundary.
type RecentAnalysis struct {
	ID            uint
	CreatedAt     time.Time
	ContextAsOf   time.Time
	Provider      string
	Model         string
	PromptID      string
	PromptVersion int
	Response      ai.CounselorResponse
}

type recentAnalysisRow struct {
	ID                   uint
	CompletedAt          time.Time
	ContextAsOf          time.Time
	Provider             string
	Model                string
	PromptID             string
	PromptVersion        int
	ParsedResponseJSON   string
	ValidationResultJSON string
}

// QueryRecentAnalyses returns newest validated results while excluding prompts, parameters, transcripts, and raw output at query time.
func (s *Store) QueryRecentAnalyses(ctx context.Context, limit int) ([]RecentAnalysis, error) {
	if limit <= 0 || limit > maxRecentAnalyses {
		limit = maxRecentAnalyses
	}
	var rows []recentAnalysisRow
	err := s.db.WithContext(ctx).Model(&PromptExecutionRecord{}).
		Select("id", "completed_at", "context_as_of", "provider", "model", "prompt_id", "prompt_version", "parsed_response_json", "validation_result_json").
		Where("status = ? AND parsed_response_json <> ''", "success").
		Order("completed_at DESC, id DESC").
		Limit(limit * 2).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("query recent counselor analyses: %w", err)
	}

	analyses := make([]RecentAnalysis, 0, len(rows))
	for _, row := range rows {
		var validation ai.CounselorValidationResult
		if err := json.Unmarshal([]byte(row.ValidationResultJSON), &validation); err != nil || !validation.Valid {
			continue
		}
		response, err := ai.DecodeStoredCounselorResponse(row.ParsedResponseJSON)
		if err != nil {
			continue
		}
		analyses = append(analyses, RecentAnalysis{
			ID: row.ID, CreatedAt: row.CompletedAt.UTC(), ContextAsOf: row.ContextAsOf.UTC(),
			Provider: row.Provider, Model: row.Model, PromptID: row.PromptID,
			PromptVersion: row.PromptVersion, Response: *response,
		})
		if len(analyses) == limit {
			break
		}
	}
	return analyses, nil
}
