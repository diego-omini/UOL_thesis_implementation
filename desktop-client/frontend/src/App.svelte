<script lang="ts">
  // App.svelte contains the root frontend component rendered by the Wails application.
  import DOMPurify from 'dompurify'
  import { marked } from 'marked'
  import { onDestroy, onMount, tick } from "svelte";
  import {
    ChooseGatewayDirectory,
    GetPortfolio,
    GetPortfolioNews,
    GetPortfolioResume,
    GetSessionStatus,
    GetStockHistory,
    GetStockSnapshot,
    InitializeSession,
    KeepSessionAlive,
    Logout,
    OpenIBKRLogin,
    SearchStocks,
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

  type InstrumentSearchResultView = {
    ContractID: number
    Symbol: string
    Name: string
    Exchange: string
    AssetClass: string
  }

  type OptionalNumberView = {
    Value: number
    Available: boolean
    UnavailableReason: string
  }

  type StockSnapshotView = {
    Instrument: InstrumentView
    Currency: string
    OpenPrice: OptionalNumberView
    ClosePrice: OptionalNumberView
    BidPrice: OptionalNumberView
    AskPrice: OptionalNumberView
    Volume: OptionalNumberView
    High52Week: OptionalNumberView
    Low52Week: OptionalNumberView
    MarketDataStatus: string
    AsOf: string
  }

  type StockHistoryPointView = {
    Time: string
    Open: number
    High: number
    Low: number
    Close: number
    Volume: number
  }

  type StockHistoryView = {
    Range: string
    Points: StockHistoryPointView[]
    MarketDataStatus: string
    DelaySeconds: number
    UnavailableReason: string
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
  let portfolioNews: AIResponseView | null = null
  let portfolioNewsError = ''
  let portfolioNewsLoading = false
  let positionsPromptsOpen = false
  let selectedStock: InstrumentSearchResultView | null = null
  let stockSnapshot: StockSnapshotView | null = null
  let stockSnapshotLoading = false
  let stockSnapshotError = ''
  let stockHistory: StockHistoryView | null = null
  let stockHistoryLoading = false
  let stockHistoryError = ''
  let selectedStockRange = '24h'
  let stockPageSequence = 0
  let stockSearchQuery = ''
  let stockSearchResults: InstrumentSearchResultView[] = []
  let stockSearchOpen = false
  let stockSearchLoading = false
  let stockSearchTimer: ReturnType<typeof setTimeout> | null = null
  let stockSearchSequence = 0
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
    clearStockSearchTimer()
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
    portfolioNews = null
    portfolioNewsError = ''
    resetStockPage()
    clearStockSearch()
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
      portfolioNews = null
      portfolioNewsError = ''
      resetStockPage()
      clearStockSearch()
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
    portfolioNews = null
    portfolioNewsError = ''

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
    positionsPromptsOpen = false
    portfolioResumeLoading = true
    portfolioResume = null
    portfolioResumeError = ''
    portfolioNews = null
    portfolioNewsError = ''

    try {
      portfolioResume = await GetPortfolioResume()
    } catch (error) {
      portfolioResumeError = formatError(error)
    } finally {
      portfolioResumeLoading = false
    }
  }

  // getPortfolioNews requests current public research for every normalized position.
  async function getPortfolioNews(): Promise<void> {
    positionsPromptsOpen = false
    portfolioNewsLoading = true
    portfolioNews = null
    portfolioNewsError = ''
    portfolioResume = null
    portfolioResumeError = ''

    try {
      portfolioNews = await GetPortfolioNews()
    } catch (error) {
      portfolioNewsError = formatError(error)
    } finally {
      portfolioNewsLoading = false
    }
  }

  function clearStockSearchTimer(): void {
    if (!stockSearchTimer) return
    clearTimeout(stockSearchTimer)
    stockSearchTimer = null
  }

  function clearStockSearch(): void {
    clearStockSearchTimer()
    stockSearchQuery = ''
    stockSearchResults = []
    stockSearchOpen = false
    stockSearchLoading = false
    stockSearchSequence++
  }

  function handleStockSearchInput(): void {
    clearStockSearchTimer()
    stockSearchResults = []
    const query = stockSearchQuery.trim()
    stockSearchOpen = query.length > 0
    if (!query) {
      stockSearchLoading = false
      return
    }
    stockSearchTimer = setTimeout(() => void runStockSearch(query), 250)
  }

  // runStockSearch ignores stale responses when the user types another query before IBKR responds.
  async function runStockSearch(query: string): Promise<InstrumentSearchResultView[]> {
    const sequence = ++stockSearchSequence
    stockSearchLoading = true
    try {
      const results = await SearchStocks(query)
      if (sequence === stockSearchSequence && query === stockSearchQuery.trim()) {
        stockSearchResults = results
        stockSearchOpen = true
      }
      return results
    } catch (error) {
      if (sequence === stockSearchSequence) {
        stockSearchResults = []
      }
      return []
    } finally {
      if (sequence === stockSearchSequence) {
        stockSearchLoading = false
      }
    }
  }

  async function submitStockSearch(): Promise<void> {
    clearStockSearchTimer()
    const ticker = stockSearchQuery.trim().toUpperCase()
    if (!ticker) return
    const availableResults = stockSearchResults.length > 0
      ? stockSearchResults
      : await runStockSearch(ticker)
    const exactMatch = availableResults.find(result => result.Symbol.toUpperCase() === ticker)
    if (exactMatch) {
      openStock(exactMatch)
    }
  }

  function openPosition(position: PositionView): void {
    openStock({
      ContractID: position.Instrument.ContractID,
      Symbol: position.Instrument.Symbol,
      Name: position.Instrument.Name,
      Exchange: position.Instrument.Exchange,
      AssetClass: position.Instrument.AssetClass,
    })
  }

  function openStock(stock: InstrumentSearchResultView): void {
    stockPageSequence++
    selectedStock = stock
    stockSnapshot = null
    stockSnapshotError = ''
    stockHistory = null
    stockHistoryError = ''
    selectedStockRange = '24h'
    clearStockSearch()
    window.scrollTo({ top: 0, behavior: 'smooth' })
    const sequence = stockPageSequence
    void loadStockSnapshot(stock.ContractID, sequence)
    void loadStockHistory(stock.ContractID, selectedStockRange, sequence)
  }

  function showDashboard(): void {
    resetStockPage()
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  function resetStockPage(): void {
    stockPageSequence++
    selectedStock = null
    stockSnapshot = null
    stockSnapshotLoading = false
    stockSnapshotError = ''
    stockHistory = null
    stockHistoryLoading = false
    stockHistoryError = ''
    selectedStockRange = '24h'
  }

  async function loadStockSnapshot(contractID: number, sequence: number): Promise<void> {
    stockSnapshotLoading = true
    stockSnapshotError = ''
    try {
      const result = await GetStockSnapshot(contractID)
      if (sequence === stockPageSequence) {
        stockSnapshot = result
      }
    } catch (error) {
      if (sequence === stockPageSequence) {
        stockSnapshotError = formatError(error)
      }
    } finally {
      if (sequence === stockPageSequence) {
        stockSnapshotLoading = false
      }
    }
  }

  async function loadStockHistory(contractID: number, timeRange: string, sequence: number): Promise<void> {
    stockHistoryLoading = true
    stockHistoryError = ''
    try {
      const result = await GetStockHistory(contractID, timeRange)
      if (sequence === stockPageSequence && timeRange === selectedStockRange) {
        stockHistory = result
      }
    } catch (error) {
      if (sequence === stockPageSequence && timeRange === selectedStockRange) {
        stockHistory = null
        stockHistoryError = formatError(error)
      }
    } finally {
      if (sequence === stockPageSequence && timeRange === selectedStockRange) {
        stockHistoryLoading = false
      }
    }
  }

  function selectStockRange(timeRange: string): void {
    if (!selectedStock || timeRange === selectedStockRange) return
    selectedStockRange = timeRange
    stockHistory = null
    void loadStockHistory(selectedStock.ContractID, timeRange, stockPageSequence)
  }

  function handlePositionKeydown(event: KeyboardEvent, position: PositionView): void {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      openPosition(position)
    }
  }

  function formatStockPrice(value: OptionalNumberView | undefined, currency: string): string {
    if (!value?.Available) return '-'
    return new Intl.NumberFormat(undefined, {
      style: 'currency', currency: currency || 'USD', maximumFractionDigits: 4,
    }).format(value.Value)
  }

  function formatStockNumber(value: OptionalNumberView | undefined, maximumFractionDigits = 2): string {
    if (!value?.Available) return '-'
    return new Intl.NumberFormat(undefined, { maximumFractionDigits }).format(value.Value)
  }

  function stockMetricTitle(value: OptionalNumberView | undefined): string {
    return value?.Available ? '' : value?.UnavailableReason || 'Value unavailable'
  }

  function marketDataStatusLabel(status: string): string {
    const labels: Record<string, string> = {
      R: 'Real-time', D: 'Delayed', Z: 'Frozen', Y: 'Frozen delayed',
      N: 'Not subscribed', i: 'Incomplete', v: 'Vendor-display exempt',
    }
    return labels[status.charAt(0)] || status
  }

  function averageCostForChart(position: PositionView | null, snapshot: StockSnapshotView | null): number | null {
    if (!position || position.Quantity === 0 || !position.AverageCost.Available) return null
    const chartCurrency = snapshot?.Currency || position.Instrument.Currency
    const averageCurrency = position.AverageCost.Currency
    if (chartCurrency && averageCurrency && chartCurrency.trim().toUpperCase() !== averageCurrency.trim().toUpperCase()) return null
    return Number.isFinite(position.AverageCost.Amount) && position.AverageCost.Amount > 0
      ? position.AverageCost.Amount
      : null
  }

  function chartValues(history: StockHistoryView, averageCost: number | null): number[] {
    const closes = history.Points.map(point => point.Close).filter(Number.isFinite)
    if (averageCost !== null) closes.push(averageCost)
    return closes
  }

  function wholePriceStep(rawStep: number): number {
    const minimumStep = Math.max(1, rawStep)
    const magnitude = 10 ** Math.floor(Math.log10(minimumStep))
    for (const factor of [1, 2.5, 5, 10]) {
      const candidate = factor * magnitude
      if (Number.isInteger(candidate) && candidate >= minimumStep) return candidate
    }
    return 10 * magnitude
  }

  // chartPriceAxis expands the data to equally spaced whole-number price ticks.
  function chartPriceAxis(history: StockHistoryView, averageCost: number | null): { min: number; max: number; step: number } {
    const values = chartValues(history, averageCost)
    if (values.length === 0) return { min: 0, max: 1, step: 1 }
    const dataMin = Math.min(...values)
    const dataMax = Math.max(...values)
    const step = wholePriceStep(dataMin === dataMax ? Math.abs(dataMin) * 0.02 : (dataMax - dataMin) / 4)
    let min = Math.floor(dataMin / step) * step
    let max = Math.ceil(dataMax / step) * step
    if (min === dataMin) min -= step
    if (max === dataMax) max += step
    if (dataMin >= 0) min = Math.max(0, min)
    if (min === max) max = min + step
    return { min, max, step }
  }

  // chartTimeTicks distributes labels across actual bars so the plotted line fills the available width.
  function chartTimeTicks(history: StockHistoryView): Array<{ label: string; x: number }> {
    const pointCount = history.Points.length
    if (pointCount === 0) return []
    const tickCount = Math.min(5, pointCount)
    if (tickCount === 1) {
      return [{ label: formatChartDate(history.Points[0].Time, selectedStockRange), x: 480 }]
    }
    const indexes = Array.from({ length: tickCount }, (_, index) =>
      Math.round(index * (pointCount - 1) / (tickCount - 1))
    )
    return indexes.map(pointIndex => ({
      label: formatChartDate(history.Points[pointIndex].Time, selectedStockRange),
      x: pointIndex / (pointCount - 1) * 960,
    }))
  }

  // chartPolyline maps the selected close-price series into a stable SVG coordinate system.
  function chartPolyline(history: StockHistoryView, averageCost: number | null): string {
    if (history.Points.length === 0) return ''
    const { min, max } = chartPriceAxis(history, averageCost)
    const width = 960
    const height = 280
    return history.Points.map((point, index) => {
      const x = history.Points.length === 1 ? width / 2 : index / (history.Points.length - 1) * width
      const y = height - ((point.Close - min) / (max - min)) * height
      return `${x.toFixed(2)},${y.toFixed(2)}`
    }).join(' ')
  }

  function chartYTicks(history: StockHistoryView, averageCost: number | null): Array<{ value: number; y: number }> {
    const { min, max, step } = chartPriceAxis(history, averageCost)
    const tickCount = Math.round((max - min) / step)
    return Array.from({ length: tickCount + 1 }, (_, index) => {
      const value = max - step * index
      return { value, y: (max - value) / (max - min) * 280 }
    })
  }

  function formatChartPrice(value: number, currency: string): string {
    return new Intl.NumberFormat(undefined, {
      style: 'currency', currency: currency || 'USD', minimumFractionDigits: 0, maximumFractionDigits: 0,
    }).format(value)
  }

  function averageCostY(history: StockHistoryView, averageCost: number | null): number {
    if (averageCost === null) return 0
    const { min, max } = chartPriceAxis(history, averageCost)
    return 280 - (averageCost - min) / (max - min) * 280
  }

  function positionChartResult(history: StockHistoryView, averageCost: number | null): 'gain' | 'loss' | null {
    if (!selectedPosition || averageCost === null || history.Points.length === 0) return null
    const latestClose = history.Points[history.Points.length - 1].Close
    const profitable = selectedPosition.Quantity < 0 ? latestClose <= averageCost : latestClose >= averageCost
    return profitable ? 'gain' : 'loss'
  }

  function formatChartDate(value: string, timeRange: string): string {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return '-'
    if (timeRange === '24h') return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    if (timeRange === '1y') return date.toLocaleDateString([], { month: 'short', year: 'numeric' })
    if (timeRange === 'all') return String(date.getFullYear())
    return date.toLocaleDateString()
  }

  $: selectedPosition = selectedStock && portfolio
    ? portfolio.Positions.find(position =>
        position.Quantity !== 0 && (
          position.Instrument.ContractID === selectedStock?.ContractID ||
          position.Instrument.Symbol.toUpperCase() === selectedStock?.Symbol.toUpperCase()
        )
      ) ?? null
    : null
  $: chartAverageCost = averageCostForChart(selectedPosition, stockSnapshot)

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
  <header class="app-header">
    <button class="dashboard-link" type="button" on:click={showDashboard} aria-label="Open portfolio dashboard">
      <span class="eyebrow">IBKR paper trading</span>
      <h1>Portfolio dashboard</h1>
    </button>

    <form class="stock-search" role="search" on:submit|preventDefault={submitStockSearch}>
      <label class="sr-only" for="stock-search-input">Search by ticker or company name</label>
      <div class="stock-search-field">
        <span class="search-icon" aria-hidden="true"></span>
        <input
          id="stock-search-input"
          type="search"
          placeholder="Search ticker or company"
          autocomplete="off"
          bind:value={stockSearchQuery}
          on:input={handleStockSearchInput}
          on:focus={() => stockSearchOpen = stockSearchQuery.trim().length > 0}
          on:blur={() => window.setTimeout(() => stockSearchOpen = false, 120)}
        />
        {#if stockSearchLoading}<span class="search-loading" aria-label="Searching"></span>{/if}
      </div>

      {#if stockSearchOpen}
        <div class="stock-search-results" role="listbox" aria-label="Stock search results">
          {#each stockSearchResults as result}
            <button type="button" role="option" aria-selected="false" on:mousedown|preventDefault on:click={() => openStock(result)}>
              <strong>{result.Symbol}</strong>
              <span>{result.Name || 'Unnamed stock'}</span>
              <small>{result.Exchange || 'Exchange unavailable'}</small>
            </button>
          {:else}
            <div class="stock-search-empty">
              {stockSearchLoading ? 'Searching...' : 'No matching stocks'}
            </div>
          {/each}
        </div>
      {/if}
    </form>

    <div class="actions">
      <button class="button" disabled={loading} on:click={refreshPortfolio}>
        {loading ? 'Loading...' : 'Refresh portfolio'}
      </button>
      <button class="secondary-button" disabled={authAction} on:click={logout}>Log out</button>
    </div>
  </header>

  {#if errorMessage}
    <section class="error" aria-live="assertive">
      <strong>Gateway error</strong>
      <span>{errorMessage}</span>
    </section>
  {/if}

  {#if selectedStock}
    <section class="stock-page" aria-label={`${selectedStock.Symbol} stock details`}>
      <button class="back-button" type="button" on:click={showDashboard}>← Back to portfolio</button>
      <div class="stock-heading">
        <div>
          <span class="eyebrow">Stock</span>
          <h2>{stockSnapshot?.Instrument.Symbol || selectedStock.Symbol}</h2>
          <p>{stockSnapshot?.Instrument.Name || selectedStock.Name || 'Company name unavailable'}</p>
        </div>
        <div class="stock-badges">
          <span>{stockSnapshot?.Instrument.AssetClass || selectedStock.AssetClass || 'STK'}</span>
          {#if stockSnapshot?.Instrument.Exchange || selectedStock.Exchange}
            <span>{stockSnapshot?.Instrument.Exchange || selectedStock.Exchange}</span>
          {/if}
          {#if stockSnapshot?.Currency}<span>{stockSnapshot.Currency}</span>{/if}
        </div>
      </div>

      <section class="stock-chart-card" aria-label="Historical stock price">
        <div class="stock-chart-heading">
          <div>
            <h2>Price history</h2>
            {#if stockHistory?.Points.length}
              <span>
                {formatChartDate(stockHistory.Points[0].Time, selectedStockRange)} –
                {formatChartDate(stockHistory.Points[stockHistory.Points.length - 1].Time, selectedStockRange)}
              </span>
            {:else}
              <span>IBKR trade-price history</span>
            {/if}
          </div>
          <div class="range-selector" aria-label="Price history range">
            {#each [['24h', '24 hours'], ['1m', '1 month'], ['1y', '1 year'], ['all', 'All']] as range}
              <button
                type="button"
                class:active={selectedStockRange === range[0]}
                aria-pressed={selectedStockRange === range[0]}
                on:click={() => selectStockRange(range[0])}
              >{range[1]}</button>
            {/each}
          </div>
        </div>

        {#if stockHistoryLoading}
          <div class="chart-message" role="status">Loading price history...</div>
        {:else if stockHistoryError}
          <div class="chart-message error-text" role="alert">{stockHistoryError}</div>
        {:else if stockHistory && stockHistory.Points.length > 0}
          <div class="chart-area">
            <div class="chart-y-labels" aria-hidden="true">
              {#each chartYTicks(stockHistory, chartAverageCost) as tick}
                <span>{formatChartPrice(tick.value, stockSnapshot?.Currency || selectedPosition?.AverageCost.Currency || 'USD')}</span>
              {/each}
            </div>
            <div class="chart-plot">
              <svg viewBox="0 0 960 280" preserveAspectRatio="none" role="img" aria-label={`${selectedStock.Symbol} closing price chart`}>
                {#each chartYTicks(stockHistory, chartAverageCost) as tick}
                  <line class="chart-grid-line" x1="0" y1={tick.y} x2="960" y2={tick.y}></line>
                {/each}
                {#each chartTimeTicks(stockHistory) as tick}
                  <line class="chart-grid-line" x1={tick.x} y1="0" x2={tick.x} y2="280"></line>
                {/each}
                <polyline points={chartPolyline(stockHistory, chartAverageCost)}></polyline>
                {#if chartAverageCost !== null}
                  <line
                    class="average-cost-line"
                    class:gain={positionChartResult(stockHistory, chartAverageCost) === 'gain'}
                    class:loss={positionChartResult(stockHistory, chartAverageCost) === 'loss'}
                    x1="0"
                    y1={averageCostY(stockHistory, chartAverageCost)}
                    x2="960"
                    y2={averageCostY(stockHistory, chartAverageCost)}
                  ></line>
                {/if}
              </svg>
              <div class="chart-x-labels" aria-hidden="true">
                {#each chartTimeTicks(stockHistory) as tick}
                  <span style={`left: ${tick.x / 9.6}%`}>{tick.label}</span>
                {/each}
              </div>
            </div>
          </div>
          <div class="chart-meta">
            <span>{stockHistory.Points.length} bars</span>
            {#if chartAverageCost !== null}
              <span
                class:gain-text={positionChartResult(stockHistory, chartAverageCost) === 'gain'}
                class:loss-text={positionChartResult(stockHistory, chartAverageCost) === 'loss'}
              >Average cost: {formatMoney(selectedPosition?.AverageCost)}</span>
            {/if}
            {#if selectedStockRange === 'all'}<span>Maximum available range: 15 years</span>{/if}
            {#if stockHistory.DelaySeconds > 0}<span>Delayed {stockHistory.DelaySeconds / 60} minutes</span>{/if}
          </div>
        {:else}
          <div class="chart-message">{stockHistory?.UnavailableReason || 'No historical prices are available.'}</div>
        {/if}
      </section>

      {#if stockSnapshotLoading}
        <section class="stock-data-status" role="status">Loading current market data...</section>
      {:else if stockSnapshotError}
        <section class="stock-data-status error-text" role="alert">{stockSnapshotError}</section>
      {:else if stockSnapshot}
        <section class="stock-metrics-grid" aria-label="Current stock data">
          <article class="stock-metric" title={stockMetricTitle(stockSnapshot.OpenPrice)}>
            <span>Last open</span><strong>{formatStockPrice(stockSnapshot.OpenPrice, stockSnapshot.Currency)}</strong>
          </article>
          <article class="stock-metric" title={stockMetricTitle(stockSnapshot.ClosePrice)}>
            <span>Last close</span><strong>{formatStockPrice(stockSnapshot.ClosePrice, stockSnapshot.Currency)}</strong>
          </article>
          <article class="stock-metric" title={stockMetricTitle(stockSnapshot.BidPrice)}>
            <span>Bid</span><strong>{formatStockPrice(stockSnapshot.BidPrice, stockSnapshot.Currency)}</strong>
          </article>
          <article class="stock-metric" title={stockMetricTitle(stockSnapshot.AskPrice)}>
            <span>Ask</span><strong>{formatStockPrice(stockSnapshot.AskPrice, stockSnapshot.Currency)}</strong>
          </article>
          <article class="stock-metric" title={stockMetricTitle(stockSnapshot.Volume)}>
            <span>Volume</span><strong>{formatStockNumber(stockSnapshot.Volume, 0)}</strong>
          </article>
          <article class="stock-metric" title={stockMetricTitle(stockSnapshot.High52Week)}>
            <span>52-week high</span><strong>{formatStockPrice(stockSnapshot.High52Week, stockSnapshot.Currency)}</strong>
          </article>
          <article class="stock-metric" title={stockMetricTitle(stockSnapshot.Low52Week)}>
            <span>52-week low</span><strong>{formatStockPrice(stockSnapshot.Low52Week, stockSnapshot.Currency)}</strong>
          </article>
        </section>
        <div class="stock-data-footnote">
          <span>Updated {formatDate(stockSnapshot.AsOf)}</span>
          {#if stockSnapshot.MarketDataStatus}<span>IBKR availability: {marketDataStatusLabel(stockSnapshot.MarketDataStatus)}</span>{/if}
          <span>Hover unavailable values for details.</span>
        </div>
      {/if}

      {#if selectedPosition}
        <section class="stock-position-card" aria-label="Current portfolio position">
          <div class="section-heading">
            <div class="section-title">
              <h2>Current position</h2>
              <span>{formatPercent(selectedPosition.PortfolioWeight)} of portfolio</span>
            </div>
          </div>
          <div class="stock-position-grid">
            <div><span>Quantity</span><strong>{formatNumber(selectedPosition.Quantity)}</strong></div>
            <div><span>Average cost</span><strong>{formatMoney(selectedPosition.AverageCost)}</strong></div>
            <div><span>Market price</span><strong>{formatMoney(selectedPosition.MarketPrice)}</strong></div>
            <div><span>Market value</span><strong>{formatMoney(selectedPosition.MarketValue)}</strong></div>
            <div>
              <span>Unrealized P/L</span>
              <strong class:loss={selectedPosition.UnrealizedPnL.Available && selectedPosition.UnrealizedPnL.Amount < 0} class="pnl">
                {formatMoney(selectedPosition.UnrealizedPnL)}
              </strong>
            </div>
          </div>
        </section>
      {:else}
        <section class="stock-not-held">
          This stock is not part of the current portfolio. Market data and trading controls will be added in their dedicated milestones.
        </section>
      {/if}
    </section>
  {:else if portfolio}
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
        <div class="section-title">
          <h2>Positions</h2>
          <span>{portfolio.Positions.length} open</span>
        </div>

        <details class="prompt-dropdown" bind:open={positionsPromptsOpen}>
          <summary>Positions prompts</summary>
          <div class="prompt-menu">
            <button
              disabled={portfolio.Empty || portfolioResumeLoading || portfolioNewsLoading}
              on:click={getPortfolioResume}
            >
              {portfolioResumeLoading ? 'Generating portfolio resume...' : 'Get portfolio resume'}
            </button>
            <button
              disabled={portfolio.Empty || portfolioResumeLoading || portfolioNewsLoading}
              on:click={getPortfolioNews}
            >
              {portfolioNewsLoading ? 'Researching portfolio news...' : 'Get portfolio news'}
            </button>
          </div>
        </details>
      </div>

      {#if portfolioResumeLoading || portfolioNewsLoading}
        <div class="ai-status" role="status" aria-live="polite">
          {portfolioNewsLoading ? 'Researching the latest news for every position...' : 'Waiting for the configured AI provider...'}
        </div>
      {:else if portfolioNewsError}
        <div class="ai-error" role="alert" aria-live="assertive">
          <strong>Portfolio news unavailable</strong>
          <span>{portfolioNewsError}</span>
        </div>
      {:else if portfolioResumeError}
        <div class="ai-error" role="alert" aria-live="assertive">
          <strong>Portfolio resume unavailable</strong>
          <span>{portfolioResumeError}</span>
        </div>
      {:else if portfolioNews}
        <div class="ai-response" aria-live="polite">
          <div class="ai-response-content">
            {@html renderMarkdown(portfolioNews.Text)}
          </div>
          <small>
            {portfolioNews.Provider} · {portfolioNews.Model} · {portfolioNews.PromptID} v{portfolioNews.PromptVersion}
          </small>
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
                <tr
                  class="position-row"
                  role="link"
                  tabindex="0"
                  aria-label={`Open ${position.Instrument.Symbol || position.Instrument.Name} stock page`}
                  on:click={() => openPosition(position)}
                  on:keydown={(event) => handlePositionKeydown(event, position)}
                >
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

  .app-header {
    position: sticky;
    z-index: 10;
    top: 0;
    display: grid;
    grid-template-columns: auto minmax(220px, 1fr) auto;
    align-items: center;
    gap: 24px;
    max-width: 1180px;
    margin: -16px auto 20px;
    padding: 16px;
    border: 1px solid #d8e0e8;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.97);
    box-shadow: 0 4px 16px rgba(24, 33, 47, 0.09);
    backdrop-filter: blur(8px);
  }

  .dashboard-link {
    display: grid;
    gap: 2px;
    padding: 0;
    border: 0;
    background: transparent;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .dashboard-link h1 {
    font-size: 22px;
  }

  .stock-search {
    position: relative;
    width: min(100%, 460px);
    justify-self: center;
  }

  .stock-search-field {
    position: relative;
  }

  .stock-search input {
    width: 100%;
    min-height: 42px;
    box-sizing: border-box;
    padding: 0 42px;
    border: 1px solid #b8c4cf;
    border-radius: 6px;
    background: #f9fbfc;
    color: #18212f;
    font: inherit;
  }

  .stock-search input:focus {
    border-color: #1f6f8b;
    background: white;
    box-shadow: 0 0 0 3px rgba(31, 111, 139, 0.14);
    outline: none;
  }

  .search-icon {
    position: absolute;
    z-index: 1;
    top: 12px;
    left: 15px;
    width: 13px;
    height: 13px;
    border: 2px solid #637282;
    border-radius: 50%;
  }

  .search-icon::after {
    position: absolute;
    right: -5px;
    bottom: -4px;
    width: 7px;
    height: 2px;
    background: #637282;
    content: '';
    transform: rotate(45deg);
  }

  .search-loading {
    position: absolute;
    top: 13px;
    right: 15px;
    width: 14px;
    height: 14px;
    border: 2px solid #cbd9df;
    border-top-color: #1f6f8b;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  .stock-search-results {
    position: absolute;
    z-index: 12;
    top: calc(100% + 7px);
    right: 0;
    left: 0;
    overflow: hidden;
    border: 1px solid #d8e0e8;
    border-radius: 6px;
    background: white;
    box-shadow: 0 12px 28px rgba(24, 33, 47, 0.18);
  }

  .stock-search-results button {
    display: grid;
    grid-template-columns: 62px minmax(0, 1fr) auto;
    gap: 10px;
    width: 100%;
    padding: 11px 13px;
    border: 0;
    border-bottom: 1px solid #edf1f5;
    background: white;
    color: #293646;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .stock-search-results button:last-child {
    border-bottom: 0;
  }

  .stock-search-results button:hover,
  .stock-search-results button:focus-visible {
    background: #edf5f8;
    outline: none;
  }

  .stock-search-results button span {
    overflow: hidden;
    color: #516171;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .stock-search-results button small {
    color: #6f7b89;
  }

  .stock-search-empty {
    padding: 13px;
    color: #6f7b89;
    font-size: 14px;
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    clip-path: inset(50%);
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
  .table-section,
  .loading-state {
    max-width: 1180px;
    margin: 0 auto 16px;
  }

  .stock-page {
    display: grid;
    gap: 16px;
    max-width: 1180px;
    margin: 0 auto;
  }

  .back-button {
    width: max-content;
    padding: 0;
    border: 0;
    background: transparent;
    color: #1f6f8b;
    font: inherit;
    font-weight: 700;
    cursor: pointer;
  }

  .stock-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
    padding: 26px;
    border: 1px solid #d8e0e8;
    border-radius: 8px;
    background: white;
  }

  .stock-heading h2 {
    margin-top: 4px;
    font-size: 36px;
  }

  .stock-heading p {
    margin: 5px 0 0;
    color: #5d6a7a;
  }

  .stock-badges {
    display: flex;
    gap: 8px;
  }

  .stock-badges span {
    padding: 5px 9px;
    border-radius: 999px;
    background: #edf5f8;
    color: #14566e;
    font-size: 12px;
    font-weight: 800;
  }

  .stock-chart-card {
    overflow: hidden;
    border: 1px solid #d8e0e8;
    border-radius: 8px;
    background: white;
  }

  .stock-chart-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 18px;
    padding: 18px 20px;
    border-bottom: 1px solid #edf1f5;
  }

  .stock-chart-heading > div:first-child {
    display: grid;
    gap: 4px;
  }

  .stock-chart-heading h2 {
    font-size: 18px;
  }

  .stock-chart-heading span,
  .chart-meta,
  .stock-data-footnote {
    color: #6f7b89;
    font-size: 12px;
  }

  .range-selector {
    display: flex;
    gap: 4px;
    padding: 3px;
    border-radius: 6px;
    background: #edf1f5;
  }

  .range-selector button {
    min-height: 32px;
    padding: 0 11px;
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: #4c5b6b;
    font: inherit;
    font-size: 13px;
    font-weight: 700;
    cursor: pointer;
  }

  .range-selector button:hover,
  .range-selector button:focus-visible {
    color: #14566e;
    outline: 2px solid rgba(31, 111, 139, 0.25);
  }

  .range-selector button.active {
    background: white;
    color: #14566e;
    box-shadow: 0 1px 3px rgba(24, 33, 47, 0.12);
  }

  .chart-area {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: 12px;
    height: 326px;
    padding: 22px 20px 8px;
  }

  .chart-y-labels {
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    padding-bottom: 30px;
    color: #6f7b89;
    font-size: 11px;
  }

  .chart-plot {
    display: grid;
    grid-template-rows: 280px 30px;
    min-width: 0;
  }

  .chart-plot svg {
    width: 100%;
    height: 280px;
    overflow: visible;
  }

  .chart-grid-line {
    stroke: #e7edf2;
    stroke-width: 1;
    vector-effect: non-scaling-stroke;
  }

  .chart-plot .average-cost-line {
    stroke: #b67b12;
    stroke-dasharray: 7 5;
    stroke-width: 2;
    vector-effect: non-scaling-stroke;
  }

  .chart-plot .average-cost-line.gain {
    stroke: #237a4b;
  }

  .chart-plot .average-cost-line.loss {
    stroke: #b33b47;
  }

  .chart-plot polyline {
    fill: none;
    stroke: #1f6f8b;
    stroke-linecap: round;
    stroke-linejoin: round;
    stroke-width: 2.5;
    vector-effect: non-scaling-stroke;
  }

  .chart-x-labels {
    position: relative;
    color: #6f7b89;
    font-size: 11px;
  }

  .chart-x-labels span {
    position: absolute;
    bottom: 0;
    transform: translateX(-50%);
    white-space: nowrap;
  }

  .chart-x-labels span:first-child {
    transform: none;
  }

  .chart-x-labels span:last-child {
    transform: translateX(-100%);
  }

  .gain-text {
    color: #237a4b;
    font-weight: 700;
  }

  .loss-text {
    color: #b33b47;
    font-weight: 700;
  }

  .chart-message,
  .stock-data-status {
    display: grid;
    min-height: 180px;
    place-items: center;
    padding: 24px;
    color: #6f7b89;
    text-align: center;
  }

  .chart-meta,
  .stock-data-footnote {
    display: flex;
    flex-wrap: wrap;
    gap: 8px 18px;
    padding: 0 20px 14px;
  }

  .stock-metrics-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 12px;
  }

  .stock-metric {
    display: grid;
    gap: 8px;
    min-width: 0;
    padding: 18px;
    border: 1px solid #d8e0e8;
    border-radius: 6px;
    background: white;
  }

  .stock-metric span {
    color: #5d6a7a;
    font-size: 13px;
  }

  .stock-metric strong {
    overflow: hidden;
    font-size: 21px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .stock-data-footnote {
    padding-top: 0;
  }

  .error-text {
    color: #a62f3a;
  }

  .stock-position-card {
    border: 1px solid #d8e0e8;
    border-radius: 8px;
    background: white;
  }

  .stock-position-grid {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: 1px;
    background: #e5ebf1;
  }

  .stock-position-grid div {
    display: grid;
    gap: 7px;
    padding: 18px;
    background: white;
  }

  .stock-position-grid span {
    color: #5d6a7a;
    font-size: 13px;
  }

  .stock-not-held {
    padding: 18px;
    border-left: 4px solid #1f6f8b;
    background: white;
    color: #5d6a7a;
    line-height: 1.5;
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
    margin: 0;
    font-size: 18px;
  }

  .section-title {
    display: flex;
    align-items: baseline;
    gap: 10px;
  }

  .section-title span {
    color: #5d6a7a;
    font-size: 14px;
  }

  .prompt-dropdown {
    position: relative;
  }

  .prompt-dropdown summary {
    min-height: 38px;
    padding: 0 34px 0 14px;
    border: 1px solid #b8c4cf;
    border-radius: 4px;
    background: white;
    color: #293646;
    font-size: 14px;
    font-weight: 700;
    line-height: 36px;
    cursor: pointer;
    list-style: none;
    user-select: none;
  }

  .prompt-dropdown summary::-webkit-details-marker {
    display: none;
  }

  .prompt-dropdown summary::after {
    position: absolute;
    top: 16px;
    right: 14px;
    width: 7px;
    height: 7px;
    border-right: 2px solid currentColor;
    border-bottom: 2px solid currentColor;
    content: '';
    transform: rotate(45deg);
  }

  .prompt-dropdown[open] summary::after {
    top: 20px;
    transform: rotate(225deg);
  }

  .prompt-menu {
    position: absolute;
    z-index: 2;
    top: calc(100% + 6px);
    right: 0;
    min-width: 230px;
    padding: 6px;
    border: 1px solid #d8e0e8;
    border-radius: 4px;
    background: white;
    box-shadow: 0 8px 22px rgb(36 52 68 / 16%);
  }

  .prompt-menu button {
    width: 100%;
    padding: 10px 12px;
    border: 0;
    border-radius: 3px;
    background: transparent;
    color: #293646;
    font: inherit;
    font-size: 14px;
    font-weight: 700;
    text-align: left;
    cursor: pointer;
  }

  .prompt-menu button:hover:not(:disabled),
  .prompt-menu button:focus-visible {
    background: #edf5f8;
    color: #14566e;
  }

  .prompt-menu button:disabled {
    cursor: wait;
    opacity: 0.55;
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

  .position-row {
    cursor: pointer;
    transition: background-color 120ms ease;
  }

  .position-row:hover,
  .position-row:focus-visible {
    background: #f1f7f9;
    outline: none;
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

    .app-header {
      position: sticky;
      grid-template-columns: 1fr;
      gap: 12px;
      margin-top: -10px;
    }

    .stock-search {
      width: 100%;
      justify-self: stretch;
    }

    .actions {
      align-items: stretch;
      flex-direction: column;
    }

    .stock-heading {
      flex-direction: column;
    }

    .stock-metrics-grid,
    .stock-position-grid {
      grid-template-columns: 1fr;
    }

    .stock-chart-heading {
      align-items: stretch;
      flex-direction: column;
    }

    .range-selector {
      display: grid;
      grid-template-columns: repeat(4, 1fr);
    }

    .range-selector button {
      padding: 0 6px;
    }

    .chart-area {
      height: 250px;
      padding-right: 12px;
      padding-left: 12px;
    }

    .chart-area svg {
      height: 220px;
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

    .section-heading {
      align-items: stretch;
      flex-direction: column;
    }

    .prompt-dropdown summary {
      width: 100%;
      box-sizing: border-box;
    }

    .prompt-menu {
      right: auto;
      left: 0;
      width: 100%;
      box-sizing: border-box;
    }
  }
</style>
