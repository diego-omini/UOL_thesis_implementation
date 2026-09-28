// Package trading coordinates validated, explicitly confirmed manual paper orders.
package trading

// This file enforces application-level order validation and short-lived confirmation state.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"wails/broker"
	"wails/portfolio"
)

const previewLifetime = 45 * time.Second

// Preview is the complete information the user must inspect before confirming an order.
type Preview struct {
	Token             string
	ExpiresAt         time.Time
	Instrument        portfolio.Instrument
	Side              broker.OrderSide
	Quantity          float64
	EstimatedPrice    portfolio.Money
	OrderAmount       portfolio.Money
	Commission        portfolio.Money
	Total             portfolio.Money
	CashBalance       portfolio.Money
	CurrentPosition   float64
	ResultingPosition float64
	Warnings          []string
}

// Result reports an acknowledged order or a broker warning requiring another explicit decision.
type Result struct {
	OrderID         string
	Status          string
	WarningToken    string
	WarningMessages []string
}

type pendingPreview struct {
	order           broker.MarketOrder
	accountID       string
	instrument      portfolio.Instrument
	preview         broker.OrderPreview
	currentPosition float64
	expiresAt       time.Time
}

type pendingWarning struct {
	token   string
	replyID string
	pending pendingPreview
}

// Service owns validation and confirmation state independently of the IBKR adapter.
type Service struct {
	portfolio broker.PortfolioProvider
	stocks    broker.StockDataProvider
	orders    broker.TradingProvider
	recorder  AuditRecorder
	now       func() time.Time

	mu       sync.Mutex
	previews map[string]pendingPreview
	warning  *pendingWarning
}

// SetAuditRecorder enables durable order audit events for subsequent manual actions.
func (s *Service) SetAuditRecorder(recorder AuditRecorder) {
	s.recorder = recorder
}

// NewService constructs the manual trading workflow from application-owned broker boundaries.
func NewService(portfolios broker.PortfolioProvider, stocks broker.StockDataProvider, orders broker.TradingProvider) *Service {
	return &Service{
		portfolio: portfolios,
		stocks:    stocks,
		orders:    orders,
		now:       time.Now,
		previews:  make(map[string]pendingPreview),
	}
}

// Reset discards confirmation state when the authenticated broker session changes.
func (s *Service) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.previews = make(map[string]pendingPreview)
	s.warning = nil
}

// PreviewOrder validates fresh account state and obtains IBKR's what-if estimate.
func (s *Service) PreviewOrder(ctx context.Context, contractID int64, side string, quantity float64) (preview Preview, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event := AuditEvent{EventType: AuditPreviewFailed, Side: broker.OrderSide(strings.ToUpper(strings.TrimSpace(side))), Quantity: quantity}
	defer func() {
		if err != nil {
			event.EventType = AuditPreviewFailed
			event.ErrorText = err.Error()
			s.recordAuditBestEffort(ctx, event)
		}
	}()

	if s.warning != nil {
		return Preview{}, errors.New("respond to the outstanding IBKR order warning before creating another order")
	}
	order, err := validateOrder(contractID, side, quantity)
	if err != nil {
		return Preview{}, err
	}
	snapshot, err := s.stocks.FetchStockSnapshot(ctx, contractID)
	if err != nil {
		return Preview{}, fmt.Errorf("load current stock data for order preview: %w", err)
	}
	if !strings.EqualFold(snapshot.Instrument.AssetClass, "STK") {
		return Preview{}, errors.New("manual trading currently supports stocks only")
	}
	event.Instrument = snapshot.Instrument
	price := snapshot.AskPrice
	if order.Side == broker.OrderSideSell {
		price = snapshot.BidPrice
	}
	if !price.Available || price.Value <= 0 {
		return Preview{}, errors.New("a current executable quote is required before previewing a market order")
	}

	account, err := s.portfolio.FetchPortfolio(ctx)
	if err != nil {
		return Preview{}, fmt.Errorf("load current portfolio for order validation: %w", err)
	}
	event.AccountID = account.AccountSummary.AccountID
	currentQuantity := positionQuantity(account.Positions, contractID)
	if order.Side == broker.OrderSideSell && quantity > math.Max(currentQuantity, 0) {
		return Preview{}, fmt.Errorf("cannot sell %s shares; only %s shares are currently available", formatQuantity(quantity), formatQuantity(math.Max(currentQuantity, 0)))
	}

	brokerPreview, err := s.orders.PreviewOrder(ctx, account.AccountSummary.AccountID, order)
	if err != nil {
		return Preview{}, fmt.Errorf("IBKR order preview failed: %w", err)
	}
	if order.Side == broker.OrderSideBuy {
		if err := validateCash(account, brokerPreview.Total); err != nil {
			return Preview{}, err
		}
	}
	event.EventType = AuditPreviewCreated
	event.OrderAmount = brokerPreview.OrderAmount
	event.Commission = brokerPreview.Commission
	event.Total = brokerPreview.Total
	event.CurrentPosition = currentQuantity
	event.ResultingPosition = brokerPreview.ResultingPosition
	if err := s.recordAudit(ctx, event); err != nil {
		return Preview{}, fmt.Errorf("persist order preview audit: %w", err)
	}

	token, err := randomToken()
	if err != nil {
		return Preview{}, fmt.Errorf("create order confirmation: %w", err)
	}
	expiresAt := s.now().Add(previewLifetime)
	s.removeExpiredPreviews()
	s.previews[token] = pendingPreview{
		order: order, accountID: account.AccountSummary.AccountID, instrument: snapshot.Instrument,
		preview: brokerPreview, currentPosition: currentQuantity, expiresAt: expiresAt,
	}
	return Preview{
		Token: token, ExpiresAt: expiresAt, Instrument: snapshot.Instrument,
		Side: order.Side, Quantity: quantity,
		EstimatedPrice: portfolio.Money{Amount: price.Value, Currency: snapshot.Currency, Available: true},
		OrderAmount:    brokerPreview.OrderAmount, Commission: brokerPreview.Commission, Total: brokerPreview.Total,
		CashBalance:     account.AccountSummary.CashBalance,
		CurrentPosition: currentQuantity, ResultingPosition: brokerPreview.ResultingPosition,
		Warnings: brokerPreview.Warnings,
	}, nil
}

