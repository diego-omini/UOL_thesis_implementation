package main

// app.go defines the Wails application object and its frontend-facing operations.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"wails/ai"
	"wails/analytics"
	"wails/broker"
	"wails/config"
	"wails/fundholdings"
	"wails/gateway"
	"wails/ibkr"
	"wails/persistence"
	"wails/portfolio"
	"wails/portfolioanalysis"
	"wails/portfoliohistory"
	"wails/positionanalysis"
	"wails/snaptradehistory"
	"wails/stockanalysis"
	"wails/trading"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails application object whose exported methods can be called from the frontend.
type App struct {
	ctx              context.Context
	cfg              config.AppConfig
	portfolioBroker  broker.PortfolioProvider
	portfolioHistory broker.PortfolioHistoryProvider
	snapTradeHistory snapTradeHistoryProvider
	instrumentSearch broker.InstrumentSearchProvider
	stockData        broker.StockDataProvider
	sessionBroker    broker.SessionProvider
	gateway          *gateway.Controller
	aiCounselor      *ai.Counselor
	aiProviderName   string
	trading          *trading.Service
	store            *persistence.Store
	fundHoldings     *fundholdings.Service
	historyMu        sync.RWMutex
	historyAccountID string
	historyImportMu  sync.Mutex
	historyImports   map[string]historyImportState
	operationMu      sync.Mutex
	operations       map[string]operationState
	operationID      uint64
}

type operationState struct {
	id     uint64
	cancel context.CancelFunc
}

const (
	brokerOperationTimeout    = 45 * time.Second
	counselorOperationTimeout = 15 * time.Minute
	riskPositionLimit         = 50
	riskRequestSpacing        = 100 * time.Millisecond
)

type historyImportState struct {
	attemptedAt        time.Time
	warning            string
	externalConfigured bool
	connectionRequired bool
}

type snapTradeHistoryProvider interface {
	Configured() bool
	ConnectionPortalURL(context.Context) (string, error)
	FetchPortfolioNAVHistory(context.Context, string) (portfoliohistory.NAVHistory, error)
}

// NewApp creates the Wails application object.
func NewApp() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load application configuration: %w", err)
	}

	client := ibkr.NewClient(
		cfg.IBKR.BaseURL,
		cfg.IBKR.SkipTLSVerifyLocal,
	)
	snapTradeHistory := snaptradehistory.NewClient(cfg.SnapTrade.ClientID, cfg.SnapTrade.ConsumerKey, nil)

	var aiProvider ai.Provider
	var researchProvider ai.ResearchProvider
	switch cfg.AI.Provider {
	case "ollama":
		aiProvider = ai.NewOllamaProvider(cfg.AI.BaseURL, cfg.AI.Model)
		researchProvider = ai.NewOllamaResearchProvider(cfg.AI.SearchBaseURL, cfg.AI.SearchAPIKey)
	case "openai":
		aiProvider = ai.NewOpenAIProvider(cfg.AI.BaseURL, cfg.AI.APIKey, cfg.AI.Model)
		researchProvider = ai.NewOpenAIResearchProvider(
			cfg.AI.BaseURL,
			cfg.AI.APIKey,
			cfg.AI.Model,
			cfg.AI.SearchAllowedDomains,
		)
	default:
		return nil, fmt.Errorf("unsupported AI provider %q", cfg.AI.Provider)
	}
	promptCatalog, err := ai.NewCatalog()
	if err != nil {
		return nil, fmt.Errorf("failed to load prompt catalog: %w", err)
	}
	promptExecutor := ai.NewExecutor(promptCatalog, aiProvider)
	researchTools := ai.NewResearchToolsFactory(
		researchProvider,
		cfg.AI.SearchAllowedDomains,
		cfg.AI.SearchMaxResults,
	)

	tradingService := trading.NewService(client, client, client)
	fundHoldingsService := fundholdings.NewService(fundholdings.NewISharesSource(nil))
	var store *persistence.Store
	if !generatingBindings {
		dataDirectory, err := config.DataDirectory()
		if err != nil {
			return nil, fmt.Errorf("locate application data directory: %w", err)
		}
		store, err = persistence.Open(filepath.Join(dataDirectory, "portfolio-tracker.db"))
		if err != nil {
			return nil, fmt.Errorf("initialize application persistence: %w", err)
		}
		promptExecutor.SetExecutionRecorder(cfg.AI.Provider, store)
		tradingService.SetAuditRecorder(store)
	}
	return &App{
		cfg:              cfg,
		portfolioBroker:  client,
		portfolioHistory: client,
		snapTradeHistory: snapTradeHistory,
		instrumentSearch: client,
		stockData:        client,
		sessionBroker:    client,
		gateway:          gateway.NewController(cfg.IBKR.GatewayDirectory),
		aiCounselor:      ai.NewCounselor(promptExecutor, researchTools),
		aiProviderName:   cfg.AI.Provider,
		trading:          tradingService,
		store:            store,
		fundHoldings:     fundHoldingsService,
		historyImports:   make(map[string]historyImportState),
		operations:       make(map[string]operationState),
	}, nil
}

