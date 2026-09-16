// Package ibkr implements portfolio access through the IBKR Client Portal Gateway.
package ibkr

// This file manages the IBKR session, retrieves account data, and maps responses into broker-independent domain models.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"wails/broker"
	"wails/portfolio"
)

const (
	maxPositionPages                      = 100
	ledgerRateSource                      = "IBKR account ledger"
	brokerageSessionInitializationTimeout = 5 * time.Second
	brokerageSessionPollInterval          = 250 * time.Millisecond
)

// Client is the IBKR Client Portal Gateway adapter.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Information about authentication.
type authStatusResponse struct {
	Authenticated bool  `json:"authenticated"`
	Connected     bool  `json:"connected"`
	Competing     bool  `json:"competing"`
	Established   *bool `json:"established"`
}

type ssoValidationResponse struct {
	Result bool `json:"RESULT"`
}

type brokerageSessionInitRequest struct {
	Publish bool `json:"publish"`
	Compete bool `json:"compete"`
}

type tradingAccountsResponse struct {
	IsPaper *bool `json:"isPaper"`
}

type logoutResponse struct {
	Status bool `json:"status"`
}

type httpStatusError struct {
	statusCode int
}

func (e httpStatusError) Error() string {
	return fmt.Sprintf("gateway returned HTTP %d", e.statusCode)
}

// Mirror for entries returned by /portfolio/accounts.
// Depending on version id or accountId is used so we accept both.
type accountResponse struct {
	ID           string `json:"id"`
	AccountID    string `json:"accountId"`
	AccountAlias string `json:"accountAlias"`
	AccountTitle string `json:"accountTitle"`
	DisplayName  string `json:"displayName"`
	AccountType  string `json:"type"`
	Currency     string `json:"currency"`
}

func (a accountResponse) accountID() string {
	return firstNonEmpty(a.AccountID, a.ID)
}

// Account summary value.
type summaryValueResponse struct {
	Amount   decimalNumber `json:"amount"`
	Value    string        `json:"value"`
	Currency string        `json:"currency"`
}

type ledgerValueResponse struct {
	Currency     string        `json:"currency"`
	ExchangeRate decimalNumber `json:"exchangerate"`
	Timestamp    int64         `json:"timestamp"`
}

// Mirrors response from /portfolio/{account}/positions/{page}.
type positionResponse struct {
	AccountID         string        `json:"acctId"`
	ContractID        int64         `json:"conid"`
	Symbol            string        `json:"ticker"`
	ContractDesc      string        `json:"contractDesc"`
	Name              string        `json:"name"`
	AssetClass        string        `json:"assetClass"`
	Sector            string        `json:"sector"`
	SectorGroup       string        `json:"sectorGroup"`
	Exchange          string        `json:"exchange"`
	ListingExchange   string        `json:"listingExchange"`
	Currency          string        `json:"currency"`
	PageSize          int           `json:"pageSize"`
	Position          decimalNumber `json:"position"`
	AverageCost       decimalNumber `json:"avgCost"`
	AveragePrice      decimalNumber `json:"avgPrice"`
	MarketPrice       decimalNumber `json:"mktPrice"`
	MarketValue       decimalNumber `json:"mktValue"`
	BaseMarketValue   decimalNumber `json:"baseMktValue"`
	UnrealizedPnL     decimalNumber `json:"unrealizedPnl"`
	UnrealizedPnLAlt  decimalNumber `json:"unrealizedPNL"`
	BaseUnrealizedPnL decimalNumber `json:"baseUnrealizedPnl"`
}

// IBKR returns numbers as JSON numbers, strings, empty string, or null.
// Using this struct keeps response mapping independent of IBKR's numeric representation.
type decimalNumber struct {
	value float64
	valid bool
}

// UnmarshalJSON accepts the numeric, string, empty, and null representations returned by IBKR.
func (n *decimalNumber) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))

	if text == "" || text == "null" {
		n.value = 0
		n.valid = false
		return nil
	}

	if strings.HasPrefix(text, "\"") {
		var raw string
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}

		raw = strings.TrimSpace(strings.ReplaceAll(raw, ",", ""))

		if raw == "" {
			n.value = 0
			n.valid = false
			return nil
		}

		parsed, err := strconv.ParseFloat(raw, 64)

		if err != nil {
			return fmt.Errorf("parse numeric string %q: %w", raw, err)
		}

		n.value = parsed
		n.valid = true
		return nil
	}

	parsed, err := strconv.ParseFloat(text, 64)

	if err != nil {
		return fmt.Errorf("parse numeric value %q: %w", text, err)
	}

	n.value = parsed
	n.valid = true
	return nil
}

