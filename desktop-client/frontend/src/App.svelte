<script lang="ts">
  // App.svelte contains the root frontend component rendered by the Wails application.
  import DOMPurify from 'dompurify'
  import { marked } from 'marked'
  import { onDestroy, onMount, tick } from "svelte";
  import {
    ChooseGatewayDirectory,
    GetPortfolio,
    GetPortfolioResume,
    GetSessionStatus,
    InitializeSession,
    KeepSessionAlive,
    Logout,
    OpenIBKRLogin,
    StartGateway,
  } from '../wailsjs/go/main/App.js'

  type SessionStatus = {
    State: string
    GatewayReachable: boolean
    Authenticated: boolean
    Connected: boolean
    Competing: boolean
    Paper: boolean
    PaperKnown: boolean
    Ready: boolean
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

  type AIResponseView = {
    Text: string
    Provider: string
    Model: string
    PromptID: string
    PromptVersion: number
  }

  let initializing = true
  let authAction = false
  let loading = false
  let waitingForLogin = false
  let applicationFocused = true
  let checkingSession = false
  let session: SessionStatus | null = null
  let portfolio: PortfolioView | null = null
  let portfolioResume: AIResponseView | null = null
  let portfolioResumeError = ''
  let portfolioResumeLoading = false
  let errorMessage = ''
  let authenticationPoll: ReturnType<typeof setInterval> | null = null
  let keepAliveTimer: ReturnType<typeof setInterval> | null = null

  onMount(() => {
    window.addEventListener('focus', handleWindowFocus)
    window.addEventListener('blur', handleWindowBlur)
    void initializeAuthentication()
  })

  onDestroy(() => {
    window.removeEventListener('focus', handleWindowFocus)
    window.removeEventListener('blur', handleWindowBlur)
    stopAuthenticationPolling()
    stopKeepAlive()
  })

  // handleWindowFocus immediately checks IBKR after the user returns from browser authentication.
  async function handleWindowFocus(): Promise<void> {
    applicationFocused = true
    if (!initializing && !session?.Ready) {
      waitingForLogin = true
      stopAuthenticationPolling()
      await renderAuthenticationProgress()
      await waitForActiveSessionRefresh()
      await refreshSession()
      if (!session?.Ready && applicationFocused) {
        waitingForLogin = false
        startAuthenticationPolling()
      }
    }
  }

  function handleWindowBlur(): void {
    applicationFocused = false
    if (!session?.Ready) {
      waitingForLogin = true
      stopAuthenticationPolling()
    }
  }

  async function waitForActiveSessionRefresh(): Promise<void> {
    while (checkingSession) {
      await new Promise(resolve => window.setTimeout(resolve, 25))
    }
  }

  async function renderAuthenticationProgress(): Promise<void> {
    await tick()
    await new Promise(resolve => window.requestAnimationFrame(() => resolve(undefined)))
  }

  async function initializeAuthentication(): Promise<void> {
    try {
      applySession(await InitializeSession())
    } catch (error) {
      session = {
        State: 'error',
        GatewayReachable: false,
        Authenticated: false,
        Connected: false,
        Competing: false,
        Paper: false,
        PaperKnown: false,
        Ready: false,
        Message: formatError(error),
      }
    } finally {
      initializing = false
    }
  }

  function applySession(next: SessionStatus): void {
    session = next
    if (next.Ready) {
      waitingForLogin = false
      errorMessage = ''
      stopAuthenticationPolling()
      startKeepAlive()
      if (!portfolio && !loading) {
        void refreshPortfolio()
      }
      return
    }

    portfolio = null
    portfolioResume = null
    portfolioResumeError = ''
    stopKeepAlive()
    if (applicationFocused && next.State !== 'gateway_configuration_required' && next.State !== 'live_account') {
      startAuthenticationPolling()
    }
  }

  function startAuthenticationPolling(): void {
    if (authenticationPoll) return
    authenticationPoll = setInterval(() => void refreshSession(), 2000)
  }

  function stopAuthenticationPolling(): void {
    if (!authenticationPoll) return
    clearInterval(authenticationPoll)
    authenticationPoll = null
  }

  function startKeepAlive(): void {
    if (keepAliveTimer) return
    keepAliveTimer = setInterval(() => void maintainSession(), 60000)
  }

  function stopKeepAlive(): void {
    if (!keepAliveTimer) return
    clearInterval(keepAliveTimer)
    keepAliveTimer = null
  }

  async function refreshSession(): Promise<void> {
    if (checkingSession) return
    checkingSession = true
    try {
      applySession(await GetSessionStatus())
    } catch (error) {
      errorMessage = formatError(error)
    } finally {
      checkingSession = false
    }
  }

  async function maintainSession(): Promise<void> {
    try {
      applySession(await KeepSessionAlive())
    } catch (error) {
      errorMessage = formatError(error)
      await refreshSession()
    }
  }

  async function chooseGatewayDirectory(): Promise<void> {
    authAction = true
    errorMessage = ''
    try {
      applySession(await ChooseGatewayDirectory())
    } catch (error) {
      errorMessage = formatError(error)
    } finally {
      authAction = false
    }
  }

  async function startGateway(): Promise<void> {
    authAction = true
    errorMessage = ''
    try {
      applySession(await StartGateway())
    } catch (error) {
      errorMessage = formatError(error)
    } finally {
      authAction = false
    }
  }

  async function openIBKRLogin(): Promise<void> {
    authAction = true
    waitingForLogin = true
    errorMessage = ''
    try {
      await renderAuthenticationProgress()
      await OpenIBKRLogin()
    } catch (error) {
      waitingForLogin = false
      errorMessage = formatError(error)
    } finally {
      authAction = false
    }
  }

  async function retryAuthentication(): Promise<void> {
    authAction = true
    errorMessage = ''
    try {
      applySession(await InitializeSession())
    } catch (error) {
      errorMessage = formatError(error)
    } finally {
      authAction = false
    }
  }

  async function logout(): Promise<void> {
    if (!window.confirm('Log out of the current IBKR session?')) return
    authAction = true
    errorMessage = ''
    try {
      portfolio = null
      portfolioResume = null
      portfolioResumeError = ''
      applySession(await Logout())
    } catch (error) {
      errorMessage = formatError(error)
      await refreshSession()
    } finally {
      authAction = false
    }
  }

  // refreshPortfolio reloads the broker snapshot and keeps connection and error state consistent.
  async function refreshPortfolio(): Promise<void> {
    loading = true
    errorMessage = ''
    portfolioResume = null
    portfolioResumeError = ''

    try {
      portfolio = await GetPortfolio()
    } catch (error) {
      portfolio = null
      await refreshSession()
      if (session?.Ready) {
        errorMessage = formatError(error)
      }
    } finally {
      loading = false
    }
  }

  // getPortfolioResume requests a fresh backend-owned snapshot and shows provider errors separately.
  async function getPortfolioResume(): Promise<void> {
    portfolioResumeLoading = true
    portfolioResume = null
    portfolioResumeError = ''

    try {
      portfolioResume = await GetPortfolioResume()
    } catch (error) {
      portfolioResumeError = formatError(error)
    } finally {
      portfolioResumeLoading = false
    }
  }

  // renderMarkdown converts provider output to sanitized HTML before it enters the document.
  function renderMarkdown(value: string): string {
    const rendered = marked.parse(value, { async: false, gfm: true })
    return DOMPurify.sanitize(rendered)
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

  function authenticationHeading(state: string | undefined): string {
    switch (state) {
      case 'gateway_configuration_required': return 'Gateway setup required'
      case 'gateway_unavailable': return 'Gateway unavailable'
      case 'gateway_starting': return 'Starting IBKR gateway'
      case 'authentication_required': return 'Authentication required'
      case 'competing_session': return 'Competing IBKR session'
      case 'live_account': return 'Paper account required'
      default: return 'Connection problem'
    }
  }
</script>

<main class:auth-page={!session?.Ready} class="page">
  {#if initializing}
    <section class="auth-card" aria-live="polite">
      <h1>AI-assisted portfolio tracker</h1>
      <p>Checking the local IBKR connection...</p>
    </section>
  {:else if !session?.Ready}
    <section class="auth-card">
      <div class="auth-heading">
        <span class="eyebrow">Paper trading</span>
        <h1>AI-assisted portfolio tracker</h1>
        <p>Connect your IBKR paper account to load the portfolio dashboard.</p>
      </div>

      <div class:auth-warning={session?.State === 'live_account' || session?.State === 'competing_session'} class="auth-status" aria-live="polite">
        <strong>{waitingForLogin ? 'Waiting for IBKR authentication' : authenticationHeading(session?.State)}</strong>
        <span>{session?.Message || 'The current connection state is unavailable.'}</span>
      </div>

      {#if session?.State === 'gateway_configuration_required'}
        <button class="button auth-button" disabled={authAction} on:click={chooseGatewayDirectory}>
          {authAction ? 'Opening...' : 'Select gateway folder'}
        </button>
      {:else if session?.State === 'gateway_unavailable'}
        <div class="auth-actions">
          <button class="button auth-button" disabled={authAction} on:click={startGateway}>
            {authAction ? 'Starting...' : 'Start IBKR gateway'}
          </button>
          <button class="secondary-button" disabled={authAction} on:click={chooseGatewayDirectory}>Change folder</button>
        </div>
      {:else if session?.State === 'gateway_starting'}
        <button class="button auth-button" disabled>Starting gateway...</button>
      {:else if session?.State === 'authentication_required' || session?.State === 'competing_session'}
        {#if waitingForLogin}
          <div class="authentication-progress" role="status" aria-live="polite">
            <span class="spinner" aria-hidden="true"></span>
            <span>Checking IBKR authentication...</span>
          </div>
        {:else}
          <button class="button auth-button" disabled={authAction} on:click={openIBKRLogin}>
            Log in to IBKR
          </button>
        {/if}
      {:else}
        <button class="button auth-button" disabled={authAction} on:click={retryAuthentication}>
          {authAction ? 'Checking...' : 'Retry connection'}
        </button>
      {/if}

      {#if errorMessage}
        <div class="auth-error" aria-live="assertive">{errorMessage}</div>
      {/if}

      <small class="auth-note">Authentication is handled by Interactive Brokers and is limited to the paper-trading environment.</small>
    </section>
  {:else}
  <section class="panel">
    <div>
      <span class="eyebrow">IBKR paper trading</span>
      <h1>Portfolio dashboard</h1>
    </div>

    <div class="actions">
      <button class="button" disabled={loading} on:click={refreshPortfolio}>
        {loading ? 'Loading...' : 'Refresh portfolio'}
      </button>
      <button class="secondary-button" disabled={authAction} on:click={logout}>Log out</button>
    </div>
  </section>

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

    <section class="ai-panel" aria-label="AI portfolio resume">
      <div class="ai-heading">
        <div>
          <h2>Portfolio resume</h2>
          <span>Generated from current normalized positions and portfolio analytics.</span>
        </div>
        <button
          class="button"
          disabled={portfolio.Empty || portfolioResumeLoading}
          on:click={getPortfolioResume}
        >
          {portfolioResumeLoading ? 'Generating...' : 'Get portfolio resume'}
        </button>
      </div>

      {#if portfolioResumeLoading}
        <div class="ai-status" role="status" aria-live="polite">Waiting for the configured AI provider...</div>
      {:else if portfolioResumeError}
        <div class="ai-error" role="alert" aria-live="assertive">
          <strong>Portfolio resume unavailable</strong>
          <span>{portfolioResumeError}</span>
        </div>
      {:else if portfolioResume}
        <div class="ai-response" aria-live="polite">
          <div class="ai-response-content">
            {@html renderMarkdown(portfolioResume.Text)}
          </div>
          <small>
            {portfolioResume.Provider} · {portfolioResume.Model} · {portfolioResume.PromptID} v{portfolioResume.PromptVersion}
          </small>
        </div>
      {/if}
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

  .auth-page {
    display: grid;
    place-items: center;
    padding: 24px;
    background:
      radial-gradient(circle at top left, rgba(31, 111, 139, 0.16), transparent 38%),
      #f5f7fa;
  }

  .auth-card {
    display: grid;
    gap: 22px;
    width: min(100%, 440px);
    padding: 38px;
    border: 1px solid #d8e0e8;
    border-radius: 12px;
    background: white;
    box-shadow: 0 16px 45px rgba(24, 33, 47, 0.12);
  }

  .auth-heading {
    display: grid;
    gap: 8px;
  }

  .auth-heading h1,
  .auth-heading p {
    margin: 0;
  }

  .auth-heading p,
  .auth-status span,
  .auth-note {
    color: #5d6a7a;
    line-height: 1.5;
  }

  .eyebrow {
    color: #1f6f8b;
    font-size: 12px;
    font-weight: 800;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .auth-status {
    display: grid;
    gap: 5px;
    padding: 14px 16px;
    border-left: 4px solid #1f6f8b;
    background: #f5f9fb;
  }

  .auth-status.auth-warning {
    border-color: #c98716;
    background: #fffaf0;
  }

  .auth-actions {
    display: grid;
    gap: 10px;
  }

  .auth-button {
    width: 100%;
    min-height: 46px;
  }

  .authentication-progress {
    display: flex;
    min-height: 46px;
    align-items: center;
    justify-content: center;
    gap: 12px;
    color: #334354;
    font-weight: 700;
  }

  .spinner {
    width: 22px;
    height: 22px;
    box-sizing: border-box;
    border: 3px solid #cbd9df;
    border-top-color: #1f6f8b;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .secondary-button {
    min-height: 40px;
    padding: 0 16px;
    border: 1px solid #b8c4cf;
    border-radius: 4px;
    background: white;
    color: #293646;
    font-weight: 700;
    cursor: pointer;
  }

  .secondary-button:disabled {
    cursor: wait;
    opacity: 0.65;
  }

  .auth-error {
    color: #a62f3a;
    font-size: 14px;
    line-height: 1.45;
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

  .error,
  .warning,
  .exchange-rates,
  .summary-grid,
  .analytics-heading,
  .allocation-grid,
  .ai-panel,
  .table-section,
  .loading-state {
    max-width: 1180px;
    margin: 0 auto 16px;
  }

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

  .ai-panel {
    padding: 18px;
    border: 1px solid #d8e0e8;
    border-radius: 6px;
    background: white;
  }

  .ai-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
  }

  .ai-heading div {
    display: grid;
    gap: 4px;
  }

  .ai-heading h2 {
    margin: 0;
  }

  .ai-heading span,
  .ai-response small,
  .ai-status,
  .ai-error span {
    color: #5d6a7a;
  }

  .ai-status,
  .ai-response,
  .ai-error {
    margin-top: 16px;
    padding: 14px 16px;
    border-left: 4px solid #1f6f8b;
    background: #f5f9fb;
    line-height: 1.55;
  }

  .ai-response {
    display: grid;
    gap: 10px;
  }

  .ai-response-content :global(h2) {
    margin: 18px 0 6px;
    color: #243444;
    font-size: 18px;
    line-height: 1.35;
  }

  .ai-response-content :global(h2:first-child),
  .ai-response-content :global(p:first-child) {
    margin-top: 0;
  }

  .ai-response-content :global(h3) {
    margin: 14px 0 6px;
    color: #32485c;
    font-size: 16px;
  }

  .ai-response-content :global(p) {
    margin: 8px 0;
  }

  .ai-response-content :global(ul),
  .ai-response-content :global(ol) {
    margin: 8px 0;
    padding-left: 24px;
  }

  .ai-response-content :global(li + li) {
    margin-top: 5px;
  }

  .ai-response-content :global(a) {
    color: #14637d;
    overflow-wrap: anywhere;
  }

  .ai-response-content :global(code) {
    padding: 1px 4px;
    border-radius: 3px;
    background: #e8f0f4;
    font-family: Consolas, monospace;
  }

  .ai-error {
    display: grid;
    gap: 4px;
    border-color: #c2414b;
    background: #fff7f7;
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

    .auth-card {
      padding: 28px 22px;
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

    .ai-heading {
      align-items: stretch;
      flex-direction: column;
    }
  }
</style>
