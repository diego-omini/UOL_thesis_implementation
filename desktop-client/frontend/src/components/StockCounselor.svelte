<script lang="ts">
  // StockCounselor.svelte owns grouped stock analyses and one stale-safe result area.
  import { createEventDispatcher, onDestroy } from 'svelte'
  import { GetPositionRiskAnalysis, GetStockNews } from '../../wailsjs/go/main/App.js'
  import { acceptCounselorResponse, beginCounselorRequest, emptyCounselorState, rejectCounselorResponse } from '../lib/counselor-state'
  import { classifyCounselorError } from '../lib/counselor-errors'
  import { workflowAvailability, workflowsForOwner, type CounselorWorkflowCopy } from '../lib/counselor-workflows'
  import { formatError } from '../lib/format'
  import type { InstrumentSearchResultView, PositionView } from '../lib/types'
  import CounselorResponse from './CounselorResponse.svelte'

  export let stock: InstrumentSearchResultView
  export let position: PositionView | null
  export let currentContextAsOf = ''

  const dispatch = createEventDispatcher<{ sessionIssue: void }>()
  const workflows = workflowsForOwner('stock')
  let state = emptyCounselorState()
  let sourceKey = ''

  $: {
    const nextKey = `${stock.ContractID}:${position?.MarketValue.Amount ?? 'not-held'}:${position?.UnrealizedPnL.Amount ?? 'not-held'}`
    if (nextKey !== sourceKey) {
      sourceKey = nextKey
      state = emptyCounselorState(state.generation + 1)
    }
  }

  onDestroy(() => state = emptyCounselorState(state.generation + 1))

  async function run(workflow: CounselorWorkflowCopy): Promise<void> {
    const availability = workflowAvailability(workflow, { stockSelected: true, positionHeld: Boolean(position) })
    if (!availability.available) return
    state = beginCounselorRequest(state, workflow.id)
    const request = state.generation
    try {
      const response = workflow.id === 'stock_news'
        ? await GetStockNews(stock.ContractID)
        : await GetPositionRiskAnalysis(stock.ContractID)
      state = acceptCounselorResponse(state, request, response)
    } catch (error) {
      const message = formatError(error)
      state = rejectCounselorResponse(state, request, message)
      const kind = classifyCounselorError(message).kind
      if (state.generation === request && (kind === 'session' || kind === 'broker')) dispatch('sessionIssue')
    }
  }
</script>

<section class="stock-counselor" aria-labelledby="stock-counselor-title">
  <div class="heading"><div><span>Read-only counselor</span><h2 id="stock-counselor-title">Stock analysis</h2></div><small>Analyses cannot create or confirm an order.</small></div>
  <div class="choices">
    {#each workflows as workflow}
      <article class:active={state.active === workflow.id}>
        <div><strong>{workflow.title}</strong><span>{workflow.research ? 'External research' : 'No external research'}</span></div>
        <p>{workflow.description}</p>
        <small><b>Required:</b> {workflow.requiredData}</small>
        {#if !workflowAvailability(workflow, { stockSelected: true, positionHeld: Boolean(position) }).available}
          <small class="unavailable">{workflowAvailability(workflow, { stockSelected: true, positionHeld: Boolean(position) }).reason}</small>
        {/if}
        <button type="button" disabled={state.loading || !workflowAvailability(workflow, { stockSelected: true, positionHeld: Boolean(position) }).available} on:click={() => run(workflow)}>
          {state.loading && state.active === workflow.id ? workflow.loading : workflow.action}
        </button>
      </article>
    {/each}
  </div>
  {#if state.loading}
    <div class="state" role="status">{state.active ? workflows.find(item => item.id === state.active)?.loading : 'Loading analysis...'}</div>
  {:else if state.error}
    <div class="state error" role="alert"><strong>{classifyCounselorError(state.error).title}</strong><span>{classifyCounselorError(state.error).guidance}</span><small>{state.error}</small></div>
  {:else if state.response}
    <CounselorResponse response={state.response} {currentContextAsOf} />
  {/if}
</section>

<style>
  .stock-counselor { display: grid; gap: 14px; padding: 18px; border: 1px solid #d8e0e8; border-radius: 8px; background: white; }
  .heading { display: flex; align-items: end; justify-content: space-between; gap: 18px; }
  .heading div { display: grid; gap: 3px; }
  .heading span { color: #1f6f8b; font-size: 11px; font-weight: 800; letter-spacing: .08em; text-transform: uppercase; }
  h2 { margin: 0; font-size: 18px; }
  .heading small, article p, article small, .state span, .state small { color: #5d6a7a; line-height: 1.45; }
  .choices { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
  article { display: grid; align-content: start; gap: 8px; padding: 13px; border: 1px solid #dce4ea; border-radius: 5px; }
  article.active { border-color: #1f6f8b; }
  article > div { display: flex; justify-content: space-between; gap: 10px; }
  article > div span { color: #536677; font-size: 10px; }
  article p { margin: 0; font-size: 13px; }
  article button { justify-self: start; min-height: 36px; padding: 0 12px; border: 1px solid #14566e; border-radius: 4px; background: white; color: #14566e; font: inherit; font-weight: 700; cursor: pointer; }
  button:disabled { cursor: not-allowed; opacity: .55; }
  .unavailable { color: #a04a24; }
  .state { display: grid; gap: 3px; padding: 12px 14px; border-left: 4px solid #1f6f8b; background: #f5f9fb; }
  .state.error { border-color: #c2414b; background: #fff7f7; }
  @media (max-width: 700px) { .heading { align-items: stretch; flex-direction: column; } .choices { grid-template-columns: 1fr; } }
</style>