// shutdown closes local persistence after Wails stops dispatching application calls.
func (a *App) shutdown(_ context.Context) {
	a.cancelOperations()
	if a.store != nil {
		_ = a.store.Close()
	}
}

// startup stores the Wails runtime context when the application starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// CheckConnection reports whether the configured broker gateway is ready for portfolio requests.
func (a *App) CheckConnection() ConnectionStatusView {
	ctx := a.ctx

	if ctx == nil {
		ctx = context.Background()
	}

	status := a.portfolioBroker.CheckConnection(ctx)

	return toConnectionStatusView(status.Connected, status.Message)
}

// InitializeSession checks the current session and starts the configured local gateway when necessary.
func (a *App) InitializeSession() SessionStatusView {
	status := a.checkSession()
	if status.GatewayReachable {
		return toSessionStatusView(status)
	}

	if a.gateway.Directory() == "" {
		return toSessionStatusView(broker.SessionStatus{
			State:   broker.SessionGatewayConfigurationRequired,
			Message: "Select the extracted clientportal.gw directory to continue",
		})
	}

	return a.startGateway()
}

// GetSessionStatus returns authentication and paper-environment state without changing the session.
func (a *App) GetSessionStatus() SessionStatusView {
	return toSessionStatusView(a.checkSession())
}

// StartGateway launches the configured local Client Portal Gateway in the background.
func (a *App) StartGateway() SessionStatusView {
	if status := a.checkSession(); status.GatewayReachable {
		return toSessionStatusView(status)
	}
	return a.startGateway()
}

func (a *App) startGateway() SessionStatusView {
	if err := a.gateway.Start(); err != nil {
		state := broker.SessionGatewayUnavailable
		if a.gateway.Directory() == "" {
			state = broker.SessionGatewayConfigurationRequired
		}
		return toSessionStatusView(broker.SessionStatus{State: state, Message: err.Error()})
	}

	return toSessionStatusView(broker.SessionStatus{
		State:   broker.SessionGatewayStarting,
		Message: "Starting the IBKR Client Portal Gateway",
	})
}

// ChooseGatewayDirectory prompts for and persists an extracted Client Portal Gateway directory.
func (a *App) ChooseGatewayDirectory() (SessionStatusView, error) {
	if a.ctx == nil {
		return SessionStatusView{}, fmt.Errorf("application is not ready to open a directory dialog")
	}

	directory, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select the extracted clientportal.gw directory",
	})
	if err != nil {
		return SessionStatusView{}, fmt.Errorf("select Client Portal Gateway directory: %w", err)
	}
	if directory == "" {
		return toSessionStatusView(broker.SessionStatus{
			State:   broker.SessionGatewayConfigurationRequired,
			Message: "A Client Portal Gateway directory is required",
		}), nil
	}

	if err := a.gateway.SetDirectory(directory); err != nil {
		return toSessionStatusView(broker.SessionStatus{
			State:   broker.SessionGatewayConfigurationRequired,
			Message: err.Error(),
		}), nil
	}
	if err := config.SaveGatewayDirectory(directory); err != nil {
		return SessionStatusView{}, fmt.Errorf("save Client Portal Gateway directory: %w", err)
	}
	a.cfg.IBKR.GatewayDirectory = directory
	return a.startGateway(), nil
}

// OpenIBKRLogin opens the gateway-owned authentication page in the system browser.
func (a *App) OpenIBKRLogin() error {
	if a.ctx == nil {
		return fmt.Errorf("application is not ready to open the browser")
	}

	status := a.checkSession()
	if !status.GatewayReachable {
		return fmt.Errorf("the IBKR Client Portal Gateway is not reachable")
	}

	loginURL, err := gatewayLoginURL(a.cfg.IBKR.BaseURL)
	if err != nil {
		return err
	}
	runtime.BrowserOpenURL(a.ctx, loginURL)
	return nil
}

