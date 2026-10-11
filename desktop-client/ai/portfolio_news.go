package ai

// This file maps public position identities into the portfolio-news prompt contract.

import (
	"context"
	"fmt"
	"strings"
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
	if len(snapshot.Positions) > maxPortfolioNewsPositions {
		return Response{}, fmt.Errorf("portfolio news supports at most %d open positions so every position can receive bounded research coverage", maxPortfolioNewsPositions)
	}
	for index, position := range snapshot.Positions {
		if strings.TrimSpace(position.Instrument.Name) == "" && strings.TrimSpace(position.Instrument.Symbol) == "" {
			return Response{}, fmt.Errorf("portfolio position %d has no public name or ticker to research", index+1)
		}
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