// NewClient returns a basic client for the portal gateway.
func NewClient(baseURL string, skipTLSVerify bool) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: skipTLSVerify,
		},
	}

	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}
}

// CheckConnection checks that the gateway is reachable and authenticated.
func (c *Client) CheckConnection(ctx context.Context) broker.ConnectionStatus {
	status := c.CheckSession(ctx)
	return broker.ConnectionStatus{Connected: status.Ready(), Message: status.Message}
}

// CheckSession verifies gateway authentication and rejects sessions that are not explicitly marked as paper trading.
func (c *Client) CheckSession(ctx context.Context) broker.SessionStatus {
	var sso ssoValidationResponse
	if err := c.getJSON(ctx, "/sso/validate", &sso); err != nil {
		return sessionRequestFailure(err, "IBKR browser session validation failed")
	}
	if !sso.Result {
		return broker.SessionStatus{
			State:            broker.SessionAuthenticationRequired,
			GatewayReachable: true,
			Message:          "Authenticate through the IBKR Client Portal Gateway",
		}
	}

	var authentication authStatusResponse
	if err := c.postJSON(ctx, "/iserver/auth/status", &authentication); err != nil {
		return sessionRequestFailure(err, "IBKR gateway authentication check failed")
	}

	if !brokerageSessionReady(authentication) && !authentication.Competing {
		initialized, err := c.initializeBrokerageSession(ctx)
		if err != nil {
			return sessionRequestFailure(err, "IBKR brokerage session initialization failed")
		}
		authentication = initialized
	}

	if authentication.Competing {
		return broker.SessionStatus{
			State:            broker.SessionCompeting,
			GatewayReachable: true,
			Connected:        authentication.Connected,
			Competing:        true,
			Message:          "This IBKR username has an active brokerage session elsewhere",
		}
	}

	if !brokerageSessionReady(authentication) {
		return broker.SessionStatus{
			State:            broker.SessionError,
			GatewayReachable: true,
			Authenticated:    authentication.Authenticated,
			Connected:        authentication.Connected,
			Message:          "The IBKR browser login succeeded, but the brokerage session did not become ready",
		}
	}

	var accounts tradingAccountsResponse
	if err := c.getJSON(ctx, "/iserver/accounts", &accounts); err != nil {
		var statusErr httpStatusError
		if errors.As(err, &statusErr) && statusErr.statusCode == http.StatusUnauthorized {
			return broker.SessionStatus{
				State:            broker.SessionAuthenticationRequired,
				GatewayReachable: true,
				Message:          "The IBKR session is no longer authenticated",
			}
		}
		return broker.SessionStatus{
			State:            broker.SessionError,
			GatewayReachable: true,
			Authenticated:    true,
			Connected:        true,
			Message:          fmt.Sprintf("IBKR account validation failed: %v", err),
		}
	}

	if accounts.IsPaper == nil {
		return broker.SessionStatus{
			State:            broker.SessionError,
			GatewayReachable: true,
			Authenticated:    true,
			Connected:        true,
			Message:          "IBKR did not report whether this is a paper-trading session",
		}
	}

	if !*accounts.IsPaper {
		return broker.SessionStatus{
			State:            broker.SessionLiveAccount,
			GatewayReachable: true,
			Authenticated:    true,
			Connected:        true,
			PaperKnown:       true,
			Message:          "This application only supports IBKR paper-trading sessions",
		}
	}

	return broker.SessionStatus{
		State:            broker.SessionAuthenticated,
		GatewayReachable: true,
		Authenticated:    true,
		Connected:        true,
		Paper:            true,
		PaperKnown:       true,
		Message:          "IBKR paper-trading session is authenticated",
	}
}

