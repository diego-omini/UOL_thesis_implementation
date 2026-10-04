// portfolio-history.ts contains pure range, metric, and chart calculations for saved portfolio snapshots.
import type {
  PortfolioHistoryMetricView,
  PortfolioHistoryPointView,
  PortfolioHistorySeriesView,
} from './types'

export const historyChartWidth = 960
export const historyChartHeight = 260

export const historyRanges = [
  { value: '7d', label: '7 days' },
  { value: '30d', label: '30 days' },
  { value: '90d', label: '90 days' },
  { value: '1y', label: '1 year' },
] as const

export type PortfolioHistoryRange = typeof historyRanges[number]['value']
export type PortfolioHistoryMetric =
  | 'netLiquidation'
  | 'cash'
  | 'grossExposure'
  | 'netMarketValue'
  | 'unrealizedPnL'
  | 'concentration'
  | 'positionCount'

export const accountValueMetrics: ReadonlyArray<{ value: PortfolioHistoryMetric; label: string; ratio?: boolean; count?: boolean }> = [
  { value: 'netLiquidation', label: 'Account value / net liquidation' },
]

export const trackingMetrics: ReadonlyArray<{ value: PortfolioHistoryMetric; label: string; ratio?: boolean; count?: boolean }> = [
  { value: 'cash', label: 'Cash' },
  { value: 'grossExposure', label: 'Gross exposure' },
  { value: 'netMarketValue', label: 'Net market value' },
  { value: 'unrealizedPnL', label: 'Unrealized P/L' },
  { value: 'concentration', label: 'Top 3 concentration', ratio: true },
  { value: 'positionCount', label: 'Position count', count: true },
]

export const historyMetrics = [...accountValueMetrics, ...trackingMetrics]

export interface HistoryChartPoint {
  x: number
  y: number
  value: number
  capturedAt: string
  source: string
  endOfDay: boolean
}

export interface HistoryChart {
  segments: HistoryChartPoint[][]
  min: number
  max: number
  from: string
  to: string
  firstObservation: string
  lastObservation: string
  availableCount: number
}

export interface HistoryAxisTick {
  x: number
  timestamp: string
}

export interface HistoryDetailRange {
  from: string
  to: string
}

export function isHistoryRange(value: string): value is PortfolioHistoryRange {
  return historyRanges.some(range => range.value === value)
}

export function historyMetric(point: PortfolioHistoryPointView, metric: PortfolioHistoryMetric): PortfolioHistoryMetricView {
  switch (metric) {
    case 'netLiquidation': return point.NetLiquidation
    case 'cash': return point.Cash
    case 'grossExposure': return point.GrossExposure
    case 'netMarketValue': return point.NetMarketValue
    case 'unrealizedPnL': return point.UnrealizedPnL
    case 'concentration': return point.TopThreeConcentration
    case 'positionCount': return point.PositionCount
  }
}

