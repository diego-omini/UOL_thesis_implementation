<script lang="ts">
  // PortfolioDashboard.svelte presents analytics, positions, and read-only counselor actions.
  import { createEventDispatcher, onDestroy } from 'svelte'
  import { GetPortfolioNews, GetPortfolioResume } from '../../wailsjs/go/main/App.js'
  import CounselorResponse from '../components/CounselorResponse.svelte'
  import {
    formatDate,
    formatError,
    formatExchangeRate,
    formatMoney,
    formatNumber,
    formatPercent,
    formatRatio,
  } from '../lib/format'
  import type { AIResponseView, PortfolioView, PositionView } from '../lib/types'

  export let portfolio: PortfolioView

  const dispatch = createEventDispatcher<{ stock: PositionView; sessionIssue: void }>()

  let portfolioResume: AIResponseView | null = null
  let portfolioResumeError = ''
  let portfolioResumeLoading = false
  let portfolioNews: AIResponseView | null = null
  let portfolioNewsError = ''
  let portfolioNewsLoading = false
  let positionsPromptsOpen = false
  let analysisSequence = 0

  onDestroy(() => analysisSequence++)

  function resetAnalysisState(): void {
    portfolioResume = null
    portfolioResumeError = ''
    portfolioResumeLoading = false
    portfolioNews = null
    portfolioNewsError = ''
    portfolioNewsLoading = false
  }

  // getPortfolioResume rejects a response when this dashboard generation is no longer active.
  async function getPortfolioResume(): Promise<void> {
    const sequence = ++analysisSequence
    positionsPromptsOpen = false
    resetAnalysisState()
    portfolioResumeLoading = true
    try {
      const response = await GetPortfolioResume()
      if (sequence === analysisSequence) portfolioResume = response
    } catch (error) {
      if (sequence === analysisSequence) {
        portfolioResumeError = formatError(error)
        dispatch('sessionIssue')
      }
    } finally {
      if (sequence === analysisSequence) portfolioResumeLoading = false
    }
  }

  // getPortfolioNews rejects a response when this dashboard generation is no longer active.
  async function getPortfolioNews(): Promise<void> {
    const sequence = ++analysisSequence
    positionsPromptsOpen = false
    resetAnalysisState()
    portfolioNewsLoading = true
    try {
      const response = await GetPortfolioNews()
      if (sequence === analysisSequence) portfolioNews = response
    } catch (error) {
      if (sequence === analysisSequence) {
        portfolioNewsError = formatError(error)
        dispatch('sessionIssue')
      }
    } finally {
      if (sequence === analysisSequence) portfolioNewsLoading = false
    }
  }
</script>

<section class="summary-grid" aria-label="Account summary">
  <article class="metric"><span>Account</span><strong>{portfolio.AccountSummary.AccountID || '-'}</strong></article>
  <article class="metric"><span>Net liquidation</span><strong>{formatMoney(portfolio.AccountSummary.NetLiquidation)}</strong></article>
  <article class="metric"><span>Cash balance</span><strong>{formatMoney(portfolio.AccountSummary.CashBalance)}</strong></article>
</section>

<div class="analytics-heading">
  <h2>Portfolio analytics</h2>
  <span>Calculated {formatDate(portfolio.Analytics.CalculatedAt)} in {portfolio.Analytics.BaseCurrency || 'unknown currency'}</span>
</div>

<section class="summary-grid" aria-label="Portfolio analytics summary">
  <article class="metric"><span>Net position value</span><strong>{formatMoney(portfolio.Analytics.NetMarketValue)}</strong></article>
  <article class="metric"><span>Gross exposure</span><strong>{formatMoney(portfolio.Analytics.GrossExposure)}</strong></article>
  <article class="metric">
    <span>Unrealized P/L</span>
    <strong class:loss={portfolio.Analytics.TotalUnrealizedPnL.Available && portfolio.Analytics.TotalUnrealizedPnL.Amount < 0} class="pnl">
      {formatMoney(portfolio.Analytics.TotalUnrealizedPnL)}
    </strong>
  </article>
  <article class="metric"><span>Top 3 concentration</span><strong>{formatPercent(portfolio.Analytics.TopThreeConcentration)}</strong></article>
  <article class="metric"><span>Concentration index</span><strong>{formatRatio(portfolio.Analytics.HHI)}</strong></article>