// KeepSessionAlive refreshes an authenticated session and returns its resulting state.
func (a *App) KeepSessionAlive() SessionStatusView {
	ctx, cancel := context.WithTimeout(a.context(), brokerOperationTimeout)
	defer cancel()
	return toSessionStatusView(a.sessionBroker.KeepAlive(ctx))
}

// Logout ends the IBKR session and returns the authentication-required state.
func (a *App) Logout() (SessionStatusView, error) {
	a.cancelOperations()
	a.trading.Reset()
	a.setHistoryAccount("")
	a.historyImportMu.Lock()
	a.historyImports = make(map[string]historyImportState)
	a.historyImportMu.Unlock()
	ctx, cancel := context.WithTimeout(a.context(), brokerOperationTimeout)
	defer cancel()
	if err := a.sessionBroker.Logout(ctx); err != nil {
		return toSessionStatusView(a.checkSession()), err
	}
	return toSessionStatusView(broker.SessionStatus{
		State:            broker.SessionAuthenticationRequired,
		GatewayReachable: true,
		Message:          "The IBKR session has been logged out",
	}), nil
}

// GetPortfolioHistory refreshes and returns bounded portfolio history for the active account.
func (a *App) GetPortfolioHistory(timeRange string) (PortfolioHistoryView, error) {
	ctx, finish := a.beginOperation("history", brokerOperationTimeout)
	defer finish()
	result, err := a.loadPortfolioHistory(ctx, timeRange)
	if err != nil {
		return PortfolioHistoryView{}, err
	}
	return toPortfolioHistoryView(timeRange, result), nil
}

// loadPortfolioHistory keeps the displayed history and counselor summary on one normalized path.
func (a *App) loadPortfolioHistory(ctx context.Context, timeRange string) (portfoliohistory.Result, error) {
	if a.store == nil {
		return portfoliohistory.Result{}, fmt.Errorf("portfolio history is unavailable while generating bindings")
	}
	to := time.Now().UTC()
	from, err := portfolioHistoryStart(timeRange, to)
	if err != nil {
		return portfoliohistory.Result{}, err
	}
	accountID := a.getHistoryAccount()
	if accountID == "" {
		return portfoliohistory.Result{}, fmt.Errorf("load the current portfolio before requesting its history")
	}
	importState := a.syncPortfolioNAVHistory(ctx, accountID, to)
	result, err := a.store.QueryPortfolioHistory(ctx, accountID, portfoliohistory.Query{
		From: from, To: to, MaxPoints: portfoliohistory.MaxPoints,
	})
	if err != nil {
		return portfoliohistory.Result{}, err
	}
	result.ImportWarning = importState.warning
	result.ExternalConfigured = importState.externalConfigured
	result.ConnectionRequired = importState.connectionRequired
	return result, nil
}

