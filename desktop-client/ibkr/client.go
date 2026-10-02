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
	"math"
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
	marketDataSnapshotRetryDelay          = 250 * time.Millisecond
	marketDataSnapshotAttempts            = 6
	marketDataFields                      = "84,86,6509,7293,7294,7295,7296,7741,7762"
)

// Client is the IBKR Client Portal Gateway adapter.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// Information about authentication.
type authStatusResponse struct {
	Authenticated bool   `json:"authenticated"`
	Connected     bool   `json:"connected"`
	Competing     bool   `json:"competing"`
	Established   *bool  `json:"established"`
	Fail          string `json:"fail"`
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
	message    string
}

func (e httpStatusError) Error() string {
	if e.message != "" {
		return fmt.Sprintf("gateway returned HTTP %d: %s", e.statusCode, e.message)
	}
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

type securityDefinitionSearchResponse struct {
	ContractID    json.RawMessage                   `json:"conid"`
	Symbol        string                            `json:"symbol"`
	CompanyName   string                            `json:"companyName"`
	CompanyHeader string                            `json:"companyHeader"`
	Description   string                            `json:"description"`
	SecurityType  string                            `json:"secType"`
	Sections      []securityDefinitionSearchSection `json:"sections"`
}

type securityDefinitionSearchSection struct {
	SecurityType string `json:"secType"`
}

type contractInfoResponse struct {
	ContractID         int64  `json:"con_id"`
	ContractIDAlt      int64  `json:"conid"`
	Symbol             string `json:"symbol"`
	SecurityType       string `json:"instrument_type"`
	SecurityTypeAlt    string `json:"secType"`
	CompanyName        string `json:"company_name"`
	CompanyNameAlt     string `json:"companyName"`
	Currency           string `json:"currency"`
	Exchange           string `json:"exchange"`
	ListingExchange    string `json:"listing_exchange"`
	ListingExchangeAlt string `json:"listingExchange"`
}

type marketDataHistoryResponse struct {
	Availability string                 `json:"mdAvailability"`
	DelaySeconds int                    `json:"mktDataDelay"`
	Data         []marketDataHistoryBar `json:"data"`
}

type marketDataHistoryBar struct {
	Open   float64 `json:"o"`
	Close  float64 `json:"c"`
	Low    float64 `json:"l"`
	High   float64 `json:"h"`
	Volume float64 `json:"v"`
	Time   int64   `json:"t"`
}

type stockHistoryRequest struct {
	Period     string
	Bar        string
	OutsideRTH bool
}

type orderRequestEnvelope struct {
	Orders []orderRequest `json:"orders"`
}

type orderRequest struct {
	AccountID   string           `json:"acctId"`
	ContractID  int64            `json:"conid"`
	OrderType   string           `json:"orderType"`
	Side        broker.OrderSide `json:"side"`
	TimeInForce string           `json:"tif"`
	Quantity    float64          `json:"quantity"`
}

type orderPreviewResponse struct {
	Amount struct {
		Amount     string `json:"amount"`
		Commission string `json:"commission"`
		Total      string `json:"total"`
	} `json:"amount"`
	Position struct {
		Current string `json:"current"`
		After   string `json:"after"`
	} `json:"position"`
	Warning string `json:"warn"`
	Error   string `json:"error"`
}

type orderResponseItem struct {
	OrderID json.RawMessage `json:"order_id"`
	Status  string          `json:"order_status"`
	ReplyID string          `json:"id"`
	Message []string        `json:"message"`
	Error   string          `json:"error"`
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
		forceCompetition := requiresForcedCompetition(authentication.Fail)
		initialized, err := c.initializeBrokerageSession(
			ctx,
			forceCompetition,
		)
		if err != nil {
			return sessionRequestFailure(err, "IBKR brokerage session initialization failed")
		}
		authentication = initialized
		if !forceCompetition && !authentication.Competing && requiresForcedCompetition(authentication.Fail) {
			authentication, err = c.initializeBrokerageSession(ctx, true)
			if err != nil {
				return sessionRequestFailure(err, "IBKR brokerage session initialization failed")
			}
		}
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

// initializeBrokerageSession starts the brokerage session and waits for account services to become ready.
func (c *Client) initializeBrokerageSession(ctx context.Context, compete bool) (authStatusResponse, error) {
	var status authStatusResponse
	err := c.postJSONBody(ctx, "/iserver/auth/ssodh/init", brokerageSessionInitRequest{
		Publish: true,
		Compete: compete,
	}, &status)
	if err != nil {
		return authStatusResponse{}, err
	}
	if brokerageSessionReady(status) || status.Competing {
		return status, nil
	}
	if !compete && requiresForcedCompetition(status.Fail) {
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
			if brokerageSessionReady(status) || status.Competing || (!compete && requiresForcedCompetition(status.Fail)) {
				return status, nil
			}
		}
	}
}

// requiresForcedCompetition recognizes the gateway response that explicitly requires the compete flag.
func requiresForcedCompetition(failure string) bool {
	return strings.Contains(
		strings.ToLower(failure),
		"force compete capability must be used together with compete flag",
	)
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

// SearchStocks merges ticker and company-name results from the authenticated gateway session.
func (c *Client) SearchStocks(ctx context.Context, query string) ([]portfolio.InstrumentSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []portfolio.InstrumentSearchResult{}, nil
	}

	tickerResults, tickerErr := c.searchSecurityDefinitions(ctx, strings.ToUpper(query), false)
	nameResults, nameErr := c.searchSecurityDefinitions(ctx, query, true)
	if tickerErr != nil && nameErr != nil {
		return nil, fmt.Errorf("failed to search IBKR stock contracts by ticker (%v) and company name (%v)", tickerErr, nameErr)
	}

	response := append(tickerResults, nameResults...)
	results := make([]portfolio.InstrumentSearchResult, 0, len(response))
	seen := make(map[int64]bool)
	for _, contract := range response {
		if !strings.EqualFold(contract.SecurityType, "STK") && !hasSecurityType(contract.Sections, "STK") {
			continue
		}
		contractID, ok := parseContractID(contract.ContractID)
		if !ok || seen[contractID] {
			continue
		}
		seen[contractID] = true
		name, exchange := normalizeSearchIdentity(contract)
		results = append(results, portfolio.InstrumentSearchResult{
			ContractID: contractID,
			Symbol:     strings.ToUpper(strings.TrimSpace(contract.Symbol)),
			Name:       name,
			Exchange:   exchange,
			AssetClass: "STK",
		})
	}

	normalizedQuery := strings.ToUpper(query)
	sort.SliceStable(results, func(i, j int) bool {
		iExact := results[i].Symbol == normalizedQuery
		jExact := results[j].Symbol == normalizedQuery
		if iExact != jExact {
			return iExact
		}
		iNamePrefix := strings.HasPrefix(strings.ToUpper(results[i].Name), normalizedQuery)
		jNamePrefix := strings.HasPrefix(strings.ToUpper(results[j].Name), normalizedQuery)
		if iNamePrefix != jNamePrefix {
			return iNamePrefix
		}
		if results[i].Symbol != results[j].Symbol {
			return results[i].Symbol < results[j].Symbol
		}
		return false
	})
	if len(results) > 8 {
		results = results[:8]
	}
	return results, nil
}

func (c *Client) searchSecurityDefinitions(ctx context.Context, query string, byName bool) ([]securityDefinitionSearchResponse, error) {
	path := "/iserver/secdef/search?symbol=" + url.QueryEscape(query)
	if byName {
		path += "&name=true"
	}
	var response []securityDefinitionSearchResponse
	return response, c.getJSON(ctx, path, &response)
}

func normalizeSearchIdentity(contract securityDefinitionSearchResponse) (string, string) {
	name := strings.TrimSpace(contract.CompanyName)
	exchange := strings.TrimSpace(contract.Description)
	header := strings.TrimSpace(contract.CompanyHeader)
	if exchange == "" && strings.HasSuffix(header, ")") {
		if start := strings.LastIndex(header, "("); start >= 0 {
			exchange = strings.TrimSpace(header[start+1 : len(header)-1])
		}
	}
	if exchange != "" {
		suffix := " (" + exchange + ")"
		name = strings.TrimSpace(strings.TrimSuffix(name, suffix))
	}
	return name, exchange
}

// FetchStockSnapshot primes IBKR market data and maps available quote fields without inferring missing fundamentals.
func (c *Client) FetchStockSnapshot(ctx context.Context, contractID int64) (portfolio.StockSnapshot, error) {
	var info contractInfoResponse
	infoPath := fmt.Sprintf("/iserver/contract/%d/info", contractID)
	if err := c.getJSON(ctx, infoPath, &info); err != nil {
		return portfolio.StockSnapshot{}, fmt.Errorf("failed to fetch IBKR stock contract details: %w", err)
	}

	path := fmt.Sprintf("/iserver/marketdata/snapshot?conids=%d&fields=%s", contractID, url.QueryEscape(marketDataFields))
	fields, err := c.fetchMarketDataFields(ctx, path)
	if err != nil {
		return portfolio.StockSnapshot{}, fmt.Errorf("failed to fetch IBKR stock market data: %w", err)
	}
	availability := rawString(fields["6509"])
	unavailableReason := marketDataUnavailableReason(availability)

	return portfolio.StockSnapshot{
		Instrument: portfolio.Instrument{
			ContractID: contractID,
			Symbol:     firstNonEmpty(info.Symbol),
			Name:       firstNonEmpty(info.CompanyName, info.CompanyNameAlt),
			AssetClass: firstNonEmpty(info.SecurityType, info.SecurityTypeAlt),
			Exchange:   firstNonEmpty(info.ListingExchange, info.ListingExchangeAlt, info.Exchange),
			Currency:   info.Currency,
		},
		Currency:         info.Currency,
		OpenPrice:        optionalMarketNumber(fields["7295"], unavailableReason),
		ClosePrice:       firstAvailableMarketNumber(unavailableReason, fields["7741"], fields["7296"]),
		BidPrice:         optionalMarketNumber(fields["84"], unavailableReason),
		AskPrice:         optionalMarketNumber(fields["86"], unavailableReason),
		Volume:           optionalMarketNumber(fields["7762"], unavailableReason),
		High52Week:       optionalMarketNumber(fields["7293"], unavailableReason),
		Low52Week:        optionalMarketNumber(fields["7294"], unavailableReason),
		MarketDataStatus: availability,
		AsOf:             time.Now().UTC(),
	}, nil
}

// FetchStockHistory requests a bounded OHLCV series for one supported chart range.
func (c *Client) FetchStockHistory(ctx context.Context, contractID int64, timeRange string) (portfolio.StockHistory, error) {
	request, ok := stockHistoryRanges()[timeRange]
	if !ok {
		return portfolio.StockHistory{}, fmt.Errorf("unsupported stock history range %q", timeRange)
	}
	query := url.Values{
		"conid":      {strconv.FormatInt(contractID, 10)},
		"period":     {request.Period},
		"bar":        {request.Bar},
		"outsideRth": {strconv.FormatBool(request.OutsideRTH)},
		"source":     {"Last"},
	}
	var response marketDataHistoryResponse
	if err := c.getJSON(ctx, "/iserver/marketdata/history?"+query.Encode(), &response); err != nil {
		return portfolio.StockHistory{}, fmt.Errorf("failed to fetch IBKR stock history: %w", err)
	}

	points := make([]portfolio.StockHistoryPoint, 0, len(response.Data))
	for _, bar := range response.Data {
		if bar.Time <= 0 {
			continue
		}
		points = append(points, portfolio.StockHistoryPoint{
			Time: marketDataTime(bar.Time), Open: bar.Open, High: bar.High,
			Low: bar.Low, Close: bar.Close, Volume: bar.Volume,
		})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Time.Before(points[j].Time) })
	reason := ""
	if len(points) == 0 {
		reason = "IBKR returned no historical bars for this range or market-data entitlement"
	}
	return portfolio.StockHistory{
		Range: timeRange, Points: points, MarketDataStatus: response.Availability,
		DelaySeconds: response.DelaySeconds, UnavailableReason: reason,
	}, nil
}

