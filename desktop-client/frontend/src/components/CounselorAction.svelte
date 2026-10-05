<script lang="ts">
  // CounselorAction.svelte owns one focused tool-free counselor request and its stale-response state.
  import { createEventDispatcher, onDestroy } from 'svelte'
  import type { AIResponseView } from '../lib/types'
  import type { CounselorWorkflowCopy } from '../lib/counselor-workflows'
  import { formatError } from '../lib/format'
  import { classifyCounselorError } from '../lib/counselor-errors'
  import CounselorResponse from './CounselorResponse.svelte'

  export let workflow: CounselorWorkflowCopy
  export let action: () => Promise<AIResponseView>
  export let disabled = false
  export let resetOn = ''
  export let unavailableReason = ''
  export let currentContextAsOf = ''

  const dispatch = createEventDispatcher<{ sessionIssue: void }>()
  let response: AIResponseView | null = null
  let errorMessage = ''
  let loading = false
  let sequence = 0
  let previousReset = resetOn

  $: if (resetOn !== previousReset) {
    previousReset = resetOn
    sequence++
    response = null
    errorMessage = ''
    loading = false
  }

  onDestroy(() => sequence++)

  async function run(): Promise<void> {
    const request = ++sequence
    response = null
    errorMessage = ''
    loading = true
    try {
      const result = await action()
      if (request === sequence) response = result
    } catch (error) {
      if (request === sequence) {
        errorMessage = formatError(error)
        const kind = classifyCounselorError(errorMessage).kind
        if (kind === 'session' || kind === 'broker') dispatch('sessionIssue')
      }
    } finally {
      if (request === sequence) loading = false
    }
  }
</script>

<div class="counselor-action">
  <div class="action-copy"><strong>{workflow.title}</strong><span>{workflow.description}</span><small><b>Required:</b> {workflow.requiredData} · {workflow.research ? 'Uses external research' : 'No external research'}</small>{#if disabled && unavailableReason}<small class="unavailable">{unavailableReason}</small>{/if}</div>
  <button type="button" disabled={disabled || loading} on:click={run}>{loading ? workflow.loading : workflow.action}</button>
</div>
{#if loading}
  <div class="action-state" role="status" aria-live="polite">{workflow.loading}</div>
{:else if errorMessage}
  <div class="action-state error" role="alert"><strong>{classifyCounselorError(errorMessage).title}</strong><span>{classifyCounselorError(errorMessage).guidance}</span><small>{errorMessage}</small></div>
{:else if response}
  <CounselorResponse {response} {currentContextAsOf} />
{/if}

<style>
  .counselor-action { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-top: 14px; padding-top: 14px; border-top: 1px solid #e5ebf1; }
  .action-copy { display: grid; gap: 3px; }
  .action-copy span, .action-copy small, .action-state span, .action-state small { color: #5d6a7a; font-size: 13px; }
  .action-copy .unavailable { color: #a04a24; }
  button { min-height: 36px; padding: 0 13px; border: 1px solid #1f6f8b; border-radius: 4px; background: white; color: #14566e; font: inherit; font-size: 13px; font-weight: 700; cursor: pointer; }
  button:disabled { cursor: wait; opacity: .55; }
  .action-state { display: grid; gap: 4px; margin-top: 12px; padding: 12px 14px; border-left: 4px solid #4a7c91; background: #f2f7f9; }
  .action-state.error { border-color: #c2414b; background: #fff7f7; }
  @media (max-width: 700px) { .counselor-action { align-items: stretch; flex-direction: column; } }
</style>