// ConfirmOrder consumes a preview once, rechecks holdings and cash, and submits its exact order.
func (s *Service) ConfirmOrder(ctx context.Context, token string) (result Result, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending, ok := s.previews[token]
	delete(s.previews, token)
	if !ok || !s.now().Before(pending.expiresAt) {
		return Result{}, errors.New("the order preview expired; create a new preview before confirming")
	}
	event := auditEventFromPending(pending)
	defer func() {
		if err != nil {
			event.EventType = AuditSubmissionFailed
			event.ErrorText = err.Error()
			s.recordAuditBestEffort(ctx, event)
		}
	}()
	if s.warning != nil {
		return Result{}, errors.New("respond to the outstanding IBKR order warning before submitting another order")
	}
	account, err := s.portfolio.FetchPortfolio(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("refresh portfolio before order submission: %w", err)
	}
	if account.AccountSummary.AccountID != pending.accountID {
		return Result{}, errors.New("the active broker account changed; create a new order preview")
	}
	pending.currentPosition = positionQuantity(account.Positions, pending.order.ContractID)
	if pending.order.Side == broker.OrderSideSell {
		available := math.Max(pending.currentPosition, 0)
		if pending.order.Quantity > available {
			return Result{}, fmt.Errorf("the available position changed to %s shares; create a new order preview", formatQuantity(available))
		}
		pending.preview.ResultingPosition = pending.currentPosition - pending.order.Quantity
	} else {
		freshPreview, previewErr := s.orders.PreviewOrder(ctx, pending.accountID, pending.order)
		if previewErr != nil {
			return Result{}, fmt.Errorf("refresh IBKR order preview: %w", previewErr)
		}
		if err := validateCash(account, freshPreview.Total); err != nil {
			return Result{}, err
		}
		pending.preview = freshPreview
	}
	event = auditEventFromPending(pending)
	event.EventType = AuditSubmissionAttempted
	if err := s.recordAudit(ctx, event); err != nil {
		return Result{}, fmt.Errorf("persist order submission audit: %w", err)
	}

	brokerResult, err := s.orders.SubmitOrder(ctx, pending.accountID, pending.order)
	if err != nil {
		return Result{}, fmt.Errorf("submit order to IBKR: %w", err)
	}
	result, err = s.mapResult(brokerResult, pending)
	if err != nil {
		return Result{}, err
	}
	event.OrderID = result.OrderID
	event.Status = result.Status
	event.Messages = append([]string(nil), result.WarningMessages...)
	if result.WarningToken != "" {
		event.EventType = AuditBrokerWarning
	} else {
		event.EventType = AuditSubmissionAcknowledged
	}
	s.recordAuditBestEffort(ctx, event)
	return result, nil
}

// RespondToWarning sends the user's explicit decision for an IBKR order warning.
func (s *Service) RespondToWarning(ctx context.Context, token string, confirmed bool) (result Result, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.warning == nil || token == "" || token != s.warning.token {
		return Result{}, errors.New("the IBKR order warning is no longer active")
	}
	pending := s.warning.pending
	event := auditEventFromPending(pending)
	if confirmed {
		event.EventType = AuditWarningAcceptanceAttempted
	} else {
		event.EventType = AuditWarningRejectionAttempted
	}
	if err := s.recordAudit(ctx, event); err != nil {
		return Result{}, fmt.Errorf("persist order warning decision audit: %w", err)
	}
	brokerResult, err := s.orders.ReplyToOrder(ctx, s.warning.replyID, confirmed)
	if err != nil {
		event.EventType = AuditWarningResponseFailed
		event.ErrorText = err.Error()
		s.recordAuditBestEffort(ctx, event)
		return Result{}, fmt.Errorf("respond to IBKR order warning: %w", err)
	}
	s.warning = nil
	if !confirmed {
		event.EventType = AuditWarningRejected
		event.Status = "Cancelled"
		s.recordAuditBestEffort(ctx, event)
		return Result{Status: "Cancelled"}, nil
	}
	mapped, err := s.mapResult(brokerResult, pending)
	if err != nil {
		event.EventType = AuditWarningResponseFailed
		event.ErrorText = err.Error()
		s.recordAuditBestEffort(ctx, event)
		return Result{}, err
	}
	event.OrderID = mapped.OrderID
	event.Status = mapped.Status
	event.Messages = append([]string(nil), mapped.WarningMessages...)
	if mapped.WarningToken != "" {
		event.EventType = AuditBrokerWarning
	} else {
		event.EventType = AuditSubmissionAcknowledged
	}
	s.recordAuditBestEffort(ctx, event)
	return mapped, nil
}

