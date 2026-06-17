<script lang="ts">
  // App.svelte contains the root frontend component rendered by the Wails application.
  import { onMount } from "svelte";
  import { CheckConnection, GetPortfolio } from '../wailsjs/go/main/App.js'

  type ConnectionStatus = {
    Connected: boolean
    Message: string
  }

  type MoneyView = {
    Amount: number
    Currency: string
  }

  type InstrumentView = {
    Symbol: string
    Name: string
    AssetClass: string
    Exchange: string
    Currency: string
  }

  type AccountSummaryView = {
    AccountID: string
    NetLiquidation: MoneyView
    CashBalance: MoneyView
  }

  type PositionView = {
    Instrument: InstrumentView
    Quantity: number
    AverageCost: MoneyView
    MarketPrice: MoneyView
    MarketValue: MoneyView
    UnrealizedPnL: MoneyView
  }

  type PortfolioView = {
    AccountSummary: AccountSummaryView
    Positions: PositionView[]
    Empty: boolean
  }

  let checking = false
  let loading = false
  let status: ConnectionStatus | null = null
  let portfolio: PortfolioView | null = null
  let errorMessage = ''

  onMount(() => {
    void refreshPortfolio()
  })

  async function checkConnection(): Promise<void> {
    checking = true

    try {
      status = await CheckConnection()
    } catch (error) {
      status = {
        Connected: false,
        Message: `Failed to call backend: ${error}`,
      }
    } finally {
      checking = false
    }
  }

  async function refreshPortfolio(): Promise<void> {
    loading = true
    errorMessage = ''

    try {
      portfolio = await GetPortfolio()
      status = {
        Connected: true,
        Message: 'Broker gateway is authenticated and portfolio data was loaded'
      }
    } catch (error) {
      portfolio = null
      status = {
        Connected: false,
        Message: 'Portfolio data could not be loaded'
      }
      errorMessage = formatError(error)
    } finally {
      loading = false
    }
  }

  function formatMoney(value: MoneyView | undefined): string {
    if (!value) {
      return '-'
    }

    const currency = value.Currency || 'USD'

    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency,
      maximumFractionDigits: 2,
    }).format(value.Amount ?? 0)
  }

  function formatNumber(value: number | undefined): string {
    return new Intl.NumberFormat(undefined, {
      maximumFractionDigits: 4,
    }).format(value ?? 0)
  }

  function formatError(error: unknown): string {
    if (error instanceof Error) {
      return error.message
    }

    return String(error)
  }
</script>

