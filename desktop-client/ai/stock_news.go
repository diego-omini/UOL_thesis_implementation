package ai

// This file maps one public stock identity into the stock-news prompt contract.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wails/portfolio"
)

const StockNewsPromptAlias = "stock_news"

// StockNews researches recent material developments for one stock.
func (c *Counselor) StockNews(
	ctx context.Context,
	instrument portfolio.Instrument,
	researchTime time.Time,
) (Response, error) {
	publicInstrument := PublicInstrument{
		Name:   strings.TrimSpace(instrument.Name),
		Ticker: strings.TrimSpace(instrument.Symbol),
	}
	if publicInstrument.Name == "" && publicInstrument.Ticker == "" {
		return Response{}, fmt.Errorf("the stock has no public name or ticker to research")
	}

	facts := stockNewsFacts(instrument, researchTime)
	parameters := map[string]any{"facts": facts}

	var tools []Tool
	if c.researchFactory != nil {
		tools = c.researchFactory.ForInstruments([]PublicInstrument{publicInstrument})
	}
	requiredCalls := requiredInstrumentSearchCalls(
		[]PublicInstrument{publicInstrument}, "latest_news", "recent_events", "regulatory_filings",
	)
	return c.executor.ExecuteCounselorWithRequiredTools(ctx, StockNewsPromptAlias, parameters, tools, facts, requiredCalls)
}
