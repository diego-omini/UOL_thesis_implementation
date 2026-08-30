// Package ibkr contains specific implementation to connect to the ibkr gateway
//
// The package is responsible for http calls to ibkr and for mapping responses to broker-indipendent domain models.
package ibkr

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"wails/broker"
	"wails/portfolio"
)

const maxPositionPages = 100

// IBKR client portal gateway adapter
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Information about authentication.
type authStatusResponse struct {
	Authenticated bool `json:"authenticated"`
	Connected     bool `json:"connected"`
	Competing     bool `json:"competing"`
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

// Mirrors response from /portfolio/{account}/positions/{page}.
type positionResponse struct {
	AccountID        string        `json:"acctId"`
	ContractID       int64         `json:"conid"`
	Symbol           string        `json:"ticker"`
	ContractDesc     string        `json:"contractDesc"`
	Name             string        `json:"name"`
	AssetClass       string        `json:"assetClass"`
	Sector           string        `json:"sector"`
	SectorGroup      string        `json:"sectorGroup"`
	Exchange         string        `json:"exchange"`
	ListingExchange  string        `json:"listingExchange"`
	Currency         string        `json:"currency"`
	PageSize         int           `json:"pageSize"`
	Position         decimalNumber `json:"position"`
	AverageCost      decimalNumber `json:"avgCost"`
	AveragePrice     decimalNumber `json:"avgPrice"`
	MarketPrice      decimalNumber `json:"mktPrice"`
	MarketValue      decimalNumber `json:"mktValue"`
	UnrealizedPnL    decimalNumber `json:"unrealizedPnl"`
	UnrealizedPnLAlt decimalNumber `json:"unrealizedPNL"`
}

// IBKR returns numbers as JSON numbers, strings, empty string, or null.
// Using this struct keeps the mapper simplier.
type decimalNumber struct {
	value float64
	valid bool
}

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
	var status authStatusResponse

	if err := c.getJSON(ctx, "/iserver/auth/status", &status); err != nil {
		return broker.ConnectionStatus{
			Connected: false,
			Message:   fmt.Sprintf("IBKR gateway authentication check failed: %v", err),
		}
	}

	if status.Competing {
		return broker.ConnectionStatus{
			Connected: false,
			Message:   "IBKR session is competing with another active session",
		}
	}

	if status.Authenticated && status.Connected {
		return broker.ConnectionStatus{
			Connected: true,
			Message:   "IBKR gateway is reachable and authenticated",
		}
	}

	return broker.ConnectionStatus{
		Connected: false,
		Message:   "IBKR gateway is reachable but not authenticated",
	}
}

// FetchPortfolio fetches and maps the paper account info to domain models.
func (c *Client) FetchPortfolio(ctx context.Context) (portfolio.Portfolio, error) {
	status := c.CheckConnection(ctx)

	if !status.Connected {
		return portfolio.Portfolio{}, errors.New(status.Message)
	}

	account, err := c.fetchAccount(ctx)

	if err != nil {
		return portfolio.Portfolio{}, err
	}

	summary, err := c.fetchAccountSummary(ctx, account)

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
	}, nil
}

// fetchAccount retrieves account from /portfolio/accounts.
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

// fetchAccountSummary retrieves account infos.
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

// getJSON centralizes basic gateway http handling.
func (c *Client) getJSON(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)

	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)

	if err != nil {
		return fmt.Errorf("gateway is not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("gateway returned HTTP %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// mapPosition converts IBKR data to broker-indipendent model.
func mapPosition(response positionResponse, fallbackCurrency string) portfolio.Position {
	currency := firstNonEmpty(response.Currency, fallbackCurrency)

	averageCost := firstDecimalValue(response.AverageCost, response.AveragePrice)

	if averageCost == 0 {
		averageCost = response.AveragePrice.value
	}

	unrealizedPnL := firstDecimalValue(response.UnrealizedPnL, response.UnrealizedPnLAlt)
	if unrealizedPnL == 0 {
		unrealizedPnL = response.UnrealizedPnLAlt.value
	}

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
		Quantity: response.Position.value,
		AverageCost: portfolio.Money{
			Amount:   averageCost,
			Currency: currency,
		},
		MarketPrice: portfolio.Money{
			Amount:   response.MarketPrice.value,
			Currency: currency,
		},
		MarketValue: portfolio.Money{
			Amount:   response.MarketValue.value,
			Currency: currency,
		},
		UnrealizedPnL: portfolio.Money{
			Amount:   unrealizedPnL,
			Currency: currency,
		},
	}
}

// summaryMoney returns information about account balance
func summaryMoney(values map[string]summaryValueResponse, fallbackCurrency string, keys ...string) portfolio.Money {
	for _, key := range keys {
		value, ok := values[key]

		if !ok {
			continue
		}

		return portfolio.Money{
			Amount:   summaryAmount(value),
			Currency: firstNonEmpty(value.Currency, fallbackCurrency),
		}
	}

	return portfolio.Money{
		Amount:   0,
		Currency: fallbackCurrency,
	}
}

func summaryAmount(value summaryValueResponse) float64 {
	if value.Amount.valid {
		return value.Amount.value
	}

	parsed, ok := parseDecimalString(value.Value)

	if !ok {
		return 0
	}

	return parsed
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

// firstNonEmpty returns the first element that has an assigned value
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)

		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

// firstDecimalValue returns the first numeric value that is present.
func firstDecimalValue(values ...decimalNumber) float64 {
	for _, value := range values {
		if value.valid {
			return value.value
		}
	}

	return 0
}
