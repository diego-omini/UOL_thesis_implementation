package main

// dto.go defines Wails-facing view models and maps domain results into them.

import (
	"time"

	"wails/ai"
	"wails/analytics"
	"wails/broker"
	"wails/persistence"
	"wails/portfolio"
	"wails/portfolioanalysis"
	"wails/portfoliohistory"
	"wails/trading"
)

type ConnectionStatusView struct {
	Connected bool
	Message   string
}

type SessionStatusView struct {
	State            string
	GatewayReachable bool
	Authenticated    bool
	Connected        bool
	Competing        bool
	Paper            bool
	PaperKnown       bool
	Ready            bool
	Message          string
}

type MoneyView struct {
	Amount    float64
	Currency  string
	Available bool
}

type RatioView struct {
	Value     float64
	Available bool
}

type AccountSummaryView struct {
	NetLiquidation MoneyView
	CashBalance    MoneyView
}

type InstrumentView struct {
	ContractID int64
	Symbol     string
	Name       string
	AssetClass string
	Exchange   string
	Currency   string
	Sector     string
	Industry   string
}

type InstrumentSearchResultView struct {
	ContractID int64
	Symbol     string
	Name       string
	Exchange   string
	AssetClass string
}

type OptionalNumberView struct {
	Value             float64
	Available         bool
	UnavailableReason string
}

type StockSnapshotView struct {
	Instrument       InstrumentView
	Currency         string
	OpenPrice        OptionalNumberView
	ClosePrice       OptionalNumberView
	BidPrice         OptionalNumberView
	AskPrice         OptionalNumberView
	Volume           OptionalNumberView
	High52Week       OptionalNumberView
	Low52Week        OptionalNumberView
	MarketDataStatus string
	AsOf             string
}

