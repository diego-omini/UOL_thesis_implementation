// Package snaptradehistory imports estimated read-only account-value history from SnapTrade.
package snaptradehistory

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

	"wails/portfoliohistory"
)

const (
	defaultBaseURL = "https://api.snaptrade.com"
	maxResponse    = 2 << 20
)

var (
	// ErrConnectionRequired indicates that no unambiguous read-only IBKR paper account is connected.
	ErrConnectionRequired = errors.New("a read-only Interactive Brokers paper account must be connected to SnapTrade")
	// ErrFeatureNotEnabled indicates that SnapTrade's free balance-history add-on is disabled.
	ErrFeatureNotEnabled = errors.New("SnapTrade balance history is not enabled for this API key")
)

// Client calls only the Personal API key endpoints required for read-only history.
type Client struct {
	clientID    string
	consumerKey string
	baseURL     string
	httpClient  *http.Client
	now         func() time.Time
}

// NewClient creates a Personal API key client. Empty credentials leave the optional integration disabled.
func NewClient(clientID, consumerKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		clientID: strings.TrimSpace(clientID), consumerKey: strings.TrimSpace(consumerKey),
		baseURL: defaultBaseURL, httpClient: httpClient, now: time.Now,
	}
}

// Configured reports whether both Personal API key values are present.
func (c *Client) Configured() bool {
	return c != nil && c.clientID != "" && c.consumerKey != ""
}

// ConnectionPortalURL creates a short-lived portal URL restricted to read-only brokerage access.
func (c *Client) ConnectionPortalURL(ctx context.Context) (string, error) {
	if !c.Configured() {
		return "", fmt.Errorf("SnapTrade Personal API credentials are not configured")
	}
	body := struct {
		ConnectionType string `json:"connectionType"`
	}{ConnectionType: "read"}
	var response struct {
		RedirectURI string `json:"redirectURI"`
	}
	if err := c.do(ctx, http.MethodPost, "/snapTrade/login", body, &response); err != nil {
		return "", fmt.Errorf("create SnapTrade connection portal: %w", err)
	}
	parsed, err := url.ParseRequestURI(response.RedirectURI)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", fmt.Errorf("SnapTrade returned an invalid connection portal URL")
	}
	return response.RedirectURI, nil
}

// FetchPortfolioNAVHistory retrieves at most one year of estimated account value for one IBKR paper account.
func (c *Client) FetchPortfolioNAVHistory(ctx context.Context, applicationAccountID string) (portfoliohistory.NAVHistory, error) {
	if !c.Configured() {
		return portfoliohistory.NAVHistory{}, fmt.Errorf("SnapTrade Personal API credentials are not configured")
	}
	var accounts []account
	if err := c.do(ctx, http.MethodGet, "/accounts", nil, &accounts); err != nil {
		return portfoliohistory.NAVHistory{}, fmt.Errorf("list SnapTrade accounts: %w", err)
	}
	selected, err := selectIBKRPaperAccount(accounts, applicationAccountID)
	if err != nil {
		return portfoliohistory.NAVHistory{}, err
	}
	var response balanceHistoryResponse
	if err := c.do(ctx, http.MethodGet, "/accounts/"+url.PathEscape(selected.ID)+"/balanceHistory", nil, &response); err != nil {
		return portfoliohistory.NAVHistory{}, fmt.Errorf("fetch SnapTrade account history: %w", err)
	}
	return mapHistory(applicationAccountID, response, c.now().UTC())
}

type account struct {
	ID                   string  `json:"id"`
	Number               string  `json:"number"`
	InstitutionAccountID *string `json:"institution_account_id"`
	InstitutionName      string  `json:"institution_name"`
	IsPaper              bool    `json:"is_paper"`
}

type balanceHistoryResponse struct {
	History  []balanceHistoryItem `json:"history"`
	Currency string               `json:"currency"`
}

type balanceHistoryItem struct {
	Date       string `json:"date"`
	TotalValue string `json:"total_value"`
}

