<script lang="ts">
  // PortfolioHistory.svelte separates account-value observations from locally tracked analytics.
  import { createEventDispatcher, onDestroy, onMount } from 'svelte'
  import { GetPortfolioHistory, GetPortfolioHistorySummary, OpenPortfolioHistoryConnection } from '../../wailsjs/go/main/App.js'
  import CounselorAction from './CounselorAction.svelte'
  import { formatDate, formatError } from '../lib/format'
  import {
    historyChartHeight,
    historyChartWidth,
    accountValueMetrics,
    historyAxisTicks,
    historyDetailRange,
    historyMetrics,
    historyPolyline,
    historyRanges,
    mostRecentSeriesIndex,
    portfolioHistoryChart,
    trackingMetrics,
  } from '../lib/portfolio-history'
  import type { PortfolioHistoryMetric, PortfolioHistoryRange } from '../lib/portfolio-history'
  import type { PortfolioHistoryView } from '../lib/types'
  import { workflowCopy } from '../lib/counselor-workflows'
  import { classifyCounselorError } from '../lib/counselor-errors'

  const dispatch = createEventDispatcher<{ sessionIssue: void }>()

  let selectedRange: PortfolioHistoryRange = '30d'
  let selectedMode: 'value' | 'tracking' = 'value'
  let selectedMetric: PortfolioHistoryMetric = 'netLiquidation'
  let history: PortfolioHistoryView | null = null
  let selectedSeries = 0
  let loading = false
  let errorMessage = ''
  let requestSequence = 0
  let connecting = false

  $: availableSeries = selectedMode === 'value' ? (history?.AccountValueSeries ?? []) : (history?.TrackingSeries ?? [])
  $: availableMetrics = selectedMode === 'value' ? accountValueMetrics : trackingMetrics
  $: series = availableSeries[selectedSeries] ?? null
  $: chartFrom = history?.From ?? ''
  $: chartTo = history?.To ?? ''
  $: chart = series && history ? portfolioHistoryChart(series, selectedMetric, chartFrom, chartTo) : null
  $: axisTicks = chart ? historyAxisTicks(chart) : []
  $: detailRange = chart ? historyDetailRange(chart) : null
  $: detailChart = detailRange && series ? portfolioHistoryChart(series, selectedMetric, detailRange.from, detailRange.to) : null
  $: detailTicks = detailChart ? historyAxisTicks(detailChart, 3) : []
  $: chartPoints = chart ? chart.segments.flat() : []
  $: firstChartPoint = chartPoints[0] ?? null
  $: markerRadius = chart && chart.availableCount <= 5 ? 7 : 4
  $: sourceClasses = Array.from(new Set(chartPoints.map(point => point.source)))
  $: observationDateCount = new Set(chartPoints.map(point => point.capturedAt.slice(0, 10))).size
  $: leadingGap = Boolean(chart?.firstObservation && new Date(chart.firstObservation).getTime() > new Date(chart.from).getTime())
  $: metricDefinition = historyMetrics.find(metric => metric.value === selectedMetric) ?? historyMetrics[0]
  $: selectedPointCount = selectedMode === 'value' ? (history?.AccountValuePointCount ?? 0) : (history?.TrackingPointCount ?? 0)

  onMount(() => {
    const refreshAfterConnection = () => {
      if (history?.ConnectionRequired) void loadHistory()
    }
    window.addEventListener('focus', refreshAfterConnection)
    void loadHistory()
    return () => window.removeEventListener('focus', refreshAfterConnection)
  })
  onDestroy(() => requestSequence++)

  // loadHistory rejects a response after another range request or component destruction.
  async function loadHistory(): Promise<void> {
    const sequence = ++requestSequence
    loading = true
    errorMessage = ''
    try {
      const response = await GetPortfolioHistory(selectedRange)
      if (sequence !== requestSequence) return
      history = response
      selectedSeries = mostRecentSeriesIndex(selectedMode === 'value' ? response.AccountValueSeries : response.TrackingSeries)
    } catch (error) {
      if (sequence === requestSequence) {
        history = null
        errorMessage = formatError(error)
        const kind = classifyCounselorError(errorMessage).kind
        if (kind === 'session' || kind === 'broker') dispatch('sessionIssue')
      }
    } finally {
      if (sequence === requestSequence) loading = false
    }
  }

  function selectRange(range: PortfolioHistoryRange): void {
    if (range === selectedRange) return
    selectedRange = range
    void loadHistory()
  }

  function selectMode(mode: 'value' | 'tracking'): void {
    if (mode === selectedMode) return
    selectedMode = mode
    selectedMetric = mode === 'value' ? 'netLiquidation' : 'cash'
    selectedSeries = mostRecentSeriesIndex(mode === 'value' ? (history?.AccountValueSeries ?? []) : (history?.TrackingSeries ?? []))
  }

  async function connectHistory(): Promise<void> {
    connecting = true
    errorMessage = ''
    try {
      await OpenPortfolioHistoryConnection()
    } catch (error) {
      errorMessage = formatError(error)
    } finally {
      connecting = false
    }
  }

  function formatChartValue(value: number): string {
    if (metricDefinition.ratio) return `${(value * 100).toFixed(1)}%`
    if (metricDefinition.count) return Math.round(value).toLocaleString()
    if (!series?.BaseCurrency) return value.toLocaleString(undefined, { maximumFractionDigits: 2 })
    try {
      return new Intl.NumberFormat(undefined, { style: 'currency', currency: series.BaseCurrency }).format(value)
    } catch {
      return `${value.toLocaleString(undefined, { maximumFractionDigits: 2 })} ${series.BaseCurrency}`
    }
  }

  function formatAxisValue(value: number): string {
    if (metricDefinition.ratio) return `${Math.round(value * 100)}%`
    if (metricDefinition.count) return Math.round(value).toLocaleString()
    if (!series?.BaseCurrency) return Math.round(value).toLocaleString()
    try {
      return new Intl.NumberFormat(undefined, { style: 'currency', currency: series.BaseCurrency, maximumFractionDigits: 0 }).format(value)
    } catch {
      return `${Math.round(value).toLocaleString()} ${series.BaseCurrency}`
    }
  }

  function selectSeries(event: Event): void {
    selectedSeries = Number((event.currentTarget as HTMLSelectElement).value)
  }

  function sourceLabel(source: string, endOfDay: boolean): string {
    if (source === 'ibkr_portfolio_analyst') return 'IBKR PortfolioAnalyst end-of-day'
    if (source === 'snaptrade_balance_history') return 'SnapTrade estimated end-of-day account value'
    return endOfDay ? 'End-of-day observation' : 'Local portfolio capture'
  }

  function sourceClass(source: string): string {
    if (source === 'ibkr_portfolio_analyst') return 'official'
    if (source === 'snaptrade_balance_history') return 'estimated'
    return 'local'
  }

  function formatAxisDate(value: string): string {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return '-'
    if (selectedRange === '1y') return date.toLocaleDateString(undefined, { month: 'short', year: 'numeric' })
    return date.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })
  }

  function formatObservationDate(value: string, endOfDay: boolean): string {
    if (!endOfDay) return formatDate(value)
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? '-' : date.toLocaleDateString()
  }