// portfolioHistoryChart positions observations inside the selected range and splits lines at unavailable values.
export function portfolioHistoryChart(
  series: PortfolioHistorySeriesView,
  metric: PortfolioHistoryMetric,
  rangeFrom: string,
  rangeTo: string,
): HistoryChart {
  if (series.Points.length === 0) {
    return {
      segments: [], min: 0, max: 1, from: rangeFrom, to: rangeTo,
      firstObservation: '', lastObservation: '', availableCount: 0,
    }
  }
  const times = series.Points.map(point => new Date(point.CapturedAt).getTime())
  const validTimes = times.filter(Number.isFinite)
  const firstStoredTime = validTimes.length > 0 ? Math.min(...validTimes) : 0
  const lastStoredTime = validTimes.length > 0 ? Math.max(...validTimes) : firstStoredTime
  const requestedFrom = new Date(rangeFrom).getTime()
  const requestedTo = new Date(rangeTo).getTime()
  const validRequestedRange = Number.isFinite(requestedFrom) && Number.isFinite(requestedTo) && requestedTo > requestedFrom
  const fromTime = validRequestedRange ? requestedFrom : firstStoredTime
  const toTime = validRequestedRange ? requestedTo : lastStoredTime
  const available = series.Points.map((point, index) => ({ value: historyMetric(point, metric), timestamp: times[index] }))
    .filter(item => item.value.Available && Number.isFinite(item.value.Value) && Number.isFinite(item.timestamp))
  const values = available.map(item => item.value.Value)
  if (values.length === 0) {
    return {
      segments: [], min: 0, max: 1,
      from: new Date(fromTime).toISOString(), to: new Date(toTime).toISOString(),
      firstObservation: '', lastObservation: '',
      availableCount: 0,
    }
  }
  const firstObservationTime = Math.min(...available.map(item => item.timestamp))
  const lastObservationTime = Math.max(...available.map(item => item.timestamp))
  const observedMin = Math.min(...values)
  const observedMax = Math.max(...values)
  const centre = (observedMin + observedMax) / 2
  const observedSpan = observedMax - observedMin
  const minimumSpan = metric === 'concentration'
    ? 0.05
    : metric === 'positionCount'
      ? 2
      : Math.max(Math.abs(centre) * 0.01, 1)
  const span = Math.max(observedSpan * 1.2, minimumSpan)
  const min = centre - span / 2
  const max = centre + span / 2

  const segments: HistoryChartPoint[][] = []
  let active: HistoryChartPoint[] = []
  let previous: HistoryChartPoint | null = null
  series.Points.forEach((point, index) => {
    const value = historyMetric(point, metric)
    const timestamp = times[index]
    if (!value.Available || !Number.isFinite(value.Value) || !Number.isFinite(timestamp)) {
      if (active.length > 0) segments.push(active)
      active = []
      previous = null
      return
    }
    const position = toTime === fromTime ? 0.5 : (timestamp - fromTime) / (toTime - fromTime)
    const x = Math.max(0, Math.min(historyChartWidth, position * historyChartWidth))
    const y = historyChartHeight - (value.Value - min) / (max - min) * historyChartHeight
    const chartPoint = { x, y, value: value.Value, capturedAt: point.CapturedAt, source: point.Source, endOfDay: point.EndOfDay }
    if (previous && !observationsAreContinuous(previous, chartPoint)) {
      if (active.length > 0) segments.push(active)
      active = []
    }
    active.push(chartPoint)
    previous = chartPoint
  })
  if (active.length > 0) segments.push(active)
  return {
    segments, min, max,
    from: new Date(fromTime).toISOString(), to: new Date(toTime).toISOString(),
    firstObservation: new Date(firstObservationTime).toISOString(),
    lastObservation: new Date(lastObservationTime).toISOString(),
    availableCount: values.length,
  }
}

function observationsAreContinuous(previous: HistoryChartPoint, current: HistoryChartPoint): boolean {
  if (previous.source !== current.source || previous.endOfDay !== current.endOfDay) return false
  if (!previous.endOfDay) return false
  const elapsed = new Date(current.capturedAt).getTime() - new Date(previous.capturedAt).getTime()
  if (!Number.isFinite(elapsed) || elapsed <= 0) return false
  return elapsed <= 36 * 60 * 60 * 1000
}

// historyDetailRange expands the observed active period slightly without changing the selected main range.
export function historyDetailRange(chart: HistoryChart): HistoryDetailRange | null {
  if (chart.availableCount === 0) return null
  const first = new Date(chart.firstObservation).getTime()
  const last = new Date(chart.lastObservation).getTime()
  if (!Number.isFinite(first) || !Number.isFinite(last)) return null
  const selectedFrom = new Date(chart.from).getTime()
  const selectedTo = new Date(chart.to).getTime()
  const selectedSpan = selectedTo - selectedFrom
  const activeSpan = last - first
  if (chart.availableCount > 5 && activeSpan >= selectedSpan * 0.35) return null
  const padding = activeSpan > 0 ? Math.max(activeSpan * 0.05, 30 * 60 * 1000) : 12 * 60 * 60 * 1000
  return {
    from: new Date(Math.max(selectedFrom, first - padding)).toISOString(),
    to: new Date(Math.min(selectedTo, last + padding)).toISOString(),
  }
}

// historyAxisTicks returns evenly spaced timestamps across the selected range.
export function historyAxisTicks(chart: HistoryChart, count = 5): HistoryAxisTick[] {
  const from = new Date(chart.from).getTime()
  const to = new Date(chart.to).getTime()
  if (!Number.isFinite(from) || !Number.isFinite(to) || to <= from || count < 2) return []
  return Array.from({ length: count }, (_, index) => ({
    x: index * historyChartWidth / (count - 1),
    timestamp: new Date(from + index * (to - from) / (count - 1)).toISOString(),
  }))
}

export function historyPolyline(points: HistoryChartPoint[]): string {
  return points.map(point => `${point.x.toFixed(2)},${point.y.toFixed(2)}`).join(' ')
}

export function mostRecentSeriesIndex(series: PortfolioHistorySeriesView[]): number {
  let selected = 0
  let latest = Number.NEGATIVE_INFINITY
  series.forEach((value, index) => {
    const timestamp = new Date(value.Points[value.Points.length - 1]?.CapturedAt ?? '').getTime()
    if (Number.isFinite(timestamp) && timestamp > latest) {
      selected = index
      latest = timestamp
    }
  })
  return selected
}
