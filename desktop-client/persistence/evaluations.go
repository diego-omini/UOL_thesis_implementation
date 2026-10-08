package persistence

// This file persists complete evaluation bundles alongside their file-based export.

import (
	"context"
	"encoding/json"
	"fmt"

	"wails/evaluation"
)

// SaveEvaluationBundle stores one immutable evaluation output bundle.
func (s *Store) SaveEvaluationBundle(ctx context.Context, bundle evaluation.Bundle) error {
	contents, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("encode evaluation bundle: %w", err)
	}
	record := EvaluationOutputRecord{
		CreatedAt: bundle.CreatedAt.UTC(), SchemaVersion: SchemaVersion,
		SuiteID: bundle.Manifest.SuiteID, SuiteVersion: bundle.Manifest.Version,
		Model: bundle.Environment.Model, ModelDigest: bundle.Environment.ModelDigest,
		RunCount: len(bundle.Runs), BundleJSON: string(contents),
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("store evaluation output: %w", err)
	}
	return nil
}

// SaveV2EvaluationBundle stores one immutable version 2 evaluation output bundle.
func (s *Store) SaveV2EvaluationBundle(ctx context.Context, bundle evaluation.V2Bundle) error {
	contents, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("encode version 2 evaluation bundle: %w", err)
	}
	record := EvaluationOutputRecord{
		CreatedAt: bundle.CreatedAt.UTC(), SchemaVersion: SchemaVersion,
		SuiteID: bundle.Manifest.SuiteID, SuiteVersion: bundle.Manifest.Version,
		Model: bundle.Environment.Model, ModelDigest: bundle.Environment.ModelDigest,
		RunCount: len(bundle.Runs), BundleJSON: string(contents),
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("store version 2 evaluation output: %w", err)
	}
	return nil
}
