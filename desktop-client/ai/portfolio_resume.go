package ai

// This file maps normalized portfolio data into the portfolio-resume prompt contract.

import (
	"context"
	"fmt"
	"strings"

	"wails/analytics"
	"wails/portfolio"
)

const PortfolioResumePromptAlias = "portfolio_resume"

// ResearchToolFactory creates privacy-constrained tools for one portfolio execution.
type ResearchToolFactory interface {
	ForInstruments(instruments []PublicInstrument) []Tool
}

// Counselor prepares portfolio prompt parameters and prompt-scoped research tools.
type Counselor struct {
	executor        *Executor
	researchFactory ResearchToolFactory
}

// NewCounselor creates a read-only portfolio counselor backed by a prompt executor.
func NewCounselor(executor *Executor, researchFactory ResearchToolFactory) *Counselor {
	return &Counselor{executor: executor, researchFactory: researchFactory}
}

// PortfolioResume executes the cataloged portfolio analysis with normalized deterministic context.
func (c *Counselor) PortfolioResume(
	ctx context.Context,
	snapshot portfolio.Portfolio,
	result analytics.Result,
) (Response, error) {
	if len(snapshot.Positions) == 0 {
		return Response{}, fmt.Errorf("the portfolio has no open positions to summarize")
	}

	facts := portfolioResumeFacts(snapshot, result)
	parameters := map[string]any{"facts": facts}

	var tools []Tool
	if c.researchFactory != nil {
		positions, _, _ := boundedPortfolioPositions(snapshot, result, maxPortfolioResumePositions)
		tools = c.researchFactory.ForInstruments(toPublicInstruments(positions))
	}
	return c.executor.ExecuteCounselor(ctx, PortfolioResumePromptAlias, parameters, tools, facts)
}

func toPublicInstruments(positions []portfolio.Position) []PublicInstrument {
	instruments := make([]PublicInstrument, 0, len(positions))
	for _, position := range positions {
		instruments = append(instruments, PublicInstrument{
			Name:   strings.TrimSpace(position.Instrument.Name),
			Ticker: strings.TrimSpace(position.Instrument.Symbol),
		})
	}
	return instruments
}

func valueOrUnavailable(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unavailable"
	}
	return strings.TrimSpace(value)
}
