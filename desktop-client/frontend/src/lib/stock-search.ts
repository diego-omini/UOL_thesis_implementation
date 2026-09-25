// stock-search.ts contains deterministic selection rules for stock-search interactions.
import type { InstrumentSearchResultView } from './types'

export function exactTickerMatch(results: InstrumentSearchResultView[], ticker: string): InstrumentSearchResultView | undefined {
  const normalizedTicker = ticker.trim().toUpperCase()
  return results.find(result => result.Symbol.toUpperCase() === normalizedTicker)
}

export function nextActiveResultIndex(currentIndex: number, resultCount: number, direction: 1 | -1): number {
  if (resultCount <= 0) return -1
  if (currentIndex < 0 || currentIndex >= resultCount) return direction === 1 ? 0 : resultCount - 1
  return (currentIndex + direction + resultCount) % resultCount
}
