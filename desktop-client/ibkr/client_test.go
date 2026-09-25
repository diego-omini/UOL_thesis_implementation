package ibkr

// client_test.go verifies IBKR response decoding, mapping, pagination, and exchange-rate behavior.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"wails/broker"
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

func TestSearchStocksFiltersNonStocksAndOrdersExactTickerFirst(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/iserver/secdef/search" || r.URL.Query().Get("symbol") != "AA" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"conid":"2","symbol":"AAPL","companyName":"Apple Inc.","description":"NASDAQ","sections":[{"secType":"STK"}]},
			{"conid":3,"symbol":"AA","companyName":"Alcoa Corporation","description":"NYSE","sections":[{"secType":"STK"}]},
			{"conid":"4","symbol":"AA","companyName":"Alcoa option","description":"CBOE","sections":[{"secType":"OPT"}]}
		]`))
	}))
	defer server.Close()

	results, err := NewClient(server.URL, false).SearchStocks(context.Background(), " aa ")
	if err != nil {
		t.Fatalf("SearchStocks() returned an error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("SearchStocks() returned %d results, want 2: %#v", len(results), results)
	}
	if results[0].Symbol != "AA" || results[0].ContractID != 3 {
		t.Fatalf("first result = %#v, want exact AA stock", results[0])
	}
	if results[1].Symbol != "AAPL" || results[1].AssetClass != "STK" {
		t.Fatalf("second result = %#v, want AAPL stock", results[1])
	}
}

func TestSearchStocksIncludesCompanyNameMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/iserver/secdef/search" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("name") == "true" {
			_, _ = w.Write([]byte(`[{"conid":"265598","symbol":"AAPL","companyName":"APPLE INC (NASDAQ)","companyHeader":"AAPL APPLE INC (NASDAQ)","secType":"STK","sections":[]}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	results, err := NewClient(server.URL, false).SearchStocks(context.Background(), "Apple")
	if err != nil {
		t.Fatalf("SearchStocks() returned an error: %v", err)
	}
	if len(results) != 1 || results[0].Symbol != "AAPL" || results[0].Name != "APPLE INC" || results[0].Exchange != "NASDAQ" {
		t.Fatalf("unexpected company-name search results: %#v", results)
	}
}

func TestFetchStockSnapshotMapsQuoteFieldsAfterPreflight(t *testing.T) {
	snapshotCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/iserver/contract/265598/info":
			_, _ = w.Write([]byte(`{"con_id":265598,"symbol":"AAPL","instrument_type":"STK","company_name":"APPLE INC","currency":"USD","exchange":"SMART","listing_exchange":"NASDAQ"}`))
		case "/iserver/marketdata/snapshot":
			snapshotCalls++
			if r.URL.Query().Get("conids") != "265598" {
				http.Error(w, "unexpected conid", http.StatusBadRequest)
				return
			}
			if snapshotCalls == 1 {
				_, _ = w.Write([]byte(`[null]`))
				return
			}
			_, _ = w.Write([]byte(`[{"84":"224.10","86":"224.14","6509":"DPB","7293":"237.49","7294":"164.08","7295":"223.50","7741":"222.91","7762":"45,120,100"}]`))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	snapshot, err := NewClient(server.URL, false).FetchStockSnapshot(context.Background(), 265598)
	if err != nil {
		t.Fatalf("FetchStockSnapshot() returned an error: %v", err)
	}
	if snapshotCalls != 2 || snapshot.Instrument.Symbol != "AAPL" || snapshot.Currency != "USD" {
		t.Fatalf("unexpected snapshot identity or calls: calls=%d snapshot=%#v", snapshotCalls, snapshot)
	}
	if !snapshot.BidPrice.Available || snapshot.BidPrice.Value != 224.10 || snapshot.Volume.Value != 45120100 {
		t.Fatalf("unexpected quote mapping: %#v", snapshot)
	}
}

func TestFetchStockSnapshotMergesValuesFromSuccessiveResponses(t *testing.T) {
	snapshotCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/iserver/contract/265598/info":
			_, _ = w.Write([]byte(`{"con_id":265598,"symbol":"AAPL","currency":"USD"}`))
		case "/iserver/marketdata/snapshot":
			snapshotCalls++
			responses := []string{
				`[{"6509":"DPB","84":"224.10","86":"224.14"}]`,
				`[{"7293":"237.49","7294":"164.08","7295":"223.50"}]`,
				`[{"7741":"222.91","7762":"45,120,100"}]`,
			}
			index := snapshotCalls - 1
			if index >= len(responses) {
				index = len(responses) - 1
			}
			_, _ = w.Write([]byte(responses[index]))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	snapshot, err := NewClient(server.URL, false).FetchStockSnapshot(context.Background(), 265598)
	if err != nil {
		t.Fatalf("FetchStockSnapshot() returned an error: %v", err)
	}
	if snapshotCalls != 3 || !snapshot.BidPrice.Available || !snapshot.High52Week.Available || !snapshot.Volume.Available {
		t.Fatalf("snapshot responses were not merged: calls=%d snapshot=%#v", snapshotCalls, snapshot)
	}
}

