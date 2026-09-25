// stock-chart.test.ts verifies deterministic chart axes, labels, and position comparisons.
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { chartPriceAxis, chartTimeTicks, positionChartResult } from './stock-chart'
import type { PositionView, StockHistoryView } from './types'

function history(closes: number[]): StockHistoryView {
  return {
    Range: '1d',
    Points: closes.map((close, index) => ({
      Time: `2026-09-${String(index + 1).padStart(2, '0')}T12:00:00Z`,
      Open: close,
      High: close,
      Low: close,
      Close: close,
      Volume: 1,
    })),
    MarketDataStatus: 'R',
    DelaySeconds: 0,
    UnavailableReason: '',
  } as StockHistoryView
}

function position(quantity: number): PositionView {
  return { Quantity: quantity } as PositionView
}

describe('stock chart calculations', () => {
  it('creates whole-number price steps and includes average cost', () => {
    const axis = chartPriceAxis(history([101.2, 108.7]), 95)

    assert.equal(Number.isInteger(axis.step), true)
    assert.ok(axis.min <= 95)
    assert.ok(axis.max > 108.7)
  })

  it('places time labels at both plot edges', () => {
    const ticks = chartTimeTicks(history([1, 2, 3, 4, 5, 6]), '1y')

    assert.equal(ticks[0].x, 0)
    assert.equal(ticks[ticks.length - 1].x, 960)
    assert.match(ticks[0].label, /2026/)
  })

  it('evaluates long and short positions in the correct direction', () => {
    const risingHistory = history([90, 110])

    assert.equal(positionChartResult(risingHistory, 100, position(2)), 'gain')
    assert.equal(positionChartResult(risingHistory, 100, position(-2)), 'loss')
  })
})
