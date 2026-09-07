package ibkr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchPortfolioDoesNotRequireTradingSession(t *testing.T) {
	requestedPaths := make([]string, 0, 4)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/portfolio/accounts":
			_, _ = w.Write([]byte(`[{"accountId":"DU123456","currency":"CHF"}]`))
		case "/portfolio/DU123456/summary":
			_, _ = w.Write([]byte(`{
				"netliquidation":{"amount":12500,"currency":"CHF"},
				"totalcashvalue":{"amount":2500,"currency":"CHF"}
			}`))
		case "/portfolio/DU123456/ledger":
			_, _ = w.Write([]byte(`{
				"CHF":{"currency":"CHF","exchangerate":1,"timestamp":1788768000},
				"USD":{"currency":"USD","exchangerate":0.875,"timestamp":1788768000}
			}`))
		case "/portfolio/DU123456/positions/0":
			_, _ = w.Write([]byte(`[{
				"acctId":"DU123456",
				"conid":8314,
				"ticker":"IBM",
				"currency":"USD",
				"position":5,
				"mktValue":1000,
				"baseMktValue":875,
				"pageSize":100
			}]`))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, false)
	result, err := client.FetchPortfolio(context.Background())
	if err != nil {
		t.Fatalf("FetchPortfolio() returned an error: %v", err)
	}

	wantPaths := []string{
		"/portfolio/accounts",
		"/portfolio/DU123456/summary",
		"/portfolio/DU123456/ledger",
		"/portfolio/DU123456/positions/0",
	}
	if len(requestedPaths) != len(wantPaths) {
		t.Fatalf("requested paths = %v, want %v", requestedPaths, wantPaths)
	}
	for index := range wantPaths {
		if requestedPaths[index] != wantPaths[index] {
			t.Fatalf("requested paths = %v, want %v", requestedPaths, wantPaths)
		}
	}

	if result.AccountSummary.AccountID != "DU123456" {
		t.Fatalf("unexpected account ID: %s", result.AccountSummary.AccountID)
	}
	if len(result.Positions) != 1 || result.Positions[0].Instrument.Symbol != "IBM" {
		t.Fatalf("unexpected positions: %#v", result.Positions)
	}
	if len(result.ExchangeRates) != 2 {
		t.Fatalf("unexpected exchange rates: %#v", result.ExchangeRates)
	}
	usdRate := result.ExchangeRates[1]
	if usdRate.Currency != "USD" || usdRate.BaseCurrency != "CHF" || usdRate.BaseAmountPerUnit != 0.875 || !usdRate.Available {
		t.Fatalf("unexpected USD exchange rate: %#v", usdRate)
	}
	if usdRate.Source != ledgerRateSource || usdRate.AsOf.Unix() != 1788768000 {
		t.Fatalf("unexpected exchange-rate metadata: %#v", usdRate)
	}
}

func TestLedgerTimeUsesRetrievalTimeWhenTimestampIsMissing(t *testing.T) {
	fallback := time.Date(2026, time.September, 7, 12, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	if got := ledgerTime(0, fallback); !got.Equal(fallback.UTC()) {
		t.Fatalf("ledgerTime() = %s, want %s", got, fallback.UTC())
	}
}

func TestPositionResponseMapping(t *testing.T) {
	const responseJSON = `{
		"acctId": "DU123456",
		"conid": 8314,
		"ticker": "IBM",
		"contractDesc": "IBM",
		"name": "INTL BUSINESS MACHINES CORP",
		"assetClass": "STK",
		"listingExchange": "NYSE",
		"currency": "USD",
		"sector": "Technology",
		"sectorGroup": "Computer Services",
		"position": "12",
		"avgCost": 144.46141,
		"mktPrice": 185.17599485,
		"mktValue": 2222.11,
		"baseMktValue": 1950.42,
		"unrealizedPnl": 488.57,
		"baseUnrealizedPnl": 428.84,
		"pageSize": 100
	}`

	var response positionResponse
	if err := json.Unmarshal([]byte(responseJSON), &response); err != nil {
		t.Fatalf("failed to decode position response: %v", err)
	}

	position := mapPosition(response, "CHF")
	if position.Instrument.ContractID != 8314 {
		t.Fatalf("unexpected contract ID: %d", position.Instrument.ContractID)
	}
	if position.Instrument.Symbol != "IBM" {
		t.Fatalf("unexpected symbol: %s", position.Instrument.Symbol)
	}
	if position.Instrument.Name != "INTL BUSINESS MACHINES CORP" {
		t.Fatalf("unexpected name: %s", position.Instrument.Name)
	}
	if position.Instrument.Sector != "Technology" || position.Instrument.Industry != "Computer Services" {
		t.Fatalf("unexpected classification: %s / %s", position.Instrument.Sector, position.Instrument.Industry)
	}
	if position.Quantity != 12 {
		t.Fatalf("unexpected quantity: %f", position.Quantity)
	}
	if position.MarketValue.Currency != "USD" {
		t.Fatalf("unexpected currency: %s", position.MarketValue.Currency)
	}
	if !position.MarketValue.Available || position.MarketValue.Amount != 2222.11 {
		t.Fatalf("unexpected market value: %#v", position.MarketValue)
	}
	if !position.BaseMarketValue.Available || position.BaseMarketValue.Amount != 1950.42 || position.BaseMarketValue.Currency != "CHF" {
		t.Fatalf("unexpected base market value: %#v", position.BaseMarketValue)
	}
	if !position.BaseUnrealizedPnL.Available || position.BaseUnrealizedPnL.Amount != 428.84 {
		t.Fatalf("unexpected base unrealized profit and loss: %#v", position.BaseUnrealizedPnL)
	}
}

func TestHasMorePositionPages(t *testing.T) {
	tests := []struct {
		name      string
		responses []positionResponse
		want      bool
	}{
		{name: "empty page", responses: nil, want: false},
		{name: "partial page", responses: []positionResponse{{PageSize: 2}}, want: false},
		{name: "full page", responses: []positionResponse{{PageSize: 2}, {PageSize: 2}}, want: true},
		{name: "unknown page size", responses: []positionResponse{{}}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := hasMorePositionPages(test.responses); got != test.want {
				t.Fatalf("hasMorePositionPages() = %t, want %t", got, test.want)
			}
		})
	}
}