</script>

<section class="history-card" aria-labelledby="portfolio-history-title">
  <div class="history-heading">
    <div>
      <h2 id="portfolio-history-title">Historical data</h2>
      <span>Account value and locally tracked portfolio analytics are kept separate</span>
    </div>
    <div class="range-buttons" aria-label="Portfolio history range">
      {#each historyRanges as range}
        <button type="button" class:active={selectedRange === range.value} aria-pressed={selectedRange === range.value} on:click={() => selectRange(range.value)}>{range.label}</button>
      {/each}
    </div>
  </div>

  <div class="history-mode" role="group" aria-label="Historical data type">
    <button type="button" class:active={selectedMode === 'value'} aria-pressed={selectedMode === 'value'} on:click={() => selectMode('value')}>Account value</button>
    <button type="button" class:active={selectedMode === 'tracking'} aria-pressed={selectedMode === 'tracking'} on:click={() => selectMode('tracking')}>Tracked analytics</button>
  </div>

  <CounselorAction
    workflow={workflowCopy('portfolio_history_summary')}
    action={() => GetPortfolioHistorySummary(selectedRange)}
    disabled={loading || !history || history.AccountValuePointCount + history.TrackingPointCount === 0}
    unavailableReason={loading ? 'Historical data is still loading.' : 'No observations are available in the selected range.'}
    resetOn={`${selectedRange}:${history?.To ?? ''}`}
    currentContextAsOf={history?.To ?? ''}
    on:sessionIssue={() => dispatch('sessionIssue')}
  />

  <div class="data-definition">
    {#if selectedMode === 'value'}
      <strong>Account value, not investment return</strong>
      <span>These observations do not separate deposits and withdrawals, so percentage performance is not calculated.</span>
    {:else}
      <strong>Available from local tracking only</strong>
      <span>{history?.TrackingStartedAt ? `Tracking began ${formatDate(history.TrackingStartedAt)}. ` : ''}Dates before the first saved snapshot remain unavailable.</span>
    {/if}
  </div>

  {#if loading}
    <div class="history-state" role="status" aria-live="polite">Loading saved portfolio history...</div>
  {:else if errorMessage}
    <div class="history-state error" role="alert"><strong>Portfolio history unavailable</strong><span>{errorMessage}</span></div>
  {:else if !history || selectedPointCount === 0}
    <div class="history-state">
      {#if selectedMode === 'value'}
        <strong>{history?.ConnectionRequired ? 'Optional estimated history connection available' : (history?.ImportWarning ? 'Imported account value unavailable' : 'No account-value observations in this range')}</strong>
        <span>No broker-provided or local account-value observation was stored in this interval.</span>
        {#if history?.ConnectionRequired}<button class="connect" type="button" disabled={connecting} on:click={connectHistory}>{connecting ? 'Opening connection...' : 'Connect optional estimated history'}</button>{/if}
      {:else}
        <strong>No locally tracked analytics in this range</strong>
        <span>Refreshing the portfolio creates a local analytical snapshot. Historical analytics are not inferred from account-value observations.</span>
      {/if}
    </div>
  {:else}
    <div class="history-controls">
      <label>Metric
        <select bind:value={selectedMetric}>
          {#each availableMetrics as metric}<option value={metric.value}>{metric.label}</option>{/each}
        </select>
      </label>
      {#if availableSeries.length > 1}
        <label>Base currency
          <select value={selectedSeries} on:change={selectSeries}>
            {#each availableSeries as value, index}<option value={index}>{value.BaseCurrency || 'Unknown'}</option>{/each}
          </select>
        </label>
      {/if}
    </div>

    {#if series && chart}
      <div class="chart-context">
        <strong>{metricDefinition.label} · {metricDefinition.ratio || metricDefinition.count ? 'unitless' : (series.BaseCurrency || 'unknown base currency')} · {chart.availableCount} observation{chart.availableCount === 1 ? '' : 's'}</strong>
        <span>Selected range: {formatDate(chart.from)} to {formatDate(chart.to)} · {observationDateCount} observed date{observationDateCount === 1 ? '' : 's'}{selectedMode === 'value' ? ' · imported dates are end-of-day' : ''}</span>
      </div>
      <div class="source-legend" aria-label="Portfolio history sources">
        {#each sourceClasses as source}<span class={sourceClass(source)}><i></i>{sourceLabel(source, source !== 'local_snapshot')}</span>{/each}
      </div>
      {#if leadingGap}
        <div class="coverage" role="status">Available observations begin {formatObservationDate(chart.firstObservation, firstChartPoint?.endOfDay ?? false)}. Earlier dates in the selected range remain blank; no account-start date is inferred.</div>
      {/if}
      {#if chart.availableCount === 0}
        <div class="history-state"><strong>This metric is unavailable for every capture</strong><span>The stored availability flags are preserved; unavailable values are not plotted as zero.</span></div>
      {:else}
        <div class="chart-wrap">
          <svg viewBox={`-90 -20 ${historyChartWidth + 110} ${historyChartHeight + 70}`} role="img" aria-label={`${metricDefinition.label} portfolio history in ${series.BaseCurrency || 'unknown base currency'} from ${formatDate(chart.from)} to ${formatDate(chart.to)}. ${chart.availableCount} available observations.`}>
            <title>{metricDefinition.label} portfolio history; unavailable captures appear as gaps.</title>
            <line class="axis" x1="0" y1={historyChartHeight} x2={historyChartWidth} y2={historyChartHeight} />
            <line class="axis" x1="0" y1="0" x2="0" y2={historyChartHeight} />
            <text class="axis-label" x="-12" y="8" text-anchor="end">{formatAxisValue(chart.max)}</text>
            <text class="axis-label" x="-12" y={historyChartHeight / 2 + 4} text-anchor="end">{formatAxisValue((chart.min + chart.max) / 2)}</text>
            <text class="axis-label" x="-12" y={historyChartHeight} text-anchor="end">{formatAxisValue(chart.min)}</text>
            {#each axisTicks as tick, index}
              {#if index > 0}<line class="grid" x1={tick.x} y1="0" x2={tick.x} y2={historyChartHeight} />{/if}
              <text class="axis-label" x={tick.x} y={historyChartHeight + 28} text-anchor={index === 0 ? 'start' : (index === axisTicks.length - 1 ? 'end' : 'middle')}>{formatAxisDate(tick.timestamp)}</text>
            {/each}
            {#each chart.segments as segment}
              {#if segment.length > 1}<polyline class={sourceClass(segment[0].source)} points={historyPolyline(segment)} />{/if}
              {#each segment as point}<circle class={sourceClass(point.source)} cx={point.x} cy={point.y} r={markerRadius}><title>{formatObservationDate(point.capturedAt, point.endOfDay)}: {formatChartValue(point.value)} · {sourceLabel(point.source, point.endOfDay)}</title></circle>{/each}
            {/each}
          </svg>
        </div>
        {#if detailChart}
          <div class="active-detail">
            <strong>Active-period detail</strong>
            <span>The selected range remains fixed above; this inset enlarges only its observed period.</span>
            <div class="chart-wrap detail-chart">
              <svg viewBox={`-90 -20 ${historyChartWidth + 110} ${historyChartHeight + 70}`} role="img" aria-label={`Detailed observed period from ${formatDate(detailChart.from)} to ${formatDate(detailChart.to)}.`}>
                <line class="axis" x1="0" y1={historyChartHeight} x2={historyChartWidth} y2={historyChartHeight} />
                <line class="axis" x1="0" y1="0" x2="0" y2={historyChartHeight} />
                <text class="axis-label" x="-12" y="8" text-anchor="end">{formatAxisValue(detailChart.max)}</text>
                <text class="axis-label" x="-12" y={historyChartHeight} text-anchor="end">{formatAxisValue(detailChart.min)}</text>
                {#each detailTicks as tick, index}
                  {#if index > 0}<line class="grid" x1={tick.x} y1="0" x2={tick.x} y2={historyChartHeight} />{/if}
                  <text class="axis-label" x={tick.x} y={historyChartHeight + 28} text-anchor={index === 0 ? 'start' : (index === detailTicks.length - 1 ? 'end' : 'middle')}>{formatAxisDate(tick.timestamp)}</text>
                {/each}
                {#each detailChart.segments as segment}
                  {#if segment.length > 1}<polyline class={sourceClass(segment[0].source)} points={historyPolyline(segment)} />{/if}
                  {#each segment as point}<circle class={sourceClass(point.source)} cx={point.x} cy={point.y} r={markerRadius}><title>{formatObservationDate(point.capturedAt, point.endOfDay)}: {formatChartValue(point.value)} · {sourceLabel(point.source, point.endOfDay)}</title></circle>{/each}
                {/each}
              </svg>
            </div>
          </div>
        {/if}
        <details class="observation-details">
          <summary>Observation details ({chartPoints.length})</summary>
          <div class="observation-table-wrap"><table><thead><tr><th>Date</th><th>Value</th><th>Source</th></tr></thead><tbody>
            {#each chartPoints as point}<tr><td>{formatObservationDate(point.capturedAt, point.endOfDay)}</td><td>{formatChartValue(point.value)}</td><td>{sourceLabel(point.source, point.endOfDay)}</td></tr>{/each}
          </tbody></table></div>
        </details>
      {/if}
    {/if}

  {/if}
</section>

<style>
  .history-card { max-width: 1144px; margin: 0 auto 16px; padding: 18px; border: 1px solid #d8e0e8; border-radius: 6px; background: white; }
  .history-heading, .history-controls, .chart-context { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
  h2 { margin: 0; }
  .history-heading span, .chart-context span { color: #5d6a7a; font-size: 13px; }
  .range-buttons { display: flex; flex-wrap: wrap; gap: 6px; }
  .range-buttons button { padding: 7px 10px; border: 1px solid #b8c4cf; border-radius: 4px; background: white; color: #293646; cursor: pointer; }
  .range-buttons button.active { border-color: #1f6f8b; background: #e9f4f7; color: #14566e; }
  .history-mode { display: inline-flex; gap: 4px; margin-top: 18px; padding: 4px; border-radius: 6px; background: #edf1f5; }
  .history-mode button { min-height: 36px; padding: 0 14px; border: 0; border-radius: 4px; background: transparent; color: #4c5b6b; font: inherit; font-weight: 700; cursor: pointer; }
  .history-mode button.active { background: white; color: #14566e; box-shadow: 0 1px 3px rgba(24, 33, 47, .14); }
  .data-definition { display: grid; gap: 3px; margin-top: 12px; padding: 11px 13px; border-left: 4px solid #1f6f8b; background: #f5f9fb; }
  .data-definition span { color: #5d6a7a; font-size: 13px; }
  .history-controls { justify-content: flex-start; margin-top: 18px; }
  label { display: grid; gap: 4px; color: #5d6a7a; font-size: 12px; font-weight: 700; }
  select { min-width: 180px; padding: 7px 9px; border: 1px solid #b8c4cf; border-radius: 4px; background: white; color: #293646; font: inherit; }
  .chart-context { align-items: baseline; margin: 16px 0 8px; }
  .chart-wrap { overflow-x: auto; }
  svg { display: block; width: 100%; min-width: 620px; height: auto; }
  .axis { stroke: #9aa7b5; stroke-width: 1; }
  .grid { stroke: #e4e9ee; stroke-width: 1; }
  .axis-label { fill: #5d6a7a; font-size: 12px; }
  polyline { fill: none; stroke-linejoin: round; stroke-width: 3; }
  polyline.official, circle.official { stroke: #1f6f8b; fill: #1f6f8b; }
  polyline.estimated { stroke: #b56a12; stroke-dasharray: 9 6; }
  circle.estimated { fill: white; stroke: #b56a12; stroke-width: 3; }
  polyline.local, circle.local { stroke: #27806f; fill: #27806f; }
  .source-legend { display: flex; flex-wrap: wrap; gap: 12px; color: #5d6a7a; font-size: 12px; }
  .source-legend span { display: inline-flex; align-items: center; gap: 6px; }
  .source-legend i { width: 10px; height: 10px; border-radius: 50%; background: #27806f; }
  .source-legend .official i { background: #1f6f8b; }
  .source-legend .estimated i { box-sizing: border-box; border: 2px solid #b56a12; background: white; }
  .active-detail { display: grid; gap: 4px; margin-top: 14px; padding-top: 12px; border-top: 1px solid #e4e9ee; color: #5d6a7a; font-size: 13px; }
  .detail-chart svg { max-height: 210px; }
  .observation-details { margin-top: 12px; color: #355166; }
  .observation-details summary { cursor: pointer; font-weight: 700; }
  .observation-table-wrap { margin-top: 8px; overflow-x: auto; }
  .observation-details table { width: 100%; border-collapse: collapse; color: #293646; font-size: 13px; }
  .observation-details th, .observation-details td { padding: 7px 8px; border-top: 1px solid #e4e9ee; text-align: left; }
  .history-state { display: grid; gap: 4px; margin-top: 16px; padding: 14px 16px; border-left: 4px solid #7a8797; background: #f6f8fa; color: #5d6a7a; }
  .coverage { margin: 8px 0 4px; color: #5d6a7a; font-size: 13px; }
  .history-state.error { border-color: #c2414b; background: #fff7f7; }
  .connect { justify-self: start; margin-top: 8px; padding: 8px 12px; border: 1px solid #1f6f8b; border-radius: 4px; background: #1f6f8b; color: white; cursor: pointer; }
  .connect:disabled { cursor: wait; opacity: .65; }
  @media (max-width: 780px) { .history-heading, .chart-context { align-items: stretch; flex-direction: column; } }
</style>
