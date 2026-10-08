package trading

// This file verifies cash, position, confirmation, and broker-warning safety rules.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wails/broker"
	"wails/portfolio"
)

type testAuditRecorder struct {
	events     []AuditEvent
	failOnType string
}

func (r *testAuditRecorder) RecordOrderAudit(_ context.Context, event AuditEvent) error {
	r.events = append(r.events, event)
	if event.EventType == r.failOnType {
		return errors.New("audit unavailable")
	}
	return nil
}

type testBroker struct {
	portfolio    portfolio.Portfolio
	preview      broker.OrderPreview
	result       broker.OrderResult
	previewCalls int
	submitCalls  int
	replyCalls   int
}

func (b *testBroker) CheckConnection(context.Context) broker.ConnectionStatus {
	return broker.ConnectionStatus{}
}
func (b *testBroker) FetchPortfolio(context.Context) (portfolio.Portfolio, error) {
	return b.portfolio, nil
}
func (b *testBroker) FetchStockHistory(context.Context, int64, string) (portfolio.StockHistory, error) {
	return portfolio.StockHistory{}, nil
}
func (b *testBroker) FetchStockSnapshot(context.Context, int64) (portfolio.StockSnapshot, error) {
	return portfolio.StockSnapshot{
		Instrument: portfolio.Instrument{ContractID: 42, Symbol: "TEST", AssetClass: "STK", Currency: "USD"},
		Currency:   "USD", BidPrice: portfolio.OptionalNumber{Value: 9.9, Available: true},
		AskPrice: portfolio.OptionalNumber{Value: 10, Available: true},
	}, nil
}
func (b *testBroker) PreviewOrder(context.Context, string, broker.MarketOrder) (broker.OrderPreview, error) {
	b.previewCalls++
	return b.preview, nil
}
func (b *testBroker) SubmitOrder(context.Context, string, broker.MarketOrder) (broker.OrderResult, error) {
	b.submitCalls++
	return b.result, nil
}
func (b *testBroker) ReplyToOrder(context.Context, string, bool) (broker.OrderResult, error) {
	b.replyCalls++
	return broker.OrderResult{OrderID: "200", Status: "Submitted"}, nil
}

func newTestBroker() *testBroker {
	return &testBroker{
		portfolio: portfolio.Portfolio{
			AccountSummary: portfolio.AccountSummary{AccountID: "DU1", CashBalance: portfolio.Money{Amount: 100, Currency: "CHF", Available: true}},
			Positions:      []portfolio.Position{{Instrument: portfolio.Instrument{ContractID: 42}, Quantity: 5}},
			ExchangeRates:  []portfolio.ExchangeRate{{Currency: "USD", BaseCurrency: "CHF", BaseAmountPerUnit: .8, Available: true}},
		},
		preview: broker.OrderPreview{
			OrderAmount:     portfolio.Money{Amount: 50, Currency: "USD", Available: true},
			Commission:      portfolio.Money{Amount: 1, Currency: "USD", Available: true},
			Total:           portfolio.Money{Amount: 51, Currency: "USD", Available: true},
			CurrentPosition: 5, ResultingPosition: 10,
		},
		result: broker.OrderResult{OrderID: "100", Status: "Submitted"},
	}
}

func TestPreviewOrderRejectsInsufficientConvertedCash(t *testing.T) {
	b := newTestBroker()
	b.preview.Total.Amount = 130
	service := NewService(b, b, b)

	_, err := service.PreviewOrder(context.Background(), 42, "BUY", 5)
	if err == nil || !strings.Contains(err.Error(), "insufficient cash") {
		t.Fatalf("PreviewOrder() error = %v, want insufficient cash", err)
	}
	if b.submitCalls != 0 {
		t.Fatalf("SubmitOrder() calls = %d, want 0", b.submitCalls)
	}
}

func TestPreviewOrderRejectsOversellBeforeBrokerPreview(t *testing.T) {
	b := newTestBroker()
	service := NewService(b, b, b)

	_, err := service.PreviewOrder(context.Background(), 42, "SELL", 6)
	if err == nil || !strings.Contains(err.Error(), "only 5 shares") {
		t.Fatalf("PreviewOrder() error = %v, want available-share error", err)
	}
	if b.previewCalls != 0 {
		t.Fatalf("PreviewOrder broker calls = %d, want 0", b.previewCalls)
	}
}

