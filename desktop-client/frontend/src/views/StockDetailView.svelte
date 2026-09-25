<script lang="ts">
  // StockDetailView.svelte owns market-data loading and presentation for one selected stock.
  import { createEventDispatcher, onDestroy, onMount } from 'svelte'
  import { GetStockHistory, GetStockNews, GetStockSnapshot } from '../../wailsjs/go/main/App.js'
  import CounselorResponse from '../components/CounselorResponse.svelte'
  import { formatCurrencyAmount, formatDate, formatError, formatMoney, formatNumber, formatOptionalCurrency, formatOptionalNumber, formatPercent } from '../lib/format'
  import {
    averageCostForChart,
    averageCostY,
    chartPolyline,
    chartTimeTicks,
    chartYTicks,
    formatChartDate,
    positionChartResult,
  } from '../lib/stock-chart'
  import type {
    AIResponseView,
    InstrumentSearchResultView,
    OptionalNumberView,
    PositionView,
    StockHistoryRange,
    StockHistoryView,
    StockSnapshotView,
  } from '../lib/types'

  export let stock: InstrumentSearchResultView
  export let position: PositionView | null

  const dispatch = createEventDispatcher<{ back: void; sessionIssue: void }>()

  let snapshot: StockSnapshotView | null = null
  let snapshotLoading = false
  let snapshotError = ''
  let history: StockHistoryView | null = null
  let historyLoading = false
  let historyError = ''
  let selectedRange: StockHistoryRange = '24h'
  let stockNews: AIResponseView | null = null
  let stockNewsLoading = false
  let stockNewsError = ''
  let stockPromptsOpen = false
  let lifecycleSequence = 0
  let historySequence = 0
  let newsSequence = 0
  const ranges: Array<{ value: StockHistoryRange; label: string }> = [
    { value: '24h', label: '24 hours' },
    { value: '1m', label: '1 month' },
    { value: '1y', label: '1 year' },
    { value: 'all', label: 'All' },
  ]

  $: chartAverageCost = averageCostForChart(position, snapshot)

  onMount(() => {
    const lifecycle = lifecycleSequence
    void loadSnapshot(lifecycle)
    void loadHistory(selectedRange, lifecycle)
  })

  onDestroy(() => {
    lifecycleSequence++
    historySequence++
    newsSequence++
  })

  async function loadSnapshot(lifecycle: number): Promise<void> {
    snapshotLoading = true
    snapshotError = ''
    try {
      const result = await GetStockSnapshot(stock.ContractID)
      if (lifecycle === lifecycleSequence) snapshot = result
    } catch (error) {
      if (lifecycle === lifecycleSequence) {
        snapshotError = formatError(error)
        dispatch('sessionIssue')
      }
    } finally {
      if (lifecycle === lifecycleSequence) snapshotLoading = false
    }
  }

  // loadHistory accepts only the latest range request in the active component lifecycle.
  async function loadHistory(timeRange: StockHistoryRange, lifecycle: number): Promise<void> {
    const request = ++historySequence
    historyLoading = true
    historyError = ''
    try {
      const result = await GetStockHistory(stock.ContractID, timeRange)
      if (lifecycle === lifecycleSequence && request === historySequence && timeRange === selectedRange) history = result
    } catch (error) {
      if (lifecycle === lifecycleSequence && request === historySequence && timeRange === selectedRange) {
        history = null
        historyError = formatError(error)
        dispatch('sessionIssue')
      }
    } finally {
      if (lifecycle === lifecycleSequence && request === historySequence && timeRange === selectedRange) historyLoading = false
    }
  }

  function selectRange(timeRange: StockHistoryRange): void {
    if (timeRange === selectedRange) return
    selectedRange = timeRange
    history = null
    void loadHistory(timeRange, lifecycleSequence)
  }

  // getStockNews accepts only the latest request in the active component lifecycle.
  async function getStockNews(): Promise<void> {
    const request = ++newsSequence
    stockPromptsOpen = false
    stockNews = null
    stockNewsError = ''
    stockNewsLoading = true
    try {
      const response = await GetStockNews(stock.ContractID)
      if (request === newsSequence) stockNews = response
    } catch (error) {
      if (request === newsSequence) {
        stockNewsError = formatError(error)
        dispatch('sessionIssue')
      }
    } finally {
      if (request === newsSequence) stockNewsLoading = false
    }
  }

  function stockMetricTitle(value: OptionalNumberView | undefined): string {
    return value?.Available ? '' : value?.UnavailableReason || 'Value unavailable'
  }

  function marketDataStatusLabel(status: string): string {
    const labels: Record<string, string> = {
      R: 'Real-time', D: 'Delayed', Z: 'Frozen', Y: 'Frozen delayed',
      N: 'Not subscribed', i: 'Incomplete', v: 'Vendor-display exempt',
    }
    return labels[status.charAt(0)] || status
  }
