<script lang="ts">
  // TradingPanel.svelte owns order analysis, market-order entry, preview, and explicit confirmation.
  import { createEventDispatcher, onDestroy } from 'svelte'
  import { ConfirmOrder, GetOrderAnalysis, PreviewOrder, RespondToOrderWarning } from '../../wailsjs/go/main/App.js'
  import CounselorResponse from './CounselorResponse.svelte'
  import { formatDate, formatError, formatMoney, formatNumber } from '../lib/format'
  import { classifyCounselorError } from '../lib/counselor-errors'
  import { workflowAvailability, workflowCopy } from '../lib/counselor-workflows'
  import { canPreviewOrder, type OrderSide } from '../lib/trading'
  import type { AIResponseView, InstrumentSearchResultView, OrderPreviewView, OrderResultView, PositionView } from '../lib/types'

  export let stock: InstrumentSearchResultView
  export let position: PositionView | null
  export let currentContextAsOf = ''

  const dispatch = createEventDispatcher<{ completed: void; sessionIssue: void }>()
  const analysisWorkflow = workflowCopy('order_analysis')

  let side: OrderSide = 'BUY'
  let quantity = 1
  let preview: OrderPreviewView | null = null
  let result: OrderResultView | null = null
  let loading = false
  let errorMessage = ''
  let requestSequence = 0
  let analysis: AIResponseView | null = null
  let analysisLoading = false
  let analysisError = ''
  let analysisSequence = 0

  $: availableToSell = Math.max(position?.Quantity || 0, 0)

  onDestroy(() => {
    requestSequence++
    analysisSequence++
    if (result?.WarningToken) void RespondToOrderWarning(result.WarningToken, false)
  })

  function selectSide(next: OrderSide): void {
    if (side === next) return
    side = next
    quantity = 1
    invalidateDecision()
  }

  function invalidateDecision(): void {
    requestSequence++
    analysisSequence++
    preview = null
    result = null
    errorMessage = ''
    analysis = null
    analysisError = ''
    analysisLoading = false
  }

  async function getOrderAnalysis(): Promise<void> {
    const request = ++analysisSequence
    analysis = null
    analysisError = ''
    analysisLoading = true
    try {
      const next = await GetOrderAnalysis(stock.ContractID, side, Number(quantity))
      if (request === analysisSequence) analysis = next
    } catch (error) {
      if (request === analysisSequence) {
        analysisError = formatError(error)
        reportSessionIssue(analysisError)
      }
    } finally {
      if (request === analysisSequence) analysisLoading = false
    }
  }

  async function createPreview(): Promise<void> {
    const request = ++requestSequence
    preview = null
    result = null
    errorMessage = ''
    loading = true
    try {
      const next = await PreviewOrder(stock.ContractID, side, Number(quantity))
      if (request === requestSequence) preview = next
    } catch (error) {
      if (request === requestSequence) {
        errorMessage = formatError(error)
        reportSessionIssue(errorMessage)
      }
    } finally {
      if (request === requestSequence) loading = false
    }
  }

  function reportSessionIssue(message: string): void {
    const kind = classifyCounselorError(message).kind
    if (kind === 'session' || kind === 'broker') dispatch('sessionIssue')
  }

  async function confirmOrder(): Promise<void> {
    if (!preview) return
    const request = ++requestSequence
    const token = preview.Token
    errorMessage = ''
    loading = true
    try {
      const next = await ConfirmOrder(token)
      if (request !== requestSequence) return
      preview = null
      result = next
      if (!next.WarningToken) dispatch('completed')
    } catch (error) {
      if (request === requestSequence) {
        preview = null
        errorMessage = formatError(error)
        reportSessionIssue(errorMessage)
      }
    } finally {
      if (request === requestSequence) loading = false
    }
  }

  async function respondToWarning(confirmed: boolean): Promise<void> {
    if (!result?.WarningToken) return
    const request = ++requestSequence
    const token = result.WarningToken
    errorMessage = ''
    loading = true
    try {
      const next = await RespondToOrderWarning(token, confirmed)
      if (request !== requestSequence) return
      result = next
      if (!next.WarningToken && confirmed) dispatch('completed')
    } catch (error) {
      if (request === requestSequence) {
        errorMessage = formatError(error)
        reportSessionIssue(errorMessage)
      }
    } finally {
      if (request === requestSequence) loading = false
    }
  }
</script>

