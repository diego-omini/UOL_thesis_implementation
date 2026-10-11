// counselor-workflows.ts defines the user-facing analysis library and its availability rules.

export type CounselorWorkflowID =
  | 'portfolio_resume' | 'portfolio_risk' | 'portfolio_news' | 'stock_news' | 'order_analysis'
  | 'currency_exposure' | 'effective_diversification' | 'portfolio_history_summary' | 'position_risk' | 'portfolio_data_quality'

export type CounselorWorkflowOwner = 'positions' | 'modeled-risk' | 'stock' | 'order' | 'history'
export type CounselorPurpose = 'Portfolio overview' | 'Risk' | 'Diversification' | 'History' | 'Research' | 'Stock analysis' | 'Proposed-order analysis'

export interface CounselorWorkflowCopy {
  id: CounselorWorkflowID
  owner: CounselorWorkflowOwner
  purpose: CounselorPurpose
  title: string
  description: string
  requiredData: string
  action: string
  loading: string
  unavailable: string
  research: boolean
}

export interface CounselorAvailabilityContext {
  positionCount?: number
  stockSelected?: boolean
  positionHeld?: boolean
  riskObservations?: number
  historyPoints?: number
  validOrder?: boolean
}

export interface CounselorAvailability {
  available: boolean
  reason: string
}

export const counselorWorkflows: CounselorWorkflowCopy[] = [
  { id: 'portfolio_resume', owner: 'positions', purpose: 'Portfolio overview', title: 'Portfolio overview', description: 'Explains bounded current exposure, concentration, look-through coverage, and material data limits; it may research a material structural-risk point.', requiredData: 'Open positions and current deterministic portfolio analytics', action: 'Run overview', loading: 'Preparing portfolio overview...', unavailable: 'Portfolio overview unavailable', research: true },
  { id: 'portfolio_news', owner: 'positions', purpose: 'Research', title: 'Portfolio news', description: 'Researches bounded recent material developments for up to the 12 largest open positions.', requiredData: 'At least one open position with a public name or ticker', action: 'Research portfolio news', loading: 'Researching portfolio news...', unavailable: 'Portfolio news unavailable', research: true },
  { id: 'portfolio_risk', owner: 'modeled-risk', purpose: 'Risk', title: 'Modeled portfolio risk', description: 'Explains the deterministic current-weight risk model; it is not a realized portfolio-return calculation.', requiredData: 'At least 20 common daily returns from eligible stock positions', action: 'Explain calculation', loading: 'Explaining modeled risk...', unavailable: 'Risk explanation unavailable', research: false },
  { id: 'stock_news', owner: 'stock', purpose: 'Stock analysis', title: 'Stock news', description: 'Researches recent material developments for the selected stock.', requiredData: 'A broker-verified stock identity', action: 'Research stock news', loading: 'Researching stock news...', unavailable: 'Stock news unavailable', research: true },
  { id: 'order_analysis', owner: 'order', purpose: 'Proposed-order analysis', title: 'Proposed-order analysis', description: 'Explains deterministic order impact with bounded company research. It cannot preview or submit an order.', requiredData: 'Selected stock, side, valid whole-share quantity, quote, portfolio, and one-year price history', action: 'Analyze proposed order', loading: 'Analyzing proposed order...', unavailable: 'Order analysis unavailable', research: true },
  { id: 'currency_exposure', owner: 'positions', purpose: 'Diversification', title: 'Currency exposure', description: 'Explains instrument currencies and conversion coverage, not each issuer’s full economic currency exposure.', requiredData: 'Open positions, currency allocations, and broker exchange rates', action: 'Explain currency exposure', loading: 'Analyzing currency exposure...', unavailable: 'Currency analysis unavailable', research: false },
  { id: 'effective_diversification', owner: 'positions', purpose: 'Diversification', title: 'Effective diversification', description: 'Explains direct and fund look-through concentration without inventing holdings.', requiredData: 'Open positions, concentration metrics, and supported issuer holdings', action: 'Explain diversification', loading: 'Analyzing effective diversification...', unavailable: 'Diversification analysis unavailable', research: false },
  { id: 'portfolio_history_summary', owner: 'history', purpose: 'History', title: 'Historical-data summary', description: 'Describes available observations and changes without calling them investment returns.', requiredData: 'At least one observation in the selected history range', action: 'Summarize this history', loading: 'Summarizing historical data...', unavailable: 'History summary unavailable', research: false },
  { id: 'position_risk', owner: 'stock', purpose: 'Risk', title: 'Current-position risk', description: 'Explains current holding exposure and deterministic one-year price behavior, not unmeasured business risk.', requiredData: 'A current holding, quote, portfolio weight, and one-year price history', action: 'Analyze position risk', loading: 'Analyzing current position risk...', unavailable: 'Position risk analysis unavailable', research: false },
  { id: 'portfolio_data_quality', owner: 'positions', purpose: 'Portfolio overview', title: 'Portfolio data quality', description: 'Audits metric availability, classifications, conversions, and look-through coverage.', requiredData: 'Open positions and current analytics availability metadata', action: 'Audit data quality', loading: 'Auditing portfolio data quality...', unavailable: 'Data-quality audit unavailable', research: false },
]

export function workflowCopy(id: CounselorWorkflowID): CounselorWorkflowCopy {
  const workflow = counselorWorkflows.find(item => item.id === id)
  if (!workflow) throw new Error(`Unknown counselor workflow: ${id}`)
  return workflow
}

export function workflowsForOwner(owner: CounselorWorkflowOwner): CounselorWorkflowCopy[] {
  return counselorWorkflows.filter(workflow => workflow.owner === owner)
}

export function workflowAvailability(workflow: CounselorWorkflowCopy, context: CounselorAvailabilityContext): CounselorAvailability {
  switch (workflow.owner) {
    case 'positions':
      return (context.positionCount ?? 0) > 0 ? { available: true, reason: '' } : { available: false, reason: 'Requires at least one open position.' }
    case 'stock':
      if (!context.stockSelected) return { available: false, reason: 'Select a stock first.' }
      if (workflow.id === 'position_risk' && !context.positionHeld) return { available: false, reason: 'Available only for a stock currently held in the portfolio.' }
      return { available: true, reason: '' }
    case 'modeled-risk':
      return (context.riskObservations ?? 0) >= 20 ? { available: true, reason: '' } : { available: false, reason: 'Calculate risk with at least 20 common daily returns first.' }
    case 'history':
      return (context.historyPoints ?? 0) > 0 ? { available: true, reason: '' } : { available: false, reason: 'No observations are available in the selected range.' }
    case 'order':
      return context.validOrder ? { available: true, reason: '' } : { available: false, reason: 'Enter a valid whole-share quantity for the selected side.' }
  }
}
