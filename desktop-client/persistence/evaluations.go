package persistence

// This file persists complete evaluation bundles alongside their file-based export.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

// SaveV3EvaluationBundle stores one immutable OpenAI version 3 evaluation output bundle.
func (s *Store) SaveV3EvaluationBundle(ctx context.Context, bundle evaluation.V3Bundle) error {
	contents, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("encode version 3 evaluation bundle: %w", err)
	}
	record := EvaluationOutputRecord{
		CreatedAt: bundle.CreatedAt.UTC(), SchemaVersion: SchemaVersion,
		SuiteID: bundle.Manifest.SuiteID, SuiteVersion: bundle.Manifest.Version,
		Model: bundle.Manifest.Model, RunCount: len(bundle.Runs), BundleJSON: string(contents),
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("store version 3 evaluation output: %w", err)
	}
	return nil
}

// SaveV4EvaluationBundle stores one immutable corrected OpenAI evaluation output bundle.
func (s *Store) SaveV4EvaluationBundle(ctx context.Context, bundle evaluation.V4Bundle) error {
	contents, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("encode version 4 evaluation bundle: %w", err)
	}
	record := EvaluationOutputRecord{
		CreatedAt: bundle.CreatedAt.UTC(), SchemaVersion: SchemaVersion,
		SuiteID: bundle.Manifest.SuiteID, SuiteVersion: bundle.Manifest.Version,
		Model: bundle.Manifest.Model, RunCount: len(bundle.Runs), BundleJSON: string(contents),
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("store version 4 evaluation output: %w", err)
	}
	return nil
}

// SaveV5EvaluationBundle stores one immutable cross-model version 5 evaluation output bundle.
func (s *Store) SaveV5EvaluationBundle(ctx context.Context, bundle evaluation.V5Bundle) error {
	contents, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("encode version 5 evaluation bundle: %w", err)
	}
	models := make([]string, 0, len(bundle.ExecutedConfigurations))
	for _, configurationID := range bundle.ExecutedConfigurations {
		for _, configuration := range bundle.Manifest.Configurations {
			if configuration.ID == configurationID {
				models = append(models, configuration.Model)
				break
			}
		}
	}
	record := EvaluationOutputRecord{
		CreatedAt: bundle.CreatedAt.UTC(), SchemaVersion: SchemaVersion,
		SuiteID: bundle.Manifest.SuiteID, SuiteVersion: bundle.Manifest.Version,
		Model: strings.Join(models, ","), RunCount: len(bundle.Runs), BundleJSON: string(contents),
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("store version 5 evaluation output: %w", err)
	}
	return nil
}
