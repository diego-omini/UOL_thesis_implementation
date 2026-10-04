<script lang="ts">
  // PortfolioRiskAnalysis.svelte loads and presents deterministic modeled portfolio risk.
  import { createEventDispatcher, onDestroy } from 'svelte'
  import { GetPortfolioRiskAnalysis, GetPortfolioRiskCounsel } from '../../wailsjs/go/main/App.js'
  import CounselorResponse from './CounselorResponse.svelte'
  import { formatDate, formatError, formatMoney, formatPercent } from '../lib/format'
  import type { AIResponseView, PortfolioRiskAnalysisView } from '../lib/types'

  const dispatch = createEventDispatcher<{ sessionIssue: void }>()

  let result: PortfolioRiskAnalysisView | null = null
  let counselorResponse: AIResponseView | null = null
  let loading = false
  let counselorLoading = false
  let errorMessage = ''
  let counselorError = ''
  let sequence = 0

  onDestroy(() => sequence++)

  async function calculate(): Promise<void> {
    const request = ++sequence
    loading = true
    errorMessage = ''
    counselorResponse = null
    counselorError = ''
    try {
      const response = await GetPortfolioRiskAnalysis()
      if (request === sequence) result = response
    } catch (error) {
      if (request === sequence) {
        errorMessage = formatError(error)
        dispatch('sessionIssue')
      }
    } finally {
      if (request === sequence) loading = false
    }
  }

  // explain refreshes the calculation so the displayed figures exactly match the counselor facts.
  async function explain(): Promise<void> {
    const request = ++sequence
    counselorLoading = true
    counselorError = ''
    counselorResponse = null
    try {
      const response = await GetPortfolioRiskCounsel()
      if (request === sequence) {
        result = response.Analysis
        counselorResponse = response.Response
      }
    } catch (error) {
      if (request === sequence) {
        counselorError = formatError(error)
        dispatch('sessionIssue')
      }
    } finally {
      if (request === sequence) counselorLoading = false
    }
  }

  function ratio(value: { Value: number; Available: boolean }): string {
    return value.Available ? `${(value.Value * 100).toFixed(2)}%` : 'Unavailable'
  }
</script>

