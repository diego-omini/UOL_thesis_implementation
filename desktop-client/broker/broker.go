// broker package has been introduced to allow broker-indipendent implementation in case we decide to move on from ibkr
// or to support multiple brokers.

package broker

import (
	"context"
	"wails/portfolio"
)

type ConnectionStatus struct {
	Connected bool
	Message   string
}

type PortfolioProvider interface {
	CheckConnection(ctx context.Context) ConnectionStatus
	FetchPortfolio(ctx context.Context) (portfolio.Portfolio, error)
}
