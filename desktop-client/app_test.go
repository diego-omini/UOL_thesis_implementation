package main

// This file verifies application-level gateway URL mapping used by browser authentication.

import "testing"

func TestGatewayLoginURL(t *testing.T) {
	got, err := gatewayLoginURL("https://localhost:5000/v1/api")
	if err != nil {
		t.Fatalf("gatewayLoginURL() returned an error: %v", err)
	}
	if got != "https://localhost:5000/" {
		t.Fatalf("gatewayLoginURL() = %q", got)
	}
}