type StockHistoryPointView struct {
	Time   string
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

type StockHistoryView struct {
	Range             string
	Points            []StockHistoryPointView
	MarketDataStatus  string
	DelaySeconds      int
	UnavailableReason string
}

type PositionView struct {
	Instrument        InstrumentView
	Quantity          float64
	AverageCost       MoneyView
	MarketPrice       MoneyView
	MarketValue       MoneyView
	BaseMarketValue   MoneyView
	UnrealizedPnL     MoneyView
	BaseUnrealizedPnL MoneyView
	PortfolioWeight   RatioView
}

type AllocationView struct {
	Label       string
	MarketValue MoneyView
	Exposure    MoneyView
	Weight      RatioView
}

type PositionWeightView struct {
	ContractID  int64
	Symbol      string
	Name        string
	MarketValue MoneyView
	Weight      RatioView
}

type DataWarningView struct {
	Code    string
	Message string
	Count   int
}

type EffectiveExposureView struct {
	MetricID    string
	SecurityID  string
	Symbol      string
	Name        string
	Sector      string
	MarketValue MoneyView
	Exposure    MoneyView
	Weight      RatioView
}

type FundLookThroughView struct {
	FundSymbol                string
	FundName                  string
	Available                 bool
	Complete                  bool
	SourceURL                 string
	SourceType                string
	RetrievedAt               string
	ExpiresAt                 string
	EffectiveDate             string
	CoveragePercentage        float64
	CoverageAvailable         bool
	TopHoldingsOnly           bool
	CurrencyAssumptions       []string
	ClassificationAssumptions []string
	Warnings                  []string
	Confidence                string
}

type ExchangeRateView struct {
	Currency          string
	BaseCurrency      string
	BaseAmountPerUnit float64
	Available         bool
	Source            string
	AsOf              string
}

type AnalyticsView struct {
	CalculatedAt              string
	BaseCurrency              string
	NetMarketValue            MoneyView
	GrossExposure             MoneyView
	TotalUnrealizedPnL        MoneyView
	LargestPositions          []PositionWeightView
	CurrencyAllocation        []AllocationView
	AssetClassAllocation      []AllocationView
	SectorAllocation          []AllocationView
	TopThreeConcentration     RatioView
	HHI                       RatioView
	ExchangeRates             []ExchangeRateView
	Warnings                  []DataWarningView
	EffectiveSecurityExposure []EffectiveExposureView
	EffectiveSectorAllocation []AllocationView
	FundLookThrough           []FundLookThroughView
	EffectiveExposureComplete bool
}

type PortfolioView struct {
	AccountSummary AccountSummaryView
	Positions      []PositionView
	Analytics      AnalyticsView
	Empty          bool
}

type PortfolioRiskPositionView struct {
	ContractID           int64
	Symbol               string
	Name                 string
	SignedWeight         float64
	TotalReturn          RatioView
	AnnualizedVolatility RatioView
	ReturnContribution   RatioView
	RiskContribution     RatioView
}

type PortfolioCorrelationView struct {
	LeftSymbol  string
	RightSymbol string
	Value       float64
}

type PortfolioScenarioView struct {
	ID          string
	Name        string
	Description string
	Impact      MoneyView
	ImpactRatio RatioView
}

type PortfolioRiskAnalysisView struct {
	CalculatedAt          string
	BaseCurrency          string
	PeriodStart           string
	PeriodEnd             string
	Observations          int
	IncludedPositionCount int
	TotalPositionCount    int
	ModeledTotalReturn    RatioView
	AnnualizedVolatility  RatioView
	DownsideDeviation     RatioView
	MaximumDrawdown       RatioView
	Positions             []PortfolioRiskPositionView
	Correlations          []PortfolioCorrelationView
	Scenarios             []PortfolioScenarioView
	Warnings              []string
}

type PortfolioRiskCounselView struct {
	Analysis PortfolioRiskAnalysisView
	Response AIResponseView
}

type PortfolioHistoryMetricView struct {
	Value     float64
	Currency  string
	Available bool
}

type PortfolioHistoryWarningView struct {
	Code    string
	Message string
	Count   int
}

type PortfolioHistoryPointView struct {
	CapturedAt            string
	Source                string
	EndOfDay              bool
	NetLiquidation        PortfolioHistoryMetricView
	Cash                  PortfolioHistoryMetricView
	GrossExposure         PortfolioHistoryMetricView
	NetMarketValue        PortfolioHistoryMetricView
	UnrealizedPnL         PortfolioHistoryMetricView
	TopThreeConcentration PortfolioHistoryMetricView
	HHI                   PortfolioHistoryMetricView
	PositionCount         PortfolioHistoryMetricView
	Warnings              []PortfolioHistoryWarningView
}

type PortfolioHistorySeriesView struct {
	BaseCurrency string
	Points       []PortfolioHistoryPointView
}

type PortfolioHistoryView struct {
	Range                           string
	From                            string
	To                              string
	AccountValueSeries              []PortfolioHistorySeriesView
	TrackingSeries                  []PortfolioHistorySeriesView
	TrackingStartedAt               string
	StoredPointCount                int
	AccountValuePointCount          int
	TrackingPointCount              int
	AccountValueDuplicatePointCount int
	TrackingDuplicatePointCount     int
	AccountValueDownsampled         bool
	TrackingDownsampled             bool
	OfficialPointCount              int
	EstimatedPointCount             int
	ImportWarning                   string
	ExternalConfigured              bool
	ConnectionRequired              bool
}

type AIResponseView struct {
	Text          string
	Structured    CounselorResponseView
	HasStructured bool
	Provider      string
	Model         string
	PromptID      string
	PromptVersion int
	CreatedAt     string
	ContextAsOf   string
}

type RecentAnalysisView struct {
	ID       uint
	Response AIResponseView
}

type ReportedValueView struct {
	FactID string
	Value  float64
	Unit   string
}

type CitationView struct {
	Title string
	URL   string
}

type CounselorObservationView struct {
	Category          string
	Statement         string
	SupportingFactIDs []string
	ReportedValues    []ReportedValueView
	Severity          string
	Confidence        string
	Limitations       []string
	Explanation       string
	Citations         []CitationView
}

type CounselorLimitationView struct {
	Statement string
	FactIDs   []string
}

type CounselorResponseView struct {
	SchemaVersion int
	Summary       string
	Observations  []CounselorObservationView
	Limitations   []CounselorLimitationView
}

type OrderPreviewView struct {
	ExpiresAt         string
	Instrument        InstrumentView
	Side              string
	Quantity          float64
	EstimatedPrice    MoneyView
	OrderAmount       MoneyView
	Commission        MoneyView
	Total             MoneyView
	CashBalance       MoneyView
	CurrentPosition   float64
	ResultingPosition float64
	Warnings          []string
}

type OrderResultView struct {
	OrderID              string
	Status               string
	ConfirmationRequired bool
	WarningMessages      []string
}

func toConnectionStatusView(status bool, message string) ConnectionStatusView {
	return ConnectionStatusView{Connected: status, Message: message}
}

func toSessionStatusView(status broker.SessionStatus) SessionStatusView {
	return SessionStatusView{
		State:            string(status.State),
		GatewayReachable: status.GatewayReachable,
		Authenticated:    status.Authenticated,
		Connected:        status.Connected,
		Competing:        status.Competing,
		Paper:            status.Paper,
		PaperKnown:       status.PaperKnown,
		Ready:            status.Ready(),
		Message:          status.Message,
	}
}

// toPortfolioView assembles the complete frontend payload from portfolio and analytics domain results.
func toPortfolioView(p portfolio.Portfolio, result analytics.Result) PortfolioView {
	positions := make([]PositionView, 0, len(p.Positions))

	for index, pos := range p.Positions {
		weight := RatioView{}
		if index < len(result.PositionWeights) {
			weight = toRatioView(result.PositionWeights[index].Weight)
		}

		positions = append(positions, PositionView{
			Instrument:        InstrumentView(pos.Instrument),
			Quantity:          pos.Quantity,
			AverageCost:       toMoneyView(pos.AverageCost),
			MarketPrice:       toMoneyView(pos.MarketPrice),
			MarketValue:       toMoneyView(pos.MarketValue),
			BaseMarketValue:   toMoneyView(pos.BaseMarketValue),
			UnrealizedPnL:     toMoneyView(pos.UnrealizedPnL),
			BaseUnrealizedPnL: toMoneyView(pos.BaseUnrealizedPnL),
			PortfolioWeight:   weight,
		})
	}

	return PortfolioView{
		AccountSummary: AccountSummaryView{
			NetLiquidation: toMoneyView(p.AccountSummary.NetLiquidation),
			CashBalance:    toMoneyView(p.AccountSummary.CashBalance),
		},
		Positions: positions,
		Analytics: toAnalyticsView(result),
		Empty:     len(positions) == 0,
	}
}

func toMoneyView(m portfolio.Money) MoneyView {
	return MoneyView{Amount: m.Amount, Currency: m.Currency, Available: m.Available}
}

func toPortfolioRiskAnalysisView(result portfolioanalysis.Result) PortfolioRiskAnalysisView {
	view := PortfolioRiskAnalysisView{
		CalculatedAt: result.CalculatedAt.Format(time.RFC3339), BaseCurrency: result.BaseCurrency,
		Observations: result.Observations, IncludedPositionCount: result.IncludedPositionCount, TotalPositionCount: result.TotalPositionCount,
		ModeledTotalReturn: RatioView(result.ModeledTotalReturn), AnnualizedVolatility: RatioView(result.AnnualizedVolatility),
		DownsideDeviation: RatioView(result.DownsideDeviation), MaximumDrawdown: RatioView(result.MaximumDrawdown),
		Positions: make([]PortfolioRiskPositionView, 0, len(result.Positions)), Correlations: make([]PortfolioCorrelationView, 0, len(result.Correlations)),
		Scenarios: make([]PortfolioScenarioView, 0, len(result.Scenarios)), Warnings: append([]string{}, result.Warnings...),
	}
	if !result.PeriodStart.IsZero() {
		view.PeriodStart = result.PeriodStart.Format(time.RFC3339)
	}
	if !result.PeriodEnd.IsZero() {
		view.PeriodEnd = result.PeriodEnd.Format(time.RFC3339)
	}
	for _, position := range result.Positions {
		view.Positions = append(view.Positions, PortfolioRiskPositionView{ContractID: position.ContractID, Symbol: position.Symbol, Name: position.Name, SignedWeight: position.SignedWeight, TotalReturn: RatioView(position.TotalReturn), AnnualizedVolatility: RatioView(position.AnnualizedVolatility), ReturnContribution: RatioView(position.ReturnContribution), RiskContribution: RatioView(position.RiskContribution)})
	}
	for _, correlation := range result.Correlations {
		view.Correlations = append(view.Correlations, PortfolioCorrelationView{LeftSymbol: correlation.LeftSymbol, RightSymbol: correlation.RightSymbol, Value: correlation.Value})
	}
	for _, scenario := range result.Scenarios {
		view.Scenarios = append(view.Scenarios, PortfolioScenarioView{ID: scenario.ID, Name: scenario.Name, Description: scenario.Description, Impact: toMoneyView(scenario.Impact), ImpactRatio: RatioView(scenario.ImpactRatio)})
	}
	return view
}

// toPortfolioHistoryView maps domain history without exposing account identity or persisted payloads.
func toPortfolioHistoryView(timeRange string, result portfoliohistory.Result) PortfolioHistoryView {
	view := PortfolioHistoryView{
		Range: timeRange, From: result.From.Format(time.RFC3339), To: result.To.Format(time.RFC3339),
		StoredPointCount:       result.StoredPointCount,
		AccountValuePointCount: result.AccountValuePointCount, TrackingPointCount: result.TrackingPointCount,
		AccountValueDuplicatePointCount: result.AccountValueDuplicatePointCount,
		TrackingDuplicatePointCount:     result.TrackingDuplicatePointCount,
		AccountValueDownsampled:         result.AccountValueDownsampled, TrackingDownsampled: result.TrackingDownsampled,
		OfficialPointCount: result.OfficialPointCount, EstimatedPointCount: result.EstimatedPointCount,
		ImportWarning: result.ImportWarning, ExternalConfigured: result.ExternalConfigured,
		ConnectionRequired: result.ConnectionRequired,
		AccountValueSeries: make([]PortfolioHistorySeriesView, 0, len(result.AccountValueSeries)),
		TrackingSeries:     make([]PortfolioHistorySeriesView, 0, len(result.TrackingSeries)),
	}
	if !result.TrackingStartedAt.IsZero() {
		view.TrackingStartedAt = result.TrackingStartedAt.Format(time.RFC3339)
	}
	view.AccountValueSeries = historySeriesViews(result.AccountValueSeries)
	view.TrackingSeries = historySeriesViews(result.TrackingSeries)
	return view
}

func historySeriesViews(seriesValues []portfoliohistory.Series) []PortfolioHistorySeriesView {
	views := make([]PortfolioHistorySeriesView, 0, len(seriesValues))
	for _, series := range seriesValues {
		seriesView := PortfolioHistorySeriesView{
			BaseCurrency: series.BaseCurrency,
			Points:       make([]PortfolioHistoryPointView, 0, len(series.Points)),
		}
		for _, point := range series.Points {
			warnings := make([]PortfolioHistoryWarningView, 0, len(point.Warnings))
			for _, warning := range point.Warnings {
				warnings = append(warnings, PortfolioHistoryWarningView(warning))
			}
			seriesView.Points = append(seriesView.Points, PortfolioHistoryPointView{
				CapturedAt: point.CapturedAt.Format(time.RFC3339), Source: point.Source, EndOfDay: point.EndOfDay,
				NetLiquidation: historyMetricView(point.NetLiquidation), Cash: historyMetricView(point.Cash),
				GrossExposure: historyMetricView(point.GrossExposure), NetMarketValue: historyMetricView(point.NetMarketValue),
				UnrealizedPnL: historyMetricView(point.UnrealizedPnL), TopThreeConcentration: historyMetricView(point.TopThreeConcentration),
				HHI: historyMetricView(point.HHI), PositionCount: historyMetricView(point.PositionCount), Warnings: warnings,
			})
		}
		views = append(views, seriesView)
	}
	return views
}

func historyMetricView(metric portfoliohistory.Metric) PortfolioHistoryMetricView {
	return PortfolioHistoryMetricView(metric)
}

func toInstrumentSearchResultViews(results []portfolio.InstrumentSearchResult) []InstrumentSearchResultView {
	views := make([]InstrumentSearchResultView, 0, len(results))
	for _, result := range results {
		views = append(views, InstrumentSearchResultView(result))
	}
	return views
}

func toStockSnapshotView(snapshot portfolio.StockSnapshot) StockSnapshotView {
	return StockSnapshotView{
		Instrument:       InstrumentView(snapshot.Instrument),
		Currency:         snapshot.Currency,
		OpenPrice:        OptionalNumberView(snapshot.OpenPrice),
		ClosePrice:       OptionalNumberView(snapshot.ClosePrice),
		BidPrice:         OptionalNumberView(snapshot.BidPrice),
		AskPrice:         OptionalNumberView(snapshot.AskPrice),
		Volume:           OptionalNumberView(snapshot.Volume),
		High52Week:       OptionalNumberView(snapshot.High52Week),
		Low52Week:        OptionalNumberView(snapshot.Low52Week),
		MarketDataStatus: snapshot.MarketDataStatus,
		AsOf:             snapshot.AsOf.Format(time.RFC3339),
	}
}

func toStockHistoryView(history portfolio.StockHistory) StockHistoryView {
	points := make([]StockHistoryPointView, 0, len(history.Points))
	for _, point := range history.Points {
		points = append(points, StockHistoryPointView{
			Time: point.Time.Format(time.RFC3339), Open: point.Open, High: point.High,
			Low: point.Low, Close: point.Close, Volume: point.Volume,
		})
	}
	return StockHistoryView{
		Range: history.Range, Points: points, MarketDataStatus: history.MarketDataStatus,
		DelaySeconds: history.DelaySeconds, UnavailableReason: history.UnavailableReason,
	}
}

func toOrderPreviewView(preview trading.Preview) OrderPreviewView {
	return OrderPreviewView{
		ExpiresAt:  preview.ExpiresAt.Format(time.RFC3339),
		Instrument: InstrumentView(preview.Instrument), Side: string(preview.Side), Quantity: preview.Quantity,
		EstimatedPrice: toMoneyView(preview.EstimatedPrice), OrderAmount: toMoneyView(preview.OrderAmount),
		Commission: toMoneyView(preview.Commission), Total: toMoneyView(preview.Total), CashBalance: toMoneyView(preview.CashBalance),
		CurrentPosition: preview.CurrentPosition, ResultingPosition: preview.ResultingPosition, Warnings: preview.Warnings,
	}
}

func toOrderResultView(result trading.Result) OrderResultView {
	return OrderResultView{
		OrderID: result.OrderID, Status: result.Status,
		ConfirmationRequired: result.ConfirmationRequired, WarningMessages: result.WarningMessages,
	}
}

// toAIResponseView maps validated counselor output into generated-binding-safe view types.
func toAIResponseView(response ai.Response, provider string) AIResponseView {
	view := AIResponseView{
		Text: response.Text, Provider: provider, Model: response.Model,
		PromptID: response.PromptID, PromptVersion: response.PromptVersion,
		CreatedAt: formatOptionalTime(response.CreatedAt), ContextAsOf: formatOptionalTime(response.ContextAsOf),
	}
	if response.Structured == nil {
		return view
	}
	view.HasStructured = true
	view.Structured = CounselorResponseView{
		SchemaVersion: response.Structured.SchemaVersion,
		Summary:       response.Structured.Summary,
		Observations:  make([]CounselorObservationView, 0, len(response.Structured.Observations)),
		Limitations:   make([]CounselorLimitationView, 0, len(response.Structured.Limitations)),
	}
	for _, observation := range response.Structured.Observations {
		reportedValues := make([]ReportedValueView, 0, len(observation.ReportedValues))
		for _, value := range observation.ReportedValues {
			reportedValues = append(reportedValues, ReportedValueView(value))
		}
		citations := make([]CitationView, 0, len(observation.Citations))
		for _, citation := range observation.Citations {
			citations = append(citations, CitationView(citation))
		}
		view.Structured.Observations = append(view.Structured.Observations, CounselorObservationView{
			Category: string(observation.Category), Statement: observation.Statement,
			SupportingFactIDs: observation.SupportingFactIDs, ReportedValues: reportedValues,
			Severity: string(observation.Severity), Confidence: string(observation.Confidence),
			Limitations: observation.Limitations, Explanation: observation.Explanation, Citations: citations,
		})
	}
	for _, limitation := range response.Structured.Limitations {
		view.Structured.Limitations = append(view.Structured.Limitations, CounselorLimitationView(limitation))
	}
	return view
}

func toRecentAnalysisViews(records []persistence.RecentAnalysis) []RecentAnalysisView {
	views := make([]RecentAnalysisView, 0, len(records))
	for _, record := range records {
		views = append(views, RecentAnalysisView{ID: record.ID, Response: toAIResponseView(ai.Response{
			Structured: &record.Response, Model: record.Model, PromptID: record.PromptID,
			PromptVersion: record.PromptVersion, CreatedAt: record.CreatedAt, ContextAsOf: record.ContextAsOf,
		}, record.Provider)})
	}
	return views
}

func toRatioView(ratio analytics.Ratio) RatioView {
	return RatioView{Value: ratio.Value, Available: ratio.Available}
}

// toAnalyticsView maps the complete analytics result into Wails-safe view models.
func toAnalyticsView(result analytics.Result) AnalyticsView {
	return AnalyticsView{
		CalculatedAt:              result.CalculatedAt.Format(time.RFC3339),
		BaseCurrency:              result.BaseCurrency,
		NetMarketValue:            toMoneyView(result.NetMarketValue),
		GrossExposure:             toMoneyView(result.GrossExposure),
		TotalUnrealizedPnL:        toMoneyView(result.TotalUnrealizedPnL),
		LargestPositions:          toPositionWeightViews(result.LargestPositions),
		CurrencyAllocation:        toAllocationViews(result.CurrencyAllocation),
		AssetClassAllocation:      toAllocationViews(result.AssetClassAllocation),
		SectorAllocation:          toAllocationViews(result.SectorAllocation),
		TopThreeConcentration:     toRatioView(result.TopThreeConcentration),
		HHI:                       toRatioView(result.HHI),
		ExchangeRates:             toExchangeRateViews(result.ExchangeRates),
		Warnings:                  toWarningViews(result.Warnings),
		EffectiveSecurityExposure: toEffectiveExposureViews(result.EffectiveSecurityExposure),
		EffectiveSectorAllocation: toAllocationViews(result.EffectiveSectorAllocation),
		FundLookThrough:           toFundLookThroughViews(result.FundLookThrough),
		EffectiveExposureComplete: result.EffectiveExposureComplete,
	}
}

func toEffectiveExposureViews(exposures []analytics.EffectiveExposure) []EffectiveExposureView {
	views := make([]EffectiveExposureView, 0, len(exposures))
	for _, exposure := range exposures {
		views = append(views, EffectiveExposureView{
			MetricID: exposure.MetricID, SecurityID: exposure.SecurityID, Symbol: exposure.Symbol,
			Name: exposure.Name, Sector: exposure.Sector, MarketValue: toMoneyView(exposure.MarketValue),
			Exposure: toMoneyView(exposure.Exposure), Weight: toRatioView(exposure.Weight),
		})
	}
	return views
}

func toFundLookThroughViews(results []portfolio.FundHoldings) []FundLookThroughView {
	views := make([]FundLookThroughView, 0, len(results))
	for _, result := range results {
		views = append(views, FundLookThroughView{
			FundSymbol: result.Fund.Symbol, FundName: result.Fund.Name, Available: result.Available,
			Complete: result.Complete, SourceURL: result.SourceURL, SourceType: result.SourceType,
			RetrievedAt: formatOptionalTime(result.RetrievedAt), ExpiresAt: formatOptionalTime(result.ExpiresAt),
			EffectiveDate: formatOptionalTime(result.EffectiveDate), CoveragePercentage: result.CoveragePercentage.Value,
			CoverageAvailable: result.CoveragePercentage.Available, TopHoldingsOnly: result.TopHoldingsOnly,
			CurrencyAssumptions:       append([]string(nil), result.CurrencyAssumptions...),
			ClassificationAssumptions: append([]string(nil), result.ClassificationAssumptions...),
			Warnings:                  append([]string(nil), result.Warnings...), Confidence: result.Confidence,
		})
	}
	return views
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func toExchangeRateViews(rates []portfolio.ExchangeRate) []ExchangeRateView {
	views := make([]ExchangeRateView, 0, len(rates))
	for _, rate := range rates {
		views = append(views, ExchangeRateView{
			Currency:          rate.Currency,
			BaseCurrency:      rate.BaseCurrency,
			BaseAmountPerUnit: rate.BaseAmountPerUnit,
			Available:         rate.Available,
			Source:            rate.Source,
			AsOf:              rate.AsOf.Format(time.RFC3339),
		})
	}
	return views
}

func toPositionWeightViews(positions []analytics.PositionWeight) []PositionWeightView {
	views := make([]PositionWeightView, 0, len(positions))
	for _, position := range positions {
		views = append(views, PositionWeightView{
			ContractID:  position.ContractID,
			Symbol:      position.Symbol,
			Name:        position.Name,
			MarketValue: toMoneyView(position.MarketValue),
			Weight:      toRatioView(position.Weight),
		})
	}
	return views
}

func toAllocationViews(allocations []analytics.Allocation) []AllocationView {
	views := make([]AllocationView, 0, len(allocations))
	for _, allocation := range allocations {
		views = append(views, AllocationView{
			Label:       allocation.Label,
			MarketValue: toMoneyView(allocation.MarketValue),
			Exposure:    toMoneyView(allocation.Exposure),
			Weight:      toRatioView(allocation.Weight),
		})
	}
	return views
}

func toWarningViews(warnings []analytics.DataWarning) []DataWarningView {
	views := make([]DataWarningView, 0, len(warnings))
	for _, warning := range warnings {
		views = append(views, DataWarningView(warning))
	}
	return views
}
