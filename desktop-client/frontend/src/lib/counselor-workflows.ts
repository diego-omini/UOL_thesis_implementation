// counselor-workflows.ts defines stable UI ownership and concise copy for catalog workflows.

export type CounselorWorkflowID =
  | 'portfolio_resume' | 'portfolio_risk' | 'portfolio_news' | 'stock_news' | 'order_analysis'
  | 'currency_exposure' | 'effective_diversification' | 'portfolio_history_summary' | 'position_risk' | 'portfolio_data_quality'

export type CounselorWorkflowOwner = 'positions' | 'modeled-risk' | 'stock' | 'order' | 'currency' | 'effective-exposure' | 'history' | 'current-position' | 'data-quality'

export interface CounselorWorkflowCopy {
  id: CounselorWorkflowID
  owner: CounselorWorkflowOwner
  title: string
  description: string
  action: string
  loading: string
  unavailable: string
  research: boolean
}

export const counselorWorkflows: CounselorWorkflowCopy[] = [
  { id: 'portfolio_resume', owner: 'positions', title: 'Portfolio resume', description: 'Summarizes current normalized holdings and analytics.', action: 'Get portfolio resume', loading: 'Generating portfolio resume...', unavailable: 'Portfolio resume unavailable', research: true },
  { id: 'portfolio_news', owner: 'positions', title: 'Portfolio news', description: 'Researches recent material developments for each open position.', action: 'Get portfolio news', loading: 'Researching portfolio news...', unavailable: 'Portfolio news unavailable', research: true },
  { id: 'portfolio_risk', owner: 'modeled-risk', title: 'Modeled risk explanation', description: 'Explains the deterministic current-weight risk calculation.', action: 'Explain with counselor', loading: 'Explaining modeled risk...', unavailable: 'Risk explanation unavailable', research: false },
  { id: 'stock_news', owner: 'stock', title: 'Stock news', description: 'Researches recent material developments for the selected stock.', action: 'Get stock news', loading: 'Researching stock news...', unavailable: 'Stock news unavailable', research: true },
  { id: 'order_analysis', owner: 'order', title: 'Proposed-order analysis', description: 'Explains deterministic order impact with bounded company research.', action: 'Get order analysis', loading: 'Analyzing proposed order...', unavailable: 'Order analysis unavailable', research: true },
  { id: 'currency_exposure', owner: 'currency', title: 'Currency exposure', description: 'Explains instrument-currency concentration and broker conversion coverage.', action: 'Explain currency exposure', loading: 'Analyzing currency exposure...', unavailable: 'Currency analysis unavailable', research: false },
  { id: 'effective_diversification', owner: 'effective-exposure', title: 'Effective diversification', description: 'Explains direct and fund look-through concentration without inventing holdings.', action: 'Explain effective diversification', loading: 'Analyzing effective diversification...', unavailable: 'Diversification analysis unavailable', research: false },
  { id: 'portfolio_history_summary', owner: 'history', title: 'Historical-data summary', description: 'Describes available observations and changes without calling them investment returns.', action: 'Summarize this history', loading: 'Summarizing historical data...', unavailable: 'History summary unavailable', research: false },
  { id: 'position_risk', owner: 'current-position', title: 'Current-position risk', description: 'Explains current holding exposure and deterministic one-year price behavior.', action: 'Analyze current position risk', loading: 'Analyzing current position risk...', unavailable: 'Position risk analysis unavailable', research: false },
  { id: 'portfolio_data_quality', owner: 'data-quality', title: 'Portfolio data quality', description: 'Audits metric availability, classifications, conversions, and look-through coverage.', action: 'Audit portfolio data quality', loading: 'Auditing portfolio data quality...', unavailable: 'Data-quality audit unavailable', research: false },
]

export function workflowCopy(id: CounselorWorkflowID): CounselorWorkflowCopy {
  const workflow = counselorWorkflows.find(item => item.id === id)
  if (!workflow) throw new Error(`Unknown counselor workflow: ${id}`)
  return workflow
}