func TestConfirmOrderConsumesPreviewOnce(t *testing.T) {
	b := newTestBroker()
	service := NewService(b, b, b)
	_, err := service.PreviewOrder(context.Background(), 42, "SELL", 2)
	if err != nil {
		t.Fatalf("PreviewOrder() error: %v", err)
	}
	result, err := service.ConfirmOrder(context.Background())
	if err != nil || result.OrderID != "100" {
		t.Fatalf("ConfirmOrder() = %#v, %v", result, err)
	}
	if _, err := service.ConfirmOrder(context.Background()); err == nil {
		t.Fatal("second ConfirmOrder() succeeded, want consumed preview error")
	}
	if b.submitCalls != 1 {
		t.Fatalf("SubmitOrder() calls = %d, want 1", b.submitCalls)
	}
}

func TestConfirmOrderRejectsExpiredPreview(t *testing.T) {
	b := newTestBroker()
	service := NewService(b, b, b)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	_, err := service.PreviewOrder(context.Background(), 42, "SELL", 1)
	if err != nil {
		t.Fatalf("PreviewOrder() error: %v", err)
	}
	now = now.Add(previewLifetime)
	if _, err := service.ConfirmOrder(context.Background()); err == nil {
		t.Fatal("ConfirmOrder() succeeded with expired preview")
	}
}

func TestBrokerWarningKeepsReplyIdentifierInBackend(t *testing.T) {
	b := newTestBroker()
	b.result = broker.OrderResult{ReplyID: "broker-secret-id", Messages: []string{"Confirm price warning"}}
	service := NewService(b, b, b)
	_, err := service.PreviewOrder(context.Background(), 42, "SELL", 1)
	if err != nil {
		t.Fatalf("PreviewOrder() error: %v", err)
	}
	result, err := service.ConfirmOrder(context.Background())
	if err != nil {
		t.Fatalf("ConfirmOrder() error: %v", err)
	}
	if !result.ConfirmationRequired {
		t.Fatal("broker warning did not require explicit confirmation")
	}
	final, err := service.RespondToWarning(context.Background(), true)
	if err != nil || final.OrderID != "200" || b.replyCalls != 1 {
		t.Fatalf("RespondToWarning() = %#v, %v; calls=%d", final, err, b.replyCalls)
	}
}

func TestSubmissionAuditFailurePreventsBrokerMutation(t *testing.T) {
	b := newTestBroker()
	recorder := &testAuditRecorder{failOnType: AuditSubmissionAttempted}
	service := NewService(b, b, b)
	service.SetAuditRecorder(recorder)
	_, err := service.PreviewOrder(context.Background(), 42, "SELL", 1)
	if err != nil {
		t.Fatalf("PreviewOrder returned an error: %v", err)
	}
	if _, err := service.ConfirmOrder(context.Background()); err == nil || !strings.Contains(err.Error(), "persist order submission audit") {
		t.Fatalf("expected audit error, got %v", err)
	}
	if b.submitCalls != 0 {
		t.Fatalf("broker submission occurred despite missing pre-audit: %d", b.submitCalls)
	}
}

func TestOrderAuditRecordsTransitionsWithoutConfirmationIdentifiers(t *testing.T) {
	b := newTestBroker()
	recorder := &testAuditRecorder{}
	service := NewService(b, b, b)
	service.SetAuditRecorder(recorder)
	_, err := service.PreviewOrder(context.Background(), 42, "SELL", 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ConfirmOrder(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.OrderID == "" || len(recorder.events) != 3 {
		t.Fatalf("unexpected result or audit events: %+v %+v", result, recorder.events)
	}
	want := []string{AuditPreviewCreated, AuditSubmissionAttempted, AuditSubmissionAcknowledged}
	for index, eventType := range want {
		if recorder.events[index].EventType != eventType {
			t.Fatalf("event %d = %q, want %q", index, recorder.events[index].EventType, eventType)
		}
	}
}

func TestCancelPendingClearsPreviewWithoutBrokerMutation(t *testing.T) {
	b := newTestBroker()
	service := NewService(b, b, b)
	if _, err := service.PreviewOrder(context.Background(), 42, "SELL", 1); err != nil {
		t.Fatal(err)
	}
	if err := service.CancelPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmOrder(context.Background()); err == nil {
		t.Fatal("cancelled preview remained confirmable")
	}
	if b.submitCalls != 0 || b.replyCalls != 0 {
		t.Fatalf("cancelled preview mutated broker state: submit=%d reply=%d", b.submitCalls, b.replyCalls)
	}
}
