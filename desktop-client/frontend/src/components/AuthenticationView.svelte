<script lang="ts">
  // AuthenticationView.svelte presents gateway setup and IBKR paper-session connection states.
  import { createEventDispatcher } from 'svelte'
  import type { SessionStatus } from '../lib/types'

  export let initializing: boolean
  export let session: SessionStatus | null
  export let waitingForLogin: boolean
  export let authAction: boolean
  export let errorMessage: string

  const dispatch = createEventDispatcher<{
    chooseGatewayDirectory: void
    startGateway: void
    openLogin: void
    retry: void
  }>()

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

{#if initializing}
  <section class="auth-card" aria-live="polite">
    <h1>AI-assisted portfolio tracker</h1>
    <p>Checking the local IBKR connection...</p>
  </section>
{:else}
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
      <button class="primary-button auth-button" disabled={authAction} on:click={() => dispatch('chooseGatewayDirectory')}>
        {authAction ? 'Opening...' : 'Select gateway folder'}
      </button>
    {:else if session?.State === 'gateway_unavailable'}
      <div class="auth-actions">
        <button class="primary-button auth-button" disabled={authAction} on:click={() => dispatch('startGateway')}>
          {authAction ? 'Starting...' : 'Start IBKR gateway'}
        </button>
        <button class="secondary-button" disabled={authAction} on:click={() => dispatch('chooseGatewayDirectory')}>Change folder</button>
      </div>
    {:else if session?.State === 'gateway_starting'}
      <button class="primary-button auth-button" disabled>Starting gateway...</button>
    {:else if session?.State === 'authentication_required' || session?.State === 'competing_session'}
      {#if waitingForLogin}
        <div class="authentication-progress" role="status" aria-live="polite">
          <span class="spinner" aria-hidden="true"></span>
          <span>Checking IBKR authentication...</span>
        </div>
      {:else}
        <button class="primary-button auth-button" disabled={authAction} on:click={() => dispatch('openLogin')}>
          Log in to IBKR
        </button>
      {/if}
    {:else}
      <button class="primary-button auth-button" disabled={authAction} on:click={() => dispatch('retry')}>
        {authAction ? 'Checking...' : 'Retry connection'}
      </button>
    {/if}

    {#if errorMessage}
      <div class="auth-error" aria-live="assertive">{errorMessage}</div>
    {/if}

    <small class="auth-note">Authentication is handled by Interactive Brokers and is limited to the paper-trading environment.</small>
  </section>
{/if}

<style>
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

  .auth-card > h1,
  .auth-card > p,
  .auth-heading h1,
  .auth-heading p {
    margin: 0;
  }

  .auth-heading {
    display: grid;
    gap: 8px;
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

  .auth-error {
    color: #a62f3a;
    font-size: 14px;
    line-height: 1.45;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  @media (max-width: 780px) {
    .auth-card { padding: 28px 22px; }
  }
</style>
