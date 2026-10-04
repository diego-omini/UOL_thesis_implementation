package ai

// This file maps deterministic current-holding risk metrics into a focused tool-free prompt.

import (
	"context"
	"fmt"

	"wails/positionanalysis"
)

const PositionRiskPromptAlias = "position_risk"

// PositionRisk explains current holding exposure and historical price behavior without order guidance.
func (c *Counselor) PositionRisk(ctx context.Context, result positionanalysis.Result) (Response, error) {
	if result.Instrument.ContractID <= 0 {
		return Response{}, fmt.Errorf("position risk requires a valid current holding")
	}
	facts := positionRiskFacts(result)
	return c.executor.ExecuteCounselor(ctx, PositionRiskPromptAlias, map[string]any{"facts": facts}, nil, facts)
}
