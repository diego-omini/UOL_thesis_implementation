<script lang="ts">
  // App.svelte coordinates session lifecycle, authenticated data, and top-level frontend views.
  import { onDestroy, onMount, tick } from 'svelte'
  import {
    ChooseGatewayDirectory,
    GetPortfolio,
    GetSessionStatus,
    InitializeSession,
    KeepSessionAlive,
    Logout,
    OpenIBKRLogin,
    StartGateway,
  } from '../wailsjs/go/main/App.js'
  import AppHeader from './components/AppHeader.svelte'
  import AuthenticationView from './components/AuthenticationView.svelte'
  import { formatError } from './lib/format'
  import { acceptsAuthenticatedResponse } from './lib/session-requests'
  import type { InstrumentSearchResultView, PortfolioView, PositionView, SessionStatus } from './lib/types'
  import PortfolioDashboard from './views/PortfolioDashboard.svelte'
  import StockDetailView from './views/StockDetailView.svelte'

  let initializing = true
  let authAction = false
  let loading = false
  let waitingForLogin = false
  let applicationFocused = true
  let checkingSessionSequence: number | null = null
  let session: SessionStatus | null = null
  let portfolio: PortfolioView | null = null
  let selectedStock: InstrumentSearchResultView | null = null
  let errorMessage = ''
  let authenticationPoll: ReturnType<typeof setInterval> | null = null
  let keepAliveTimer: ReturnType<typeof setInterval> | null = null
  let sessionSequence = 0
  let dataGeneration = 0
  let appHeader: AppHeader

  $: selectedPosition = selectedStock && portfolio
    ? portfolio.Positions.find(position =>
        position.Quantity !== 0 && (
          position.Instrument.ContractID === selectedStock?.ContractID ||
          position.Instrument.Symbol.toUpperCase() === selectedStock?.Symbol.toUpperCase()
        )
      ) ?? null
    : null

  onMount(() => {
    window.addEventListener('focus', handleWindowFocus)
    window.addEventListener('blur', handleWindowBlur)
    void initializeAuthentication()
  })

  onDestroy(() => {
    sessionSequence++
    dataGeneration++
    window.removeEventListener('focus', handleWindowFocus)
    window.removeEventListener('blur', handleWindowBlur)
    stopAuthenticationPolling()
    stopKeepAlive()
  })

  // invalidateAuthenticatedData prevents pending portfolio and view work from crossing a session boundary.
  function invalidateAuthenticatedData(): void {
    dataGeneration++
    portfolio = null
    selectedStock = null
    loading = false
  }

  // applySession owns all transitions between authenticated and unauthenticated application state.
  function applySession(next: SessionStatus): void {
    session = next
    if (next.Ready) {
      waitingForLogin = false
      errorMessage = ''
      stopAuthenticationPolling()
      startKeepAlive()
      if (!portfolio && !loading) void refreshPortfolio()
      return
    }

    sessionSequence++
    waitingForLogin = false
    invalidateAuthenticatedData()
    stopKeepAlive()
    if (applicationFocused && next.State !== 'gateway_configuration_required' && next.State !== 'live_account') {
      startAuthenticationPolling()
    }
  }

  async function initializeAuthentication(): Promise<void> {
    const sequence = ++sessionSequence
    try {
      const next = await InitializeSession()
      if (sequence === sessionSequence) applySession(next)
    } catch (error) {
      if (sequence === sessionSequence) {
        applySession({
          State: 'error', GatewayReachable: false, Authenticated: false, Connected: false,
          Competing: false, Paper: false, PaperKnown: false, Ready: false, Message: formatError(error),
        } as SessionStatus)
      }
    } finally {
      initializing = false
    }
  }

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
    while (checkingSessionSequence === sessionSequence) {
      await new Promise(resolve => window.setTimeout(resolve, 25))
    }
  }

  async function renderAuthenticationProgress(): Promise<void> {
    await tick()
    await new Promise(resolve => window.requestAnimationFrame(() => resolve(undefined)))
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
    const sequence = sessionSequence
    if (checkingSessionSequence === sequence) return
    checkingSessionSequence = sequence
    try {
      const next = await GetSessionStatus()
      if (sequence === sessionSequence) applySession(next)
    } catch (error) {
      if (sequence === sessionSequence) errorMessage = formatError(error)
    } finally {
      if (checkingSessionSequence === sequence) checkingSessionSequence = null
    }
  }

  async function maintainSession(): Promise<void> {
    const sequence = sessionSequence
    try {
      const next = await KeepSessionAlive()
      if (sequence === sessionSequence) applySession(next)
    } catch (error) {
      if (sequence !== sessionSequence) return
      errorMessage = formatError(error)
      await refreshSession()
    }
  }

  // runAuthenticationAction isolates explicit setup actions from polling responses already in flight.
  async function runAuthenticationAction(action: () => Promise<SessionStatus>): Promise<void> {
    const sequence = ++sessionSequence
    stopAuthenticationPolling()
    authAction = true
    errorMessage = ''
    try {
      const next = await action()
      if (sequence === sessionSequence) applySession(next)
    } catch (error) {
      if (sequence === sessionSequence) {
        errorMessage = formatError(error)
        if (!session?.Ready && applicationFocused) startAuthenticationPolling()
      }
    } finally {
      authAction = false
    }
  }

  function chooseGatewayDirectory(): void {
    void runAuthenticationAction(ChooseGatewayDirectory)
  }

  function startGateway(): void {
    void runAuthenticationAction(StartGateway)
  }

  function retryAuthentication(): void {
    void runAuthenticationAction(InitializeSession)
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

  // logout invalidates every earlier session and data response before contacting the gateway.
  async function logout(): Promise<void> {
    if (!window.confirm('Log out of the current IBKR session?')) return
    const sequence = ++sessionSequence
    stopAuthenticationPolling()
    stopKeepAlive()
    invalidateAuthenticatedData()
    session = {
      ...session,
      State: 'authentication_required',
      Authenticated: false,
      Connected: false,
      Ready: false,
      Message: 'Logging out of the current IBKR session...',
    } as SessionStatus
    authAction = true
    errorMessage = ''
    try {
      const next = await Logout()
      if (sequence === sessionSequence) applySession(next)
    } catch (error) {
      if (sequence === sessionSequence) {
        errorMessage = formatError(error)
        await refreshSession()
      }
    } finally {
      authAction = false
    }
  }

  // refreshPortfolio accepts a snapshot only for the active authenticated data generation.
  async function refreshPortfolio(): Promise<void> {
    if (!session?.Ready || loading) return
    const request = { session: sessionSequence, generation: dataGeneration + 1 }
    const generation = ++dataGeneration
    loading = true
    errorMessage = ''
    try {
      const nextPortfolio = await GetPortfolio()
      if (acceptsAuthenticatedResponse(request, sessionSequence, dataGeneration, Boolean(session?.Ready))) {
        portfolio = nextPortfolio
      }
    } catch (error) {
      if (!acceptsAuthenticatedResponse(request, sessionSequence, dataGeneration, Boolean(session?.Ready))) return
      portfolio = null
      await refreshSession()
      if (session?.Ready) errorMessage = formatError(error)
    } finally {
      if (request.session === sessionSequence && generation === dataGeneration) loading = false
    }
  }

  function openPosition(event: CustomEvent<PositionView>): void {
    const position = event.detail
    openStock({
      ContractID: position.Instrument.ContractID,
      Symbol: position.Instrument.Symbol,
      Name: position.Instrument.Name,
      Exchange: position.Instrument.Exchange,
      AssetClass: position.Instrument.AssetClass,
    } as InstrumentSearchResultView)
  }

  function openStock(stock: InstrumentSearchResultView): void {
    selectedStock = stock
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  function showDashboard(): void {
    selectedStock = null
    window.scrollTo({ top: 0, behavior: 'smooth' })
    void tick().then(() => appHeader?.focusDashboard())
  }
</script>

<main class:auth-page={!session?.Ready} class="page">
  {#if initializing || !session?.Ready}
    <AuthenticationView
      {initializing}
      {session}
      {waitingForLogin}
      {authAction}
      {errorMessage}
      on:chooseGatewayDirectory={chooseGatewayDirectory}
      on:startGateway={startGateway}
      on:openLogin={openIBKRLogin}
      on:retry={retryAuthentication}
    />
  {:else}
    <AppHeader
      bind:this={appHeader}
      portfolioLoading={loading}
      {authAction}
      on:dashboard={showDashboard}
      on:refresh={() => void refreshPortfolio()}
      on:logout={() => void logout()}
      on:stock={(event) => openStock(event.detail)}
      on:sessionIssue={() => void refreshSession()}
    />

    {#if errorMessage}
      <section class="gateway-error" aria-live="assertive"><strong>Gateway error</strong><span>{errorMessage}</span></section>
    {/if}

    {#if selectedStock}
      {#key selectedStock.ContractID}
        <StockDetailView
          stock={selectedStock}
          position={selectedPosition}
          on:back={showDashboard}
          on:sessionIssue={() => void refreshSession()}
          on:tradeComplete={() => void refreshPortfolio()}
        />
      {/key}
    {:else if loading}
      <section class="loading-state" aria-live="polite">Loading portfolio...</section>
    {:else if portfolio}
      {#key dataGeneration}
        <PortfolioDashboard {portfolio} on:stock={openPosition} on:sessionIssue={() => void refreshSession()} />
      {/key}
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
    background: radial-gradient(circle at top left, rgba(31, 111, 139, 0.16), transparent 38%), #f5f7fa;
  }

  .gateway-error,
  .loading-state {
    display: grid;
    gap: 6px;
    max-width: 1180px;
    margin: 0 auto 16px;
    padding: 14px 16px;
    border-left: 4px solid;
    background: white;
    box-shadow: 0 1px 3px rgba(24, 33, 47, 0.08);
  }

  .gateway-error { border-color: #c2414b; }
  .loading-state { border-color: #7a8797; }
  .gateway-error span { color: #5d6a7a; line-height: 1.45; }

  @media (max-width: 780px) {
    .page { padding: 20px; }
  }
</style>
