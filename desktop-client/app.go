package main

// app.go defines the Wails application object and its frontend-facing operations.

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"wails/ai"
	"wails/analytics"
	"wails/broker"
	"wails/config"
	"wails/gateway"
	"wails/ibkr"
	"wails/persistence"
	"wails/portfolio"
	"wails/stockanalysis"
	"wails/trading"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the Wails application object whose exported methods can be called from the frontend.
type App struct {
	ctx              context.Context
	cfg              config.AppConfig
	portfolioBroker  broker.PortfolioProvider
	instrumentSearch broker.InstrumentSearchProvider
	stockData        broker.StockDataProvider
	sessionBroker    broker.SessionProvider
	gateway          *gateway.Controller
	aiCounselor      *ai.Counselor
	aiProviderName   string
	trading          *trading.Service
	store            *persistence.Store
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
		instrumentSearch: client,
		stockData:        client,
		sessionBroker:    client,
		gateway:          gateway.NewController(cfg.IBKR.GatewayDirectory),
		aiCounselor:      ai.NewCounselor(promptExecutor, researchTools),
		aiProviderName:   cfg.AI.Provider,
		trading:          tradingService,
		store:            store,
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
	if err := a.sessionBroker.Logout(a.context()); err != nil {
		return toSessionStatusView(a.checkSession()), err
	}
	return toSessionStatusView(broker.SessionStatus{
		State:            broker.SessionAuthenticationRequired,
		GatewayReachable: true,
		Message:          "The IBKR session has been logged out",
	}), nil
}

// GetPortfolio fetches the current broker portfolio and returns its deterministic analytics.
func (a *App) GetPortfolio() (PortfolioView, error) {
	portfolioSnapshot, portfolioAnalytics, err := a.fetchPortfolio()
	if err != nil {
		return PortfolioView{}, err
	}
	return toPortfolioView(portfolioSnapshot, portfolioAnalytics), nil
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
	portfolioAnalytics := analytics.Calculate(portfolioSnapshot, time.Now())
	if a.store != nil {
		if err := a.store.SavePortfolioSnapshot(ctx, portfolioSnapshot, portfolioAnalytics); err != nil {
			return portfolio.Portfolio{}, analytics.Result{}, fmt.Errorf("persist portfolio snapshot: %w", err)
		}
	}
	return portfolioSnapshot, portfolioAnalytics, nil
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