</script>

<section class="stock-page" aria-label={`${stock.Symbol} stock details`}>
  <button class="back-button" type="button" on:click={() => dispatch('back')}>← Back to portfolio</button>
  <div class="stock-heading">
    <div>
      <span class="eyebrow">Stock</span>
      <h2>{snapshot?.Instrument.Symbol || stock.Symbol}</h2>
      <p>{snapshot?.Instrument.Name || stock.Name || 'Company name unavailable'}</p>
    </div>
    <div class="stock-heading-actions">
      <div class="stock-badges">
        <span>{snapshot?.Instrument.AssetClass || stock.AssetClass || 'STK'}</span>
        {#if snapshot?.Instrument.Exchange || stock.Exchange}<span>{snapshot?.Instrument.Exchange || stock.Exchange}</span>{/if}
        {#if snapshot?.Currency}<span>{snapshot.Currency}</span>{/if}
      </div>
      <details class="prompt-dropdown" bind:open={stockPromptsOpen}>
        <summary>Stock prompts</summary>
        <div class="prompt-menu">
          <button type="button" disabled={stockNewsLoading} on:click={getStockNews}>
            {stockNewsLoading ? 'Researching stock news...' : 'Get stock news'}
          </button>
        </div>
      </details>
    </div>
  </div>

  {#if stockNewsLoading}
    <div class="ai-status" role="status" aria-live="polite">Researching the latest reliable news for this company...</div>
  {:else if stockNewsError}
    <div class="ai-error" role="alert" aria-live="assertive"><strong>Stock news unavailable</strong><span>{stockNewsError}</span></div>
  {:else if stockNews}
    <CounselorResponse response={stockNews} />
  {/if}

  <section class="stock-chart-card" aria-label="Historical stock price">
    <div class="stock-chart-heading">
      <div>
        <h2>Price history</h2>
        {#if history?.Points.length}
          <span>{formatChartDate(history.Points[0].Time, selectedRange)} – {formatChartDate(history.Points[history.Points.length - 1].Time, selectedRange)}</span>
        {:else}
          <span>IBKR trade-price history</span>
        {/if}
      </div>
      <div class="range-selector" aria-label="Price history range">
        {#each ranges as range}
          <button type="button" class:active={selectedRange === range.value} aria-pressed={selectedRange === range.value} on:click={() => selectRange(range.value)}>{range.label}</button>
        {/each}
      </div>
    </div>

    {#if historyLoading}
      <div class="chart-message" role="status">Loading price history...</div>
    {:else if historyError}
      <div class="chart-message error-text" role="alert">{historyError}</div>
    {:else if history && history.Points.length > 0}
      <div class="chart-area">
        <div class="chart-y-labels" aria-hidden="true">
          {#each chartYTicks(history, chartAverageCost) as tick}
            <span>{formatCurrencyAmount(tick.value, snapshot?.Currency || position?.AverageCost.Currency || 'USD', 0, 0)}</span>
          {/each}
        </div>
        <div class="chart-plot">
          <svg viewBox="0 0 960 280" preserveAspectRatio="none" role="img" aria-label={`${stock.Symbol} closing price chart`}>
            {#each chartYTicks(history, chartAverageCost) as tick}<line class="chart-grid-line" x1="0" y1={tick.y} x2="960" y2={tick.y}></line>{/each}
            {#each chartTimeTicks(history, selectedRange) as tick}<line class="chart-grid-line" x1={tick.x} y1="0" x2={tick.x} y2="280"></line>{/each}
            <polyline points={chartPolyline(history, chartAverageCost)}></polyline>
            {#if chartAverageCost !== null}
              <line
                class="average-cost-line"
                class:gain={positionChartResult(history, chartAverageCost, position) === 'gain'}
                class:loss={positionChartResult(history, chartAverageCost, position) === 'loss'}
                x1="0" y1={averageCostY(history, chartAverageCost)} x2="960" y2={averageCostY(history, chartAverageCost)}
              ></line>
            {/if}
          </svg>
          <div class="chart-x-labels" aria-hidden="true">
            {#each chartTimeTicks(history, selectedRange) as tick}<span style={`left: ${tick.x / 9.6}%`}>{tick.label}</span>{/each}
          </div>
        </div>
      </div>
      <div class="chart-meta">
        <span>{history.Points.length} bars</span>
        {#if chartAverageCost !== null}
          <span class:gain-text={positionChartResult(history, chartAverageCost, position) === 'gain'} class:loss-text={positionChartResult(history, chartAverageCost, position) === 'loss'}>
            Average cost: {formatMoney(position?.AverageCost)}
          </span>
        {/if}
        {#if selectedRange === 'all'}<span>Maximum available range: 15 years</span>{/if}
        {#if history.DelaySeconds > 0}<span>Delayed {history.DelaySeconds / 60} minutes</span>{/if}
      </div>
    {:else}
      <div class="chart-message">{history?.UnavailableReason || 'No historical prices are available.'}</div>
    {/if}
  </section>

  {#if snapshotLoading}
    <section class="stock-data-status" role="status">Loading current market data...</section>
  {:else if snapshotError}
    <section class="stock-data-status error-text" role="alert">{snapshotError}</section>
  {:else if snapshot}
    <section class="stock-metrics-grid" aria-label="Current stock data">
      <article class="stock-metric" title={stockMetricTitle(snapshot.OpenPrice)}><span>Last open</span><strong>{formatOptionalCurrency(snapshot.OpenPrice, snapshot.Currency)}</strong></article>
      <article class="stock-metric" title={stockMetricTitle(snapshot.ClosePrice)}><span>Last close</span><strong>{formatOptionalCurrency(snapshot.ClosePrice, snapshot.Currency)}</strong></article>
      <article class="stock-metric" title={stockMetricTitle(snapshot.BidPrice)}><span>Bid</span><strong>{formatOptionalCurrency(snapshot.BidPrice, snapshot.Currency)}</strong></article>
      <article class="stock-metric" title={stockMetricTitle(snapshot.AskPrice)}><span>Ask</span><strong>{formatOptionalCurrency(snapshot.AskPrice, snapshot.Currency)}</strong></article>
      <article class="stock-metric" title={stockMetricTitle(snapshot.Volume)}><span>Volume</span><strong>{formatOptionalNumber(snapshot.Volume, 0)}</strong></article>
      <article class="stock-metric" title={stockMetricTitle(snapshot.High52Week)}><span>52-week high</span><strong>{formatOptionalCurrency(snapshot.High52Week, snapshot.Currency)}</strong></article>
      <article class="stock-metric" title={stockMetricTitle(snapshot.Low52Week)}><span>52-week low</span><strong>{formatOptionalCurrency(snapshot.Low52Week, snapshot.Currency)}</strong></article>
    </section>
    <div class="stock-data-footnote">
      <span>Updated {formatDate(snapshot.AsOf)}</span>
      {#if snapshot.MarketDataStatus}<span>IBKR availability: {marketDataStatusLabel(snapshot.MarketDataStatus)}</span>{/if}
      <span>Hover unavailable values for details.</span>
    </div>
  {/if}

  {#if position}
    <section class="stock-position-card" aria-label="Current portfolio position">
      <div class="section-heading"><div class="section-title"><h2>Current position</h2><span>{formatPercent(position.PortfolioWeight)} of portfolio</span></div></div>
      <div class="stock-position-grid">
        <div><span>Quantity</span><strong>{formatNumber(position.Quantity)}</strong></div>
        <div><span>Average cost</span><strong>{formatMoney(position.AverageCost)}</strong></div>
        <div><span>Market price</span><strong>{formatMoney(position.MarketPrice)}</strong></div>
        <div><span>Market value</span><strong>{formatMoney(position.MarketValue)}</strong></div>
        <div><span>Unrealized P/L</span><strong class:loss={position.UnrealizedPnL.Available && position.UnrealizedPnL.Amount < 0} class="pnl">{formatMoney(position.UnrealizedPnL)}</strong></div>
      </div>
    </section>
  {:else}
    <section class="stock-not-held">This stock is not part of the current portfolio. Market data and trading controls will be added in their dedicated milestones.</section>
  {/if}
</section>

<style>
  .stock-page { display: grid; gap: 16px; max-width: 1180px; margin: 0 auto; }
  .back-button { width: max-content; padding: 0; border: 0; background: transparent; color: #1f6f8b; font: inherit; font-weight: 700; cursor: pointer; }
  .eyebrow { color: #1f6f8b; font-size: 12px; font-weight: 800; letter-spacing: 0.08em; text-transform: uppercase; }
  .stock-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 24px; padding: 26px; border: 1px solid #d8e0e8; border-radius: 8px; background: white; }
  .stock-heading h2 { margin: 4px 0 0; font-size: 36px; }
  .stock-heading p { margin: 5px 0 0; color: #5d6a7a; }
  .stock-heading-actions { display: grid; justify-items: end; gap: 12px; }
  .stock-badges { display: flex; gap: 8px; }
  .stock-badges span { padding: 5px 9px; border-radius: 999px; background: #edf5f8; color: #14566e; font-size: 12px; font-weight: 800; }
  .prompt-dropdown { position: relative; }
  .prompt-dropdown summary { min-height: 38px; padding: 0 34px 0 14px; border: 1px solid #b8c4cf; border-radius: 4px; background: white; color: #293646; font-size: 14px; font-weight: 700; line-height: 36px; cursor: pointer; list-style: none; user-select: none; }
  .prompt-dropdown summary::-webkit-details-marker { display: none; }
  .prompt-dropdown summary::after { position: absolute; top: 16px; right: 14px; width: 7px; height: 7px; border-right: 2px solid currentColor; border-bottom: 2px solid currentColor; content: ''; transform: rotate(45deg); }
  .prompt-dropdown[open] summary::after { top: 20px; transform: rotate(225deg); }
  .prompt-menu { position: absolute; z-index: 2; top: calc(100% + 6px); right: 0; min-width: 230px; padding: 6px; border: 1px solid #d8e0e8; border-radius: 4px; background: white; box-shadow: 0 8px 22px rgb(36 52 68 / 16%); }
  .prompt-menu button { width: 100%; padding: 10px 12px; border: 0; border-radius: 3px; background: transparent; color: #293646; font: inherit; font-size: 14px; font-weight: 700; text-align: left; cursor: pointer; }
  .prompt-menu button:hover:not(:disabled), .prompt-menu button:focus-visible { background: #edf5f8; color: #14566e; }
  .prompt-menu button:disabled { cursor: wait; opacity: 0.55; }
  .ai-status, .ai-error { padding: 14px 16px; border-left: 4px solid #1f6f8b; background: #f5f9fb; color: #5d6a7a; line-height: 1.55; }
  .ai-error { display: grid; gap: 4px; border-color: #c2414b; background: #fff7f7; }
  .stock-chart-card { overflow: hidden; border: 1px solid #d8e0e8; border-radius: 8px; background: white; }
  .stock-chart-heading { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 18px 20px; border-bottom: 1px solid #edf1f5; }
  .stock-chart-heading > div:first-child { display: grid; gap: 4px; }
  .stock-chart-heading h2 { margin: 0; font-size: 18px; }
  .stock-chart-heading span, .chart-meta, .stock-data-footnote { color: #6f7b89; font-size: 12px; }
  .range-selector { display: flex; gap: 4px; padding: 3px; border-radius: 6px; background: #edf1f5; }
  .range-selector button { min-height: 32px; padding: 0 11px; border: 0; border-radius: 4px; background: transparent; color: #4c5b6b; font: inherit; font-size: 13px; font-weight: 700; cursor: pointer; }
  .range-selector button:hover, .range-selector button:focus-visible { color: #14566e; outline: 2px solid rgba(31, 111, 139, 0.25); }
  .range-selector button.active { background: white; color: #14566e; box-shadow: 0 1px 3px rgba(24, 33, 47, 0.12); }
  .chart-area { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 12px; height: 326px; padding: 22px 20px 8px; }
  .chart-y-labels { display: flex; flex-direction: column; justify-content: space-between; padding-bottom: 30px; color: #6f7b89; font-size: 11px; }
  .chart-plot { display: grid; grid-template-rows: 280px 30px; min-width: 0; }
  .chart-plot svg { width: 100%; height: 280px; overflow: visible; }
  .chart-grid-line { stroke: #e7edf2; stroke-width: 1; vector-effect: non-scaling-stroke; }
  .chart-plot .average-cost-line { stroke: #b67b12; stroke-dasharray: 7 5; stroke-width: 2; vector-effect: non-scaling-stroke; }
  .chart-plot .average-cost-line.gain { stroke: #237a4b; }
  .chart-plot .average-cost-line.loss { stroke: #b33b47; }
  .chart-plot polyline { fill: none; stroke: #1f6f8b; stroke-linecap: round; stroke-linejoin: round; stroke-width: 2.5; vector-effect: non-scaling-stroke; }
  .chart-x-labels { position: relative; color: #6f7b89; font-size: 11px; }
  .chart-x-labels span { position: absolute; bottom: 0; transform: translateX(-50%); white-space: nowrap; }
  .chart-x-labels span:first-child { transform: none; }
  .chart-x-labels span:last-child { transform: translateX(-100%); }
  .gain-text { color: #237a4b; font-weight: 700; }
  .loss-text { color: #b33b47; font-weight: 700; }
  .chart-message, .stock-data-status { display: grid; min-height: 180px; place-items: center; padding: 24px; color: #6f7b89; text-align: center; }
  .chart-meta, .stock-data-footnote { display: flex; flex-wrap: wrap; gap: 8px 18px; padding: 0 20px 14px; }
  .stock-metrics-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
  .stock-metric { display: grid; gap: 8px; min-width: 0; padding: 18px; border: 1px solid #d8e0e8; border-radius: 6px; background: white; }
  .stock-metric span { color: #5d6a7a; font-size: 13px; }
  .stock-metric strong { overflow: hidden; font-size: 21px; text-overflow: ellipsis; white-space: nowrap; }
  .stock-data-footnote { padding-top: 0; }
  .error-text { color: #a62f3a; }
  .stock-position-card { border: 1px solid #d8e0e8; border-radius: 8px; background: white; }
  .section-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 16px 18px; border-bottom: 1px solid #e5ebf1; }
  .section-heading h2 { margin: 0; font-size: 18px; }
  .section-title { display: flex; align-items: baseline; gap: 10px; }
  .section-title span { color: #5d6a7a; font-size: 14px; }
  .stock-position-grid { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 1px; background: #e5ebf1; }
  .stock-position-grid div { display: grid; gap: 7px; padding: 18px; background: white; }
  .stock-position-grid span { color: #5d6a7a; font-size: 13px; }
  .stock-not-held { padding: 18px; border-left: 4px solid #1f6f8b; background: white; color: #5d6a7a; line-height: 1.5; }
  .pnl { color: #16835b; font-weight: 700; }
  .pnl.loss { color: #c2414b; }

  @media (max-width: 780px) {
    .stock-heading, .stock-chart-heading, .section-heading { align-items: stretch; flex-direction: column; }
    .stock-heading-actions { justify-items: stretch; }
    .stock-badges { flex-wrap: wrap; }
    .prompt-dropdown summary { width: 100%; box-sizing: border-box; }
    .prompt-menu { right: auto; left: 0; width: 100%; box-sizing: border-box; }
    .stock-metrics-grid, .stock-position-grid { grid-template-columns: 1fr; }
    .range-selector { display: grid; grid-template-columns: repeat(4, 1fr); }
    .range-selector button { padding: 0 6px; }
    .chart-area { height: 250px; padding-right: 12px; padding-left: 12px; }
    .chart-plot { grid-template-rows: 220px 30px; }
    .chart-plot svg { height: 220px; }
  }
</style>
