package fundholdings

// This file parses the official iShares full-holdings CSV for an exact registry of supported funds.

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"wails/portfolio"
)

const (
	iSharesSourceType = "official_issuer_full_holdings_csv"
	iSharesCacheTTL   = 6 * time.Hour
	iSharesStaleAfter = 14 * 24 * time.Hour
)

type iSharesFund struct {
	Symbol string
	Name   string
	URL    string
}

type cachedHoldings struct {
	value portfolio.FundHoldings
}

// ISharesSource downloads full holdings only for exact fund identities in its fixed registry.
type ISharesSource struct {
	client   *http.Client
	funds    []iSharesFund
	mu       sync.Mutex
	cache    map[string]cachedHoldings
	cacheTTL time.Duration
}

// NewISharesSource creates the production adapter for explicitly verified issuer downloads.
func NewISharesSource(client *http.Client) *ISharesSource {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &ISharesSource{
		client: client, cache: make(map[string]cachedHoldings), cacheTTL: iSharesCacheTTL,
		funds: []iSharesFund{{
			Symbol: "IVV", Name: "iShares Core S&P 500 ETF",
			URL: "https://www.ishares.com/us/products/239726/ishares-core-s-p-500-etf/latest-holdings.csv",
		}},
	}
}

// Supports requires both the public ticker and issuer fund name to match the fixed registry.
func (s *ISharesSource) Supports(instrument portfolio.Instrument) bool {
	_, ok := s.lookup(instrument)
	return ok
}

// Fetch retrieves, normalizes, validates, and briefly caches one official issuer file.
func (s *ISharesSource) Fetch(ctx context.Context, instrument portfolio.Instrument, retrievedAt time.Time) (portfolio.FundHoldings, error) {
	fund, ok := s.lookup(instrument)
	if !ok {
		return portfolio.FundHoldings{}, fmt.Errorf("unsupported iShares fund identity")
	}
	failure := portfolio.FundHoldings{
		Fund: instrument, SourceURL: fund.URL, SourceType: iSharesSourceType,
		RetrievedAt: retrievedAt.UTC(), Confidence: "unavailable",
	}
	key := strings.ToUpper(fund.Symbol)
	s.mu.Lock()
	if cached, ok := s.cache[key]; ok && retrievedAt.Before(cached.value.ExpiresAt) {
		value := cloneFundHoldings(cached.value)
		s.mu.Unlock()
		return value, nil
	}
	s.mu.Unlock()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fund.URL, nil)
	if err != nil {
		return failure, err
	}
	request.Header.Set("Accept", "text/csv")
	response, err := s.client.Do(request)
	if err != nil {
		return failure, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return failure, fmt.Errorf("issuer returned HTTP %d", response.StatusCode)
	}

	value, err := parseISharesCSV(response.Body, instrument, fund.URL, retrievedAt)
	if err != nil {
		return failure, err
	}
	value.ExpiresAt = retrievedAt.UTC().Add(s.cacheTTL)
	s.mu.Lock()
	s.cache[key] = cachedHoldings{value: cloneFundHoldings(value)}
	s.mu.Unlock()
	return value, nil
}

func (s *ISharesSource) lookup(instrument portfolio.Instrument) (iSharesFund, bool) {
	symbol := strings.ToUpper(strings.TrimSpace(instrument.Symbol))
	name := strings.TrimSpace(instrument.Name)
	for _, fund := range s.funds {
		if symbol == fund.Symbol && strings.EqualFold(name, fund.Name) {
			return fund, true
		}
	}
	return iSharesFund{}, false
}

