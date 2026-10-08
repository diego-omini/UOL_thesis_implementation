package trading

// This file defines the narrow persistence boundary for manual-order audit events.

import (
	"context"
	"time"

	"wails/broker"
	"wails/portfolio"
)

const (
	AuditPreviewCreated             = "preview_created"
	AuditPreviewFailed              = "preview_failed"
	AuditSubmissionAttempted        = "submission_attempted"
	AuditSubmissionFailed           = "submission_failed"
	AuditSubmissionAcknowledged     = "submission_acknowledged"
	AuditBrokerWarning              = "broker_warning"
	AuditWarningAcceptanceAttempted = "warning_acceptance_attempted"
	AuditWarningRejectionAttempted  = "warning_rejection_attempted"
	AuditWarningResponseFailed      = "warning_response_failed"
	AuditWarningRejected            = "warning_rejected"
)

// AuditEvent records an order transition without retaining broker confirmation identifiers.
type AuditEvent struct {
	OccurredAt        time.Time
	EventType         string
	AccountID         string
	Instrument        portfolio.Instrument
	Side              broker.OrderSide
	Quantity          float64
	OrderAmount       portfolio.Money
	Commission        portfolio.Money
	Total             portfolio.Money
	CurrentPosition   float64
	ResultingPosition float64
	OrderID           string
	Status            string
	Messages          []string
	ErrorText         string
}

// AuditRecorder persists manual-order events without receiving submission capabilities or broker reply identifiers.
type AuditRecorder interface {
	RecordOrderAudit(ctx context.Context, event AuditEvent) error
}
