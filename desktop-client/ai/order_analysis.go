package ai

// This file maps deterministic stock and proposed-order metrics into the order-analysis prompt.

import (
	"context"
	"fmt"
	"strings"

	"wails/stockanalysis"
)

const OrderAnalysisPromptAlias = "order_analysis"

// OrderAnalysis explains deterministic market and portfolio metrics with bounded public research.
func (c *Counselor) OrderAnalysis(ctx context.Context, result stockanalysis.Result) (Response, error) {
	publicInstrument := PublicInstrument{
		Name: strings.TrimSpace(result.Instrument.Name), Ticker: strings.TrimSpace(result.Instrument.Symbol),
	}
	if publicInstrument.Name == "" && publicInstrument.Ticker == "" {
		return Response{}, fmt.Errorf("the stock has no public name or ticker to research")
	}
	facts := orderAnalysisFacts(result)
	parameters := map[string]any{"facts": facts}
	var tools []Tool
	if c.researchFactory != nil {
		tools = c.researchFactory.ForInstruments([]PublicInstrument{publicInstrument})
	}
	return c.executor.ExecuteCounselorWithRequiredTools(
		ctx, OrderAnalysisPromptAlias, parameters, tools, facts,
		requiredInstrumentSearchCalls(
			[]PublicInstrument{publicInstrument},
			"valuation", "financial_condition", "company_risk", "regulatory_filings", "latest_news",
		),
	)
}
