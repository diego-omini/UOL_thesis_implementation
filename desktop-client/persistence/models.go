package persistence

// This file defines GORM records separately from portfolio, AI, evaluation, and trading domain types.

import "time"

// PortfolioSnapshotRecord stores one sanitized broker snapshot and its deterministic analytics.
type PortfolioSnapshotRecord struct {
	ID                 uint      `gorm:"primaryKey"`
	CreatedAt          time.Time `gorm:"not null;index"`
	SchemaVersion      int       `gorm:"not null"`
	CapturedAt         time.Time `gorm:"not null;index"`
	AccountFingerprint string    `gorm:"index"`
	BaseCurrency       string    `gorm:"size:16"`
	PositionCount      int       `gorm:"not null"`
	SnapshotHash       string    `gorm:"size:64;not null;index"`
	SnapshotJSON       string    `gorm:"type:text;not null"`
	AnalyticsJSON      string    `gorm:"type:text;not null"`
}

// PortfolioNAVHistoryRecord stores one broker-reported end-of-day portfolio value.
type PortfolioNAVHistoryRecord struct {
	ID                 uint      `gorm:"primaryKey"`
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
	SchemaVersion      int       `gorm:"not null"`
	AccountFingerprint string    `gorm:"size:64;not null;uniqueIndex:idx_nav_history_identity"`
	ObservedDate       string    `gorm:"size:10;not null;uniqueIndex:idx_nav_history_identity"`
	BaseCurrency       string    `gorm:"size:16;not null;uniqueIndex:idx_nav_history_identity"`
	Source             string    `gorm:"size:64;not null;uniqueIndex:idx_nav_history_identity"`
	NAV                float64   `gorm:"not null"`
	Frequency          string    `gorm:"size:8;not null"`
	ImportedAt         time.Time `gorm:"not null;index"`
}

// PortfolioNAVImportRecord marks every successful source refresh, including an empty dataset.
type PortfolioNAVImportRecord struct {
	ID                 uint      `gorm:"primaryKey"`
	CreatedAt          time.Time `gorm:"not null"`
	SchemaVersion      int       `gorm:"not null"`
	AccountFingerprint string    `gorm:"size:64;not null;index:idx_nav_import_account_source"`
	Source             string    `gorm:"size:64;not null;index:idx_nav_import_account_source"`
	ImportedAt         time.Time `gorm:"not null;index"`
}

// PromptExecutionRecord stores one successful or failed catalog-prompt execution.
type PromptExecutionRecord struct {
	ID                   uint      `gorm:"primaryKey"`
	CreatedAt            time.Time `gorm:"not null;index"`
	SchemaVersion        int       `gorm:"not null"`
	StartedAt            time.Time `gorm:"not null;index"`
	CompletedAt          time.Time `gorm:"not null"`
	ContextAsOf          time.Time `gorm:"index"`
	Provider             string    `gorm:"size:64"`
	Model                string    `gorm:"size:255"`
	PromptID             string    `gorm:"size:128;not null;index"`
	PromptVersion        int       `gorm:"not null"`
	Status               string    `gorm:"size:32;not null;index"`
	ParametersJSON       string    `gorm:"type:text;not null"`
	SystemPrompt         string    `gorm:"type:text"`
	UserPrompt           string    `gorm:"type:text"`
	TranscriptJSON       string    `gorm:"type:text"`
	ModelSettingsJSON    string    `gorm:"type:text"`
	RawResponseText      string    `gorm:"type:text"`
	RepairResponseText   string    `gorm:"type:text"`
	ResponseText         string    `gorm:"type:text"`
	ParsedResponseJSON   string    `gorm:"type:text"`
	ValidationResultJSON string    `gorm:"type:text"`
	RepairStatus         string    `gorm:"size:32"`
	ErrorText            string    `gorm:"type:text"`
}

// EvaluationOutputRecord stores one complete immutable evaluation result bundle.
type EvaluationOutputRecord struct {
	ID            uint      `gorm:"primaryKey"`
	CreatedAt     time.Time `gorm:"not null;index"`
	SchemaVersion int       `gorm:"not null"`
	SuiteID       string    `gorm:"size:128;not null;index"`
	SuiteVersion  int       `gorm:"not null"`
	Model         string    `gorm:"size:255;not null"`
	ModelDigest   string    `gorm:"size:255"`
	RunCount      int       `gorm:"not null"`
	BundleJSON    string    `gorm:"type:text;not null"`
}

// OrderAuditRecord stores a token-free history of manual paper-order transitions.
type OrderAuditRecord struct {
	ID                 uint      `gorm:"primaryKey"`
	CreatedAt          time.Time `gorm:"not null;index"`
	SchemaVersion      int       `gorm:"not null"`
	OccurredAt         time.Time `gorm:"not null;index"`
	EventType          string    `gorm:"size:64;not null;index"`
	AccountFingerprint string    `gorm:"index"`
	ContractID         int64     `gorm:"index"`
	Symbol             string    `gorm:"size:64"`
	InstrumentName     string    `gorm:"size:255"`
	Side               string    `gorm:"size:8"`
	Quantity           float64
	OrderAmountJSON    string `gorm:"type:text"`
	CommissionJSON     string `gorm:"type:text"`
	TotalJSON          string `gorm:"type:text"`
	CurrentPosition    float64
	ResultingPosition  float64
	BrokerOrderID      string `gorm:"size:255"`
	Status             string `gorm:"size:128"`
	MessagesJSON       string `gorm:"type:text"`
	ErrorText          string `gorm:"type:text"`
}
