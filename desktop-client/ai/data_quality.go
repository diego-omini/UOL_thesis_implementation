package ai

// This file maps deterministic availability and provenance checks into a data-quality audit prompt.

import (
	"context"
	"fmt"

	"wails/analytics"
	"wails/portfolio"
)

const DataQualityPromptAlias = "portfolio_data_quality"

// PortfolioDataQuality audits normalized portfolio coverage without research tools.
func (c *Counselor) PortfolioDataQuality(ctx context.Context, snapshot portfolio.Portfolio, result analytics.Result) (Response, error) {
	if len(snapshot.Positions) == 0 {
		return Response{}, fmt.Errorf("the portfolio has no open positions to audit")
	}
	facts := dataQualityFacts(snapshot, result)
	return c.executor.ExecuteCounselor(ctx, DataQualityPromptAlias, map[string]any{"facts": facts}, nil, facts)
}
