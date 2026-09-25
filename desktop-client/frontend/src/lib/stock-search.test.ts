// stock-search.test.ts verifies exact-ticker and wraparound keyboard-selection behavior.
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { exactTickerMatch, nextActiveResultIndex } from './stock-search'
import type { InstrumentSearchResultView } from './types'

const results = [
  { ContractID: 1, Symbol: 'AAPL', Name: 'Apple', Exchange: 'NASDAQ', AssetClass: 'STK' },
  { ContractID: 2, Symbol: 'APLE', Name: 'Apple Hospitality', Exchange: 'NYSE', AssetClass: 'STK' },
] as InstrumentSearchResultView[]

describe('stock search selection', () => {
  it('matches ticker input case-insensitively without selecting partial matches', () => {
    assert.equal(exactTickerMatch(results, ' aapl ')?.ContractID, 1)
    assert.equal(exactTickerMatch(results, 'app'), undefined)
  })

  it('wraps keyboard selection in both directions', () => {
    assert.equal(nextActiveResultIndex(-1, 2, 1), 0)
    assert.equal(nextActiveResultIndex(-1, 2, -1), 1)
    assert.equal(nextActiveResultIndex(1, 2, 1), 0)
    assert.equal(nextActiveResultIndex(0, 2, -1), 1)
    assert.equal(nextActiveResultIndex(0, 0, 1), -1)
  })
})