// PreviewOrder submits a non-transmitting IBKR what-if request for one market order.
func (c *Client) PreviewOrder(ctx context.Context, accountID string, order broker.MarketOrder) (broker.OrderPreview, error) {
	if err := c.primeOrderMarketData(ctx, order.ContractID); err != nil {
		return broker.OrderPreview{}, err
	}
	var response orderPreviewResponse
	path := fmt.Sprintf("/iserver/account/%s/orders/whatif", url.PathEscape(accountID))
	if err := c.postJSONBody(ctx, path, newOrderEnvelope(accountID, order), &response); err != nil {
		return broker.OrderPreview{}, err
	}
	if strings.TrimSpace(response.Error) != "" {
		return broker.OrderPreview{}, errors.New(strings.TrimSpace(response.Error))
	}
	orderAmount := parseOrderMoney(response.Amount.Amount)
	commission := parseOrderMoney(response.Amount.Commission)
	total := parseOrderMoney(response.Amount.Total)
	currentPosition, _ := parseFormattedNumber(response.Position.Current)
	resultingPosition, afterAvailable := parseFormattedNumber(response.Position.After)
	if !afterAvailable {
		resultingPosition = currentPosition
		if order.Side == broker.OrderSideBuy {
			resultingPosition += order.Quantity
		} else {
			resultingPosition -= order.Quantity
		}
	}
	warnings := []string{}
	if warning := strings.TrimSpace(response.Warning); warning != "" {
		warnings = append(warnings, warning)
	}
	return broker.OrderPreview{
		OrderAmount: orderAmount, Commission: commission, Total: total,
		CurrentPosition: currentPosition, ResultingPosition: resultingPosition, Warnings: warnings,
	}, nil
}