func selectIBKRPaperAccount(accounts []account, applicationAccountID string) (account, error) {
	target := strings.ToUpper(strings.TrimSpace(applicationAccountID))
	eligible := make([]account, 0, 1)
	matches := make([]account, 0, 1)
	for _, candidate := range accounts {
		institution := strings.ToLower(strings.TrimSpace(candidate.InstitutionName))
		if candidate.ID == "" || !candidate.IsPaper || (!strings.Contains(institution, "interactive brokers") && institution != "ibkr") {
			continue
		}
		institutionID := ""
		if candidate.InstitutionAccountID != nil {
			institutionID = strings.ToUpper(strings.TrimSpace(*candidate.InstitutionAccountID))
		}
		if target != "" && (strings.ToUpper(strings.TrimSpace(candidate.Number)) == target || institutionID == target) {
			matches = append(matches, candidate)
		}
		eligible = append(eligible, candidate)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return account{}, fmt.Errorf("%w: multiple accounts match the active IBKR account", ErrConnectionRequired)
	}
	if len(eligible) == 1 {
		candidateNumber := strings.TrimSpace(eligible[0].Number)
		institutionID := ""
		if eligible[0].InstitutionAccountID != nil {
			institutionID = strings.TrimSpace(*eligible[0].InstitutionAccountID)
		}
		if target != "" && (candidateNumber != "" || institutionID != "") {
			return account{}, fmt.Errorf("%w: the connected account does not match the active IBKR account", ErrConnectionRequired)
		}
		return eligible[0], nil
	}
	if len(eligible) == 0 {
		return account{}, ErrConnectionRequired
	}
	return account{}, fmt.Errorf("%w: multiple accounts are available and none matches the active IBKR account", ErrConnectionRequired)
}

func mapHistory(applicationAccountID string, response balanceHistoryResponse, retrievedAt time.Time) (portfoliohistory.NAVHistory, error) {
	currency := strings.ToUpper(strings.TrimSpace(response.Currency))
	if len(currency) != 3 {
		return portfoliohistory.NAVHistory{}, fmt.Errorf("SnapTrade history has an invalid base currency")
	}
	byDate := make(map[string]portfoliohistory.NAVObservation, len(response.History))
	for index, item := range response.History {
		observedAt, err := parseObservationDate(item.Date)
		if err != nil {
			return portfoliohistory.NAVHistory{}, fmt.Errorf("SnapTrade history observation %d has an invalid date: %w", index, err)
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(item.TotalValue), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return portfoliohistory.NAVHistory{}, fmt.Errorf("SnapTrade history observation %d has an invalid value", index)
		}
		dateKey := observedAt.Format("2006-01-02")
		byDate[dateKey] = portfoliohistory.NAVObservation{
			Date: observedAt, Value: value, BaseCurrency: currency, Source: portfoliohistory.SourceSnapTradeBalanceHistory,
		}
	}
	points := make([]portfoliohistory.NAVObservation, 0, len(byDate))
	for _, point := range byDate {
		points = append(points, point)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Date.Before(points[j].Date) })
	history := portfoliohistory.NAVHistory{
		AccountID: strings.TrimSpace(applicationAccountID), Frequency: "D", Source: portfoliohistory.SourceSnapTradeBalanceHistory,
		RetrievedAt: retrievedAt, Points: points,
	}
	if err := history.Validate(); err != nil {
		return portfoliohistory.NAVHistory{}, err
	}
	return history, nil
}

func parseObservationDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func (c *Client) do(ctx context.Context, method, endpoint string, body any, destination any) error {
	query := url.Values{}
	query.Set("clientId", c.clientID)
	query.Set("timestamp", strconv.FormatInt(c.now().Unix(), 10))
	encodedQuery := query.Encode()

	var bodyBytes []byte
	var err error
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	signature, err := requestSignature("/api/v1"+endpoint, encodedQuery, body, c.consumerKey)
	if err != nil {
		return err
	}
	requestURL := strings.TrimRight(c.baseURL, "/") + endpoint + "?" + encodedQuery
	req, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Signature", signature)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("SnapTrade request failed")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusForbidden && strings.HasSuffix(endpoint, "/balanceHistory") {
		return ErrFeatureNotEnabled
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponse))
		return fmt.Errorf("SnapTrade returned HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponse))
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode SnapTrade response: %w", err)
	}
	return nil
}

func requestSignature(path, query string, body any, consumerKey string) (string, error) {
	payload := struct {
		Content any    `json:"content"`
		Path    string `json:"path"`
		Query   string `json:"query"`
	}{Content: body, Path: path, Query: query}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	hash := hmac.New(sha256.New, []byte(consumerKey))
	_, _ = hash.Write(encoded)
	return base64.StdEncoding.EncodeToString(hash.Sum(nil)), nil
}