// initializeBrokerageSession starts a non-competing brokerage session and waits for account services to become ready.
func (c *Client) initializeBrokerageSession(ctx context.Context) (authStatusResponse, error) {
	var status authStatusResponse
	err := c.postJSONBody(ctx, "/iserver/auth/ssodh/init", brokerageSessionInitRequest{
		Publish: true,
		Compete: false,
	}, &status)
	if err != nil {
		return authStatusResponse{}, err
	}
	if brokerageSessionReady(status) || status.Competing {
		return status, nil
	}

	pollCtx, cancel := context.WithTimeout(ctx, brokerageSessionInitializationTimeout)
	defer cancel()
	ticker := time.NewTicker(brokerageSessionPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return status, nil
		case <-ticker.C:
			if err := c.postJSON(pollCtx, "/iserver/auth/status", &status); err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					return status, nil
				}
				return authStatusResponse{}, err
			}
			if brokerageSessionReady(status) || status.Competing {
				return status, nil
			}
		}
	}
}

func brokerageSessionReady(status authStatusResponse) bool {
	established := status.Established == nil || *status.Established
	return status.Authenticated && status.Connected && established
}

// KeepAlive prevents an active gateway session from expiring and returns its current status.
func (c *Client) KeepAlive(ctx context.Context) broker.SessionStatus {
	if err := c.postJSON(ctx, "/tickle", nil); err != nil {
		return sessionRequestFailure(err, "IBKR session keep-alive failed")
	}
	return c.CheckSession(ctx)
}

// Logout ends the current IBKR gateway session.
func (c *Client) Logout(ctx context.Context) error {
	var response logoutResponse
	if err := c.postJSON(ctx, "/logout", &response); err != nil {
		return fmt.Errorf("IBKR logout failed: %w", err)
	}
	if !response.Status {
		return errors.New("IBKR did not confirm that the session was logged out")
	}
	return nil
}

func sessionRequestFailure(err error, message string) broker.SessionStatus {
	var statusErr httpStatusError
	if errors.As(err, &statusErr) {
		if statusErr.statusCode == http.StatusUnauthorized {
			return broker.SessionStatus{
				State:            broker.SessionAuthenticationRequired,
				GatewayReachable: true,
				Message:          "The IBKR session is no longer authenticated",
			}
		}
		return broker.SessionStatus{
			State:            broker.SessionError,
			GatewayReachable: true,
			Message:          fmt.Sprintf("%s: %v", message, err),
		}
	}

	return broker.SessionStatus{
		State:   broker.SessionGatewayUnavailable,
		Message: fmt.Sprintf("%s: %v", message, err),
	}
}

// FetchPortfolio coordinates account, summary, exchange-rate, and position retrieval.
func (c *Client) FetchPortfolio(ctx context.Context) (portfolio.Portfolio, error) {
	account, err := c.fetchAccount(ctx)

	if err != nil {
		return portfolio.Portfolio{}, err
	}

	summary, err := c.fetchAccountSummary(ctx, account)

	if err != nil {
		return portfolio.Portfolio{}, err
	}

	baseCurrency := firstNonEmpty(
		account.Currency,
		summary.NetLiquidation.Currency,
		summary.CashBalance.Currency,
	)
	exchangeRates, err := c.fetchExchangeRates(ctx, account, baseCurrency, time.Now())
	if err != nil {
		return portfolio.Portfolio{}, err
	}

	positions, err := c.fetchPositions(ctx, account)

	if err != nil {
		return portfolio.Portfolio{}, err
	}

	return portfolio.Portfolio{
		AccountSummary: summary,
		Positions:      positions,
		ExchangeRates:  exchangeRates,
	}, nil
}

// fetchAccount selects the first available account for the application's single-account scope.
func (c *Client) fetchAccount(ctx context.Context) (accountResponse, error) {
	var accounts []accountResponse

	if err := c.getJSON(ctx, "/portfolio/accounts", &accounts); err != nil {
		return accountResponse{}, fmt.Errorf("failed to fetch IBKR accounts: %w", err)
	}

	if len(accounts) == 0 {
		return accountResponse{}, errors.New("no IBKR paper account is available in the session")
	}

	account := accounts[0] // single account support

	if account.accountID() == "" {
		return accountResponse{}, errors.New("IBKR account response didn't include an id")
	}

	return account, nil
}

