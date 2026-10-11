package ai

// This file maps public position identities into the portfolio-news prompt contract.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wails/analytics"
	"wails/portfolio"
)

const PortfolioNewsPromptAlias = "portfolio_news"

type promptNewsPosition struct {
	Symbol     string
	Name       string
	AssetClass string
}

// PortfolioNews researches the latest material news for up to the 12 largest normalized positions.
func (c *Counselor) PortfolioNews(
	ctx context.Context,
	snapshot portfolio.Portfolio,
	result analytics.Result,
	researchTime time.Time,
) (Response, error) {
	if len(snapshot.Positions) == 0 {
		return Response{}, fmt.Errorf("the portfolio has no open positions to research")
	}
	totalPositions := len(snapshot.Positions)
	selected, _, _ := boundedPortfolioPositions(snapshot, result, maxPortfolioNewsPositions)
	snapshot.Positions = selected
	for index, position := range selected {
		if strings.TrimSpace(position.Instrument.Name) == "" && strings.TrimSpace(position.Instrument.Symbol) == "" {
			return Response{}, fmt.Errorf("portfolio position %d has no public name or ticker to research", index+1)
		}
	}

	facts := portfolioNewsFacts(snapshot, totalPositions, researchTime)
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
