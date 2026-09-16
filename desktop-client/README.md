# Desktop client

The application starts and monitors a local Interactive Brokers Client Portal Gateway, verifies that its authenticated session is for paper trading, retrieves the account summary and all open positions, and calculates portfolio allocation and concentration metrics.

Only paper trading is within scope. The current version uses the first account returned by the gateway.

## Requirements

- Go 1.23 or later
- Node.js and npm
- Wails CLI 2.x
- Java supported by the installed Client Portal Gateway
- Interactive Brokers Client Portal Gateway
- An Interactive Brokers paper account

The gateway can be downloaded from the [Interactive Brokers API page](https://www.interactivebrokers.com/en/trading/ib-api.php).

## Configure IBKR authentication

Start the application and select the extracted `clientportal.gw` directory when prompted. The directory must contain `bin\run.bat` and `root\conf.yaml`. Its location is stored in the current user's application configuration and is not a credential.

When the gateway is unavailable, the application starts it in the background. Select **Log in to IBKR** to open the IBKR-hosted gateway page in the system browser, accept the local certificate warning if required, and authenticate with the paper-trading account. The dashboard opens automatically after the gateway reports an authenticated paper session.

IBKR requires browser authentication at least daily and does not support automated Client Portal Gateway login. While the application is open, it sends a keep-alive request approximately once per minute to avoid the shorter inactivity timeout. Use **Log out** to explicitly end the gateway session.

## Run the application

From this directory:

```powershell
wails dev
```

The application automatically starts or reconnects to the configured gateway and loads the portfolio after authentication. Use **Refresh portfolio** to retrieve the latest account values and positions.

## Configuration

The default settings target the local Client Portal Gateway:

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `IBKR_BASE_URL` | `https://localhost:5000/v1/api` | Client Portal Gateway API base URL |
| `IBKR_SKIP_TLS_VERIFY` | `true` | Accept the gateway's local self-signed certificate |
| `IBKR_GATEWAY_DIR` | Stored first-run selection | Extracted `clientportal.gw` directory |

Override a setting in PowerShell before starting Wails:

```powershell
$env:IBKR_BASE_URL = "https://localhost:5000/v1/api"
$env:IBKR_SKIP_TLS_VERIFY = "true"
$env:IBKR_GATEWAY_DIR = "C:\path\to\clientportal.gw"
wails dev
```

TLS verification should only be disabled for the locally hosted gateway. Do not use this setting for a remote endpoint.

## Analytics

Portfolio-wide calculations use the account base currency. IBKR base-currency position values are used when available. Otherwise, local-currency values are converted using the account-relative exchange rates returned by `/portfolio/{accountId}/ledger`. Original position values remain in their reported currencies, and the dashboard shows the conversion rate, source, and timestamp. Metrics that require an unavailable conversion rate are marked unavailable.

Position and allocation weights use absolute market exposure, so short positions do not cancel long positions. The concentration index is the sum of squared position weights and ranges from values near zero for broadly distributed portfolios to one for a single-position portfolio.

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

- **Gateway location required:** select the extracted `clientportal.gw` directory or set `IBKR_GATEWAY_DIR`.
- **Gateway is not reachable:** retry automatic startup, verify the configured directory, and confirm that port 5000 is available.
- **Gateway is reachable but not authenticated:** select **Log in to IBKR** and complete the paper-account login in the browser.
- **Competing session:** sign out of another active IBKR brokerage session or wait for that session to end, then authenticate again.
- **Live account session:** log out and authenticate with the paper-trading username; the application refuses access when IBKR does not report `isPaper: true`.
- **Session expired:** the application returns to the authentication page; log in again and the portfolio reloads automatically.

## Current limitations

- The first account returned by `/portfolio/accounts` is used.
- Account selection is not yet exposed in the interface.
- The application depends on IBKR's locally installed gateway and mandatory browser authentication.
- Historical portfolio snapshots, the AI counselor, and manual paper orders are not implemented yet.