// GetPortfolioHistorySummary explains the selected saved-history range without calculating investment returns.
func (a *App) GetPortfolioHistorySummary(timeRange string) (AIResponseView, error) {
	ctx, finish := a.beginOperation("history", counselorOperationTimeout)
	defer finish()
	result, err := a.loadPortfolioHistory(ctx, timeRange)
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.PortfolioHistorySummary(ctx, result)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("portfolio history summary failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// OpenPortfolioHistoryConnection opens SnapTrade's hosted portal with read-only access.
func (a *App) OpenPortfolioHistoryConnection() error {
	if a.ctx == nil {
		return fmt.Errorf("application is not ready to open the browser")
	}
	if a.snapTradeHistory == nil || !a.snapTradeHistory.Configured() {
		return fmt.Errorf("SnapTrade Personal API credentials are not configured")
	}
	ctx, cancel := context.WithTimeout(a.context(), brokerOperationTimeout)
	defer cancel()
	connectionURL, err := a.snapTradeHistory.ConnectionPortalURL(ctx)
	if err != nil {
		return err
	}
	a.historyImportMu.Lock()
	delete(a.historyImports, a.getHistoryAccount())
	a.historyImportMu.Unlock()
	runtime.BrowserOpenURL(a.ctx, connectionURL)
	return nil
}

// syncPortfolioNAVHistory respects IBKR pacing and keeps import failures separate from local history access.
func (a *App) syncPortfolioNAVHistory(ctx context.Context, accountID string, now time.Time) historyImportState {
	const minimumImportInterval = 15 * time.Minute
	const externalImportInterval = 6 * time.Hour

	a.historyImportMu.Lock()
	defer a.historyImportMu.Unlock()
	if state, ok := a.historyImports[accountID]; ok && now.Sub(state.attemptedAt) < minimumImportInterval {
		return state
	}
	state := historyImportState{attemptedAt: now, externalConfigured: a.snapTradeHistory != nil && a.snapTradeHistory.Configured()}
	a.historyImports[accountID] = state

	recent, err := a.store.PortfolioNAVHistoryImportedRecently(ctx, accountID, portfoliohistory.SourceIBKRPortfolioAnalyst, now.Add(-minimumImportInterval))
	if err != nil {
		state.warning = "Saved IBKR history could not be checked: " + safeHistoryError(err, accountID)
		a.historyImports[accountID] = state
		return state
	}
	if recent {
		return state
	}
	var ibkrWarning string
	if a.portfolioHistory == nil {
		ibkrWarning = "IBKR portfolio performance history is unavailable in this application configuration."
	} else {
		history, fetchErr := a.portfolioHistory.FetchPortfolioNAVHistory(ctx, accountID, "1Y")
		if fetchErr == nil {
			if saveErr := a.store.SavePortfolioNAVHistory(ctx, history); saveErr == nil {
				a.historyImports[accountID] = state
				return state
			} else {
				ibkrWarning = "IBKR portfolio performance history could not be saved: " + safeHistoryError(saveErr, accountID)
			}
		} else {
			ibkrWarning = "IBKR portfolio performance history could not be refreshed: " + safeHistoryError(fetchErr, accountID)
		}
	}

	if !state.externalConfigured {
		state.warning = ibkrWarning + " Configure SNAPTRADE_CLIENT_ID and SNAPTRADE_CONSUMER_KEY to enable detailed estimated history."
		a.historyImports[accountID] = state
		return state
	}
	externalRecent, err := a.store.PortfolioNAVHistoryImportedRecently(ctx, accountID, portfoliohistory.SourceSnapTradeBalanceHistory, now.Add(-externalImportInterval))
	if err != nil {
		state.warning = ibkrWarning + " Saved SnapTrade history could not be checked: " + safeHistoryError(err, accountID)
		a.historyImports[accountID] = state
		return state
	}
	if externalRecent {
		state.warning = ibkrWarning + " Showing the most recently saved estimated SnapTrade history."
		a.historyImports[accountID] = state
		return state
	}
	externalHistory, err := a.snapTradeHistory.FetchPortfolioNAVHistory(ctx, accountID)
	if err != nil {
		state.connectionRequired = errors.Is(err, snaptradehistory.ErrConnectionRequired)
		state.warning = ibkrWarning + " SnapTrade history could not be refreshed: " + safeHistoryError(err, accountID)
		a.historyImports[accountID] = state
		return state
	}
	if err := a.store.SavePortfolioNAVHistory(ctx, externalHistory); err != nil {
		state.warning = ibkrWarning + " SnapTrade history could not be saved: " + safeHistoryError(err, accountID)
		a.historyImports[accountID] = state
		return state
	}
	state.warning = ibkrWarning + " Showing estimated SnapTrade account history because IBKR PortfolioAnalyst is unavailable."
	a.historyImports[accountID] = state
	return state
}

// GetPortfolio fetches the current broker portfolio and returns its deterministic analytics.
func (a *App) GetPortfolio() (PortfolioView, error) {
	ctx, finish := a.beginOperation("portfolio", brokerOperationTimeout)
	defer finish()
	portfolioSnapshot, portfolioAnalytics, err := a.fetchPortfolio(ctx)
	if err != nil {
		return PortfolioView{}, err
	}
	return toPortfolioView(portfolioSnapshot, portfolioAnalytics), nil
}

// GetRecentAnalyses returns a bounded safe projection of validated saved counselor results.
func (a *App) GetRecentAnalyses(limit int) ([]RecentAnalysisView, error) {
	if a.store == nil {
		return []RecentAnalysisView{}, nil
	}
	ctx, cancel := context.WithTimeout(a.context(), 10*time.Second)
	defer cancel()
	records, err := a.store.QueryRecentAnalyses(ctx, limit)
	if err != nil {
		return nil, err
	}
	return toRecentAnalysisViews(records), nil
}

// GetPortfolioRiskAnalysis calculates modeled risk from current exposures and broker price history.
func (a *App) GetPortfolioRiskAnalysis() (PortfolioRiskAnalysisView, error) {
	ctx, finish := a.beginOperation("risk", counselorOperationTimeout)
	defer finish()
	_, result, err := a.calculatePortfolioRisk(ctx)
	if err != nil {
		return PortfolioRiskAnalysisView{}, err
	}
	return toPortfolioRiskAnalysisView(result), nil
}

// GetPortfolioRiskCounsel calculates fresh risk and returns its grounded counselor explanation.
func (a *App) GetPortfolioRiskCounsel() (PortfolioRiskCounselView, error) {
	ctx, finish := a.beginOperation("risk", counselorOperationTimeout)
	defer finish()
	_, result, err := a.calculatePortfolioRisk(ctx)
	if err != nil {
		return PortfolioRiskCounselView{}, err
	}
	response, err := a.aiCounselor.PortfolioRisk(ctx, result)
	if err != nil {
		return PortfolioRiskCounselView{}, fmt.Errorf("portfolio risk counseling failed: %w", err)
	}
	return PortfolioRiskCounselView{Analysis: toPortfolioRiskAnalysisView(result), Response: toAIResponseView(response, a.aiProviderName)}, nil
}

// calculatePortfolioRisk keeps the displayed calculation and counselor input on one normalized path.
func (a *App) calculatePortfolioRisk(ctx context.Context) (portfolio.Portfolio, portfolioanalysis.Result, error) {
	snapshot, current, err := a.fetchPortfolio(ctx)
	if err != nil {
		return portfolio.Portfolio{}, portfolioanalysis.Result{}, err
	}
	positions := append([]portfolio.Position(nil), snapshot.Positions...)
	weights := make(map[int64]float64, len(current.PositionWeights))
	for _, item := range current.PositionWeights {
		weights[item.ContractID] = math.Abs(item.MarketValue.Amount)
	}
	sort.SliceStable(positions, func(i, j int) bool {
		return weights[positions[i].Instrument.ContractID] > weights[positions[j].Instrument.ContractID]
	})
	histories := make(map[int64]portfolio.StockHistory)
	requested := 0
	eligible := 0
	for _, position := range positions {
		if position.Instrument.ContractID <= 0 || !strings.EqualFold(position.Instrument.AssetClass, "STK") {
			continue
		}
		eligible++
		if requested >= riskPositionLimit {
			continue
		}
		if requested > 0 {
			select {
			case <-ctx.Done():
				return portfolio.Portfolio{}, portfolioanalysis.Result{}, ctx.Err()
			case <-time.After(riskRequestSpacing):
			}
		}
		requested++
		history, historyErr := a.stockData.FetchStockHistory(ctx, position.Instrument.ContractID, "1y")
		if historyErr == nil {
			histories[position.Instrument.ContractID] = history
		}
	}
	result := portfolioanalysis.Calculate(snapshot, current, histories, time.Now())
	if eligible > riskPositionLimit {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Risk history retrieval was limited to the %d largest eligible stock exposures to bound broker work and pacing.", riskPositionLimit))
	}
	return snapshot, result, nil
}

// SearchStocks returns matching stock contracts for the dashboard ticker search.
func (a *App) SearchStocks(query string) ([]InstrumentSearchResultView, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []InstrumentSearchResultView{}, nil
	}
	if len(query) > 64 {
		return nil, fmt.Errorf("stock search query is too long")
	}

	ctx, finish := a.beginOperation("stock-search", brokerOperationTimeout)
	defer finish()
	status := a.sessionBroker.CheckSession(ctx)
	if !status.Ready() {
		return nil, fmt.Errorf("stock search requires an authenticated IBKR paper session: %s", status.Message)
	}
	results, err := a.instrumentSearch.SearchStocks(ctx, query)
	if err != nil {
		return nil, err
	}
	return toInstrumentSearchResultViews(results), nil
}

// GetStockSnapshot returns current quote and reference values for one stock contract.
func (a *App) GetStockSnapshot(contractID int64) (StockSnapshotView, error) {
	if contractID <= 0 {
		return StockSnapshotView{}, fmt.Errorf("a valid stock contract ID is required")
	}
	ctx, finish := a.beginOperation("stock-snapshot", brokerOperationTimeout)
	defer finish()
	status := a.sessionBroker.CheckSession(ctx)
	if !status.Ready() {
		return StockSnapshotView{}, fmt.Errorf("stock data requires an authenticated IBKR paper session: %s", status.Message)
	}
	snapshot, err := a.stockData.FetchStockSnapshot(ctx, contractID)
	if err != nil {
		return StockSnapshotView{}, err
	}
	return toStockSnapshotView(snapshot), nil
}

// GetStockHistory returns normalized OHLCV bars for a supported chart range.
func (a *App) GetStockHistory(contractID int64, timeRange string) (StockHistoryView, error) {
	if contractID <= 0 {
		return StockHistoryView{}, fmt.Errorf("a valid stock contract ID is required")
	}
	ctx, finish := a.beginOperation("stock-history", brokerOperationTimeout)
	defer finish()
	status := a.sessionBroker.CheckSession(ctx)
	if !status.Ready() {
		return StockHistoryView{}, fmt.Errorf("stock history requires an authenticated IBKR paper session: %s", status.Message)
	}
	history, err := a.stockData.FetchStockHistory(ctx, contractID, timeRange)
	if err != nil {
		return StockHistoryView{}, err
	}
	return toStockHistoryView(history), nil
}

// GetOrderAnalysis produces read-only analysis for one proposed stock order.
func (a *App) GetOrderAnalysis(contractID int64, side string, quantity float64) (AIResponseView, error) {
	if contractID <= 0 {
		return AIResponseView{}, fmt.Errorf("a valid stock contract ID is required")
	}
	ctx, finish := a.beginOperation("counselor", counselorOperationTimeout)
	defer finish()
	status := a.sessionBroker.CheckSession(ctx)
	if !status.Ready() {
		return AIResponseView{}, fmt.Errorf("order analysis requires an authenticated IBKR paper session: %s", status.Message)
	}
	snapshot, err := a.stockData.FetchStockSnapshot(ctx, contractID)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("load stock data for order analysis: %w", err)
	}
	history, err := a.stockData.FetchStockHistory(ctx, contractID, "1y")
	if err != nil {
		history = portfolio.StockHistory{
			Range: "1y", UnavailableReason: fmt.Sprintf("IBKR price history was unavailable: %v", err),
		}
	}
	portfolioSnapshot, _, err := a.fetchPortfolio(ctx)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("load portfolio for order analysis: %w", err)
	}
	analysis, err := stockanalysis.Calculate(snapshot, history, portfolioSnapshot, side, quantity, time.Now())
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.OrderAnalysis(ctx, analysis)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("order analysis failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// PreviewOrder validates a manual stock order and returns a short-lived confirmation preview.
func (a *App) PreviewOrder(contractID int64, side string, quantity float64) (OrderPreviewView, error) {
	ctx, finish := a.beginOperation("trading", brokerOperationTimeout)
	defer finish()
	status := a.sessionBroker.CheckSession(ctx)
	if !status.Ready() {
		return OrderPreviewView{}, fmt.Errorf("order preview requires an authenticated IBKR paper session: %s", status.Message)
	}
	preview, err := a.trading.PreviewOrder(ctx, contractID, side, quantity)
	if err != nil {
		return OrderPreviewView{}, err
	}
	return toOrderPreviewView(preview), nil
}

// ConfirmOrder submits the exact market order represented by the pending backend preview.
func (a *App) ConfirmOrder() (OrderResultView, error) {
	ctx, finish := a.beginOperation("trading", brokerOperationTimeout)
	defer finish()
	status := a.sessionBroker.CheckSession(ctx)
	if !status.Ready() {
		return OrderResultView{}, fmt.Errorf("order submission requires an authenticated IBKR paper session: %s", status.Message)
	}
	result, err := a.trading.ConfirmOrder(ctx)
	if err != nil {
		return OrderResultView{}, err
	}
	return toOrderResultView(result), nil
}

// RespondToOrderWarning forwards the user's explicit decision for an IBKR submission warning.
func (a *App) RespondToOrderWarning(confirmed bool) (OrderResultView, error) {
	ctx, finish := a.beginOperation("trading", brokerOperationTimeout)
	defer finish()
	status := a.sessionBroker.CheckSession(ctx)
	if !status.Ready() {
		return OrderResultView{}, fmt.Errorf("order warning response requires an authenticated IBKR paper session: %s", status.Message)
	}
	result, err := a.trading.RespondToWarning(ctx, confirmed)
	if err != nil {
		return OrderResultView{}, err
	}
	return toOrderResultView(result), nil
}

// CancelOrderDecision clears an unsubmitted preview or rejects an active broker warning.
func (a *App) CancelOrderDecision() error {
	ctx, finish := a.beginOperation("trading", brokerOperationTimeout)
	defer finish()
	return a.trading.CancelPending(ctx)
}

// GetStockNews researches recent material news for one broker-verified stock identity.
func (a *App) GetStockNews(contractID int64) (AIResponseView, error) {
	if contractID <= 0 {
		return AIResponseView{}, fmt.Errorf("a valid stock contract ID is required")
	}
	ctx, finish := a.beginOperation("counselor", counselorOperationTimeout)
	defer finish()
	status := a.sessionBroker.CheckSession(ctx)
	if !status.Ready() {
		return AIResponseView{}, fmt.Errorf("stock news requires an authenticated IBKR paper session: %s", status.Message)
	}
	snapshot, err := a.stockData.FetchStockSnapshot(ctx, contractID)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("stock news identity lookup failed: %w", err)
	}
	response, err := a.aiCounselor.StockNews(ctx, snapshot.Instrument, time.Now())
	if err != nil {
		return AIResponseView{}, fmt.Errorf("stock news failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// GetPositionRiskAnalysis explains deterministic risk facts for one current stock holding.
func (a *App) GetPositionRiskAnalysis(contractID int64) (AIResponseView, error) {
	if contractID <= 0 {
		return AIResponseView{}, fmt.Errorf("a valid stock contract ID is required")
	}
	ctx, finish := a.beginOperation("counselor", counselorOperationTimeout)
	defer finish()
	snapshot, current, err := a.fetchPortfolio(ctx)
	if err != nil {
		return AIResponseView{}, err
	}
	var selected *portfolio.Position
	var weight analytics.Ratio
	for index := range snapshot.Positions {
		if snapshot.Positions[index].Instrument.ContractID == contractID {
			selected = &snapshot.Positions[index]
			break
		}
	}
	for _, item := range current.PositionWeights {
		if item.ContractID == contractID {
			weight = item.Weight
			break
		}
	}
	if selected == nil {
		return AIResponseView{}, fmt.Errorf("position risk is available only for a current holding")
	}
	stockSnapshot, err := a.stockData.FetchStockSnapshot(ctx, contractID)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("load current stock data for position risk: %w", err)
	}
	history, err := a.stockData.FetchStockHistory(ctx, contractID, "1y")
	if err != nil {
		history = portfolio.StockHistory{Range: "1y", UnavailableReason: fmt.Sprintf("IBKR price history was unavailable: %v", err)}
	}
	result, err := positionanalysis.Calculate(stockSnapshot, history, *selected, weight, time.Now())
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.PositionRisk(ctx, result)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("position risk analysis failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// GetPortfolioResume generates a read-only summary from the latest normalized positions.
func (a *App) GetPortfolioResume() (AIResponseView, error) {
	ctx, finish := a.beginOperation("counselor", counselorOperationTimeout)
	defer finish()
	portfolioSnapshot, portfolioAnalytics, err := a.fetchPortfolio(ctx)
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.PortfolioResume(ctx, portfolioSnapshot, portfolioAnalytics)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("portfolio summary failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// GetCurrencyExposureAnalysis explains deterministic currency allocations and conversion provenance.
func (a *App) GetCurrencyExposureAnalysis() (AIResponseView, error) {
	ctx, finish := a.beginOperation("counselor", counselorOperationTimeout)
	defer finish()
	snapshot, result, err := a.fetchPortfolio(ctx)
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.CurrencyExposure(ctx, snapshot, result)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("currency exposure analysis failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// GetDiversificationAnalysis explains direct and effective look-through diversification.
func (a *App) GetDiversificationAnalysis() (AIResponseView, error) {
	ctx, finish := a.beginOperation("counselor", counselorOperationTimeout)
	defer finish()
	snapshot, result, err := a.fetchPortfolio(ctx)
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.EffectiveDiversification(ctx, snapshot, result)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("diversification analysis failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// GetPortfolioDataQualityAudit explains metric availability and source-coverage limitations.
func (a *App) GetPortfolioDataQualityAudit() (AIResponseView, error) {
	ctx, finish := a.beginOperation("counselor", counselorOperationTimeout)
	defer finish()
	snapshot, result, err := a.fetchPortfolio(ctx)
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.PortfolioDataQuality(ctx, snapshot, result)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("portfolio data-quality audit failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// GetPortfolioNews researches the latest material news for every open position.
func (a *App) GetPortfolioNews() (AIResponseView, error) {
	ctx, finish := a.beginOperation("counselor", counselorOperationTimeout)
	defer finish()
	portfolioSnapshot, _, err := a.fetchPortfolio(ctx)
	if err != nil {
		return AIResponseView{}, err
	}

	response, err := a.aiCounselor.PortfolioNews(ctx, portfolioSnapshot, time.Now())
	if err != nil {
		return AIResponseView{}, fmt.Errorf("portfolio news failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// fetchPortfolio coordinates broker retrieval, public fund data, deterministic analytics, and persistence.
func (a *App) fetchPortfolio(ctx context.Context) (portfolio.Portfolio, analytics.Result, error) {
	status := a.sessionBroker.CheckSession(ctx)
	if !status.Ready() {
		return portfolio.Portfolio{}, analytics.Result{}, fmt.Errorf("portfolio access requires an authenticated IBKR paper session: %s", status.Message)
	}

	portfolioSnapshot, err := a.portfolioBroker.FetchPortfolio(ctx)
	if err != nil {
		return portfolio.Portfolio{}, analytics.Result{}, fmt.Errorf("failed to fetch portfolio: %w", err)
	}
	calculatedAt := time.Now()
	instruments := make([]portfolio.Instrument, 0, len(portfolioSnapshot.Positions))
	for _, position := range portfolioSnapshot.Positions {
		instruments = append(instruments, position.Instrument)
	}
	holdings := a.fundHoldings.ResolveInstruments(ctx, instruments, calculatedAt)
	portfolioAnalytics := analytics.CalculateWithFundHoldings(portfolioSnapshot, holdings, calculatedAt)
	if a.store != nil {
		if err := a.store.SavePortfolioSnapshot(ctx, portfolioSnapshot, portfolioAnalytics); err != nil {
			return portfolio.Portfolio{}, analytics.Result{}, fmt.Errorf("persist portfolio snapshot: %w", err)
		}
	}
	a.setHistoryAccount(portfolioSnapshot.AccountSummary.AccountID)
	return portfolioSnapshot, portfolioAnalytics, nil
}

func (a *App) setHistoryAccount(accountID string) {
	a.historyMu.Lock()
	a.historyAccountID = accountID
	a.historyMu.Unlock()
}

func (a *App) getHistoryAccount() string {
	a.historyMu.RLock()
	defer a.historyMu.RUnlock()
	return a.historyAccountID
}

func portfolioHistoryStart(value string, to time.Time) (time.Time, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "7d":
		return to.Add(-7 * 24 * time.Hour), nil
	case "30d":
		return to.Add(-30 * 24 * time.Hour), nil
	case "90d":
		return to.Add(-90 * 24 * time.Hour), nil
	case "1y":
		return to.AddDate(-1, 0, 0), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported portfolio history range %q", value)
	}
}

func safeHistoryError(err error, accountID string) string {
	if err == nil {
		return "unknown error"
	}
	message := strings.Join(strings.Fields(err.Error()), " ")
	if accountID = strings.TrimSpace(accountID); accountID != "" {
		message = regexp.MustCompile(`(?i)`+regexp.QuoteMeta(accountID)).ReplaceAllString(message, "[redacted account]")
	}
	const maximumLength = 512
	if runes := []rune(message); len(runes) > maximumLength {
		message = string(runes[:maximumLength]) + "..."
	}
	return message
}

func (a *App) context() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// beginOperation cancels superseded work in the same UI request family and applies an overall deadline.
func (a *App) beginOperation(name string, timeout time.Duration) (context.Context, func()) {
	a.operationMu.Lock()
	if a.operations == nil {
		a.operations = make(map[string]operationState)
	}
	if previous, ok := a.operations[name]; ok {
		previous.cancel()
	}
	a.operationID++
	id := a.operationID
	ctx, cancel := context.WithTimeout(a.context(), timeout)
	a.operations[name] = operationState{id: id, cancel: cancel}
	a.operationMu.Unlock()
	return ctx, func() {
		cancel()
		a.operationMu.Lock()
		if current, ok := a.operations[name]; ok && current.id == id {
			delete(a.operations, name)
		}
		a.operationMu.Unlock()
	}
}

func (a *App) cancelOperations() {
	a.operationMu.Lock()
	operations := a.operations
	a.operations = make(map[string]operationState)
	a.operationMu.Unlock()
	for _, operation := range operations {
		operation.cancel()
	}
}

func (a *App) checkSession() broker.SessionStatus {
	ctx, cancel := context.WithTimeout(a.context(), brokerOperationTimeout)
	defer cancel()
	status := a.sessionBroker.CheckSession(ctx)
	if !status.Ready() && a.trading != nil {
		a.cancelOperations()
		a.trading.Reset()
	}
	return status
}

func gatewayLoginURL(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid IBKR gateway URL: %w", err)
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/v1/api")
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
