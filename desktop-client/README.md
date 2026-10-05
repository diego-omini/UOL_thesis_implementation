# Desktop client

The application starts and monitors a local Interactive Brokers Client Portal Gateway, verifies that its authenticated session is for paper trading, retrieves the account summary and all open positions, calculates portfolio allocation and concentration metrics, and stores sanitized snapshots for locally tracked analytics. It can also request read-only portfolio or stock research from a local Ollama model. A selected stock can be analyzed and traded manually through a validated IBKR what-if preview and explicit confirmation flow.

Only paper trading is within scope. The current version uses the first account in the order returned by `/portfolio/accounts`; it does not provide an account selector.

## Requirements

- Go 1.23 or later
- Node.js and npm
- Wails CLI 2.x
- Java supported by the installed Client Portal Gateway
- Interactive Brokers Client Portal Gateway
- An Interactive Brokers paper account
- Ollama with the configured model available locally
- An Ollama account and API key when web research is enabled
- Optional: a free SnapTrade Personal API key for estimated account-value history when IBKR PortfolioAnalyst is unavailable

The gateway can be downloaded from the [Interactive Brokers API page](https://www.interactivebrokers.com/en/trading/ib-api.php).

The default AI model is `llama3.2`. Install Ollama and make that model available before using the counselor prompts:

```powershell
ollama pull llama3.2
```

## Configure IBKR authentication

Start the application and select the extracted `clientportal.gw` directory when prompted. The directory must contain `bin\run.bat` and `root\conf.yaml`. Its location is stored in the current user's application configuration and is not a credential.

When the gateway is unavailable, the application starts it in the background. Select **Log in to IBKR** to open the IBKR-hosted gateway page in the system browser, accept the local certificate warning if required, and authenticate with the paper-trading account. The dashboard opens automatically after the gateway reports an authenticated paper session.

IBKR requires browser authentication at least daily and does not support automated Client Portal Gateway login. While the application is open, it sends a keep-alive request approximately once per minute to avoid the shorter inactivity timeout. Use **Log out** to explicitly end the gateway session.

The browser SSO session and the brokerage session are separate. If the brokerage session expires while browser SSO remains valid, the application attempts a non-competing initialization first. It supplies `compete: true` only when IBKR explicitly requires that capability and reports no competing session. A browser login is still required when IBKR does not re-establish the brokerage connection.

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
| `SNAPTRADE_CLIENT_ID` | Empty | Optional Personal API key client ID for read-only portfolio-history backfill |
| `SNAPTRADE_CONSUMER_KEY` | Empty | Optional Personal API key signing secret for read-only portfolio-history backfill |

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
$env:SNAPTRADE_CLIENT_ID = "your-personal-client-id"
$env:SNAPTRADE_CONSUMER_KEY = "your-personal-consumer-key"
wails dev
```

TLS verification should only be disabled for the locally hosted gateway. Do not use this setting for a remote endpoint.

The AI provider is isolated behind an application-owned interface. The first implementation supports `ollama`; selecting another provider requires a corresponding adapter. Immutable prompt versions are stored below `ai/prompts`, while `ai/prompts/catalog.yaml` selects the active version for each alias. Each prompt declares its parameters, tools, system and user templates, output contract, temperature, token ceiling, and maximum response length.

Active counselor prompts receive application-owned facts with stable identifiers. Deterministic numeric facts include an explicit unit and validation tolerance; the model may interpret or repeat these values but does not replace their calculation. Reported money accepts the greater of 0.01 currency units or one part per million, percentages accept 0.05 percentage points, HHI accepts 0.0005, share quantities accept `1e-9`, and integer counts must match exactly. Ollama receives a JSON schema for constrained output. Go normalizes representation-only mistakes such as rendered fact-line wrappers and known percent-unit aliases, removes nonnumeric entries from numeric fields, and restores required limitations from backend facts before checking fact references, numeric agreement, trading-authority claims, and citations. Citation URLs must have been accepted by the prompt's isolated research session. An invalid response receives one tool-free repair attempt and is rejected if the repaired result is still invalid. The frontend renders the validated observations, explanations, confidence, limitations, supporting fact IDs, values, and citations; Markdown explanations remain sanitized and external links open in the system browser.

The interface presents ten defined workflows in an analysis library grouped by portfolio overview, risk, diversification, history, research, stock analysis, and proposed-order analysis. Each action states its required data, whether it uses external research, and why it is unavailable. Portfolio and stock analysis areas keep one active result at a time and reject late responses after a newer analysis, navigation, input change, session change, or component destruction.

The catalog currently exposes these workflows:

| Alias | Active version | Interface location | Backend-normalized context | Research tools |
| --- | --- | --- | --- | --- |
| `portfolio_resume` | 3 | Portfolio overview | Current positions, direct and effective allocations, concentration, provenance, and warnings | Instrument search and accepted-source fetch |
| `portfolio_news` | 2 | Analysis library research | Ordered broker-resolved public identities and research timestamp | Instrument search and accepted-source fetch |
| `portfolio_risk` | 1 | Modeled portfolio risk | Current-weight modeled metrics, contributions, correlations, scenarios, periods, coverage, and warnings | None |
| `stock_news` | 2 | Stock analysis | One broker-resolved public stock identity and research timestamp | Instrument search and accepted-source fetch |
| `order_analysis` | 2 | Manual order panel | Proposed side and quantity, deterministic order impact, market metrics, timestamps, and warnings | Instrument search and accepted-source fetch |
| `currency_exposure` | 1 | Diversification | Instrument-currency allocation, broker exchange rates, source timestamps, and missing-conversion warnings | None |
| `effective_diversification` | 1 | Diversification | Direct concentration, bounded effective securities, effective sectors, fund coverage, issuer provenance, and warnings | None |
| `portfolio_history_summary` | 1 | Historical data | Selected range, source-separated observations, endpoints, absolute changes, coverage, and reduction warnings | None |
| `position_risk` | 1 | Stock analysis for a held instrument | Current holding values and deterministic one-year price behavior, with quote/history timestamps and warnings | None |
| `portfolio_data_quality` | 1 | Portfolio overview | Metric availability, classifications, currency conversion, fund-source coverage, timestamps, and warnings | None |

Tool-free prompts receive no research or broker tools. The historical-data prompt describes observed absolute account-value changes only; it cannot label them as investment returns because deposits and withdrawals are unavailable. The current-position prompt reports measured price behavior separately from structural business risk. Unsupported holdings, classifications, conversions, market data, or history remain explicit required limitations.

Prompt version, provider/model identity, result creation time, and source-data time are returned with each response. When a response and the currently displayed deterministic figures have different timestamps, the interface marks the mismatch instead of presenting them as one calculation. Search tools accept only an exact public instrument name and ticker plus a fixed research topic. Results are restricted to HTTPS URLs on the configured reliable-domain list, include UTC retrieval times, and page fetches are limited to URLs accepted earlier in the same execution. The API key is read only from the environment and is not saved by the application. Each outbound search contains one public instrument identity only; private quantities, values, weights, analytics, order details, account data, and other holdings are excluded by backend request construction. The AI path has no broker mutation or order-submission capability. Structured validation rejects claimed trading authority, prescriptive buy/sell/hold guidance, named replacement instructions, target allocations, and order-like action steps.

## Analytics

Portfolio-wide calculations use the account base currency. IBKR base-currency position values are used when available. Otherwise, local-currency values are converted using the account-relative exchange rates returned by `/portfolio/{accountId}/ledger`. Original position values remain in their reported currencies, and the dashboard shows the conversion rate, source, and timestamp. Metrics that require an unavailable conversion rate are marked unavailable.

Normal paper-account responses may omit `baseMktValue`, `baseUnrealizedPnl`, and position `pageSize`. The application continues pagination until an empty page when page metadata is absent and uses ledger conversion when direct base values are unavailable.

Position and allocation weights use absolute market exposure, so short positions do not cancel long positions. The concentration index is the sum of squared position weights and ranges from values near zero for broadly distributed portfolios to one for a single-position portfolio.

Effective exposure replaces a supported fund wrapper with its normalized underlying holdings, then combines those holdings with direct positions and holdings reached through other funds. The dashboard keeps residual, unavailable, and unclassified exposure visible and shows the issuer source, retrieval time, holdings effective date, reported coverage, assumptions, warnings, and confidence. These values are calculated in Go and supplied to the counselor under stable `effective.security.*.weight` and `effective.sector.*.weight` fact identifiers.

The dashboard can also calculate modeled portfolio risk on demand. It requests one year of daily broker price history for each eligible stock, aligns the series on common dates, and applies the current signed position values as gross-exposure weights without renormalizing excluded positions. The result includes modeled cumulative return, annualized volatility, annualized downside deviation, maximum drawdown, pairwise correlations, arithmetic return contribution, variance-based risk contribution, and fixed shocks to the broad portfolio, largest position, largest sector, and foreign-currency exposure.

These figures model the current portfolio as if it had been rebalanced to the current weights each day. They are not realized account performance or historical attribution. Positions without sufficient common history remain excluded with an explicit warning, and foreign-security returns omit historical exchange-rate movement because the broker history response does not include a matching FX series.

After a successful calculation, **Explain with counselor** refreshes the same normalized inputs and sends only application-owned risk facts to the local model. The dedicated prompt explains coverage, risk drivers, correlations, contributions, scenarios, and tradeoffs without recalculating the values. It has no research tools, cannot place orders, must retain every calculation limitation, and returns the same validated structured response used by the other counselor workflows.

The current source registry supports the iShares Core S&P 500 ETF (`IVV`) through its official full-holdings CSV. Both ticker and issuer fund name must match before the application downloads the fixed issuer URL. Downloads are cached in memory for six hours with an explicit expiry. Other funds are reported as unsupported until a reliable structured issuer adapter and exact identity mapping are added; the application does not scrape issuer pages, derive holdings from search snippets, or ask the model to supply holdings.

## Local persistence

The application creates `portfolio-tracker.db` in the current user's `uol-portfolio-tracker` configuration directory. GORM migrations create six focused SQLite tables:

- sanitized portfolio snapshots with deterministic analytics;
- imported daily portfolio values from IBKR PortfolioAnalyst or SnapTrade;
- successful portfolio-value import batches, including refreshes that return no observations;
- catalog-prompt executions with prompt version, model settings, rendered prompts, tool transcript, raw and repaired output, parsed output, validation result, repair status, and failures;
- evaluation output bundles; and
- token-free manual-order audit events.

Portfolio and order records store a one-way account fingerprint instead of the IBKR account identifier. Snapshot JSON omits the account identifier, and prompt parameters and transcripts remove fields named as account IDs, credentials, secrets, or opaque confirmation tokens. The database is local application data and must not be committed.

The analysis library can reopen up to eight recent successful validated results. Its query selects only the parsed structured response, creation and source-data timestamps, provider/model identity, and prompt ID/version. Rendered prompts, parameters, transcripts, raw and repair output, validation diagnostics, account fingerprints, credentials, and tokens do not cross the Wails frontend boundary. Older records created before source-data timestamps were stored are labeled as saved data whose current status is unknown.

When history is first requested after connecting an account, the application first requests up to one year of daily base-currency NAV from IBKR's `/pa/performance` endpoint. It uses the installed gateway's `1Y` period spelling and retries with `12M` only when another gateway version explicitly rejects that spelling. Successful IBKR imports are reused for at least 15 minutes to respect the endpoint pacing limit. This path uses the existing authenticated Client Portal Gateway session and does not require a Flex Query.

If IBKR cannot supply the series and both SnapTrade variables are configured, the application uses [SnapTrade's Personal API-key mode](https://docs.snaptrade.com/docs/authentication-methods). Select **Connect optional estimated history** once, connect the existing Interactive Brokers paper account in SnapTrade's hosted portal, and return to the application. The portal request is explicitly read-only; the integration contains no SnapTrade trading calls. The application requires one exact active-account match when SnapTrade supplies comparable account identifiers. A sole connected IBKR paper account is accepted only when no comparable identifier is available; mismatches and ambiguous matches are rejected. It then downloads and locally stores SnapTrade's estimated daily total account value, including positions and cash. SnapTrade documents the [balance-history endpoint](https://docs.snaptrade.com/reference/Account%20Information/AccountInformation_getAccountBalanceHistory) as experimental, limited to one year, disabled by default, and available through a free dashboard add-on. The credentials remain in the backend environment and are never returned to the frontend. Successful external imports are reused for six hours.

The dashboard can query the active account's stored history over 7-day, 30-day, 90-day, or one-year ranges. It presents two separate data products. **Account value** combines official PortfolioAnalyst end-of-day NAV, optional estimated SnapTrade values, and local net-liquidation captures. It is not labeled as investment performance because deposits and withdrawals are not available for cash-flow-adjusted return calculations. **Tracked analytics** contains cash, exposure, unrealized P/L, concentration, and position count only from complete local snapshots, and the interface reports the first date on which local tracking began.

Each observation preserves its source, date or timestamp, currency, availability, and data-quality warnings. Because SnapTrade dynamically regenerates its estimates, account-value queries use only its newest successful import batch, including an empty batch, without deleting older stored rows. An official IBKR value takes precedence over an estimated external value for the same date and currency; local intraday captures remain distinct. Reductions are applied independently to account-value and tracked-analytics series. Captures from the same source within 90 seconds are treated as near-duplicates and the later capture is retained. Captures with different base currencies are returned as separate series. A series above 366 points is evenly downsampled while preserving its endpoints. Charts retain the complete selected time range, separate source types visibly, and break lines across missing intervals or source changes. Sparse histories use larger points and an automatic active-period inset while missing dates remain gaps rather than zero values.

No automatic retention or deletion policy is applied. Queries are bounded to one calendar year, 20,000 stored rows, and 366 returned points per currency series, which preserves a daily annual series while limiting runtime memory without deleting existing user data. Portfolio snapshot history is separate from broker-provided stock-price history, which is requested on demand and is not stored in this database.

## Checks

Run the backend tests from this directory:

```powershell
go test ./...
go vet ./...
```

Run the frontend checks and production build:

```powershell
npm test --prefix frontend
npm run check --prefix frontend
npm run build --prefix frontend
```

Build the desktop executable:

```powershell
wails build
git diff --check
```

## Prompt evaluation

The fixed evaluation suite compares a generic baseline with the analytics-enriched portfolio prompt. Both conditions use the same Ollama model, seed, temperature, context size, token limit, frozen inputs, and strict JSON response contract. Three controlled sanitized scenarios are each executed five times per condition, with alternating condition order.

Run the suite from this directory:

```powershell
go run ./cmd/evaluate
```

The command records the Ollama version and installed model digest, writes a timestamped JSON bundle below `evaluation/results`, and stores the same bundle in the application SQLite database. For a final recorded run, use `-expected-model-digest <digest>` to reject a different installed model build. See [`evaluation/README.md`](evaluation/README.md) for the scoring definitions and fixture boundaries.

## Common connection states

- **Gateway location required:** select the extracted `clientportal.gw` directory or set `IBKR_GATEWAY_DIR`.
- **Gateway is not reachable:** retry automatic startup, verify the configured directory, and confirm that port 5000 is available.
- **Gateway is reachable but not authenticated:** select **Log in to IBKR** and complete the paper-account login in the browser.
- **Competing session:** sign out of another active IBKR brokerage session or wait for that session to end, then authenticate again.
- **Live account session:** log out and authenticate with the paper-trading username; the application refuses access when IBKR does not report `isPaper: true`.
- **Session expired:** the application returns to the authentication page; log in again and the portfolio reloads automatically.
- **Market-data fields unavailable:** snapshot bid, ask, or reference fields depend on the username's market-data permissions and may be unavailable even when historical bars are returned. The application keeps those fields explicitly unavailable rather than inferring values.

## Current limitations

- The first account in the order returned by `/portfolio/accounts` is used.
- Account selection is not yet exposed in the interface.
- The application depends on IBKR's locally installed gateway and mandatory browser authentication.
- Automatic account-value backfill is limited to one year. IBKR PortfolioAnalyst remains the authoritative first choice; SnapTrade is an optional, explicitly labeled estimated fallback when IBKR fails. Historical analytics begin with the first local snapshot, and dates absent from all sources remain missing. Cash-flow-adjusted investment returns are not calculated because deposits and withdrawals are not available.
- Modeled portfolio risk uses current weights and up to one year of daily stock-price history. It does not reconstruct past holdings, cash flows, trades, changing weights, or historical FX rates, and it does not currently calculate benchmark beta, value at risk, optimization, or forecasts.
- The `/pa/performance` request was checked against the authenticated paper account on 4 October 2026. The installed gateway rejected `12M` and accepted `1Y`, but IBKR then returned the same server-side PortfolioAnalyst processing error for both one-year and seven-day requests. The failure is therefore not explained by the account being less than one year old. SnapTrade was verified with controlled HTTP tests and compilation, but not with live credentials in this workspace. Its Personal API key and free balance-history add-on must be configured before the fallback can populate the graph.
- Manual trading is limited to whole-share DAY stock market orders. Limit orders, fractional shares, open-order management, and execution history are not implemented. Order audit records describe application transitions and broker acknowledgements, not authoritative fills.
- Current-position risk uses one year of broker price history and current holding values. It does not provide fundamental business-risk analysis, forecasts, or a reconstruction of the position through time.
- Web research requires `OLLAMA_API_KEY`; without it, the counselor continues with deterministic portfolio context and reports the research limitation.
- Deterministic look-through currently supports only the exact `IVV` issuer identity. All other ETFs and funds retain explicit unknown exposure until a source-specific structured adapter is implemented.
