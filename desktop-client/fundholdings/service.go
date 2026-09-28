// Package fundholdings retrieves and normalizes holdings from explicitly supported issuer sources.
package fundholdings

// This file resolves fund positions through source-specific adapters without exposing portfolio data.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wails/portfolio"
)

// Source resolves only fund identities whose structured issuer format it explicitly supports.
type Source interface {
	Supports(portfolio.Instrument) bool
	Fetch(context.Context, portfolio.Instrument, time.Time) (portfolio.FundHoldings, error)
}

// Service coordinates the bounded set of supported issuer adapters.
type Service struct {
	sources []Source
}

// NewService creates a holdings service from source-specific adapters.
func NewService(sources ...Source) *Service {
	return &Service{sources: append([]Source(nil), sources...)}
}

// ResolveInstruments resolves recognized funds and reports unsupported fund instruments explicitly.
func (s *Service) ResolveInstruments(ctx context.Context, instruments []portfolio.Instrument, retrievedAt time.Time) []portfolio.FundHoldings {
	results := make([]portfolio.FundHoldings, 0)
	for _, instrument := range instruments {
		source := s.sourceFor(instrument)
		if source == nil && !IsFund(instrument) {
			continue
		}
		if source == nil {
			results = append(results, unavailable(instrument, retrievedAt, "No reliable structured issuer source is configured for this fund."))
			continue
		}

		result, err := source.Fetch(ctx, instrument, retrievedAt)
		if err != nil {
			result.Fund = instrument
			result.Available = false
			result.Complete = false
			result.RetrievedAt = retrievedAt.UTC()
			result.Confidence = "unavailable"
			if result.SourceType == "" {
				result.SourceType = "configured_source"
			}
			result.Warnings = append(result.Warnings, fmt.Sprintf("Issuer holdings could not be retrieved: %v", err))
		}
		results = append(results, result)
	}
	return results
}

func (s *Service) sourceFor(instrument portfolio.Instrument) Source {
	for _, source := range s.sources {
		if source.Supports(instrument) {
			return source
		}
	}
	return nil
}

// IsFund conservatively identifies broker-labelled funds and names that explicitly identify an ETF.
func IsFund(instrument portfolio.Instrument) bool {
	assetClass := strings.ToUpper(strings.TrimSpace(instrument.AssetClass))
	if assetClass == "FUND" || assetClass == "ETF" {
		return true
	}
	name := strings.ToUpper(strings.TrimSpace(instrument.Name))
	return strings.Contains(name, " ETF") || strings.HasSuffix(name, "ETF")
}

func unavailable(instrument portfolio.Instrument, retrievedAt time.Time, warning string) portfolio.FundHoldings {
	return portfolio.FundHoldings{
		Fund: instrument, SourceType: "unsupported", RetrievedAt: retrievedAt.UTC(),
		Warnings: []string{warning}, Confidence: "unavailable",
	}
}