<main class="page">
  <section class="panel">
    <h1>IBKR connection and portfolio</h1>

    <div class="actions">
      <button class="button" disabled={checking} on:click={checkConnection}>
        {checking ? 'Checking...' : 'Check gateway connection'}
      </button>

      <button class="button" disabled={loading} on:click={refreshPortfolio}>
        {loading ? 'Loading...' : 'Refresh portfolio'}
      </button>
    </div>
  </section>

  {#if status}
  <section class:connected={status.Connected} class:failed={!status.Connected} class="status" aria-live="polite">
    <div class:connected={status.Connected} class:failed={!status.Connected} class="status">
      <strong>{status.Connected ? 'Connected' : 'Not connected'}</strong>
      <span>{status.Message}</span>
    </div>
  </section>
  {/if}

  {#if errorMessage}
    <section class="error" aria-live="assertive">
      <strong>Gateway error</strong>
      <span>{errorMessage}</span>
    </section>
  {/if}

  {#if portfolio}
    <section class="summary-grid" aria-label="Account summary">
      <article class="metric">
        <span>Account</span>
        <strong>{portfolio.AccountSummary.AccountID || '-'}</strong>
      </article>

      <article class="metric">
        <span>Net liquidation</span>
        <strong>{formatMoney(portfolio.AccountSummary.NetLiquidation)}</strong>
      </article>

      <article class="metric">
        <span>Cash balance</span>
        <strong>{formatMoney(portfolio.AccountSummary.CashBalance)}</strong>
      </article>
    </section>

    <section class="table-section" aria-label="Positions">
      <div class="section-heading">
        <h2>Positions</h2>
        <span>{portfolio.Positions.length} open</span>
      </div>

      {#if portfolio.Empty}
        <div class="empty-state">
          <strong>No open positions</strong>
          <span>The authenticated paper account has no holdings to display.</span>
        </div>
      {:else}
        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Instrument</th>
                <th>Type</th>
                <th class="numeric">Quantity</th>
                <th class="numeric">Average cost</th>
                <th class="numeric">Market price</th>
                <th class="numeric">Market value</th>
                <th class="numeric">Unrealized P/L</th>
              </tr>
            </thead>

            <tbody>
              {#each portfolio.Positions as position}
                <tr>
                  <td>
                    <strong>{position.Instrument.Symbol || '-'}</strong>
                    <span>{position.Instrument.Name || position.Instrument.Exchange || ''}</span>
                  </td>

                  <td>
                    {position.Instrument.AssetClass || '-'}
                    <span>{position.Instrument.Currency || ''}</span>
                  </td>

                  <td class="numeric">{formatNumber(position.Quantity)}</td>
                  <td class="numeric">{formatMoney(position.AverageCost)}</td>
                  <td class="numeric">{formatMoney(position.MarketPrice)}</td>
                  <td class="numeric">{formatMoney(position.MarketValue)}</td>
                  <td
                    class:loss={position.UnrealizedPnL.Amount < 0}
                    class="numeric pnl"
                  >
                    {formatMoney(position.UnrealizedPnL)}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>
  {:else if loading}
    <section class="loading-state" aria-live="polite">
      Loading portfolio...
    </section>
  {/if}
</main>

<style>
  .page {
    min-height: 100vh;
    padding: 32px;
    box-sizing: border-box;
    background: #f5f7fa;
    color: #18212f;
    text-align: left;
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 24px;
    max-width: 1180px;
    margin: 0 auto 20px;
  }

  h1,
  h2,
  p {
    margin: 0;
  }

  h1 {
    font-size: 30px;
    font-weight: 700;
  }

  .toolbar p {
    margin-top: 4px;
    color: #5d6a7a;
  }

  .actions {
    display: flex;
    gap: 10px;
  }

  .button {
    min-height: 40px;
    padding: 0 16px;
    border: 1px solid #1f6f8b;
    border-radius: 4px;
    background: #1f6f8b;
    color: white;
    font-weight: 700;
    cursor: pointer;
  }

  .button.secondary {
    background: white;
    color: #1f6f8b;
  }

  .button:disabled {
    cursor: wait;
    opacity: 0.65;
  }

  .status,
  .error,
  .summary-grid,
  .table-section,
  .loading-state {
    max-width: 1180px;
    margin: 0 auto 16px;
  }

  .status,
  .error,
  .empty-state,
  .loading-state {
    display: grid;
    gap: 6px;
    padding: 14px 16px;
    border-left: 4px solid;
    background: white;
    box-shadow: 0 1px 3px rgba(24, 33, 47, 0.08);
  }

  .connected {
    border-color: #16835b;
  }

  .failed,
  .error {
    border-color: #c2414b;
  }

  .status span,
  .error span,
  .empty-state span {
    color: #5d6a7a;
    line-height: 1.45;
  }

  .summary-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
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

  .metric span {
    color: #5d6a7a;
    font-size: 13px;
  }

  .metric strong {
    font-size: 22px;
  }

  .table-section {
    border: 1px solid #d8e0e8;
    border-radius: 6px;
    background: white;
    overflow: hidden;
  }

  .section-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 16px 18px;
    border-bottom: 1px solid #e5ebf1;
  }

  .section-heading h2 {
    font-size: 18px;
  }

  .section-heading span {
    color: #5d6a7a;
    font-size: 14px;
  }

  .table-wrap {
    overflow-x: auto;
  }

  table {
    width: 100%;
    min-width: 900px;
    border-collapse: collapse;
  }

  th,
  td {
    padding: 12px 14px;
    border-bottom: 1px solid #edf1f5;
    text-align: left;
    vertical-align: top;
  }

  th {
    color: #5d6a7a;
    font-size: 12px;
    font-weight: 700;
    text-transform: uppercase;
  }

  td span {
    display: block;
    margin-top: 3px;
    color: #6f7b89;
    font-size: 12px;
  }

  tbody tr:last-child td {
    border-bottom: none;
  }

  .numeric {
    text-align: right;
    white-space: nowrap;
  }

  .pnl {
    color: #16835b;
    font-weight: 700;
  }

  .pnl.loss {
    color: #c2414b;
  }

  .empty-state {
    margin: 18px;
    border-color: #7a8797;
    box-shadow: none;
  }

  @media (max-width: 780px) {
    .page {
      padding: 20px;
    }

    .toolbar,
    .actions {
      align-items: stretch;
      flex-direction: column;
    }

    .summary-grid {
      grid-template-columns: 1fr;
    }
  }
</style>