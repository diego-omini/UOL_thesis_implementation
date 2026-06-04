<script lang="ts">
  // App.svelte contains the root frontend component rendered by the Wails application.
  import {CheckIBKRConnection} from '../wailsjs/go/main/App.js'

  type ConnectionStatus = {
    Connected: boolean
    Message: string
  }

  let checking = false
  let status: ConnectionStatus | null = null

  async function checkConnection(): Promise<void> {
    checking = true

    try {
      status = await CheckIBKRConnection()
    } catch (error) {
      status = {
        Connected: false,
        Message: `Failed to call backend: ${error}`,
      }
    } finally {
      checking = false
    }
  }
</script>

<main class="page">
  <section class="panel">
    <h1>IBKR connection</h1>
    <button class="button" disabled={checking} on:click={checkConnection}>
      {checking ? 'Checking...' : 'Check gateway connection'}
    </button>

    {#if status}
    <div class:connected={status.Connected} class:failed={!status.Connected} class="status">
      <strong>{status.Connected ? 'Connected' : 'Not connected'}</strong>
      <span>{status.Message}</span>
    </div>
    {/if}
  </section>
</main>

<style>
  .page {
    display: grid;
    min-height: 100vh;
    place-items: center;
    padding: 32px;
    box-sizing: border-box;
  }

  .panel {
    width: min(640px, 100%);
    text-align: left;
  }

  h1 {
    margin: 0 0 12px;
    font-size: 32px;
    font-weight: 700;
  }

  .description {
    margin: 0 0 24px;
    color: rgba(255, 255, 255, 0.72);
    line-height: 1.5;
  }

  .button {
    height: 40px;
    padding: 0 16px;
    border: none;
    border-radius: 4px;
    background: #f2f5f8;
    color: #1b2636;
    font-weight: 700;
    cursor: pointer;
  }

  .button:disabled {
    cursor: wait;
    opacity: 0.7;
  }

  .status {
    display: grid;
    gap: 6px;
    margin-top: 24px;
    padding: 16px;
    border-left: 4px solid;
    background: rgba(255, 255, 255, 0.08);
  }

  .status span {
    line-height: 1.5;
  }

  .connected {
    border-color: #4ade80;
  }

  .failed {
    border-color: #fb7185;
  }
</style>
