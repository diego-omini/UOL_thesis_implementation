package ibkr

import (
	"encoding/json"
	"testing"
)

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
		"unrealizedPnl": 488.57,
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
