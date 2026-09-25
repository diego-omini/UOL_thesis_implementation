<script lang="ts">
  // AppHeader.svelte owns authenticated navigation and accessible broker-backed stock search.
  import { createEventDispatcher, onDestroy } from 'svelte'
  import { SearchStocks } from '../../wailsjs/go/main/App.js'
  import { formatError } from '../lib/format'
  import { exactTickerMatch, nextActiveResultIndex } from '../lib/stock-search'
  import type { InstrumentSearchResultView } from '../lib/types'

  export let portfolioLoading: boolean
  export let authAction: boolean

  const dispatch = createEventDispatcher<{
    dashboard: void
    refresh: void
    logout: void
    stock: InstrumentSearchResultView
    sessionIssue: void
  }>()

  let query = ''
  let results: InstrumentSearchResultView[] = []
  let searchOpen = false
  let searchLoading = false
  let searchError = ''
  let activeIndex = -1
  let searchTimer: ReturnType<typeof setTimeout> | null = null
  let closeTimer: ReturnType<typeof setTimeout> | null = null
  let searchSequence = 0

  onDestroy(() => {
    searchSequence++
    clearSearchTimer()
    clearCloseTimer()
  })

  function clearSearchTimer(): void {
    if (!searchTimer) return
    clearTimeout(searchTimer)
    searchTimer = null
  }

  function clearCloseTimer(): void {
    if (!closeTimer) return
    clearTimeout(closeTimer)
    closeTimer = null
  }

  function clearSearch(): void {
    searchSequence++
    clearSearchTimer()
    clearCloseTimer()
    query = ''
    results = []
    searchOpen = false
    searchLoading = false
    searchError = ''
    activeIndex = -1
  }

  function handleInput(): void {
    clearSearchTimer()
    results = []
    searchError = ''
    activeIndex = -1
    const normalizedQuery = query.trim()
    searchOpen = normalizedQuery.length > 0
    if (!normalizedQuery) {
      searchSequence++
      searchLoading = false
      return
    }
    searchTimer = setTimeout(() => void runSearch(normalizedQuery), 250)
  }

  // runSearch accepts results only while both the request generation and query remain current.
  async function runSearch(searchQuery: string): Promise<InstrumentSearchResultView[]> {
    const sequence = ++searchSequence
    searchLoading = true
    searchError = ''
    try {
      const nextResults = await SearchStocks(searchQuery)
      if (sequence === searchSequence && searchQuery === query.trim()) {
        results = nextResults
        searchOpen = true
        activeIndex = -1
      }
      return nextResults
    } catch (error) {
      if (sequence === searchSequence) {
        results = []
        searchError = formatError(error)
        searchOpen = true
        activeIndex = -1
        dispatch('sessionIssue')
      }
      return []
    } finally {
      if (sequence === searchSequence) searchLoading = false
    }
  }

  function selectStock(stock: InstrumentSearchResultView): void {
    clearSearch()
    dispatch('stock', stock)
  }

  async function submitSearch(): Promise<void> {
    clearSearchTimer()
    const ticker = query.trim().toUpperCase()
    if (!ticker) return
    if (activeIndex >= 0 && activeIndex < results.length) {
      selectStock(results[activeIndex])
      return
    }
    const availableResults = results.length > 0 ? results : await runSearch(ticker)
    const exactMatch = exactTickerMatch(availableResults, ticker)
    if (exactMatch) selectStock(exactMatch)
  }

  function handleSearchKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      searchOpen = false
      activeIndex = -1
      return
    }
    if (results.length === 0 || (event.key !== 'ArrowDown' && event.key !== 'ArrowUp')) return

    event.preventDefault()
    searchOpen = true
    activeIndex = nextActiveResultIndex(activeIndex, results.length, event.key === 'ArrowDown' ? 1 : -1)
  }

  function handleFocus(): void {
    clearCloseTimer()
    searchOpen = query.trim().length > 0
  }

  function handleBlur(): void {
    clearCloseTimer()
    closeTimer = setTimeout(() => {
      searchOpen = false
      activeIndex = -1
    }, 120)
  }

  function showDashboard(): void {
    clearSearch()
    dispatch('dashboard')
  }
</script>

<header class="app-header">
  <button class="dashboard-link" type="button" on:click={showDashboard} aria-label="Open portfolio dashboard">
    <span class="eyebrow">IBKR paper trading</span>
    <h1>Portfolio dashboard</h1>
  </button>

  <form class="stock-search" role="search" on:submit|preventDefault={submitSearch}>
    <label class="sr-only" for="stock-search-input">Search by ticker or company name</label>
    <div class="stock-search-field">
      <span class="search-icon" aria-hidden="true"></span>
      <input
        id="stock-search-input"
        type="search"
        placeholder="Search ticker or company"
        autocomplete="off"
        role="combobox"
        aria-autocomplete="list"
        aria-expanded={searchOpen}
        aria-controls="stock-search-results"
        aria-activedescendant={activeIndex >= 0 ? `stock-search-option-${activeIndex}` : undefined}
        bind:value={query}
        on:input={handleInput}
        on:keydown={handleSearchKeydown}
        on:focus={handleFocus}
        on:blur={handleBlur}
      />
      {#if searchLoading}<span class="search-loading" aria-label="Searching"></span>{/if}
    </div>

    {#if searchOpen}
      <div id="stock-search-results" class="stock-search-results" role="listbox" aria-label="Stock search results">
        {#each results as result, index}
          <button
            id={`stock-search-option-${index}`}
            type="button"
            role="option"
            aria-selected={activeIndex === index}
            class:active={activeIndex === index}
            on:mousedown|preventDefault
            on:mouseenter={() => activeIndex = index}
            on:click={() => selectStock(result)}
          >
            <strong>{result.Symbol}</strong>
            <span>{result.Name || 'Unnamed stock'}</span>
            <small>{result.Exchange || 'Exchange unavailable'}</small>
          </button>
        {:else}
          <div class:error={Boolean(searchError)} class="stock-search-empty" role={searchError ? 'alert' : 'status'}>
            {searchLoading ? 'Searching...' : searchError || 'No matching stocks'}
          </div>
        {/each}
      </div>
    {/if}
  </form>

  <div class="actions">
    <button class="primary-button" disabled={portfolioLoading} on:click={() => dispatch('refresh')}>
      {portfolioLoading ? 'Loading...' : 'Refresh portfolio'}
    </button>
    <button class="secondary-button" disabled={authAction} on:click={() => dispatch('logout')}>Log out</button>
  </div>
</header>

<style>
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
    margin: 0;
    font-size: 22px;
  }

  .eyebrow {
    color: #1f6f8b;
    font-size: 12px;
    font-weight: 800;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  .stock-search {
    position: relative;
    width: min(100%, 460px);
    justify-self: center;
  }

  .stock-search-field { position: relative; }

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

  .stock-search-results button:last-child { border-bottom: 0; }

  .stock-search-results button:hover,
  .stock-search-results button:focus-visible,
  .stock-search-results button.active {
    background: #edf5f8;
    outline: none;
  }

  .stock-search-results button span {
    overflow: hidden;
    color: #516171;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .stock-search-results button small { color: #6f7b89; }

  .stock-search-empty {
    padding: 13px;
    color: #6f7b89;
    font-size: 14px;
  }

  .stock-search-empty.error { color: #a62f3a; }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    clip-path: inset(50%);
  }

  .actions {
    display: flex;
    gap: 10px;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  @media (max-width: 780px) {
    .app-header {
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
  }
</style>
