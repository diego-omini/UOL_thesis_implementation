# Desktop client

The application connects to an authenticated Interactive Brokers Client Portal Gateway session, retrieves the paper account summary and all open positions, and displays them through a Wails desktop interface.

Only paper trading is within scope. The current version uses the first account returned by the gateway.

## Requirements

- Go 1.23 or later
- Node.js and npm
- Wails CLI 2.x
- Java supported by the installed Client Portal Gateway
- Interactive Brokers Client Portal Gateway
- An Interactive Brokers paper account

The gateway can be downloaded from the [Interactive Brokers API page](https://www.interactivebrokers.com/en/trading/ib-api.php).

## Start the Client Portal Gateway

Open PowerShell in the extracted gateway directory and run:

```powershell
.\bin\run.bat root\conf.yaml
```

Wait until the console reports that the server is listening on port 5000. Open [https://localhost:5000](https://localhost:5000) in a browser, accept the local certificate warning if required, and authenticate with the paper-trading account.

The browser can be closed after the gateway confirms authentication. Keep the gateway process running while using the application.

## Run the application

From this directory:

```powershell
wails dev
```

The application automatically attempts to load the portfolio when it starts. Use **Check gateway connection** to inspect the session state and **Refresh portfolio** to retrieve the latest account values and positions.

## Configuration

The default settings target the local Client Portal Gateway:

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `IBKR_BASE_URL` | `https://localhost:5000/v1/api` | Client Portal Gateway API base URL |
| `IBKR_SKIP_TLS_VERIFY` | `true` | Accept the gateway's local self-signed certificate |

Override a setting in PowerShell before starting Wails:

```powershell
$env:IBKR_BASE_URL = "https://localhost:5000/v1/api"
$env:IBKR_SKIP_TLS_VERIFY = "true"
wails dev
```

TLS verification should only be disabled for the locally hosted gateway. Do not use this setting for a remote endpoint.

## Checks

Run the backend tests from this directory:

```powershell
go test ./...
```

Run the frontend checks and production build:

```powershell
npm run check --prefix frontend
npm run build --prefix frontend
```

Build the desktop executable:

```powershell
wails build
```

## Common connection states

- **Gateway is not reachable:** start the Client Portal Gateway and verify that port 5000 is available.
- **Gateway is reachable but not authenticated:** open `https://localhost:5000` and complete the paper-account login.
- **Competing session:** sign out of another active IBKR brokerage session or wait for that session to end, then authenticate again.
- **Session expired:** authenticate through the gateway browser page again and refresh the portfolio.

## Current limitations

- The first account returned by `/portfolio/accounts` is used.
- Account selection is not yet exposed in the interface.
- The application depends on the locally authenticated gateway session.
- Historical portfolio snapshots, analytics, AI counseling, and manual paper orders are not implemented yet.
