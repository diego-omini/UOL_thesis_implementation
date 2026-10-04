package ai

// This file maps modeled portfolio-risk results into a focused counselor prompt.

import (
	"context"
	"fmt"

	"wails/portfolioanalysis"
)

const PortfolioRiskPromptAlias = "portfolio_risk"

// PortfolioRisk explains deterministic portfolio-risk results without recalculating them.
func (c *Counselor) PortfolioRisk(ctx context.Context, result portfolioanalysis.Result) (Response, error) {
	if result.Observations < 20 || result.IncludedPositionCount == 0 {
		return Response{}, fmt.Errorf("portfolio risk counseling requires at least 20 common return observations")
	}
	facts := portfolioRiskFacts(result)
	return c.executor.ExecuteCounselor(ctx, PortfolioRiskPromptAlias, map[string]any{"facts": facts}, nil, facts)
}
