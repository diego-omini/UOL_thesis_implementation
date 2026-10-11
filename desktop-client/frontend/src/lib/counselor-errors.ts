// counselor-errors.ts maps backend failures into distinct counselor-facing problem categories.

export type CounselorErrorKind = 'session' | 'broker' | 'market_data' | 'research' | 'provider' | 'validation' | 'unknown'

export interface CounselorProblem {
  kind: CounselorErrorKind
  title: string
  guidance: string
}

export function classifyCounselorError(message: string): CounselorProblem {
  const value = message.toLowerCase()
  if (/authenticated|paper session|session|gateway/.test(value)) return { kind: 'session', title: 'Broker session unavailable', guidance: 'Reauthenticate the IBKR paper session, then try again.' }
  if (/research|search|api key|credential|web_fetch|web_search/.test(value)) return { kind: 'research', title: 'External research unavailable', guidance: 'Check the hosted research credentials and source configuration. Deterministic portfolio figures remain unchanged.' }
  if (/valid final response|schema|structured|validation|prompt .* parameters/.test(value)) return { kind: 'validation', title: 'Response could not be recovered', guidance: 'Automatic cleanup could not produce a usable answer. Try again or choose a more capable model.' }
  if (/openai|ollama|provider|model|api\/chat|connection refused|deadline exceeded|timed out|timeout/.test(value)) return { kind: 'provider', title: 'AI provider unavailable', guidance: 'Check the selected provider credentials, endpoint, model, and supported request settings. Local models may also need more time or memory.' }
  if (/history|quote|market data|common return|observation|coverage/.test(value)) return { kind: 'market_data', title: 'Market data insufficient', guidance: 'The broker did not provide enough current or historical data for this analysis.' }
  if (/ibkr|broker|portfolio access|stock data/.test(value)) return { kind: 'broker', title: 'Broker data unavailable', guidance: 'IBKR could not provide the required portfolio or instrument data.' }
  return { kind: 'unknown', title: 'Analysis unavailable', guidance: 'The analysis could not be completed with the current configuration and data.' }
}
