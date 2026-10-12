# IBKR Paper Portfolio Counselor

Desktop application developed for a computer science master thesis. It connects to the Interactive Brokers Client Portal Gateway, reads a paper-trading portfolio, calculates portfolio analytics, provides read-only portfolio counseling, and supports manually previewed and confirmed paper orders.

The counselor cannot submit orders or change the portfolio. Trading is a separate workflow restricted to whole-share DAY market orders in the IBKR paper environment.

## Requirements

- Go 1.23 or later
- Node.js and npm
- [Wails CLI 2.x](https://wails.io/docs/gettingstarted/installation)
- Java supported by the installed Client Portal Gateway
- [Interactive Brokers Client Portal Gateway](https://www.interactivebrokers.com/en/trading/ib-api.php)
- An Interactive Brokers paper account
- Ollama with `qwen3:14b`, or an OpenAI API project and key
- An Ollama API key when Ollama-hosted web research is required

SnapTrade Personal API credentials are optional. They enable an estimated account-value history fallback when IBKR PortfolioAnalyst is unavailable.

## Setup and run

Clone the repository, then install the frontend dependencies from the application directory:

```powershell
cd desktop-client
npm install --prefix frontend
```

For the default local Ollama provider, install the configured model:

```powershell
ollama pull qwen3:14b
```

Start the application:

```powershell
wails dev
```

On first use, select the extracted `clientportal.gw` directory. It must contain `bin\run.bat` and `root\conf.yaml`. The application stores this non-secret path in the current user's configuration directory and starts the gateway when necessary.

Select **Log in to IBKR** and authenticate with the paper-trading account in the browser. The dashboard opens after the gateway reports an authenticated paper session. IBKR requires browser authentication at least daily; credentials are entered only on the IBKR-hosted page and are never collected by this application.

## Configuration

Configuration is read from environment variables. Provider credentials remain in the backend process environment and are not persisted by the application.

| Variable | Default | Purpose |
| --- | --- | --- |
| `IBKR_BASE_URL` | `https://localhost:5000/v1/api` | Client Portal Gateway API base URL |
| `IBKR_SKIP_TLS_VERIFY` | `true` | Accept the local gateway's self-signed certificate |
| `IBKR_GATEWAY_DIR` | Saved first-run selection | Extracted `clientportal.gw` directory |
| `AI_PROVIDER` | `ollama` | Counselor provider: `ollama` or `openai` |
| `AI_BASE_URL` | Provider default | Ollama or OpenAI API base URL |
| `AI_MODEL` | Provider default | `qwen3:14b` for Ollama; `gpt-6-luna` for OpenAI |
| `OPENAI_API_KEY` | Empty | Required when `AI_PROVIDER=openai` |
| `OLLAMA_API_KEY` | Empty | Required for Ollama-hosted web research |
| `AI_SEARCH_BASE_URL` | `https://ollama.com/api` | Ollama research API base URL |
| `AI_SEARCH_ALLOWED_DOMAINS` | Curated list | Accepted research-source domains |
| `AI_SEARCH_MAX_RESULTS` | `5` | Results requested per search, from 1 to 10 |
| `SNAPTRADE_CLIENT_ID` | Empty | Optional read-only history client ID |
| `SNAPTRADE_CONSUMER_KEY` | Empty | Optional read-only history signing secret |

Example for the default provider:

```powershell
$env:OLLAMA_API_KEY = "your-ollama-api-key"
wails dev
```

Example for OpenAI:

```powershell
$env:AI_PROVIDER = "openai"
$env:OPENAI_API_KEY = "your-openai-api-key"
wails dev
```

`IBKR_SKIP_TLS_VERIFY=true` is intended only for the locally hosted gateway. Do not use it with a remote endpoint.

## Checks and release build

Run from `desktop-client`:

```powershell
go test ./...
go vet ./...
npm test --prefix frontend
npm run check --prefix frontend
npm run build --prefix frontend
wails build
git diff --check
```

The Windows executable is written to `build/bin/paper-portfolio-counselor.exe`.

The automated suite uses controlled broker, market-history, model-provider, and persistence fixtures. It does not establish a live broker session or submit an order.

## Repository structure

```text
desktop-client/
├── ai/                 counselor workflows, providers, and prompt catalog
├── analytics/          deterministic portfolio analytics
├── broker/             application-owned broker interfaces
├── cmd/evaluate/       reproducible prompt-evaluation command
├── evaluation/         frozen protocols, fixtures, scoring, and local output instructions
├── frontend/           Svelte and TypeScript interface
├── fundholdings/       official fund-holdings adapters
├── gateway/            Client Portal Gateway process control
├── ibkr/               IBKR Client Portal Gateway adapter
├── persistence/        GORM and SQLite repositories
├── portfolio*/         broker-independent portfolio and analysis packages
├── showcase/scenarios/ controlled screenshot fixtures for tagged builds
├── snaptradehistory/   optional read-only account-value history adapter
└── trading/            manual validation and confirmation workflow
```

The `showcase*.go` files remain in the application root because they are build-tagged parts of package `main` and control application composition. The scenario data they load is isolated under `showcase/scenarios`. Normal builds do not embed or activate those fixtures.

## Data and safety boundaries

- Only IBKR paper sessions are accepted.
- The counselor receives normalized facts and deterministic analytics, not broker mutation capabilities.
- Orders require a fresh broker what-if preview and explicit user confirmation.
- Application data is stored in `portfolio-tracker.db` under the current user's `uol-portfolio-tracker` configuration directory.
- Stored account references use a one-way fingerprint; credentials and frontend confirmation identifiers are not stored.
- Local databases, build output, dependencies, environment files, and generated evaluation results are excluded from Git.

## Current limitations

- The first account returned by `/portfolio/accounts` is used; there is no account selector.
- IBKR browser authentication and the locally installed gateway are required.
- Account-value backfill is limited to one year. Historical analytics begin with the first local snapshot, and cash-flow-adjusted investment returns are not calculated.
- Modeled portfolio risk uses current weights and up to one year of stock-price history; it does not reconstruct historical holdings, trades, cash flows, or matching historical FX rates.
- Deterministic fund look-through currently supports only the exact iShares Core S&P 500 ETF (`IVV`) identity.
- Manual trading is limited to whole-share DAY stock market orders. Open-order management and execution history are not implemented.
- Live broker order acknowledgement and warning handling are covered by controlled tests but were not verified by submitting a paper order.

Evaluation protocols and commands are documented in [`desktop-client/evaluation/README.md`](desktop-client/evaluation/README.md). Generated result bundles stay local and are not part of the public repository.

## License

This project is available under the [MIT License](LICENSE).
