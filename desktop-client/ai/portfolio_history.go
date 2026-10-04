package ai

// This file maps deterministic portfolio-history summaries into a non-return counselor prompt.

import (
	"context"
	"fmt"

	"wails/portfoliohistory"
)

const PortfolioHistoryPromptAlias = "portfolio_history_summary"

// PortfolioHistorySummary explains observed history coverage without presenting value changes as returns.
func (c *Counselor) PortfolioHistorySummary(ctx context.Context, result portfoliohistory.Result) (Response, error) {
	summary := portfoliohistory.Summarize(result)
	if len(summary.Series) == 0 {
		return Response{}, fmt.Errorf("no portfolio history is available in the selected range")
	}
	facts := portfolioHistoryFacts(summary)
	return c.executor.ExecuteCounselor(ctx, PortfolioHistoryPromptAlias, map[string]any{"facts": facts}, nil, facts)
}