// parseISharesCSV accepts the issuer metadata preamble followed by the documented holdings table.
func parseISharesCSV(reader io.Reader, fund portfolio.Instrument, sourceURL string, retrievedAt time.Time) (portfolio.FundHoldings, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	records, err := csvReader.ReadAll()
	if err != nil {
		return portfolio.FundHoldings{}, fmt.Errorf("decode issuer CSV: %w", err)
	}
	result := portfolio.FundHoldings{
		Fund: fund, Available: true, SourceURL: sourceURL, SourceType: iSharesSourceType,
		RetrievedAt: retrievedAt.UTC(), TopHoldingsOnly: false, Confidence: "high",
		CurrencyAssumptions:       []string{"Holding currencies are issuer-reported; weights are applied to the fund position value in the account base currency."},
		ClassificationAssumptions: []string{"Sector and asset-class labels are used as reported by iShares; missing labels remain unclassified."},
	}

	headerIndex := -1
	for index, record := range records {
		if len(record) >= 2 && strings.EqualFold(strings.TrimSpace(record[0]), "Fund Holdings as of") {
			result.EffectiveDate, err = time.Parse("Jan 02, 2006", strings.TrimSpace(record[1]))
			if err != nil {
				return portfolio.FundHoldings{}, fmt.Errorf("invalid holdings effective date %q: %w", record[1], err)
			}
			result.EffectiveDate = result.EffectiveDate.UTC()
		}
		if len(record) > 0 && strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(record[0], "\ufeff")), "Ticker") {
			headerIndex = index
			break
		}
	}
	if headerIndex < 0 || result.EffectiveDate.IsZero() {
		return portfolio.FundHoldings{}, fmt.Errorf("issuer CSV is missing its holdings header or effective date")
	}

	headers := headerLookup(records[headerIndex])
	required := []string{"ticker", "name", "sector", "asset class", "weight (%)", "currency", "isin"}
	for _, field := range required {
		if _, ok := headers[field]; !ok {
			return portfolio.FundHoldings{}, fmt.Errorf("issuer CSV is missing %q", field)
		}
	}

	coverage := 0.0
	missingClassification := 0
	for _, record := range records[headerIndex+1:] {
		weightText := csvField(record, headers["weight (%)"])
		if weightText == "" || weightText == "-" {
			continue
		}
		weightPercent, parseErr := strconv.ParseFloat(strings.ReplaceAll(weightText, ",", ""), 64)
		if parseErr != nil {
			return portfolio.FundHoldings{}, fmt.Errorf("invalid holding weight %q: %w", weightText, parseErr)
		}
		holding := portfolio.FundHolding{
			SecurityID: csvField(record, headers["isin"]), Symbol: csvField(record, headers["ticker"]),
			Name: csvField(record, headers["name"]), Sector: csvField(record, headers["sector"]),
			AssetClass: csvField(record, headers["asset class"]), Currency: csvField(record, headers["currency"]),
			Weight: weightPercent / 100,
		}
		if holding.Symbol == "" && holding.Name == "" {
			continue
		}
		if holding.Sector == "" || holding.AssetClass == "" {
			missingClassification++
		}
		coverage += weightPercent
		result.Holdings = append(result.Holdings, holding)
	}
	if len(result.Holdings) == 0 {
		return portfolio.FundHoldings{}, fmt.Errorf("issuer CSV contains no weighted holdings")
	}
	result.CoveragePercentage = portfolio.Percentage{Value: coverage, Available: true}
	result.Complete = coverage >= 99 && coverage <= 101 && missingClassification == 0
	if coverage < 99 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Issuer holdings cover %.2f%% of fund weight.", coverage))
	}
	if coverage > 101 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Issuer holding weights total %.2f%% and are inconsistent.", coverage))
	}
	if missingClassification > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%d issuer holdings have missing sector or asset-class classifications.", missingClassification))
	}
	if retrievedAt.UTC().Sub(result.EffectiveDate) > iSharesStaleAfter {
		result.Warnings = append(result.Warnings, "Issuer holdings are older than 14 days.")
		result.Complete = false
		result.Confidence = "limited"
	}
	if result.EffectiveDate.After(retrievedAt.UTC().Add(24 * time.Hour)) {
		result.Warnings = append(result.Warnings, "Issuer holdings effective date is later than the retrieval date.")
		result.Complete = false
		result.Confidence = "limited"
	}
	if !result.Complete && result.Confidence == "high" {
		result.Confidence = "limited"
	}
	return result, nil
}

func headerLookup(record []string) map[string]int {
	lookup := make(map[string]int, len(record))
	for index, value := range record {
		lookup[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "\ufeff")))] = index
	}
	return lookup
}

func csvField(record []string, index int) string {
	if index < 0 || index >= len(record) {
		return ""
	}
	value := strings.TrimSpace(record[index])
	if value == "-" {
		return ""
	}
	return value
}

func cloneFundHoldings(value portfolio.FundHoldings) portfolio.FundHoldings {
	value.Holdings = append([]portfolio.FundHolding(nil), value.Holdings...)
	value.Warnings = append([]string(nil), value.Warnings...)
	value.CurrencyAssumptions = append([]string(nil), value.CurrencyAssumptions...)
	value.ClassificationAssumptions = append([]string(nil), value.ClassificationAssumptions...)
	return value
}
