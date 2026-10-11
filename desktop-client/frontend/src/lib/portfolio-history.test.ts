// portfolio-history.test.ts verifies history ranges and charts without a browser runtime.
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { accountValueMetrics, historyAxisTicks, historyChartHeight, historyChartWidth, historyDetailRange, historyRanges, isHistoryRange, portfolioHistoryChart, trackingMetrics } from './portfolio-history'
import type { PortfolioHistorySeriesView } from './types'

function series(values: Array<number | null>): PortfolioHistorySeriesView {
  return {
    BaseCurrency: 'CHF',
    Points: values.map((value, index) => ({
      CapturedAt: `2026-10-0${index + 1}T12:00:00Z`,
      Source: 'local_snapshot',
      EndOfDay: false,
      NetLiquidation: { Value: value ?? 0, Currency: 'CHF', Available: value !== null },
      Cash: { Value: 0, Currency: 'CHF', Available: false },
      GrossExposure: { Value: 0, Currency: 'CHF', Available: false },
      NetMarketValue: { Value: 0, Currency: 'CHF', Available: false },
      UnrealizedPnL: { Value: 0, Currency: 'CHF', Available: false },
      TopThreeConcentration: { Value: 0, Currency: '', Available: false },
      HHI: { Value: 0, Currency: '', Available: false },
      PositionCount: { Value: index, Currency: '', Available: true },
      Warnings: [],
    })),
  } as PortfolioHistorySeriesView
}

describe('portfolio history range selection', () => {
  it('exposes only backend-supported bounded ranges', () => {
  assert.deepEqual(historyRanges.map(range => range.value), ['7d', '30d', '90d', '1y'])
    assert.equal(isHistoryRange('90d'), true)
  assert.equal(isHistoryRange('available'), false)
    assert.equal(isHistoryRange('all'), false)
  })
})

describe('portfolio history data products', () => {
  it('keeps imported account value separate from locally tracked analytics', () => {
    assert.deepEqual(accountValueMetrics.map(metric => metric.value), ['netLiquidation'])
    assert.equal(trackingMetrics.some(metric => metric.value === 'netLiquidation'), false)
    assert.deepEqual(trackingMetrics.map(metric => metric.value), [
      'cash', 'grossExposure', 'netMarketValue', 'unrealizedPnL', 'concentration', 'positionCount',
    ])
  })
})

describe('portfolio history chart calculations', () => {
  it('uses actual capture time at both chart edges', () => {
    const chart = portfolioHistoryChart(series([100, 110, 120]), 'netLiquidation', '2026-10-01T12:00:00Z', '2026-10-03T12:00:00Z')

    const points = chart.segments.flat()
    assert.equal(points[0].x, 0)
    assert.equal(points[2].x, historyChartWidth)
  })

  it('splits unavailable observations instead of plotting them as zero', () => {
    const chart = portfolioHistoryChart(series([100, null, 120]), 'netLiquidation', '2026-10-01T12:00:00Z', '2026-10-03T12:00:00Z')

    assert.equal(chart.availableCount, 2)
    assert.equal(chart.segments.length, 2)
    assert.deepEqual(chart.segments.flat().map(point => point.value), [100, 120])
  })

  it('keeps the selected range visible before the first observation', () => {
    const chart = portfolioHistoryChart(series([100, 110, 120]), 'netLiquidation', '2026-09-01T12:00:00Z', '2026-10-03T12:00:00Z')

    assert.equal(chart.from, '2026-09-01T12:00:00.000Z')
    assert.ok(chart.segments[0][0].x > 0)
    assert.equal(chart.firstObservation, '2026-10-01T12:00:00.000Z')
  })

  it('creates deterministic labels across the complete selected range', () => {
    const chart = portfolioHistoryChart(series([100, 110, 120]), 'netLiquidation', '2026-09-01T12:00:00Z', '2026-10-03T12:00:00Z')
    const ticks = historyAxisTicks(chart)

    assert.equal(ticks.length, 5)
    assert.equal(ticks[0].x, 0)
    assert.equal(ticks[4].x, historyChartWidth)
    assert.equal(ticks[0].timestamp, chart.from)
    assert.equal(ticks[4].timestamp, chart.to)
  })

  it('does not connect local observations across unobserved days', () => {
    const chart = portfolioHistoryChart(series([100, 110]), 'netLiquidation', '2026-10-01T12:00:00Z', '2026-10-02T12:00:00Z')

    assert.equal(chart.segments.length, 2)
    assert.deepEqual(chart.segments.map(segment => segment.length), [1, 1])
  })

  it('keeps small changes away from both vertical edges', () => {
    const chart = portfolioHistoryChart(series([1_000_000, 1_000_100]), 'netLiquidation', '2026-10-01T12:00:00Z', '2026-10-02T12:00:00Z')
    const points = chart.segments.flat()

    assert.ok(points.every(point => point.y > 0 && point.y < historyChartHeight))
    assert.ok(Math.abs(points[0].y - points[1].y) < historyChartHeight / 4)
  })

  it('uses whole power-of-ten keypoints for large monetary values', () => {
    const chart = portfolioHistoryChart(series([1_009_493.375, 1_010_120.25]), 'netLiquidation', '2026-10-01T12:00:00Z', '2026-10-02T12:00:00Z')

    assert.equal(chart.min, 1_000_000)
    assert.equal((chart.min + chart.max) / 2, 1_010_000)
    assert.equal(chart.max, 1_020_000)
  })

  it('creates a readable active-period detail range for one point', () => {
    const chart = portfolioHistoryChart(series([100]), 'netLiquidation', '2026-09-01T12:00:00Z', '2026-10-02T12:00:00Z')
    const detail = historyDetailRange(chart)

    assert.ok(detail)
    assert.ok(new Date(detail!.from).getTime() < new Date(chart.firstObservation).getTime())
    assert.ok(new Date(detail!.to).getTime() > new Date(chart.lastObservation).getTime())
  })

  it('renders one point as one finite centred marker', () => {
    const chart = portfolioHistoryChart(series([100]), 'netLiquidation', '2026-09-01T12:00:00Z', '2026-10-02T12:00:00Z')
    const point = chart.segments[0][0]

    assert.equal(chart.availableCount, 1)
    assert.ok(Number.isFinite(point.x) && Number.isFinite(point.y))
    assert.equal(point.y, historyChartHeight / 2)
  })

  it('breaks a line when the observation source changes', () => {
    const mixed = series([100, 101])
    mixed.Points[0].Source = 'ibkr_portfolio_analyst'
    mixed.Points[0].EndOfDay = true
    mixed.Points[1].Source = 'snaptrade_balance_history'
    mixed.Points[1].EndOfDay = true
    const chart = portfolioHistoryChart(mixed, 'netLiquidation', '2026-10-01T12:00:00Z', '2026-10-02T12:00:00Z')

    assert.deepEqual(chart.segments.map(segment => segment.length), [1, 1])
  })
})