// fetchAccountSummary retrieves the account values needed by the portfolio view.
func (c *Client) fetchAccountSummary(ctx context.Context, account accountResponse) (portfolio.AccountSummary, error) {
	var response map[string]summaryValueResponse

	path := fmt.Sprintf("/portfolio/%s/summary", url.PathEscape(account.accountID()))

	if err := c.getJSON(ctx, path, &response); err != nil {
		return portfolio.AccountSummary{}, fmt.Errorf("failed to fetch account summary: %w", err)
	}

	return portfolio.AccountSummary{
		AccountID: account.accountID(),
		NetLiquidation: summaryMoney(
			response,
			account.Currency,
			"netliquidation",
			"NetLiquidation",
		),
		CashBalance: summaryMoney(
			response,
			account.Currency,
			"totalcashvalue",
			"TotalCashValue",
			"cashbalance",
			"CashBalance",
		),
	}, nil
}

// fetchExchangeRates normalizes ledger rates and adds the identity rate for the base currency.
func (c *Client) fetchExchangeRates(
	ctx context.Context,
	account accountResponse,
	baseCurrency string,
	retrievedAt time.Time,
) ([]portfolio.ExchangeRate, error) {
	var response map[string]ledgerValueResponse
	path := fmt.Sprintf("/portfolio/%s/ledger", url.PathEscape(account.accountID()))
	if err := c.getJSON(ctx, path, &response); err != nil {
		return nil, fmt.Errorf("failed to fetch account currency ledger: %w", err)
	}

	ratesByCurrency := make(map[string]portfolio.ExchangeRate)
	for key, value := range response {
		currency := strings.ToUpper(firstNonEmpty(value.Currency, key))
		if currency == "" || currency == "BASE" {
			continue
		}

		ratesByCurrency[currency] = portfolio.ExchangeRate{
			Currency:          currency,
			BaseCurrency:      strings.ToUpper(baseCurrency),
			BaseAmountPerUnit: value.ExchangeRate.value,
			Available:         value.ExchangeRate.valid && value.ExchangeRate.value > 0,
			Source:            ledgerRateSource,
			AsOf:              ledgerTime(value.Timestamp, retrievedAt),
		}
	}

	baseCurrency = strings.ToUpper(strings.TrimSpace(baseCurrency))
	if baseCurrency != "" {
		if _, exists := ratesByCurrency[baseCurrency]; !exists {
			ratesByCurrency[baseCurrency] = portfolio.ExchangeRate{
				Currency:          baseCurrency,
				BaseCurrency:      baseCurrency,
				BaseAmountPerUnit: 1,
				Available:         true,
				Source:            ledgerRateSource,
				AsOf:              retrievedAt.UTC(),
			}
		}
	}

	currencies := make([]string, 0, len(ratesByCurrency))
	for currency := range ratesByCurrency {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)

	rates := make([]portfolio.ExchangeRate, 0, len(currencies))
	for _, currency := range currencies {
		rates = append(rates, ratesByCurrency[currency])
	}
	return rates, nil
}

// ledgerTime accepts IBKR timestamps in seconds or milliseconds and falls back to retrieval time.
func ledgerTime(timestamp int64, fallback time.Time) time.Time {
	if timestamp <= 0 {
		return fallback.UTC()
	}
	if timestamp >= 1_000_000_000_000 {
		return time.UnixMilli(timestamp).UTC()
	}
	return time.Unix(timestamp, 0).UTC()
}

// fetchPositions retrieves all active positions for the account.
func (c *Client) fetchPositions(ctx context.Context, account accountResponse) ([]portfolio.Position, error) {
	positions := make([]portfolio.Position, 0)

	for page := 0; page < maxPositionPages; page++ {
		var responses []positionResponse
		path := fmt.Sprintf(
			"/portfolio/%s/positions/%d",
			url.PathEscape(account.accountID()),
			page,
		)

		if err := c.getJSON(ctx, path, &responses); err != nil {
			return nil, fmt.Errorf("failed to fetch positions page %d: %w", page, err)
		}

		for _, response := range responses {
			positions = append(positions, mapPosition(response, account.Currency))
		}

		if !hasMorePositionPages(responses) {
			return positions, nil
		}
	}

	return nil, fmt.Errorf("position response exceeded the %d-page safety limit", maxPositionPages)
}

