package snaptradehistory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"wails/portfoliohistory"
)

func TestFetchPortfolioNAVHistorySelectsActiveIBKRPaperAccountAndMapsSortedHistory(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Signature") == "" || request.URL.Query().Get("clientId") != "client" {
			t.Fatalf("request was not signed")
		}
		switch request.URL.Path {
		case "/accounts":
			_ = json.NewEncoder(response).Encode([]account{
				{ID: "live", InstitutionName: "Interactive Brokers", Number: "U1", IsPaper: false},
				{ID: "paper", InstitutionName: "Interactive Brokers", Number: "DU-1", IsPaper: true},
			})
		case "/accounts/paper/balanceHistory":
			_ = json.NewEncoder(response).Encode(balanceHistoryResponse{Currency: "gbp", History: []balanceHistoryItem{
				{Date: "2026-10-03T00:00:00Z", TotalValue: "1020.50"},
				{Date: "2026-10-01T00:00:00Z", TotalValue: "1000"},
				{Date: "2026-10-03T12:00:00Z", TotalValue: "1021.50"},
			}})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := NewClient("client", "secret", server.Client())
	client.baseURL = server.URL
	client.now = func() time.Time { return now }
	history, err := client.FetchPortfolioNAVHistory(context.Background(), "DU-1")
	if err != nil {
		t.Fatal(err)
	}
	if history.AccountID != "DU-1" || history.RetrievedAt != now || len(history.Points) != 2 {
		t.Fatalf("unexpected history: %+v", history)
	}
	if history.Points[0].Date.Format("2006-01-02") != "2026-10-01" || history.Points[1].Value != 1021.5 {
		t.Fatalf("history was not sorted and same-day duplicates were not replaced: %+v", history.Points)
	}
	if history.Points[1].BaseCurrency != "GBP" || history.Points[1].Source != portfoliohistory.SourceSnapTradeBalanceHistory {
		t.Fatalf("history metadata was not mapped: %+v", history.Points[1])
	}
}

func TestSelectIBKRPaperAccountRejectsAmbiguousAccounts(t *testing.T) {
	_, err := selectIBKRPaperAccount([]account{
		{ID: "one", InstitutionName: "Interactive Brokers", Number: "masked", IsPaper: true},
		{ID: "two", InstitutionName: "Interactive Brokers", Number: "masked", IsPaper: true},
	}, "DU-1")
	if err == nil {
		t.Fatal("expected ambiguous account selection to fail")
	}
}

func TestSelectIBKRPaperAccountRejectsSoleExplicitMismatch(t *testing.T) {
	_, err := selectIBKRPaperAccount([]account{
		{ID: "one", InstitutionName: "Interactive Brokers", Number: "DU-OTHER", IsPaper: true},
	}, "DU-1")
	if err == nil {
		t.Fatal("expected a sole explicit mismatch to fail")
	}
}

func TestSelectIBKRPaperAccountRejectsDuplicateExactMatches(t *testing.T) {
	_, err := selectIBKRPaperAccount([]account{
		{ID: "one", InstitutionName: "Interactive Brokers", Number: "DU-1", IsPaper: true},
		{ID: "two", InstitutionName: "Interactive Brokers", Number: "DU-1", IsPaper: true},
	}, "DU-1")
	if err == nil {
		t.Fatal("expected duplicate exact matches to fail")
	}
}

func TestConnectionPortalURLRequestsReadOnlyAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["connectionType"] != "read" {
			t.Fatalf("connection type = %q", body["connectionType"])
		}
		_ = json.NewEncoder(response).Encode(map[string]string{"redirectURI": "https://connect.snaptrade.com/session"})
	}))
	defer server.Close()
	client := NewClient("client", "secret", server.Client())
	client.baseURL = server.URL
	got, err := client.ConnectionPortalURL(context.Background())
	if err != nil || got != "https://connect.snaptrade.com/session" {
		t.Fatalf("ConnectionPortalURL() = %q, %v", got, err)
	}
}
