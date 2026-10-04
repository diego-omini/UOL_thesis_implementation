// types.ts exposes concise aliases for the generated Wails frontend models.
import type { main } from '../../wailsjs/go/models'

export type AIResponseView = main.AIResponseView
export type AllocationView = main.AllocationView
export type InstrumentSearchResultView = main.InstrumentSearchResultView
export type MoneyView = main.MoneyView
export type OptionalNumberView = main.OptionalNumberView
export type OrderPreviewView = main.OrderPreviewView
export type OrderResultView = main.OrderResultView
export type PortfolioView = main.PortfolioView
export type PortfolioRiskAnalysisView = main.PortfolioRiskAnalysisView
export type PortfolioRiskCounselView = main.PortfolioRiskCounselView
export type PortfolioHistoryMetricView = main.PortfolioHistoryMetricView
export type PortfolioHistoryPointView = main.PortfolioHistoryPointView
export type PortfolioHistorySeriesView = main.PortfolioHistorySeriesView
export type PortfolioHistoryView = main.PortfolioHistoryView
export type PositionView = main.PositionView
export type RatioView = main.RatioView
export type SessionStatus = main.SessionStatusView
export type StockHistoryView = main.StockHistoryView
export type StockSnapshotView = main.StockSnapshotView

export type StockHistoryRange = '24h' | '1m' | '1y' | 'all'
