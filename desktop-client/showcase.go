package main

// showcase.go defines the controlled portfolio state shared by tagged showcase builds.

import (
	"time"

	"wails/broker"
	"wails/portfolio"
)

type showcaseScenario struct {
	ID          string
	Description string
	SnapshotAt  time.Time
	Portfolio   portfolio.Portfolio
}

func showcaseSessionStatus() broker.SessionStatus {
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
