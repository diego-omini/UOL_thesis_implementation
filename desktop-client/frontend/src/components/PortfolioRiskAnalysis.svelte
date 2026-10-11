<script lang="ts">
  // PortfolioRiskAnalysis.svelte loads and presents deterministic modeled portfolio risk.
  import { createEventDispatcher, onDestroy } from 'svelte'
  import { GetPortfolioRiskAnalysis, GetPortfolioRiskCounsel } from '../../wailsjs/go/main/App.js'
  import CounselorResponse from './CounselorResponse.svelte'
  import { formatDate, formatError, formatMoney, formatPercent } from '../lib/format'
  import { classifyCounselorError } from '../lib/counselor-errors'
  import { workflowAvailability, workflowCopy } from '../lib/counselor-workflows'
  import type { AIResponseView, PortfolioRiskAnalysisView } from '../lib/types'

  const dispatch = createEventDispatcher<{ sessionIssue: void }>()
  const workflow = workflowCopy('portfolio_risk')
  export let embedded = false

  let result: PortfolioRiskAnalysisView | null = null
  let counselorResponse: AIResponseView | null = null
  let loading = false
  let counselorLoading = false
  let errorMessage = ''
  let counselorError = ''
  let sequence = 0
  $: explanationAvailability = workflowAvailability(workflow, { riskObservations: result?.Observations ?? 0 })

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
        const kind = classifyCounselorError(counselorError).kind
        if (kind === 'session' || kind === 'broker') dispatch('sessionIssue')
      }
    } finally {
      if (request === sequence) counselorLoading = false
    }
  }

  function ratio(value: { Value: number; Available: boolean }): string {
    return value.Available ? `${(value.Value * 100).toFixed(2)}%` : 'Unavailable'
  }
</script>

<section class:embedded class="risk-card" aria-label="Modeled portfolio risk analyses">
  <div class="workflow-grid">
    <article class:active={loading || Boolean(result)}>
      <div class="workflow-title"><strong>Modeled risk calculation</strong><span>No external research</span></div>
      <p>Calculates current-weight risk metrics, position contributions, correlations, and deterministic stress scenarios.</p>
      <dl><dt>Required data</dt><dd>Eligible stock positions and one year of broker daily price history</dd></dl>
      <button type="button" disabled={loading || counselorLoading} on:click={calculate}>{loading ? 'Calculating...' : (result ? 'Recalculate risk' : 'Calculate risk')}</button>
    </article>
    <article class:active={counselorLoading || Boolean(counselorResponse)}>
      <div class="workflow-title"><strong>{workflow.title}</strong><span>No external research</span></div>
      <p>{workflow.description}</p>
      <dl><dt>Required data</dt><dd>{workflow.requiredData}</dd></dl>
      {#if !explanationAvailability.available}<p class="unavailable">{explanationAvailability.reason}</p>{/if}
      <button type="button" disabled={loading || counselorLoading || !explanationAvailability.available} on:click={explain}>{counselorLoading ? workflow.loading : workflow.action}</button>
    </article>
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
    {#if counselorError}<div class="state error" role="alert"><strong>{classifyCounselorError(counselorError).title}</strong><span>{classifyCounselorError(counselorError).guidance}</span><small>{counselorError}</small></div>{/if}
    {#if counselorResponse}<CounselorResponse response={counselorResponse} currentContextAsOf={result.CalculatedAt} />{/if}
  {/if}
</section>

<style>
  .risk-card { max-width: 1144px; margin: 0 auto 16px; padding: 18px; border: 1px solid #d8e0e8; border-radius: 6px; background: white; }
  .risk-card.embedded { max-width: none; margin: 0; padding: 0; border: 0; background: transparent; }
  h3, p { margin-top: 0; }
  .workflow-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(230px, 1fr)); gap: 10px; }
  .workflow-grid > article { display: grid; align-content: start; gap: 9px; padding: 14px; border: 1px solid #dce4ea; border-radius: 6px; background: white; }
  .workflow-grid > article.active { border-color: #1f6f8b; box-shadow: inset 0 0 0 1px #1f6f8b; }
  .workflow-title { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
  .workflow-title span { padding: 2px 6px; border-radius: 999px; background: #edf3f6; color: #536677; font-size: 10px; white-space: nowrap; }
  .workflow-grid p { margin-bottom: 0; color: #5d6a7a; font-size: 13px; line-height: 1.45; }
  .workflow-grid dl { margin: 0; }
  .workflow-grid dt { color: #34495c; font-size: 11px; font-weight: 800; text-transform: uppercase; }
  .workflow-grid dd { margin: 2px 0 0; color: #5d6a7a; font-size: 12px; line-height: 1.45; }
  .workflow-grid .unavailable { color: #a04a24; }
  button { justify-self: start; min-height: 36px; padding: 0 12px; border: 1px solid #14566e; border-radius: 4px; background: #14566e; color: white; font: inherit; font-size: 13px; font-weight: 700; cursor: pointer; }
  button:disabled { cursor: not-allowed; opacity: .55; }
  .context, .state, .row small, .scenarios span { color: #5d6a7a; }
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
  .state { margin-top: 14px; padding: 12px 14px; border-left: 4px solid #7a8797; background: #f6f8fa; }
  .state.error { border-color: #c2414b; background: #fff7f7; color: #a62f3a; }
  @media (max-width: 780px) { .scenarios article { align-items: stretch; flex-direction: column; } .grids { grid-template-columns: 1fr; } .scenarios article div:last-child { text-align: left; } }
  @media (max-width: 650px) { .workflow-grid { grid-template-columns: 1fr; } .workflow-title { align-items: stretch; flex-direction: column; } .workflow-title span { width: max-content; } }
</style>
