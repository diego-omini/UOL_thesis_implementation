package persistence

// This file persists token-free manual paper-order audit transitions.

import (
	"context"
	"encoding/json"
	"fmt"

	"wails/trading"
)

// RecordOrderAudit implements the trading service's narrow audit-recorder boundary.
func (s *Store) RecordOrderAudit(ctx context.Context, event trading.AuditEvent) error {
	orderAmount, err := json.Marshal(event.OrderAmount)
	if err != nil {
		return fmt.Errorf("encode order amount: %w", err)
	}
	commission, err := json.Marshal(event.Commission)
	if err != nil {
		return fmt.Errorf("encode order commission: %w", err)
	}
	total, err := json.Marshal(event.Total)
	if err != nil {
		return fmt.Errorf("encode order total: %w", err)
	}
	messages, err := json.Marshal(event.Messages)
	if err != nil {
		return fmt.Errorf("encode order messages: %w", err)
	}
	record := OrderAuditRecord{
		SchemaVersion: SchemaVersion, OccurredAt: event.OccurredAt.UTC(), EventType: event.EventType,
		AccountFingerprint: accountFingerprint(event.AccountID),
		ContractID:         event.Instrument.ContractID, Symbol: event.Instrument.Symbol,
		InstrumentName: event.Instrument.Name, Side: string(event.Side), Quantity: event.Quantity,
		OrderAmountJSON: string(orderAmount), CommissionJSON: string(commission), TotalJSON: string(total),
		CurrentPosition: event.CurrentPosition, ResultingPosition: event.ResultingPosition,
		BrokerOrderID: event.OrderID, Status: event.Status,
		MessagesJSON: string(messages), ErrorText: event.ErrorText,
	}
	if err := s.db.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("store order audit: %w", err)
	}
	return nil
}