</section>

{#if portfolio.Analytics.ExchangeRates.some(rate => rate.Available && rate.Currency !== rate.BaseCurrency)}
  <section class="exchange-rates" aria-label="Currency conversion rates">
    <strong>Currency conversion</strong>
    {#each portfolio.Analytics.ExchangeRates as rate}
      {#if rate.Available && rate.Currency !== rate.BaseCurrency}
        <div>
          <span>1 {rate.Currency} = {formatExchangeRate(rate.BaseAmountPerUnit)} {rate.BaseCurrency}</span>
          <small>{rate.Source} · {formatDate(rate.AsOf)}</small>
        </div>
      {/if}
    {/each}
  </section>
{/if}

{#if portfolio.Analytics.Warnings.length > 0}
  <section class="warning" aria-label="Portfolio data warnings">
    <strong>Some analytics are limited by the available portfolio data</strong>
    <ul>
      {#each portfolio.Analytics.Warnings as warning}
        <li>{warning.Message}{warning.Count > 1 ? ` (${warning.Count})` : ''}</li>
      {/each}
    </ul>
  </section>
{/if}

{#if !portfolio.Empty && portfolio.Analytics.GrossExposure.Available}
  <section class="allocation-grid" aria-label="Portfolio allocation">
    <article class="allocation-card">
      <h3>By currency</h3>
      {#each portfolio.Analytics.CurrencyAllocation as allocation}
        <div class="allocation-row"><span>{allocation.Label}</span><strong>{formatPercent(allocation.Weight)}</strong><small>{formatMoney(allocation.MarketValue)}</small></div>
      {/each}
    </article>
    <article class="allocation-card">
      <h3>By asset class</h3>
      {#each portfolio.Analytics.AssetClassAllocation as allocation}
        <div class="allocation-row"><span>{allocation.Label}</span><strong>{formatPercent(allocation.Weight)}</strong><small>{formatMoney(allocation.MarketValue)}</small></div>
      {/each}
    </article>
    <article class="allocation-card">
      <h3>By sector</h3>
      {#each portfolio.Analytics.SectorAllocation as allocation}
        <div class="allocation-row"><span>{allocation.Label}</span><strong>{formatPercent(allocation.Weight)}</strong><small>{formatMoney(allocation.MarketValue)}</small></div>
      {/each}
    </article>
    <article class="allocation-card">
      <h3>Largest positions</h3>
      {#each portfolio.Analytics.LargestPositions.slice(0, 5) as position}
        <div class="allocation-row"><span>{position.Symbol || position.Name || '-'}</span><strong>{formatPercent(position.Weight)}</strong><small>{formatMoney(position.MarketValue)}</small></div>
      {/each}
    </article>
  </section>
{/if}

<section class="table-section" aria-label="Positions">
  <div class="section-heading">
    <div class="section-title"><h2>Positions</h2><span>{portfolio.Positions.length} open</span></div>
    <details class="prompt-dropdown" bind:open={positionsPromptsOpen}>
      <summary>Positions prompts</summary>
      <div class="prompt-menu">
        <button disabled={portfolio.Empty || portfolioResumeLoading || portfolioNewsLoading} on:click={getPortfolioResume}>
          {portfolioResumeLoading ? 'Generating portfolio resume...' : 'Get portfolio resume'}
        </button>
        <button disabled={portfolio.Empty || portfolioResumeLoading || portfolioNewsLoading} on:click={getPortfolioNews}>
          {portfolioNewsLoading ? 'Researching portfolio news...' : 'Get portfolio news'}
        </button>
      </div>
    </details>
  </div>

  {#if portfolioResumeLoading || portfolioNewsLoading}
    <div class="ai-status" role="status" aria-live="polite">
      {portfolioNewsLoading ? 'Researching the latest news for every position...' : 'Waiting for the configured AI provider...'}
    </div>
  {:else if portfolioNewsError}
    <div class="ai-error" role="alert" aria-live="assertive"><strong>Portfolio news unavailable</strong><span>{portfolioNewsError}</span></div>
  {:else if portfolioResumeError}
    <div class="ai-error" role="alert" aria-live="assertive"><strong>Portfolio resume unavailable</strong><span>{portfolioResumeError}</span></div>
  {:else if portfolioNews}
    <CounselorResponse response={portfolioNews} />
  {:else if portfolioResume}
    <CounselorResponse response={portfolioResume} />
  {/if}

  {#if portfolio.Empty}
    <div class="empty-state"><strong>No open positions</strong><span>The authenticated paper account has no holdings to display.</span></div>
  {:else}
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Instrument</th><th>Type</th><th class="numeric">Weight</th><th class="numeric">Quantity</th>
            <th class="numeric">Average cost</th><th class="numeric">Market price</th><th class="numeric">Market value</th><th class="numeric">Unrealized P/L</th>
          </tr>
        </thead>
        <tbody>
          {#each portfolio.Positions as position}
            <tr class="position-row">
              <td>
                <button class="position-link" type="button" on:click={() => dispatch('stock', position)} aria-label={`Open ${position.Instrument.Symbol || position.Instrument.Name} stock page`}>
                  <strong>{position.Instrument.Symbol || '-'}</strong>
                  <span>{position.Instrument.Name || position.Instrument.Exchange || ''}</span>
                </button>
              </td>
              <td>{position.Instrument.AssetClass || '-'}<span>{position.Instrument.Sector || position.Instrument.Currency || ''}</span></td>
              <td class="numeric">{formatPercent(position.PortfolioWeight)}</td>
              <td class="numeric">{formatNumber(position.Quantity)}</td>
              <td class="numeric">{formatMoney(position.AverageCost)}</td>
              <td class="numeric">{formatMoney(position.MarketPrice)}</td>
              <td class="numeric">{formatMoney(position.MarketValue)}</td>
              <td class:loss={position.UnrealizedPnL.Available && position.UnrealizedPnL.Amount < 0} class="numeric pnl">{formatMoney(position.UnrealizedPnL)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</section>

<style>
  .summary-grid,
  .analytics-heading,
  .allocation-grid,
  .warning,
  .exchange-rates,
  .table-section {
    max-width: 1180px;
    margin: 0 auto 16px;
  }

  .summary-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 12px;
  }

  .metric {
    display: grid;
    gap: 8px;
    padding: 18px;
    border: 1px solid #d8e0e8;
    border-radius: 6px;
    background: white;
  }

  .metric span { color: #5d6a7a; font-size: 13px; }
  .metric strong { font-size: 22px; }

  .analytics-heading {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 16px;
  }

  .analytics-heading h2,
  .allocation-card h3 { margin: 0; }
  .analytics-heading span { color: #5d6a7a; font-size: 13px; }

  .warning,
  .exchange-rates,
  .empty-state {
    display: grid;
    gap: 6px;
    padding: 14px 16px;
    border-left: 4px solid;
    background: white;
    box-shadow: 0 1px 3px rgba(24, 33, 47, 0.08);
  }

  .warning { border-color: #c98716; }
  .warning ul { margin: 2px 0 0; padding-left: 20px; color: #5d6a7a; }
  .exchange-rates { border-color: #7a8797; }
  .exchange-rates div { display: flex; justify-content: space-between; gap: 16px; }
  .exchange-rates small { color: #5d6a7a; }

  .allocation-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(230px, 1fr));
    gap: 12px;
  }

  .allocation-card {
    padding: 18px;
    border: 1px solid #d8e0e8;
    border-radius: 6px;
    background: white;
  }

  .allocation-card h3 { margin-bottom: 12px; font-size: 16px; }
  .allocation-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 2px 12px; padding: 9px 0; border-top: 1px solid #edf1f5; }
  .allocation-row span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .allocation-row small { grid-column: 1 / -1; color: #6f7b89; }

  .table-section { border: 1px solid #d8e0e8; border-radius: 6px; background: white; }
  .section-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 16px 18px; border-bottom: 1px solid #e5ebf1; }
  .section-heading h2 { margin: 0; font-size: 18px; }
  .section-title { display: flex; align-items: baseline; gap: 10px; }
  .section-title span { color: #5d6a7a; font-size: 14px; }

  .prompt-dropdown { position: relative; }
  .prompt-dropdown summary { min-height: 38px; padding: 0 34px 0 14px; border: 1px solid #b8c4cf; border-radius: 4px; background: white; color: #293646; font-size: 14px; font-weight: 700; line-height: 36px; cursor: pointer; list-style: none; user-select: none; }
  .prompt-dropdown summary::-webkit-details-marker { display: none; }
  .prompt-dropdown summary::after { position: absolute; top: 16px; right: 14px; width: 7px; height: 7px; border-right: 2px solid currentColor; border-bottom: 2px solid currentColor; content: ''; transform: rotate(45deg); }
  .prompt-dropdown[open] summary::after { top: 20px; transform: rotate(225deg); }
  .prompt-menu { position: absolute; z-index: 2; top: calc(100% + 6px); right: 0; min-width: 230px; padding: 6px; border: 1px solid #d8e0e8; border-radius: 4px; background: white; box-shadow: 0 8px 22px rgb(36 52 68 / 16%); }
  .prompt-menu button { width: 100%; padding: 10px 12px; border: 0; border-radius: 3px; background: transparent; color: #293646; font: inherit; font-size: 14px; font-weight: 700; text-align: left; cursor: pointer; }
  .prompt-menu button:hover:not(:disabled), .prompt-menu button:focus-visible { background: #edf5f8; color: #14566e; }
  .prompt-menu button:disabled { cursor: wait; opacity: 0.55; }

  .ai-status,
  .ai-error { margin-top: 16px; padding: 14px 16px; border-left: 4px solid #1f6f8b; background: #f5f9fb; color: #5d6a7a; line-height: 1.55; }
  .ai-error { display: grid; gap: 4px; border-color: #c2414b; background: #fff7f7; }

  .table-wrap { overflow-x: auto; }
  table { width: 100%; min-width: 900px; border-collapse: collapse; }
  th, td { padding: 12px 14px; border-bottom: 1px solid #edf1f5; text-align: left; vertical-align: top; }
  th { color: #5d6a7a; font-size: 12px; font-weight: 700; text-transform: uppercase; }
  td span { display: block; margin-top: 3px; color: #6f7b89; font-size: 12px; }
  tbody tr:last-child td { border-bottom: none; }
  .position-row { transition: background-color 120ms ease; }
  .position-row:hover, .position-row:focus-within { background: #f1f7f9; }
  .position-link { display: block; width: 100%; padding: 0; border: 0; background: transparent; color: inherit; font: inherit; text-align: left; cursor: pointer; }
  .position-link:focus-visible { border-radius: 2px; outline: 2px solid #1f6f8b; outline-offset: 3px; }
  .numeric { text-align: right; white-space: nowrap; }
  .pnl { color: #16835b; font-weight: 700; }
  .pnl.loss { color: #c2414b; }
  .empty-state { margin: 18px; border-color: #7a8797; box-shadow: none; }
  .empty-state span { color: #5d6a7a; line-height: 1.45; }

  @media (max-width: 780px) {
    .summary-grid, .allocation-grid { grid-template-columns: 1fr; }
    .analytics-heading, .section-heading { align-items: stretch; flex-direction: column; }
    .prompt-dropdown summary { width: 100%; box-sizing: border-box; }
    .prompt-menu { right: auto; left: 0; width: 100%; box-sizing: border-box; }
  }
</style>
