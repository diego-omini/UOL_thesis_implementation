package ai

// This file maps deterministic currency allocations and conversion provenance into a focused prompt.

import (
	"context"
	"fmt"

	"wails/analytics"
	"wails/portfolio"
)

const CurrencyExposurePromptAlias = "currency_exposure"

// CurrencyExposure explains current currency concentration and conversion limitations without research tools.
func (c *Counselor) CurrencyExposure(ctx context.Context, snapshot portfolio.Portfolio, result analytics.Result) (Response, error) {
	if len(snapshot.Positions) == 0 {
		return Response{}, fmt.Errorf("the portfolio has no open positions to analyze")
	}
	facts := currencyExposureFacts(snapshot, result)
	return c.executor.ExecuteCounselor(ctx, CurrencyExposurePromptAlias, map[string]any{"facts": facts}, nil, facts)
}
