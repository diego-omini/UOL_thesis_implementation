<script lang="ts">
  // CounselorAction.svelte owns one focused tool-free counselor request and its stale-response state.
  import { createEventDispatcher, onDestroy } from 'svelte'
  import type { AIResponseView } from '../lib/types'
  import type { CounselorWorkflowCopy } from '../lib/counselor-workflows'
  import { formatError } from '../lib/format'
  import CounselorResponse from './CounselorResponse.svelte'

  export let workflow: CounselorWorkflowCopy
  export let action: () => Promise<AIResponseView>
  export let disabled = false
  export let resetOn = ''

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
        dispatch('sessionIssue')
      }
    } finally {
      if (request === sequence) loading = false
    }
  }
</script>

<div class="counselor-action">
  <div class="action-copy"><strong>{workflow.title}</strong><span>{workflow.description}</span></div>
  <button type="button" disabled={disabled || loading} on:click={run}>{loading ? workflow.loading : workflow.action}</button>
</div>
{#if loading}
  <div class="action-state" role="status" aria-live="polite">{workflow.loading}</div>
{:else if errorMessage}
  <div class="action-state error" role="alert"><strong>{workflow.unavailable}</strong><span>{errorMessage}</span></div>
{:else if response}
  <CounselorResponse {response} />
{/if}

<style>
  .counselor-action { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-top: 14px; padding-top: 14px; border-top: 1px solid #e5ebf1; }
  .action-copy { display: grid; gap: 3px; }
  .action-copy span, .action-state span { color: #5d6a7a; font-size: 13px; }
  button { min-height: 36px; padding: 0 13px; border: 1px solid #1f6f8b; border-radius: 4px; background: white; color: #14566e; font: inherit; font-size: 13px; font-weight: 700; cursor: pointer; }
  button:disabled { cursor: wait; opacity: .55; }
  .action-state { display: grid; gap: 4px; margin-top: 12px; padding: 12px 14px; border-left: 4px solid #4a7c91; background: #f2f7f9; }
  .action-state.error { border-color: #c2414b; background: #fff7f7; }
  @media (max-width: 700px) { .counselor-action { align-items: stretch; flex-direction: column; } }
</style>
