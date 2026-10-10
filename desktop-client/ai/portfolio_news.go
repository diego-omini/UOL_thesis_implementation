package ai

// This file maps public position identities into the portfolio-news prompt contract.

import (
	"context"
	"fmt"
	"time"

	"wails/portfolio"
)

const PortfolioNewsPromptAlias = "portfolio_news"

type promptNewsPosition struct {
	Symbol     string
	Name       string
	AssetClass string
}

// PortfolioNews researches the latest material news for every normalized position.
func (c *Counselor) PortfolioNews(
	ctx context.Context,
	snapshot portfolio.Portfolio,
	researchTime time.Time,
) (Response, error) {
	if len(snapshot.Positions) == 0 {
		return Response{}, fmt.Errorf("the portfolio has no open positions to research")
	}

	facts := portfolioNewsFacts(snapshot, researchTime)
	parameters := map[string]any{"facts": facts}

	var tools []Tool
	instruments := toPublicInstruments(snapshot.Positions)
	if c.researchFactory != nil {
		tools = c.researchFactory.ForInstruments(instruments)
	}
	return c.executor.ExecuteCounselorWithRequiredTools(
		ctx, PortfolioNewsPromptAlias, parameters, tools, facts,
		requiredInstrumentSearchCalls(instruments, "latest_news"),
	)
}