// hasMorePositionPages applies IBKR page-size metadata while tolerating responses that omit it.
func hasMorePositionPages(responses []positionResponse) bool {
	if len(responses) == 0 {
		return false
	}

	for _, response := range responses {
		if response.PageSize > 0 {
			return len(responses) >= response.PageSize
		}
	}

	return true
}

// getJSON centralizes authenticated gateway GET handling and response decoding.
func (c *Client) getJSON(ctx context.Context, path string, target any) error {
	return c.requestJSON(ctx, http.MethodGet, path, nil, target)
}

func (c *Client) postJSON(ctx context.Context, path string, target any) error {
	return c.requestJSON(ctx, http.MethodPost, path, nil, target)
}

func (c *Client) postJSONBody(ctx context.Context, path string, body, target any) error {
	return c.requestJSON(ctx, http.MethodPost, path, body, target)
}

// requestJSON centralizes gateway request handling and optional response decoding.
func (c *Client) requestJSON(ctx context.Context, method, path string, body, target any) error {
	var encodedBody io.Reader
	if body != nil {
		contents, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to encode request body: %w", err)
		}
		encodedBody = bytes.NewReader(contents)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, encodedBody)

	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)

	if err != nil {
		return fmt.Errorf("gateway is not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return httpStatusError{statusCode: resp.StatusCode}
	}

	if target == nil {
		_, err := io.Copy(io.Discard, resp.Body)
		return err
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// mapPosition converts an IBKR position response into the broker-independent domain model.
func mapPosition(response positionResponse, fallbackCurrency string) portfolio.Position {
	currency := firstNonEmpty(response.Currency, fallbackCurrency)

	averageCost := firstDecimal(response.AverageCost, response.AveragePrice)
	unrealizedPnL := firstDecimal(response.UnrealizedPnL, response.UnrealizedPnLAlt)

	return portfolio.Position{
		Instrument: portfolio.Instrument{
			ContractID: response.ContractID,
			Symbol:     firstNonEmpty(response.Symbol, response.ContractDesc),
			Name:       firstNonEmpty(response.Name, response.ContractDesc),
			AssetClass: response.AssetClass,
			Exchange:   firstNonEmpty(response.Exchange, response.ListingExchange),
			Currency:   currency,
			Sector:     response.Sector,
			Industry:   response.SectorGroup,
		},
		Quantity:          response.Position.value,
		AverageCost:       decimalMoney(averageCost, currency),
		MarketPrice:       decimalMoney(response.MarketPrice, currency),
		MarketValue:       decimalMoney(response.MarketValue, currency),
		BaseMarketValue:   decimalMoney(response.BaseMarketValue, fallbackCurrency),
		UnrealizedPnL:     decimalMoney(unrealizedPnL, currency),
		BaseUnrealizedPnL: decimalMoney(response.BaseUnrealizedPnL, fallbackCurrency),
	}
}

func decimalMoney(value decimalNumber, currency string) portfolio.Money {
	return portfolio.Money{
		Amount:    value.value,
		Currency:  currency,
		Available: value.valid,
	}
}

// summaryMoney resolves alternate IBKR summary keys into one normalized monetary value.
func summaryMoney(values map[string]summaryValueResponse, fallbackCurrency string, keys ...string) portfolio.Money {
	for _, key := range keys {
		value, ok := values[key]

		if !ok {
			continue
		}

		amount, available := summaryAmount(value)

		return portfolio.Money{
			Amount:    amount,
			Currency:  firstNonEmpty(value.Currency, fallbackCurrency),
			Available: available,
		}
	}

	return portfolio.Money{
		Currency: fallbackCurrency,
	}
}

func summaryAmount(value summaryValueResponse) (float64, bool) {
	if value.Amount.valid {
		return value.Amount.value, true
	}

	return parseDecimalString(value.Value)
}

func parseDecimalString(value string) (float64, bool) {
	cleaned := strings.TrimSpace(strings.ReplaceAll(value, ",", ""))

	if cleaned == "" {
		return 0, false
	}

	parsed, err := strconv.ParseFloat(cleaned, 64)

	if err != nil {
		return 0, false
	}

	return parsed, true
}

// firstNonEmpty returns the first non-blank string.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)

		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

// firstDecimal returns the first numeric value that is present.
func firstDecimal(values ...decimalNumber) decimalNumber {
	for _, value := range values {
		if value.valid {
			return value
		}
	}

	return decimalNumber{}
}
