<script lang="ts">
  // CounselorWorkspace.svelte presents grouped portfolio analyses and one stale-safe result area.
  import { createEventDispatcher, onDestroy, onMount } from 'svelte'
  import {
    GetCurrencyExposureAnalysis,
    GetDiversificationAnalysis,
    GetPortfolioDataQualityAudit,
    GetPortfolioNews,
    GetPortfolioResume,
    GetRecentAnalyses,
  } from '../../wailsjs/go/main/App.js'
  import { acceptCounselorResponse, beginCounselorRequest, emptyCounselorState, rejectCounselorResponse } from '../lib/counselor-state'
  import { classifyCounselorError } from '../lib/counselor-errors'
  import { counselorWorkflows, workflowAvailability, workflowCopy, workflowsForOwner, type CounselorPurpose, type CounselorWorkflowCopy, type CounselorWorkflowID } from '../lib/counselor-workflows'
  import { formatDate, formatError } from '../lib/format'
  import type { AIResponseView, RecentAnalysisView } from '../lib/types'
  import CounselorResponse from './CounselorResponse.svelte'

  export let positionCount = 0
  export let currentPortfolioAsOf = ''

  const dispatch = createEventDispatcher<{ sessionIssue: void }>()
  const workflows = workflowsForOwner('positions')
  const purposes = Array.from(new Set(workflows.map(workflow => workflow.purpose))) as CounselorPurpose[]
  const actions: Record<string, () => Promise<AIResponseView>> = {
    portfolio_resume: GetPortfolioResume,
    portfolio_news: GetPortfolioNews,
    currency_exposure: GetCurrencyExposureAnalysis,
    effective_diversification: GetDiversificationAnalysis,
    portfolio_data_quality: GetPortfolioDataQualityAudit,
  }

  let state = emptyCounselorState()
  let recent: RecentAnalysisView[] = []
  let recentLoading = true
  let recentError = ''
  let historySequence = 0

  onMount(() => void loadRecent())
  onDestroy(() => {
    state = emptyCounselorState(state.generation + 1)
    historySequence++
  })

  async function loadRecent(): Promise<void> {
    const request = ++historySequence
    recentLoading = true
    recentError = ''
    try {
      const records = await GetRecentAnalyses(8)
      if (request === historySequence) recent = records
    } catch (error) {
      if (request === historySequence) recentError = formatError(error)
    } finally {
      if (request === historySequence) recentLoading = false
    }
  }

  async function run(workflow: CounselorWorkflowCopy): Promise<void> {
    const availability = workflowAvailability(workflow, { positionCount })
    if (!availability.available) return
    state = beginCounselorRequest(state, workflow.id)
    const request = state.generation
    try {
      const response = await actions[workflow.id]()
      state = acceptCounselorResponse(state, request, response)
      if (state.generation === request) void loadRecent()
    } catch (error) {
      const message = formatError(error)
      state = rejectCounselorResponse(state, request, message)
      const kind = classifyCounselorError(message).kind
      if (state.generation === request && (kind === 'session' || kind === 'broker')) dispatch('sessionIssue')
    }
  }

  function openSaved(record: RecentAnalysisView): void {
    state = {
      generation: state.generation + 1,
      active: record.Response.PromptID as CounselorWorkflowID,
      loading: false,
      response: record.Response,
      error: '',
    }
  }

  function dataState(record: RecentAnalysisView): string {
    const workflow = counselorWorkflows.find(item => item.id === record.Response.PromptID)
    if (!workflow || workflow.owner !== 'positions' || !record.Response.ContextAsOf || !currentPortfolioAsOf) return 'Saved data · current status unknown'
    if (record.Response.ContextAsOf === currentPortfolioAsOf) return 'Current portfolio data'
    return new Date(record.Response.ContextAsOf).getTime() < new Date(currentPortfolioAsOf).getTime()
      ? 'Older portfolio data'
      : 'Newer than displayed portfolio'
  }

  function savedWorkflowTitle(promptID: string): string {
    return counselorWorkflows.find(workflow => workflow.id === promptID)?.title ?? 'Saved analysis'
  }
</script>