// SubmitOrder transmits one previously confirmed market order to IBKR.
func (c *Client) SubmitOrder(ctx context.Context, accountID string, order broker.MarketOrder) (broker.OrderResult, error) {
	path := fmt.Sprintf("/iserver/account/%s/orders", url.PathEscape(accountID))
	return c.sendOrderRequest(ctx, path, newOrderEnvelope(accountID, order))
}

// ReplyToOrder answers an IBKR warning that interrupted an order submission.
func (c *Client) ReplyToOrder(ctx context.Context, replyID string, confirmed bool) (broker.OrderResult, error) {
	path := "/iserver/reply/" + url.PathEscape(replyID)
	return c.sendOrderRequest(ctx, path, struct {
		Confirmed bool `json:"confirmed"`
	}{Confirmed: confirmed})
}

func (c *Client) primeOrderMarketData(ctx context.Context, contractID int64) error {
	path := fmt.Sprintf("/iserver/marketdata/snapshot?conids=%d&fields=84%%2C86", contractID)
	var response []map[string]json.RawMessage
	if err := c.getJSON(ctx, path, &response); err != nil {
		return fmt.Errorf("prime market data before order preview: %w", err)
	}
	return nil
}

func newOrderEnvelope(accountID string, order broker.MarketOrder) orderRequestEnvelope {
	return orderRequestEnvelope{Orders: []orderRequest{{
		AccountID: accountID, ContractID: order.ContractID, OrderType: "MKT",
		Side: order.Side, TimeInForce: "DAY", Quantity: order.Quantity,
	}}}
}