func (s *Service) mapResult(result broker.OrderResult, pending pendingPreview) (Result, error) {
	if result.BrokerError != "" {
		return Result{}, errors.New(result.BrokerError)
	}
	if result.ReplyID != "" {
		token, err := randomToken()
		if err != nil {
			return Result{}, fmt.Errorf("create broker warning confirmation: %w", err)
		}
		s.warning = &pendingWarning{token: token, replyID: result.ReplyID, pending: pending}
		return Result{Status: "Warning", WarningToken: token, WarningMessages: result.Messages}, nil
	}
	if result.OrderID == "" {
		return Result{}, errors.New("IBKR did not return an order acknowledgement")
	}
	return Result{OrderID: result.OrderID, Status: result.Status}, nil
}

func auditEventFromPending(pending pendingPreview) AuditEvent {
	return AuditEvent{
		AccountID: pending.accountID, Instrument: pending.instrument,
		Side: pending.order.Side, Quantity: pending.order.Quantity,
		OrderAmount: pending.preview.OrderAmount, Commission: pending.preview.Commission,
		Total: pending.preview.Total, CurrentPosition: pending.currentPosition,
		ResultingPosition: pending.preview.ResultingPosition,
	}
}

func (s *Service) recordAudit(ctx context.Context, event AuditEvent) error {
	if s.recorder == nil {
		return nil
	}
	event.OccurredAt = s.now().UTC()
	return s.recorder.RecordOrderAudit(context.WithoutCancel(ctx), event)
}

func (s *Service) recordAuditBestEffort(ctx context.Context, event AuditEvent) {
	_ = s.recordAudit(ctx, event)
}

func validateOrder(contractID int64, side string, quantity float64) (broker.MarketOrder, error) {
	if contractID <= 0 {
		return broker.MarketOrder{}, errors.New("a valid stock contract ID is required")
	}
	if math.IsNaN(quantity) || math.IsInf(quantity, 0) || quantity <= 0 {
		return broker.MarketOrder{}, errors.New("quantity must be a positive number")
	}
	if math.Round(quantity) != quantity {
		return broker.MarketOrder{}, errors.New("quantity must be a whole number of shares")
	}
	normalizedSide := broker.OrderSide(strings.ToUpper(strings.TrimSpace(side)))
	if normalizedSide != broker.OrderSideBuy && normalizedSide != broker.OrderSideSell {
		return broker.MarketOrder{}, errors.New("order side must be BUY or SELL")
	}
	return broker.MarketOrder{ContractID: contractID, Side: normalizedSide, Quantity: quantity}, nil
}

func positionQuantity(positions []portfolio.Position, contractID int64) float64 {
	for _, position := range positions {
		if position.Instrument.ContractID == contractID {
			return position.Quantity
		}
	}
	return 0
}

func validateCash(account portfolio.Portfolio, total portfolio.Money) error {
	cash := account.AccountSummary.CashBalance
	if !cash.Available {
		return errors.New("available cash could not be verified")
	}
	if !total.Available || total.Amount <= 0 || total.Currency == "" {
		return errors.New("IBKR did not provide a usable total order estimate")
	}
	required, ok := convertToBase(total, cash.Currency, account.ExchangeRates)
	if !ok {
		return fmt.Errorf("cannot verify %s cash requirement in account currency %s", total.Currency, cash.Currency)
	}
	if required > cash.Amount {
		return fmt.Errorf("insufficient cash: the estimated order total requires %.2f %s but only %.2f %s is available", required, cash.Currency, cash.Amount, cash.Currency)
	}
	return nil
}

func convertToBase(value portfolio.Money, baseCurrency string, rates []portfolio.ExchangeRate) (float64, bool) {
	if strings.EqualFold(value.Currency, baseCurrency) {
		return value.Amount, true
	}
	for _, rate := range rates {
		if rate.Available && rate.BaseAmountPerUnit > 0 && strings.EqualFold(rate.Currency, value.Currency) && strings.EqualFold(rate.BaseCurrency, baseCurrency) {
			return value.Amount * rate.BaseAmountPerUnit, true
		}
	}
	return 0, false
}

func (s *Service) removeExpiredPreviews() {
	now := s.now()
	for token, preview := range s.previews {
		if !now.Before(preview.expiresAt) {
			delete(s.previews, token)
		}
	}
}

func randomToken() (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func formatQuantity(quantity float64) string {
	return fmt.Sprintf("%.0f", quantity)
}