<section class="risk-card" aria-labelledby="portfolio-risk-title">
  <div class="risk-heading">
    <div><h2 id="portfolio-risk-title">Modeled portfolio risk</h2><span>Current signed weights with one year of broker price history</span></div>
    <div class="risk-actions">
      <button type="button" disabled={loading || counselorLoading} on:click={calculate}>{loading ? 'Calculating...' : (result ? 'Recalculate' : 'Calculate risk')}</button>
      {#if result && result.Observations >= 20}<button class="secondary" type="button" disabled={loading || counselorLoading} on:click={explain}>{counselorLoading ? 'Preparing explanation...' : 'Explain with counselor'}</button>{/if}
    </div>
  </div>

  {#if errorMessage}<div class="state error" role="alert">{errorMessage}</div>{/if}
  {#if result}
    <div class="context">{result.Observations} common daily returns · {result.IncludedPositionCount}/{result.TotalPositionCount} positions · {result.PeriodStart ? `${formatDate(result.PeriodStart)} – ${formatDate(result.PeriodEnd)}` : 'insufficient common history'}</div>
    <div class="metrics">
      <article><span>Modeled return</span><strong>{ratio(result.ModeledTotalReturn)}</strong></article>
      <article><span>Annualized volatility</span><strong>{ratio(result.AnnualizedVolatility)}</strong></article>
      <article><span>Downside deviation</span><strong>{ratio(result.DownsideDeviation)}</strong></article>
      <article><span>Maximum drawdown</span><strong>{ratio(result.MaximumDrawdown)}</strong></article>
    </div>

    {#if result.Positions.length > 0}
      <div class="grids">
        <article class="panel"><h3>Position contributions</h3>{#each result.Positions as position}<div class="row"><span>{position.Symbol || position.Name}</span><strong>{ratio(position.RiskContribution)} risk</strong><small>{ratio(position.ReturnContribution)} arithmetic return contribution · {ratio(position.AnnualizedVolatility)} volatility</small></div>{/each}</article>
        <article class="panel"><h3>Strongest correlations</h3>{#each result.Correlations.slice(0, 8) as correlation}<div class="row"><span>{correlation.LeftSymbol} / {correlation.RightSymbol}</span><strong>{correlation.Value.toFixed(2)}</strong></div>{/each}{#if result.Correlations.length === 0}<p>Pairwise correlation requires at least two included positions.</p>{/if}</article>
      </div>
    {/if}

    <div class="scenarios"><h3>Deterministic stress scenarios</h3>{#each result.Scenarios as scenario}<article><div><strong>{scenario.Name}</strong><span>{scenario.Description}</span></div><div><strong>{formatMoney(scenario.Impact)}</strong><span>{formatPercent(scenario.ImpactRatio)} of net liquidation</span></div></article>{/each}</div>
    {#if result.Warnings.length > 0}<details class="limits"><summary>Method and data limitations ({result.Warnings.length})</summary><ul>{#each result.Warnings as warning}<li>{warning}</li>{/each}</ul></details>{/if}
    {#if counselorError}<div class="state error" role="alert"><strong>Risk explanation unavailable</strong><span>{counselorError}</span></div>{/if}
    {#if counselorResponse}<CounselorResponse response={counselorResponse} />{/if}
  {:else if !loading}
    <div class="state">This calculation requests one year of daily history for each eligible stock. It does not reconstruct past holdings or account performance.</div>
  {/if}
</section>

<style>
  .risk-card { max-width: 1144px; margin: 0 auto 16px; padding: 18px; border: 1px solid #d8e0e8; border-radius: 6px; background: white; }
  .risk-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
  .risk-heading div { display: grid; gap: 3px; }
  .risk-actions { display: flex !important; grid-auto-flow: column; gap: 8px !important; }
  h2, h3 { margin: 0; }
  .risk-heading span, .context, .state, .row small, .scenarios span, .limits { color: #5d6a7a; }
  button { min-height: 38px; padding: 0 14px; border: 1px solid #14566e; border-radius: 4px; background: #14566e; color: white; font: inherit; font-weight: 700; cursor: pointer; }
  button:disabled { cursor: wait; opacity: .6; }
  button.secondary { background: white; color: #14566e; }
  .context { margin-top: 14px; font-size: 13px; }
  .metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(170px, 1fr)); gap: 10px; margin-top: 12px; }
  .metrics article { display: grid; gap: 6px; padding: 13px; border: 1px solid #e1e7ec; border-radius: 5px; }
  .metrics span { color: #5d6a7a; font-size: 12px; }
  .metrics strong { font-size: 20px; }
  .grids { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin-top: 14px; }
  .panel, .scenarios { padding: 15px; border: 1px solid #e1e7ec; border-radius: 5px; }
  .panel h3, .scenarios h3 { margin-bottom: 9px; font-size: 16px; }
  .row { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 2px 10px; padding: 8px 0; border-top: 1px solid #edf1f5; }
  .row small { grid-column: 1 / -1; }
  .scenarios { margin-top: 14px; }
  .scenarios article { display: flex; justify-content: space-between; gap: 20px; padding: 9px 0; border-top: 1px solid #edf1f5; }
  .scenarios article div { display: grid; gap: 2px; }
  .scenarios article div:last-child { text-align: right; }
  .limits { margin-top: 12px; }
  .limits summary { cursor: pointer; font-weight: 700; }
  .limits ul { padding-left: 20px; }
  .state { margin-top: 14px; padding: 12px 14px; border-left: 4px solid #7a8797; background: #f6f8fa; }
  .state.error { border-color: #c2414b; background: #fff7f7; color: #a62f3a; }
  @media (max-width: 780px) { .risk-heading, .scenarios article { align-items: stretch; flex-direction: column; } .risk-actions { flex-direction: column; } .grids { grid-template-columns: 1fr; } .scenarios article div:last-child { text-align: left; } }
</style>