// sendOrderRequest decodes IBKR's acknowledgement, warning, and rejection response variants.
func (c *Client) sendOrderRequest(ctx context.Context, path string, body any) (broker.OrderResult, error) {
	var raw json.RawMessage
	if err := c.postJSONBody(ctx, path, body, &raw); err != nil {
		return broker.OrderResult{}, err
	}
	var items []orderResponseItem
	if err := json.Unmarshal(raw, &items); err != nil {
		var item orderResponseItem
		if objectErr := json.Unmarshal(raw, &item); objectErr != nil {
			return broker.OrderResult{}, fmt.Errorf("decode IBKR order response: %w", err)
		}
		items = []orderResponseItem{item}
	}
	if len(items) == 0 {
		return broker.OrderResult{}, errors.New("IBKR returned an empty order response")
	}
	item := items[0]
	return broker.OrderResult{
		OrderID: rawScalarString(item.OrderID), Status: item.Status,
		ReplyID: item.ReplyID, Messages: item.Message, BrokerError: strings.TrimSpace(item.Error),
	}, nil
}

func parseOrderMoney(value string) portfolio.Money {
	trimmed := strings.TrimSpace(value)
	if index := strings.Index(trimmed, " ("); index >= 0 {
		trimmed = trimmed[:index]
	}
	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return portfolio.Money{}
	}
	amount, ok := parseFormattedNumber(fields[0])
	if !ok {
		return portfolio.Money{}
	}
	return portfolio.Money{Amount: math.Abs(amount), Currency: fields[len(fields)-1], Available: true}
}

func parseFormattedNumber(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(value), ",", ""), 64)
	return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
}

func rawScalarString(value json.RawMessage) string {
	if len(value) == 0 || string(value) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(value, &text) == nil {
		return text
	}
	return strings.Trim(string(value), `"`)
}

func stockHistoryRanges() map[string]stockHistoryRequest {
	return map[string]stockHistoryRequest{
		"24h": {Period: "1d", Bar: "5min", OutsideRTH: true},
		"1m":  {Period: "1m", Bar: "1h", OutsideRTH: false},
		"1y":  {Period: "1y", Bar: "1d", OutsideRTH: false},
		"all": {Period: "15y", Bar: "1w", OutsideRTH: false},
	}
}

func (c *Client) fetchMarketDataFields(ctx context.Context, path string) (map[string]json.RawMessage, error) {
	fields := make(map[string]json.RawMessage)
	for attempt := 0; attempt < marketDataSnapshotAttempts; attempt++ {
		var response []map[string]json.RawMessage
		if err := c.getJSON(ctx, path, &response); err != nil {
			return nil, err
		}
		if len(response) > 0 && response[0] != nil {
			mergeMarketDataFields(fields, response[0])
			if attempt > 0 && hasCompleteStockSnapshot(fields) {
				return fields, nil
			}
		}
		if attempt < marketDataSnapshotAttempts-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(marketDataSnapshotRetryDelay):
			}
		}
	}
	return fields, nil
}

