// stock-chart.ts contains pure calculations for the responsive stock price chart.
import type { PositionView, StockHistoryRange, StockHistoryView, StockSnapshotView } from './types'

export const chartWidth = 960
export const chartHeight = 280

export function averageCostForChart(position: PositionView | null, snapshot: StockSnapshotView | null): number | null {
  if (!position || position.Quantity === 0 || !position.AverageCost.Available) return null
  const chartCurrency = snapshot?.Currency || position.Instrument.Currency
  const averageCurrency = position.AverageCost.Currency
  if (chartCurrency && averageCurrency && chartCurrency.trim().toUpperCase() !== averageCurrency.trim().toUpperCase()) return null
  return Number.isFinite(position.AverageCost.Amount) && position.AverageCost.Amount > 0
    ? position.AverageCost.Amount
    : null
}

function chartValues(history: StockHistoryView, averageCost: number | null): number[] {
  const closes = history.Points.map(point => point.Close).filter(Number.isFinite)
  if (averageCost !== null) closes.push(averageCost)
  return closes
}

function wholePriceStep(rawStep: number): number {
  const minimumStep = Math.max(1, rawStep)
  const magnitude = 10 ** Math.floor(Math.log10(minimumStep))
  for (const factor of [1, 2.5, 5, 10]) {
    const candidate = factor * magnitude
    if (Number.isInteger(candidate) && candidate >= minimumStep) return candidate
  }
  return 10 * magnitude
}

export function chartPriceAxis(history: StockHistoryView, averageCost: number | null): { min: number; max: number; step: number } {
  const values = chartValues(history, averageCost)
  if (values.length === 0) return { min: 0, max: 1, step: 1 }
  const dataMin = Math.min(...values)
  const dataMax = Math.max(...values)
  const step = wholePriceStep(dataMin === dataMax ? Math.abs(dataMin) * 0.02 : (dataMax - dataMin) / 4)
  let min = Math.floor(dataMin / step) * step
  let max = Math.ceil(dataMax / step) * step
  if (min === dataMin) min -= step
  if (max === dataMax) max += step
  if (dataMin >= 0) min = Math.max(0, min)
  if (min === max) max = min + step
  return { min, max, step }
}

export function formatChartDate(value: string, timeRange: StockHistoryRange): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  if (timeRange === '24h') return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  if (timeRange === '1y') return date.toLocaleDateString([], { month: 'short', year: 'numeric' })
  if (timeRange === 'all') return String(date.getFullYear())
  return date.toLocaleDateString()
}

export function chartTimeTicks(history: StockHistoryView, timeRange: StockHistoryRange): Array<{ label: string; x: number }> {
  const pointCount = history.Points.length
  if (pointCount === 0) return []
  const tickCount = Math.min(5, pointCount)
  if (tickCount === 1) {
    return [{ label: formatChartDate(history.Points[0].Time, timeRange), x: chartWidth / 2 }]
  }
  const indexes = Array.from({ length: tickCount }, (_, index) =>
    Math.round(index * (pointCount - 1) / (tickCount - 1))
  )
  return indexes.map(pointIndex => ({
    label: formatChartDate(history.Points[pointIndex].Time, timeRange),
    x: pointIndex / (pointCount - 1) * chartWidth,
  }))
}

export function chartPolyline(history: StockHistoryView, averageCost: number | null): string {
  if (history.Points.length === 0) return ''
  const { min, max } = chartPriceAxis(history, averageCost)
  return history.Points.map((point, index) => {
    const x = history.Points.length === 1 ? chartWidth / 2 : index / (history.Points.length - 1) * chartWidth
    const y = chartHeight - ((point.Close - min) / (max - min)) * chartHeight
    return `${x.toFixed(2)},${y.toFixed(2)}`
  }).join(' ')
}

export function chartYTicks(history: StockHistoryView, averageCost: number | null): Array<{ value: number; y: number }> {
  const { min, max, step } = chartPriceAxis(history, averageCost)
  const tickCount = Math.round((max - min) / step)
  return Array.from({ length: tickCount + 1 }, (_, index) => {
    const value = max - step * index
    return { value, y: (max - value) / (max - min) * chartHeight }
  })
}

export function averageCostY(history: StockHistoryView, averageCost: number | null): number {
  if (averageCost === null) return 0
  const { min, max } = chartPriceAxis(history, averageCost)
  return chartHeight - (averageCost - min) / (max - min) * chartHeight
}

export function positionChartResult(history: StockHistoryView, averageCost: number | null, position: PositionView | null): 'gain' | 'loss' | null {
  if (!position || averageCost === null || history.Points.length === 0) return null
  const latestClose = history.Points[history.Points.length - 1].Close
  const profitable = position.Quantity < 0 ? latestClose <= averageCost : latestClose >= averageCost
  return profitable ? 'gain' : 'loss'
}