func TestFetchStockHistoryMapsRangeAndBars(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/iserver/marketdata/history" || r.URL.Query().Get("period") != "1y" || r.URL.Query().Get("bar") != "1d" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"mdAvailability":"D","mktDataDelay":900,"data":[{"o":100,"h":105,"l":99,"c":104,"v":1200,"t":1788768000000}]}`))
	}))
	defer server.Close()

	history, err := NewClient(server.URL, false).FetchStockHistory(context.Background(), 265598, "1y")
	if err != nil {
		t.Fatalf("FetchStockHistory() returned an error: %v", err)
	}
	if len(history.Points) != 1 || history.Points[0].Close != 104 || history.DelaySeconds != 900 {
		t.Fatalf("unexpected history: %#v", history)
	}
}

func TestCheckSessionAcceptsOnlyPaperTrading(t *testing.T) {
	tests := []struct {
		name          string
		ssoResponse   string
		authResponse  string
		paperResponse string
		wantState     broker.SessionState
		wantReady     bool
	}{
		{
			name:          "authenticated paper session",
			ssoResponse:   `{"RESULT":true,"LOGIN_TYPE":1}`,
			authResponse:  `{"authenticated":true,"connected":true,"competing":false}`,
			paperResponse: `{"isPaper":true}`,
			wantState:     broker.SessionAuthenticated,
			wantReady:     true,
		},
		{
			name:          "live session",
			ssoResponse:   `{"RESULT":true,"LOGIN_TYPE":2}`,
			authResponse:  `{"authenticated":true,"connected":true,"competing":false}`,
			paperResponse: `{"isPaper":false}`,
			wantState:     broker.SessionLiveAccount,
		},
		{
			name:        "authentication required",
			ssoResponse: `{"RESULT":false}`,
			wantState:   broker.SessionAuthenticationRequired,
		},
		{
			name:         "competing session",
			ssoResponse:  `{"RESULT":true,"LOGIN_TYPE":2}`,
			authResponse: `{"authenticated":false,"connected":true,"competing":true}`,
			wantState:    broker.SessionCompeting,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/sso/validate":
					_, _ = w.Write([]byte(test.ssoResponse))
				case "/iserver/auth/status":
					if r.Method != http.MethodPost {
						http.Error(w, "authentication status must use POST", http.StatusMethodNotAllowed)
						return
					}
					_, _ = w.Write([]byte(test.authResponse))
				case "/iserver/accounts":
					_, _ = w.Write([]byte(test.paperResponse))
				default:
					http.Error(w, "unexpected request", http.StatusNotFound)
				}
			}))
			defer server.Close()

			status := NewClient(server.URL, false).CheckSession(context.Background())
			if status.State != test.wantState {
				t.Fatalf("state = %q, want %q (%s)", status.State, test.wantState, status.Message)
			}
			if status.Ready() != test.wantReady {
				t.Fatalf("Ready() = %t, want %t", status.Ready(), test.wantReady)
			}
		})
	}
}

func TestCheckSessionInitializesBrokerageSession(t *testing.T) {
	statusCalls := 0
	initCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/sso/validate":
			_, _ = w.Write([]byte(`{"RESULT":true,"LOGIN_TYPE":2}`))
		case "/iserver/auth/status":
			statusCalls++
			if statusCalls == 1 {
				_, _ = w.Write([]byte(`{"authenticated":false,"connected":true,"competing":false}`))
				return
			}
			_, _ = w.Write([]byte(`{"authenticated":true,"connected":true,"competing":false,"established":true}`))
		case "/iserver/auth/ssodh/init":
			initCalls++
			var request brokerageSessionInitRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if !request.Publish || request.Compete {
				http.Error(w, "unexpected brokerage initialization options", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"authenticated":false,"connected":true,"competing":false,"established":false}`))
		case "/iserver/accounts":
			_, _ = w.Write([]byte(`{"isPaper":true}`))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	status := NewClient(server.URL, false).CheckSession(context.Background())
	if !status.Ready() {
		t.Fatalf("session was not ready after initialization: %#v", status)
	}
	if initCalls != 1 {
		t.Fatalf("brokerage initialization calls = %d, want 1", initCalls)
	}
	if statusCalls < 2 {
		t.Fatalf("authentication status calls = %d, want at least 2", statusCalls)
	}
}

func TestCheckSessionUsesCompeteOnlyWhenGatewayRequiresIt(t *testing.T) {
	initCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/sso/validate":
			_, _ = w.Write([]byte(`{"RESULT":true,"LOGIN_TYPE":1}`))
		case "/iserver/auth/status":
			_, _ = w.Write([]byte(`{
				"authenticated":false,
				"connected":true,
				"established":false,
				"competing":false,
				"fail":"Force compete capability must be used together with compete flag"
			}`))
		case "/iserver/auth/ssodh/init":
			initCalls++
			var request brokerageSessionInitRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if !request.Publish || !request.Compete {
				http.Error(w, "gateway-required compete flag was not supplied", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{
				"authenticated":true,
				"connected":true,
				"established":true,
				"competing":false
			}`))
		case "/iserver/accounts":
			_, _ = w.Write([]byte(`{"isPaper":true}`))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	status := NewClient(server.URL, false).CheckSession(context.Background())
	if !status.Ready() {
		t.Fatalf("session was not ready after forced-competition initialization: %#v", status)
	}
	if initCalls != 1 {
		t.Fatalf("brokerage initialization calls = %d, want 1", initCalls)
	}
}

func TestLogoutPostsToGateway(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/logout" || r.Method != http.MethodPost {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":true}`))
	}))
	defer server.Close()

	if err := NewClient(server.URL, false).Logout(context.Background()); err != nil {
		t.Fatalf("Logout() returned an error: %v", err)
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