func mergeMarketDataFields(target, incoming map[string]json.RawMessage) {
	for key, value := range incoming {
		text := strings.TrimSpace(rawString(value))
		if text != "" && text != "-" {
			target[key] = value
		}
	}
}

func hasCompleteStockSnapshot(fields map[string]json.RawMessage) bool {
	for _, key := range []string{"84", "86", "7293", "7294", "7295", "7762"} {
		if _, ok := parseMarketNumber(fields[key]); !ok {
			return false
		}
	}
	_, priorCloseAvailable := parseMarketNumber(fields["7741"])
	_, closeAvailable := parseMarketNumber(fields["7296"])
	return priorCloseAvailable || closeAvailable
}

func firstAvailableMarketNumber(reason string, values ...json.RawMessage) portfolio.OptionalNumber {
	for _, value := range values {
		if parsed, ok := parseMarketNumber(value); ok {
			return portfolio.OptionalNumber{Value: parsed, Available: true}
		}
	}
	return portfolio.OptionalNumber{UnavailableReason: reason}
}

func optionalMarketNumber(value json.RawMessage, reason string) portfolio.OptionalNumber {
	if parsed, ok := parseMarketNumber(value); ok {
		return portfolio.OptionalNumber{Value: parsed, Available: true}
	}
	return portfolio.OptionalNumber{UnavailableReason: reason}
}

func parseMarketNumber(value json.RawMessage) (float64, bool) {
	text := rawString(value)
	text = strings.TrimSpace(strings.ReplaceAll(text, ",", ""))
	if text == "" || text == "-" {
		return 0, false
	}
	if (text[0] == 'C' || text[0] == 'H') && len(text) > 1 {
		text = strings.TrimSpace(text[1:])
	}
	multiplier := 1.0
	if len(text) > 1 {
		switch text[len(text)-1] {
		case 'K', 'k':
			multiplier, text = 1_000, text[:len(text)-1]
		case 'M', 'm':
			multiplier, text = 1_000_000, text[:len(text)-1]
		case 'B', 'b':
			multiplier, text = 1_000_000_000, text[:len(text)-1]
		}
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	return parsed * multiplier, err == nil
}

func rawString(value json.RawMessage) string {
	if len(value) == 0 || string(value) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(value, &text); err == nil {
		return text
	}
	return strings.TrimSpace(string(value))
}

func marketDataUnavailableReason(availability string) string {
	if strings.HasPrefix(availability, "N") {
		return "Market data is not subscribed for this contract"
	}
	return "IBKR did not return this value"
}

func marketDataTime(value int64) time.Time {
	if value >= 1_000_000_000_000 {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}

func hasSecurityType(sections []securityDefinitionSearchSection, securityType string) bool {
	for _, section := range sections {
		if strings.EqualFold(section.SecurityType, securityType) {
			return true
		}
	}
	return false
}

func parseContractID(value json.RawMessage) (int64, bool) {
	text := strings.Trim(strings.TrimSpace(string(value)), `"`)
	contractID, err := strconv.ParseInt(text, 10, 64)
	return contractID, err == nil && contractID > 0
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
		contents, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		if readErr != nil {
			return httpStatusError{statusCode: resp.StatusCode}
		}
		return httpStatusError{
			statusCode: resp.StatusCode,
			message:    gatewayErrorMessage(contents),
		}
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

// gatewayErrorMessage extracts a bounded human-readable error from IBKR's non-success response variants.
func gatewayErrorMessage(contents []byte) string {
	var response struct {
		Error   json.RawMessage `json:"error"`
		Message json.RawMessage `json:"message"`
	}
	if json.Unmarshal(contents, &response) == nil {
		if message := rawErrorMessage(response.Error); message != "" {
			return message
		}
		if message := rawErrorMessage(response.Message); message != "" {
			return message
		}
	}
	return ""
}

func rawErrorMessage(value json.RawMessage) string {
	if len(value) == 0 || string(value) == "null" {
		return ""
	}
	var message string
	if json.Unmarshal(value, &message) == nil {
		return boundedErrorMessage(message)
	}
	var messages []string
	if json.Unmarshal(value, &messages) == nil {
		return boundedErrorMessage(strings.Join(messages, "; "))
	}
	return ""
}

func boundedErrorMessage(message string) string {
	message = strings.Join(strings.Fields(message), " ")
	const maximumLength = 512
	if len(message) > maximumLength {
		return message[:maximumLength] + "..."
	}
	return message
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
