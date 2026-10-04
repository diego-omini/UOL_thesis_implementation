package ai

// This file maps deterministic direct and look-through exposure into a diversification prompt.

import (
	"context"
	"fmt"

	"wails/analytics"
	"wails/portfolio"
)

const DiversificationPromptAlias = "effective_diversification"

// EffectiveDiversification explains direct and look-through concentration without inventing classifications.
func (c *Counselor) EffectiveDiversification(ctx context.Context, snapshot portfolio.Portfolio, result analytics.Result) (Response, error) {
	if len(snapshot.Positions) == 0 {
		return Response{}, fmt.Errorf("the portfolio has no open positions to analyze")
	}
	facts := diversificationFacts(snapshot, result)
	return c.executor.ExecuteCounselor(ctx, DiversificationPromptAlias, map[string]any{"facts": facts}, nil, facts)
}
