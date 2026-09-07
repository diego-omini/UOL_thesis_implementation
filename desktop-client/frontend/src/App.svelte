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
    Available: boolean
  }

  type RatioView = {
    Value: number
    Available: boolean
  }

  type InstrumentView = {
    ContractID: number
    Symbol: string
    Name: string
    AssetClass: string
    Exchange: string
    Currency: string
    Sector: string
    Industry: string
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
    BaseMarketValue: MoneyView
    UnrealizedPnL: MoneyView
    BaseUnrealizedPnL: MoneyView
    PortfolioWeight: RatioView
  }

  type AllocationView = {
    Label: string
    MarketValue: MoneyView
    Exposure: MoneyView
    Weight: RatioView
  }

  type DataWarningView = {
	Code: string
	Message: string
	Count: number
  }

  type ExchangeRateView = {
    Currency: string
    BaseCurrency: string
    BaseAmountPerUnit: number
    Available: boolean
    Source: string
    AsOf: string
  }

  type PositionWeightView = {
    ContractID: number
    Symbol: string
    Name: string
    MarketValue: MoneyView
    Weight: RatioView
  }

  type AnalyticsView = {
    CalculatedAt: string
    BaseCurrency: string
    NetMarketValue: MoneyView
    GrossExposure: MoneyView
    TotalUnrealizedPnL: MoneyView
    LargestPositions: PositionWeightView[]
    CurrencyAllocation: AllocationView[]
    AssetClassAllocation: AllocationView[]
    SectorAllocation: AllocationView[]
	TopThreeConcentration: RatioView
	HHI: RatioView
	ExchangeRates: ExchangeRateView[]
	Warnings: DataWarningView[]
  }

  type PortfolioView = {
    AccountSummary: AccountSummaryView
    Positions: PositionView[]
    Analytics: AnalyticsView
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
        Message: 'Gateway portfolio data was loaded'
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
    if (!value || !value.Available) {
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

  function formatPercent(value: RatioView | undefined): string {
    if (!value || !value.Available) {
      return '-'
    }

    return new Intl.NumberFormat(undefined, {
      style: 'percent',
      maximumFractionDigits: 1,
    }).format(value.Value)
  }

  function formatRatio(value: RatioView | undefined): string {
    if (!value || !value.Available) {
      return '-'
    }

    return new Intl.NumberFormat(undefined, {
      minimumFractionDigits: 3,
      maximumFractionDigits: 3,
    }).format(value.Value)
  }

  function formatExchangeRate(value: number): string {
    return new Intl.NumberFormat(undefined, {
      minimumFractionDigits: 4,
      maximumFractionDigits: 6,
    }).format(value)
  }

  function formatDate(value: string): string {
    const date = new Date(value)
    return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString()
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
      <strong>{status.Connected ? 'Connected' : 'Not connected'}</strong>
      <span>{status.Message}</span>
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

    <div class="analytics-heading">
      <h2>Portfolio analytics</h2>
      <span>Calculated {formatDate(portfolio.Analytics.CalculatedAt)} in {portfolio.Analytics.BaseCurrency || 'unknown currency'}</span>
    </div>

    <section class="summary-grid" aria-label="Portfolio analytics summary">
      <article class="metric">
        <span>Net position value</span>
        <strong>{formatMoney(portfolio.Analytics.NetMarketValue)}</strong>
      </article>

      <article class="metric">
        <span>Gross exposure</span>
        <strong>{formatMoney(portfolio.Analytics.GrossExposure)}</strong>
      </article>

      <article class="metric">
        <span>Unrealized P/L</span>
        <strong
          class:loss={portfolio.Analytics.TotalUnrealizedPnL.Available && portfolio.Analytics.TotalUnrealizedPnL.Amount < 0}
          class="pnl"
        >
          {formatMoney(portfolio.Analytics.TotalUnrealizedPnL)}
        </strong>
      </article>

      <article class="metric">
        <span>Top 3 concentration</span>
        <strong>{formatPercent(portfolio.Analytics.TopThreeConcentration)}</strong>
      </article>

      <article class="metric">
        <span>Concentration index</span>
        <strong>{formatRatio(portfolio.Analytics.HHI)}</strong>
      </article>
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
            <div class="allocation-row">
              <span>{allocation.Label}</span>
              <strong>{formatPercent(allocation.Weight)}</strong>
              <small>{formatMoney(allocation.MarketValue)}</small>
            </div>
          {/each}
        </article>

        <article class="allocation-card">
          <h3>By asset class</h3>
          {#each portfolio.Analytics.AssetClassAllocation as allocation}
            <div class="allocation-row">
              <span>{allocation.Label}</span>
              <strong>{formatPercent(allocation.Weight)}</strong>
              <small>{formatMoney(allocation.MarketValue)}</small>
            </div>
          {/each}
        </article>

        <article class="allocation-card">
          <h3>By sector</h3>
          {#each portfolio.Analytics.SectorAllocation as allocation}
            <div class="allocation-row">
              <span>{allocation.Label}</span>
              <strong>{formatPercent(allocation.Weight)}</strong>
              <small>{formatMoney(allocation.MarketValue)}</small>
            </div>
          {/each}
        </article>

        <article class="allocation-card">
          <h3>Largest positions</h3>
          {#each portfolio.Analytics.LargestPositions.slice(0, 5) as position}
            <div class="allocation-row">
              <span>{position.Symbol || position.Name || '-'}</span>
              <strong>{formatPercent(position.Weight)}</strong>
              <small>{formatMoney(position.MarketValue)}</small>
            </div>
          {/each}
        </article>
      </section>
    {/if}

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
                <th class="numeric">Weight</th>
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
                    <span>
                      {position.Instrument.Sector || position.Instrument.Currency || ''}
                    </span>
                  </td>

                  <td class="numeric">{formatPercent(position.PortfolioWeight)}</td>
                  <td class="numeric">{formatNumber(position.Quantity)}</td>
                  <td class="numeric">{formatMoney(position.AverageCost)}</td>
                  <td class="numeric">{formatMoney(position.MarketPrice)}</td>
                  <td class="numeric">{formatMoney(position.MarketValue)}</td>
                  <td
                    class:loss={position.UnrealizedPnL.Available && position.UnrealizedPnL.Amount < 0}
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

  .panel {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 24px;
    max-width: 1180px;
    margin: 0 auto 20px;
  }

  h1,
  h2 {
    margin: 0;
  }

  h1 {
    font-size: 30px;
    font-weight: 700;
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

  .button:disabled {
    cursor: wait;
    opacity: 0.65;
  }

  .status,
  .error,
  .warning,
  .exchange-rates,
  .summary-grid,
  .analytics-heading,
  .allocation-grid,
  .table-section,
  .loading-state {
    max-width: 1180px;
    margin: 0 auto 16px;
  }

  .status,
  .error,
  .warning,
  .exchange-rates,
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

  .warning {
    border-color: #c98716;
  }

  .warning ul {
    margin: 2px 0 0;
    padding-left: 20px;
    color: #5d6a7a;
  }

  .exchange-rates div {
    display: flex;
    justify-content: space-between;
    gap: 16px;
  }

  .exchange-rates small {
    color: #5d6a7a;
  }

  .status span,
  .error span,
  .empty-state span {
    color: #5d6a7a;
    line-height: 1.45;
  }

  .summary-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: 12px;
  }

  .analytics-heading {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 16px;
  }

  .analytics-heading h2,
  .allocation-card h3 {
    margin: 0;
  }

  .analytics-heading span {
    color: #5d6a7a;
    font-size: 13px;
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

  .allocation-card h3 {
    margin-bottom: 12px;
    font-size: 16px;
  }

  .allocation-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 2px 12px;
    padding: 9px 0;
    border-top: 1px solid #edf1f5;
  }

  .allocation-row span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .allocation-row small {
    grid-column: 1 / -1;
    color: #6f7b89;
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

    .panel,
    .actions {
      align-items: stretch;
      flex-direction: column;
    }

    .summary-grid {
      grid-template-columns: 1fr;
    }

    .allocation-grid {
      grid-template-columns: 1fr;
    }

    .analytics-heading {
      align-items: stretch;
      flex-direction: column;
    }
  }
</style>
