package ai

// This file maps normalized portfolio data into the portfolio-resume prompt contract.

import (
	"context"
	"fmt"
	"strings"
	"time"

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

type promptPosition struct {
	Symbol          string
	Name            string
	AssetClass      string
	Sector          string
	Currency        string
	Quantity        float64
	PortfolioWeight string
}

type promptAllocation struct {
	Label  string
	Weight string
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

	parameters := map[string]any{
		"analysis_timestamp":      result.CalculatedAt.UTC().Format(time.RFC3339),
		"base_currency":           valueOrUnavailable(result.BaseCurrency),
		"positions":               toPromptPositions(snapshot, result),
		"sector_allocations":      toPromptAllocations(result.SectorAllocation),
		"asset_class_allocations": toPromptAllocations(result.AssetClassAllocation),
		"currency_allocations":    toPromptAllocations(result.CurrencyAllocation),
		"top_three_concentration": formatRatio(result.TopThreeConcentration),
		"hhi":                     formatRatio(result.HHI),
		"data_warnings":           warningMessages(result.Warnings),
	}

	var tools []Tool
	if c.researchFactory != nil {
		tools = c.researchFactory.ForInstruments(toPublicInstruments(snapshot.Positions))
	}
	return c.executor.Execute(ctx, PortfolioResumePromptAlias, parameters, tools)
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

func toPromptPositions(snapshot portfolio.Portfolio, result analytics.Result) []promptPosition {
	positions := make([]promptPosition, 0, len(snapshot.Positions))
	for index, position := range snapshot.Positions {
		weight := "unavailable"
		if index < len(result.PositionWeights) {
			weight = formatRatio(result.PositionWeights[index].Weight)
		}
		positions = append(positions, promptPosition{
			Symbol:          valueOrUnavailable(position.Instrument.Symbol),
			Name:            valueOrUnavailable(position.Instrument.Name),
			AssetClass:      valueOrUnavailable(position.Instrument.AssetClass),
			Sector:          valueOrUnavailable(position.Instrument.Sector),
			Currency:        valueOrUnavailable(position.Instrument.Currency),
			Quantity:        position.Quantity,
			PortfolioWeight: weight,
		})
	}
	return positions
}

func toPromptAllocations(allocations []analytics.Allocation) []promptAllocation {
	values := make([]promptAllocation, 0, len(allocations))
	for _, allocation := range allocations {
		values = append(values, promptAllocation{
			Label:  valueOrUnavailable(allocation.Label),
			Weight: formatRatio(allocation.Weight),
		})
	}
	return values
}

func warningMessages(warnings []analytics.DataWarning) []string {
	messages := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		message := warning.Message
		if warning.Count > 1 {
			message = fmt.Sprintf("%s (%d affected positions)", message, warning.Count)
		}
		messages = append(messages, message)
	}
	return messages
}

func formatRatio(ratio analytics.Ratio) string {
	if !ratio.Available {
		return "unavailable"
	}
	return fmt.Sprintf("%.1f%%", ratio.Value*100)
}

func valueOrUnavailable(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unavailable"
	}
	return strings.TrimSpace(value)
}
