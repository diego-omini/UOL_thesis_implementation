# Desktop client

The application starts and monitors a local Interactive Brokers Client Portal Gateway, verifies that its authenticated session is for paper trading, retrieves the account summary and all open positions, calculates portfolio allocation and concentration metrics, and can request read-only portfolio or stock research from a local Ollama model.

Only paper trading is within scope. The current version uses the first account returned by the gateway.

## Requirements

- Go 1.23 or later
- Node.js and npm
- Wails CLI 2.x
- Java supported by the installed Client Portal Gateway
- Interactive Brokers Client Portal Gateway
- An Interactive Brokers paper account
- Ollama with the configured model available locally
- An Ollama account and API key when web research is enabled

The gateway can be downloaded from the [Interactive Brokers API page](https://www.interactivebrokers.com/en/trading/ib-api.php).

The default AI model is `llama3.2`. Install Ollama and make that model available before using the counselor prompts:

```powershell
ollama pull llama3.2
```

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

The default settings target the local Client Portal Gateway and Ollama services:

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `IBKR_BASE_URL` | `https://localhost:5000/v1/api` | Client Portal Gateway API base URL |
| `IBKR_SKIP_TLS_VERIFY` | `true` | Accept the gateway's local self-signed certificate |
| `IBKR_GATEWAY_DIR` | Stored first-run selection | Extracted `clientportal.gw` directory |
| `AI_PROVIDER` | `ollama` | AI provider selected by the application composition layer |
| `AI_BASE_URL` | `http://localhost:11434/api` | Base URL for the Ollama API |
| `AI_MODEL` | `llama3.2` | Ollama model used for counselor prompts |
| `OLLAMA_API_KEY` | Empty | API key for Ollama-hosted web search and page fetch |
| `AI_SEARCH_BASE_URL` | `https://ollama.com/api` | Ollama web-research API base URL |
| `AI_SEARCH_ALLOWED_DOMAINS` | Curated default list | Comma-separated reliable domains accepted from search results |
| `AI_SEARCH_MAX_RESULTS` | `5` | Maximum results requested per instrument search, from 1 to 10 |

Override a setting in PowerShell before starting Wails:

```powershell
$env:IBKR_BASE_URL = "https://localhost:5000/v1/api"
$env:IBKR_SKIP_TLS_VERIFY = "true"
$env:IBKR_GATEWAY_DIR = "C:\path\to\clientportal.gw"
$env:AI_PROVIDER = "ollama"
$env:AI_BASE_URL = "http://localhost:11434/api"
$env:AI_MODEL = "llama3.2"
$env:OLLAMA_API_KEY = "your-ollama-api-key"
$env:AI_SEARCH_ALLOWED_DOMAINS = "sec.gov,reuters.com,example-issuer.com"
wails dev
```

TLS verification should only be disabled for the locally hosted gateway. Do not use this setting for a remote endpoint.

The AI provider is isolated behind an application-owned interface. The first implementation supports `ollama`; selecting another provider requires a corresponding adapter. Immutable prompt versions are stored below `ai/prompts`, while `ai/prompts/catalog.yaml` selects the active version for each alias. Each prompt declares its parameters, tools, system and user templates, output contract, temperature, token ceiling, and maximum response length. The generic executor validates and renders the selected prompt, runs bounded tool calls, and gives a malformed or overlong final response one tool-free repair attempt instead of exposing or truncating it.

The portfolio resume receives normalized positions and deterministic concentration, sector, asset-class, currency, and data-quality context. The portfolio-news prompt researches every open position, while the stock-news prompt summarizes recent material developments for the company open in the stock detail view. Both news prompts report when reliable recent evidence is unavailable. Prompt version and provider/model metadata are returned with each response. Search tools accept only an exact public instrument name and ticker plus a fixed research topic. Results are restricted to HTTPS URLs on the configured reliable-domain list, include UTC retrieval times, and page fetches are limited to URLs accepted earlier in the same execution. The API key is read only from the environment and is not saved by the application. The AI path has no broker mutation or order-submission capability.

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
- Historical portfolio snapshots and manual paper orders are not implemented yet.
- The prompt catalog currently contains a portfolio resume, a per-position portfolio-news digest, and a single-stock news summary.
- Web research requires `OLLAMA_API_KEY`; without it, the counselor continues with deterministic portfolio context and reports the research limitation.
- Fund holdings can be researched, but deterministic fund look-through aggregation and structured grounding checks are not implemented yet.
