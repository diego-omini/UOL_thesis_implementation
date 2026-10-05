package main

// app.go defines the Wails application object and its frontend-facing operations.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
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
}

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
	switch cfg.AI.Provider {
	case "ollama":
		aiProvider = ai.NewOllamaProvider(cfg.AI.BaseURL, cfg.AI.Model)
	default:
		return nil, fmt.Errorf("unsupported AI provider %q", cfg.AI.Provider)
	}
	promptCatalog, err := ai.NewCatalog()
	if err != nil {
		return nil, fmt.Errorf("failed to load prompt catalog: %w", err)
	}
	promptExecutor := ai.NewExecutor(promptCatalog, aiProvider)
	researchProvider := ai.NewOllamaResearchProvider(cfg.AI.SearchBaseURL, cfg.AI.SearchAPIKey)
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
	}, nil
}

// shutdown closes local persistence after Wails stops dispatching application calls.
func (a *App) shutdown(_ context.Context) {
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
	return toSessionStatusView(a.sessionBroker.KeepAlive(a.context()))
}

// Logout ends the IBKR session and returns the authentication-required state.
func (a *App) Logout() (SessionStatusView, error) {
	a.trading.Reset()
	a.setHistoryAccount("")
	a.historyImportMu.Lock()
	a.historyImports = make(map[string]historyImportState)
	a.historyImportMu.Unlock()
	if err := a.sessionBroker.Logout(a.context()); err != nil {
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
	result, err := a.loadPortfolioHistory(timeRange)
	if err != nil {
		return PortfolioHistoryView{}, err
	}
	return toPortfolioHistoryView(timeRange, result), nil
}

// loadPortfolioHistory keeps the displayed history and counselor summary on one normalized path.
func (a *App) loadPortfolioHistory(timeRange string) (portfoliohistory.Result, error) {
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
	importState := a.syncPortfolioNAVHistory(accountID, to)
	result, err := a.store.QueryPortfolioHistory(a.context(), accountID, portfoliohistory.Query{
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
	result, err := a.loadPortfolioHistory(timeRange)
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.PortfolioHistorySummary(a.context(), result)
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
	connectionURL, err := a.snapTradeHistory.ConnectionPortalURL(a.context())
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
func (a *App) syncPortfolioNAVHistory(accountID string, now time.Time) historyImportState {
	const minimumImportInterval = 15 * time.Minute
	const externalImportInterval = 6 * time.Hour

	a.historyImportMu.Lock()
	defer a.historyImportMu.Unlock()
	if state, ok := a.historyImports[accountID]; ok && now.Sub(state.attemptedAt) < minimumImportInterval {
		return state
	}
	state := historyImportState{attemptedAt: now, externalConfigured: a.snapTradeHistory != nil && a.snapTradeHistory.Configured()}
	a.historyImports[accountID] = state

	recent, err := a.store.PortfolioNAVHistoryImportedRecently(a.context(), accountID, portfoliohistory.SourceIBKRPortfolioAnalyst, now.Add(-minimumImportInterval))
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
		history, fetchErr := a.portfolioHistory.FetchPortfolioNAVHistory(a.context(), accountID, "1Y")
		if fetchErr == nil {
			if saveErr := a.store.SavePortfolioNAVHistory(a.context(), history); saveErr == nil {
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
	externalRecent, err := a.store.PortfolioNAVHistoryImportedRecently(a.context(), accountID, portfoliohistory.SourceSnapTradeBalanceHistory, now.Add(-externalImportInterval))
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
	externalHistory, err := a.snapTradeHistory.FetchPortfolioNAVHistory(a.context(), accountID)
	if err != nil {
		state.connectionRequired = errors.Is(err, snaptradehistory.ErrConnectionRequired)
		state.warning = ibkrWarning + " SnapTrade history could not be refreshed: " + safeHistoryError(err, accountID)
		a.historyImports[accountID] = state
		return state
	}
	if err := a.store.SavePortfolioNAVHistory(a.context(), externalHistory); err != nil {
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
	portfolioSnapshot, portfolioAnalytics, err := a.fetchPortfolio()
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
	records, err := a.store.QueryRecentAnalyses(a.context(), limit)
	if err != nil {
		return nil, err
	}
	return toRecentAnalysisViews(records), nil
}

// GetPortfolioRiskAnalysis calculates modeled risk from current exposures and broker price history.
func (a *App) GetPortfolioRiskAnalysis() (PortfolioRiskAnalysisView, error) {
	_, result, err := a.calculatePortfolioRisk()
	if err != nil {
		return PortfolioRiskAnalysisView{}, err
	}
	return toPortfolioRiskAnalysisView(result), nil
}

// GetPortfolioRiskCounsel calculates fresh risk and returns its grounded counselor explanation.
func (a *App) GetPortfolioRiskCounsel() (PortfolioRiskCounselView, error) {
	_, result, err := a.calculatePortfolioRisk()
	if err != nil {
		return PortfolioRiskCounselView{}, err
	}
	response, err := a.aiCounselor.PortfolioRisk(a.context(), result)
	if err != nil {
		return PortfolioRiskCounselView{}, fmt.Errorf("portfolio risk counseling failed: %w", err)
	}
	return PortfolioRiskCounselView{Analysis: toPortfolioRiskAnalysisView(result), Response: toAIResponseView(response, a.aiProviderName)}, nil
}

// calculatePortfolioRisk keeps the displayed calculation and counselor input on one normalized path.
func (a *App) calculatePortfolioRisk() (portfolio.Portfolio, portfolioanalysis.Result, error) {
	snapshot, current, err := a.fetchPortfolio()
	if err != nil {
		return portfolio.Portfolio{}, portfolioanalysis.Result{}, err
	}
	histories := make(map[int64]portfolio.StockHistory)
	for _, position := range snapshot.Positions {
		if position.Instrument.ContractID <= 0 || !strings.EqualFold(position.Instrument.AssetClass, "STK") {
			continue
		}
		history, historyErr := a.stockData.FetchStockHistory(a.context(), position.Instrument.ContractID, "1y")
		if historyErr == nil {
			histories[position.Instrument.ContractID] = history
		}
	}
	return snapshot, portfolioanalysis.Calculate(snapshot, current, histories, time.Now()), nil
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

	status := a.checkSession()
	if !status.Ready() {
		return nil, fmt.Errorf("stock search requires an authenticated IBKR paper session: %s", status.Message)
	}
	results, err := a.instrumentSearch.SearchStocks(a.context(), query)
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
	status := a.checkSession()
	if !status.Ready() {
		return StockSnapshotView{}, fmt.Errorf("stock data requires an authenticated IBKR paper session: %s", status.Message)
	}
	snapshot, err := a.stockData.FetchStockSnapshot(a.context(), contractID)
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
	status := a.checkSession()
	if !status.Ready() {
		return StockHistoryView{}, fmt.Errorf("stock history requires an authenticated IBKR paper session: %s", status.Message)
	}
	history, err := a.stockData.FetchStockHistory(a.context(), contractID, timeRange)
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
	status := a.checkSession()
	if !status.Ready() {
		return AIResponseView{}, fmt.Errorf("order analysis requires an authenticated IBKR paper session: %s", status.Message)
	}
	snapshot, err := a.stockData.FetchStockSnapshot(a.context(), contractID)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("load stock data for order analysis: %w", err)
	}
	history, err := a.stockData.FetchStockHistory(a.context(), contractID, "1y")
	if err != nil {
		history = portfolio.StockHistory{
			Range: "1y", UnavailableReason: fmt.Sprintf("IBKR price history was unavailable: %v", err),
		}
	}
	portfolioSnapshot, _, err := a.fetchPortfolio()
	if err != nil {
		return AIResponseView{}, fmt.Errorf("load portfolio for order analysis: %w", err)
	}
	analysis, err := stockanalysis.Calculate(snapshot, history, portfolioSnapshot, side, quantity, time.Now())
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.OrderAnalysis(a.context(), analysis)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("order analysis failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// PreviewOrder validates a manual stock order and returns a short-lived confirmation preview.
func (a *App) PreviewOrder(contractID int64, side string, quantity float64) (OrderPreviewView, error) {
	status := a.checkSession()
	if !status.Ready() {
		return OrderPreviewView{}, fmt.Errorf("order preview requires an authenticated IBKR paper session: %s", status.Message)
	}
	preview, err := a.trading.PreviewOrder(a.context(), contractID, side, quantity)
	if err != nil {
		return OrderPreviewView{}, err
	}
	return toOrderPreviewView(preview), nil
}

// ConfirmOrder submits the exact market order represented by a valid preview token.
func (a *App) ConfirmOrder(previewToken string) (OrderResultView, error) {
	status := a.checkSession()
	if !status.Ready() {
		return OrderResultView{}, fmt.Errorf("order submission requires an authenticated IBKR paper session: %s", status.Message)
	}
	result, err := a.trading.ConfirmOrder(a.context(), strings.TrimSpace(previewToken))
	if err != nil {
		return OrderResultView{}, err
	}
	return toOrderResultView(result), nil
}

// RespondToOrderWarning forwards the user's explicit decision for an IBKR submission warning.
func (a *App) RespondToOrderWarning(warningToken string, confirmed bool) (OrderResultView, error) {
	status := a.checkSession()
	if !status.Ready() {
		return OrderResultView{}, fmt.Errorf("order warning response requires an authenticated IBKR paper session: %s", status.Message)
	}
	result, err := a.trading.RespondToWarning(a.context(), strings.TrimSpace(warningToken), confirmed)
	if err != nil {
		return OrderResultView{}, err
	}
	return toOrderResultView(result), nil
}

// GetStockNews researches recent material news for one broker-verified stock identity.
func (a *App) GetStockNews(contractID int64) (AIResponseView, error) {
	if contractID <= 0 {
		return AIResponseView{}, fmt.Errorf("a valid stock contract ID is required")
	}
	status := a.sessionBroker.CheckSession(a.context())
	if !status.Ready() {
		return AIResponseView{}, fmt.Errorf("stock news requires an authenticated IBKR paper session: %s", status.Message)
	}
	snapshot, err := a.stockData.FetchStockSnapshot(a.context(), contractID)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("stock news identity lookup failed: %w", err)
	}
	response, err := a.aiCounselor.StockNews(a.context(), snapshot.Instrument, time.Now())
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
	snapshot, current, err := a.fetchPortfolio()
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
	stockSnapshot, err := a.stockData.FetchStockSnapshot(a.context(), contractID)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("load current stock data for position risk: %w", err)
	}
	history, err := a.stockData.FetchStockHistory(a.context(), contractID, "1y")
	if err != nil {
		history = portfolio.StockHistory{Range: "1y", UnavailableReason: fmt.Sprintf("IBKR price history was unavailable: %v", err)}
	}
	result, err := positionanalysis.Calculate(stockSnapshot, history, *selected, weight, time.Now())
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.PositionRisk(a.context(), result)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("position risk analysis failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// GetPortfolioResume generates a read-only summary from the latest normalized positions.
func (a *App) GetPortfolioResume() (AIResponseView, error) {
	portfolioSnapshot, portfolioAnalytics, err := a.fetchPortfolio()
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.PortfolioResume(a.context(), portfolioSnapshot, portfolioAnalytics)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("portfolio summary failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// GetCurrencyExposureAnalysis explains deterministic currency allocations and conversion provenance.
func (a *App) GetCurrencyExposureAnalysis() (AIResponseView, error) {
	snapshot, result, err := a.fetchPortfolio()
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.CurrencyExposure(a.context(), snapshot, result)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("currency exposure analysis failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// GetDiversificationAnalysis explains direct and effective look-through diversification.
func (a *App) GetDiversificationAnalysis() (AIResponseView, error) {
	snapshot, result, err := a.fetchPortfolio()
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.EffectiveDiversification(a.context(), snapshot, result)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("diversification analysis failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// GetPortfolioDataQualityAudit explains metric availability and source-coverage limitations.
func (a *App) GetPortfolioDataQualityAudit() (AIResponseView, error) {
	snapshot, result, err := a.fetchPortfolio()
	if err != nil {
		return AIResponseView{}, err
	}
	response, err := a.aiCounselor.PortfolioDataQuality(a.context(), snapshot, result)
	if err != nil {
		return AIResponseView{}, fmt.Errorf("portfolio data-quality audit failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// GetPortfolioNews researches the latest material news for every open position.
func (a *App) GetPortfolioNews() (AIResponseView, error) {
	portfolioSnapshot, _, err := a.fetchPortfolio()
	if err != nil {
		return AIResponseView{}, err
	}

	response, err := a.aiCounselor.PortfolioNews(a.context(), portfolioSnapshot, time.Now())
	if err != nil {
		return AIResponseView{}, fmt.Errorf("portfolio news failed: %w", err)
	}
	return toAIResponseView(response, a.aiProviderName), nil
}

// fetchPortfolio coordinates broker retrieval, public fund data, deterministic analytics, and persistence.
func (a *App) fetchPortfolio() (portfolio.Portfolio, analytics.Result, error) {
	ctx := a.context()
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
	return message
}

func (a *App) context() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) checkSession() broker.SessionStatus {
	status := a.sessionBroker.CheckSession(a.context())
	if !status.Ready() && a.trading != nil {
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