<section class="trading-card" aria-label="Paper trading">
  <div class="trading-heading">
    <div><h2>Paper trade</h2><span>Market order · Day</span></div>
    <strong>IBKR paper account</strong>
  </div>

  <div class="side-selector" aria-label="Order side">
    <button type="button" class:active={side === 'BUY'} aria-pressed={side === 'BUY'} on:click={() => selectSide('BUY')}>Buy</button>
    <button type="button" class:active={side === 'SELL'} aria-pressed={side === 'SELL'} on:click={() => selectSide('SELL')}>Sell</button>
  </div>

  <aside class="analysis-intro" aria-label="Read-only proposed-order analysis">
    <div><strong>{analysisWorkflow.title}</strong><span>{analysisWorkflow.description}</span></div>
    <dl><dt>Required data</dt><dd>{analysisWorkflow.requiredData}</dd><dt>Research</dt><dd>Uses external company research; private order and portfolio values stay local.</dd></dl>
  </aside>

  <form on:submit|preventDefault={createPreview}>
    <label for="share-quantity">Shares</label>
    <input
      id="share-quantity"
      type="number"
      min="1"
      max={side === 'SELL' ? availableToSell : undefined}
      step="1"
      bind:value={quantity}
      on:input={invalidateDecision}
      disabled={loading || analysisLoading}
      required
    />
    <div class="form-actions">
      <button class="analysis-button" type="button" disabled={loading || analysisLoading || !workflowAvailability(analysisWorkflow, { validOrder: canPreviewOrder(side, Number(quantity), availableToSell) }).available} on:click={getOrderAnalysis}>
        {analysisLoading ? analysisWorkflow.loading : analysisWorkflow.action}
      </button>
      <button class="preview-button" type="submit" disabled={loading || analysisLoading || !canPreviewOrder(side, Number(quantity), availableToSell)}>
        {loading && !preview && !result?.WarningToken ? 'Checking order...' : `Preview ${side.toLowerCase()}`}
      </button>
    </div>
  </form>

  {#if side === 'SELL'}
    <p class="availability">Available to sell: {formatNumber(availableToSell)} shares. Short selling is disabled.</p>
  {:else}
    <p class="availability">Buy orders must be covered by the account's available cash; margin funding is disabled.</p>
  {/if}

  {#if errorMessage}
    <div class="trade-error" role="alert"><strong>Order unavailable</strong><span>{errorMessage}</span></div>
  {/if}

  {#if analysisLoading}
    <div class="analysis-status" role="status" aria-live="polite">Calculating order impact and researching reliable company evidence...</div>
  {:else if analysisError}
    <div class="trade-error" role="alert"><strong>{classifyCounselorError(analysisError).title}</strong><span>{classifyCounselorError(analysisError).guidance}</span><small>{analysisError}</small></div>
  {:else if analysis}
    <CounselorResponse response={analysis} {currentContextAsOf} />
  {/if}

  {#if preview}
    <section class="order-preview" aria-label="Order confirmation">
      <div class="preview-title"><h3>Confirm {preview.Side.toLowerCase()} order</h3><span>Expires {formatDate(preview.ExpiresAt)}</span></div>
      <dl>
        <div><dt>Order</dt><dd>{formatNumber(preview.Quantity)} {preview.Instrument.Symbol} shares</dd></div>
        <div><dt>Estimated price</dt><dd>{formatMoney(preview.EstimatedPrice)}</dd></div>
        <div><dt>Estimated value</dt><dd>{formatMoney(preview.OrderAmount)}</dd></div>
        <div><dt>Estimated commission</dt><dd>{formatMoney(preview.Commission)}</dd></div>
        <div class="total"><dt>Estimated total</dt><dd>{formatMoney(preview.Total)}</dd></div>
        <div><dt>Position after order</dt><dd>{formatNumber(preview.ResultingPosition)} shares</dd></div>
        {#if preview.Side === 'BUY'}<div><dt>Available cash</dt><dd>{formatMoney(preview.CashBalance)}</dd></div>{/if}
      </dl>
      {#each preview.Warnings as warning}<p class="broker-warning">{warning}</p>{/each}
      <p class="market-note">A market order can execute at a different price. Account state and buying capacity are checked again before submission.</p>
      <div class="confirmation-actions">
        <button type="button" class="secondary" disabled={loading} on:click={invalidateDecision}>Cancel</button>
        <button type="button" class="confirm" disabled={loading} on:click={confirmOrder}>{loading ? 'Submitting...' : `Confirm ${preview.Side.toLowerCase()}`}</button>
      </div>
    </section>
  {:else if result?.WarningToken}
    <section class="order-preview warning" aria-label="IBKR order warning">
      <div class="preview-title"><h3>IBKR requires another confirmation</h3></div>
      {#each result.WarningMessages as message}<p class="broker-warning">{message}</p>{/each}
      <div class="confirmation-actions">
        <button type="button" class="secondary" disabled={loading} on:click={() => respondToWarning(false)}>Cancel order</button>
        <button type="button" class="confirm" disabled={loading} on:click={() => respondToWarning(true)}>{loading ? 'Responding...' : 'Accept warning and submit'}</button>
      </div>
    </section>
  {:else if result?.OrderID}
    <div class="trade-success" role="status"><strong>Order {result.Status || 'submitted'}</strong><span>IBKR order ID {result.OrderID}. The portfolio is being refreshed.</span></div>
  {:else if result?.Status === 'Cancelled'}
    <div class="trade-cancelled" role="status">The order was cancelled.</div>
  {/if}
</section>

<style>
  .trading-card { display: grid; gap: 16px; padding: 20px; border: 1px solid #d8e0e8; border-radius: 8px; background: white; }
  .trading-heading, .preview-title, .confirmation-actions { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
  .trading-heading div { display: grid; gap: 3px; }
  .trading-heading h2, .preview-title h3 { margin: 0; }
  .trading-heading h2 { font-size: 18px; }
  .trading-heading span, .preview-title span, .availability, .market-note { color: #657383; font-size: 13px; }
  .trading-heading > strong { color: #237a4b; font-size: 13px; }
  .side-selector { display: grid; grid-template-columns: 1fr 1fr; gap: 4px; padding: 4px; border-radius: 6px; background: #edf1f5; }
  .side-selector button { min-height: 38px; border: 0; border-radius: 4px; background: transparent; color: #4c5b6b; font: inherit; font-weight: 800; cursor: pointer; }
  .side-selector button.active { background: white; color: #14566e; box-shadow: 0 1px 3px rgba(24, 33, 47, .14); }
  .analysis-intro { display: grid; grid-template-columns: minmax(0, 1fr) minmax(280px, 1fr); gap: 16px; padding: 13px 14px; border: 1px solid #dce4ea; border-radius: 5px; background: #f8fafb; }
  .analysis-intro > div { display: grid; align-content: start; gap: 3px; }
  .analysis-intro span, .analysis-intro dd { color: #5d6a7a; font-size: 12px; line-height: 1.4; }
  .analysis-intro dl { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 3px 9px; margin: 0; background: transparent; }
  .analysis-intro dt { font-size: 11px; font-weight: 800; }
  .analysis-intro dd { padding: 0; background: transparent; text-align: left; }
  form { display: grid; grid-template-columns: auto minmax(100px, 180px) minmax(0, 1fr); align-items: center; gap: 10px; }
  form label { font-weight: 700; }
  input { min-height: 40px; padding: 0 11px; border: 1px solid #b8c4cf; border-radius: 4px; box-sizing: border-box; font: inherit; }
  button { font: inherit; }
  .form-actions { display: flex; justify-content: flex-end; gap: 8px; }
  .preview-button, .analysis-button, .confirm { min-height: 40px; padding: 0 16px; border: 1px solid #14566e; border-radius: 4px; background: #14566e; color: white; font-weight: 800; cursor: pointer; }
  .analysis-button { background: white; color: #14566e; }
  button:disabled { cursor: wait; opacity: .55; }
  .availability { margin: -7px 0 0; }
  .order-preview { display: grid; gap: 14px; padding: 17px; border: 1px solid #99b8c5; border-radius: 6px; background: #f5f9fb; }
  .order-preview.warning { border-color: #d39a35; background: #fffaf0; }
  .preview-title h3 { font-size: 16px; }
  dl { display: grid; gap: 1px; margin: 0; background: #dce5ea; }
  dl div { display: flex; justify-content: space-between; gap: 18px; padding: 9px 11px; background: white; }
  dt { color: #5d6a7a; }
  dd { margin: 0; font-weight: 700; text-align: right; }
  dl .total { border-top: 1px solid #b8c4cf; }
  .broker-warning { margin: 0; padding: 10px 12px; border-left: 4px solid #d39a35; background: #fff; line-height: 1.45; }
  .market-note { margin: 0; line-height: 1.45; }
  .confirmation-actions { justify-content: flex-end; }
  .secondary { min-height: 40px; padding: 0 16px; border: 1px solid #9aa7b4; border-radius: 4px; background: white; color: #354354; font-weight: 700; cursor: pointer; }
  .trade-error, .trade-success { display: grid; gap: 4px; padding: 12px 14px; border-left: 4px solid #c2414b; background: #fff7f7; }
  .trade-success { border-color: #237a4b; background: #f3fbf6; }
  .trade-error span, .trade-error small, .trade-success span { color: #5d6a7a; }
  .trade-cancelled { color: #5d6a7a; }
  .analysis-status { padding: 12px 14px; border-left: 4px solid #1f6f8b; background: #f5f9fb; color: #5d6a7a; }

  @media (max-width: 620px) {
    form, .analysis-intro { grid-template-columns: 1fr; }
    .form-actions { display: grid; grid-template-columns: 1fr; }
    .trading-heading, .preview-title, .confirmation-actions { align-items: stretch; flex-direction: column; }
    .confirmation-actions button { width: 100%; }
  }
</style>