<section class="workspace" aria-labelledby="counselor-title">
  <div class="workspace-heading">
    <div>
      <span class="eyebrow">Read-only counselor</span>
      <h2 id="counselor-title">Analysis library</h2>
      <p>Choose a defined analysis. Each action uses application-controlled data and cannot create, prefill, confirm, or submit an order.</p>
    </div>
  </div>

  <div class="purpose-groups">
    {#each purposes as purpose}
      <section class="purpose" aria-label={purpose}>
        <h3>{purpose}</h3>
        <div class="workflow-grid">
          {#each workflows.filter(workflow => workflow.purpose === purpose) as workflow}
            <article class:active={state.active === workflow.id}>
              <div class="workflow-title"><strong>{workflow.title}</strong><span>{workflow.research ? 'External research' : 'No external research'}</span></div>
              <p>{workflow.description}</p>
              <dl><dt>Required data</dt><dd>{workflow.requiredData}</dd></dl>
              {#if !workflowAvailability(workflow, { positionCount }).available}<p class="unavailable">{workflowAvailability(workflow, { positionCount }).reason}</p>{/if}
              <button type="button" disabled={!workflowAvailability(workflow, { positionCount }).available || state.loading} on:click={() => run(workflow)}>
                {state.loading && state.active === workflow.id ? workflow.loading : workflow.action}
              </button>
            </article>
          {/each}
        </div>
      </section>
    {/each}
  </div>

  <section class="result-area" aria-label="Active counselor result">
    {#if state.loading}
      <div class="state" role="status" aria-live="polite"><strong>{state.active ? workflowCopy(state.active).title : 'Analysis'}</strong><span>{state.active ? workflowCopy(state.active).loading : 'Loading...'}</span></div>
    {:else if state.error}
      <div class="state error" role="alert"><strong>{classifyCounselorError(state.error).title}</strong><span>{classifyCounselorError(state.error).guidance}</span><small>{state.error}</small></div>
    {:else if state.response}
      <CounselorResponse response={state.response} currentContextAsOf={currentPortfolioAsOf} />
    {:else}
      <div class="state empty"><strong>No active analysis</strong><span>Select an available analysis above or reopen a validated saved result.</span></div>
    {/if}
  </section>

  <details class="recent">
    <summary>Recent analyses{recent.length ? ` (${recent.length})` : ''}</summary>
    {#if recentLoading}
      <p role="status">Loading recent analyses...</p>
    {:else if recentError}
      <p class="recent-error" role="alert">Recent analyses could not be loaded: {recentError}</p>
    {:else if recent.length === 0}
      <p>No validated saved analyses are available yet.</p>
    {:else}
      <ul>
        {#each recent as record}
          <li>
            <button type="button" on:click={() => openSaved(record)}>
              <strong>{savedWorkflowTitle(record.Response.PromptID)}</strong>
              <span>{formatDate(record.Response.CreatedAt)} · prompt v{record.Response.PromptVersion} · {dataState(record)}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </details>
</section>

<style>
  .workspace { max-width: 1144px; margin: 0 auto 16px; padding: 18px; border: 1px solid #cbd8e2; border-radius: 8px; background: white; }
  .workspace-heading { display: flex; justify-content: space-between; gap: 20px; }
  .eyebrow { color: #1f6f8b; font-size: 11px; font-weight: 800; letter-spacing: .08em; text-transform: uppercase; }
  h2, h3, p { margin-top: 0; }
  h2 { margin-bottom: 5px; font-size: 21px; }
  h3 { margin-bottom: 9px; font-size: 15px; }
  .workspace-heading p, .purpose p, dd, .recent p, .state span, .state small { color: #5d6a7a; line-height: 1.45; }
  .workspace-heading p { max-width: 760px; margin-bottom: 0; }
  .purpose-groups { display: grid; gap: 16px; margin-top: 18px; }
  .purpose { padding-top: 14px; border-top: 1px solid #e5ebf1; }
  .workflow-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(230px, 1fr)); gap: 10px; }
  article { display: grid; align-content: start; gap: 9px; padding: 14px; border: 1px solid #dce4ea; border-radius: 6px; }
  article.active { border-color: #1f6f8b; box-shadow: inset 0 0 0 1px #1f6f8b; }
  .workflow-title { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
  .workflow-title span { padding: 2px 6px; border-radius: 999px; background: #edf3f6; color: #536677; font-size: 10px; white-space: nowrap; }
  article p { margin-bottom: 0; font-size: 13px; }
  dl { margin: 0; }
  dt { color: #34495c; font-size: 11px; font-weight: 800; text-transform: uppercase; }
  dd { margin: 2px 0 0; font-size: 12px; }
  .unavailable { color: #a04a24; }
  article button { justify-self: start; min-height: 36px; padding: 0 12px; border: 1px solid #14566e; border-radius: 4px; background: #14566e; color: white; font: inherit; font-size: 13px; font-weight: 700; cursor: pointer; }
  button:disabled { cursor: not-allowed; opacity: .55; }
  .result-area { margin-top: 18px; }
  .state { display: grid; gap: 4px; padding: 14px 16px; border-left: 4px solid #1f6f8b; background: #f5f9fb; }
  .state.error { border-color: #c2414b; background: #fff7f7; }
  .state.error small { margin-top: 3px; }
  .state.empty { border-color: #8c99a6; background: #f7f8fa; }
  .recent { margin-top: 16px; border-top: 1px solid #e5ebf1; padding-top: 12px; }
  .recent summary { cursor: pointer; font-weight: 700; }
  .recent ul { display: grid; gap: 6px; margin: 10px 0 0; padding: 0; list-style: none; }
  .recent button { display: grid; width: 100%; gap: 2px; padding: 10px 12px; border: 1px solid #dce4ea; border-radius: 4px; background: white; color: #293646; font: inherit; text-align: left; cursor: pointer; }
  .recent button:hover, .recent button:focus-visible { border-color: #1f6f8b; background: #f5f9fb; }
  .recent button span { color: #5d6a7a; font-size: 12px; }
  .recent-error { color: #a62f3a !important; }
  @media (max-width: 650px) { .workflow-grid { grid-template-columns: 1fr; } .workflow-title { align-items: stretch; flex-direction: column; } .workflow-title span { width: max-content; } }
</style>
