package fundholdings

// This file verifies exact source matching, issuer metadata parsing, freshness, and cache behavior.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"wails/portfolio"
)

func TestParseISharesCSVRetainsSourceDateCoverageAndCurrency(t *testing.T) {
	contents, err := os.Open("testdata/ivv-holdings.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer contents.Close()
	retrievedAt := time.Date(2026, time.September, 28, 10, 0, 0, 0, time.UTC)
	result, err := parseISharesCSV(contents, portfolio.Instrument{Symbol: "IVV", Name: "iShares Core S&P 500 ETF"}, "https://issuer.example/holdings.csv", retrievedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Available || !result.Complete || result.TopHoldingsOnly || result.Confidence != "high" {
		t.Fatalf("unexpected quality result: %#v", result)
	}
	if result.CoveragePercentage.Value != 100 || !result.CoveragePercentage.Available {
		t.Fatalf("unexpected coverage: %#v", result.CoveragePercentage)
	}
	if result.EffectiveDate.Format("2006-01-02") != "2026-09-25" || result.Holdings[0].Currency != "USD" {
		t.Fatalf("source date or currency was not retained: %#v", result)
	}
}

func TestParseISharesCSVMarksPartialStaleAndUnclassifiedDataIncomplete(t *testing.T) {
	csv := "Fund Holdings as of,\"Jan 01, 2026\"\nTicker,Name,Sector,Asset Class,Weight (%),Currency,ISIN\nAAA,Alpha,,Equity,80,USD,US0000000001\n"
	result, err := parseISharesCSV(strings.NewReader(csv), portfolio.Instrument{Symbol: "IVV"}, "https://issuer.example/holdings.csv", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete || result.Confidence != "limited" || len(result.Warnings) != 3 {
		t.Fatalf("partial, stale, unclassified data must be incomplete: %#v", result)
	}
}

func TestISharesSourceUsesExactIdentityAndCachesIssuerDownload(t *testing.T) {
	contents, err := os.ReadFile("testdata/ivv-holdings.csv")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = writer.Write(contents)
	}))
	defer server.Close()
	source := NewISharesSource(server.Client())
	source.funds[0].URL = server.URL
	instrument := portfolio.Instrument{Symbol: "IVV", Name: "iShares Core S&P 500 ETF"}
	if source.Supports(portfolio.Instrument{Symbol: "IVV", Name: "Different Fund"}) {
		t.Fatal("ticker-only matching could resolve the wrong fund")
	}
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	for range 2 {
		if _, err := source.Fetch(context.Background(), instrument, now); err != nil {
			t.Fatal(err)
		}
	}
	if requests != 1 {
		t.Fatalf("expected one issuer request, got %d", requests)
	}
}

func TestServiceReportsUnavailableSource(t *testing.T) {
	instruments := []portfolio.Instrument{{Symbol: "BALF", Name: "Balanced Market Fund", AssetClass: "FUND"}}
	results := NewService().ResolveInstruments(context.Background(), instruments, time.Now())
	if len(results) != 1 || results[0].Available || results[0].SourceType != "unsupported" || len(results[0].Warnings) != 1 {
		t.Fatalf("unexpected unavailable result: %#v", results)
	}
}

func TestServiceRetainsConfiguredSourceProvenanceWhenDownloadFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	source := NewISharesSource(server.Client())
	source.funds[0].URL = server.URL + "/official.csv"
	instrument := portfolio.Instrument{Symbol: "IVV", Name: "iShares Core S&P 500 ETF"}
	result := NewService(source).ResolveInstruments(context.Background(), []portfolio.Instrument{instrument}, time.Now())[0]
	if result.Available || result.SourceURL != source.funds[0].URL || result.SourceType != iSharesSourceType || result.Confidence != "unavailable" {
		t.Fatalf("failed source provenance was lost: %#v", result)
	}
}
