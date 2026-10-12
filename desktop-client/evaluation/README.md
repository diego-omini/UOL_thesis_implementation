# Portfolio prompt evaluation

This package compares two fixed prompt conditions under controlled model and provider configurations:

- `baseline` receives normalized positions and account-relative exchange rates.
- `proposed` receives the same normalized input plus deterministic backend metrics, stable metric identifiers, and grounding instructions.

Web research is disabled for both conditions so changing external evidence cannot affect the comparison. The conditions use the same strict JSON response contract. Five repetitions are planned for each of three controlled sanitized scenarios. Condition order alternates between repetitions to reduce order effects.

The embedded scenarios cover concentration, diversification, multiple currencies, long and short exposure, positive and negative unrealized results, and missing data. They are controlled evaluation fixtures and are not represented as captured customer accounts.

## Run

Version 5 is the cross-model protocol and the default suite. It reuses the corrected version 4 conditions, response contract, scenarios, scoring rules, and manual rubric. The fixed configurations are local Ollama `qwen2.5:3b`, OpenAI `gpt-6-luna`, and OpenAI `gpt-6.1-sol`. Results are executed and stored separately by configuration rather than pooled.

Start Ollama with the exact model digest recorded in `v5/manifest.yaml`, set `OPENAI_API_KEY`, and run the one-repetition pilots:

```powershell
go run ./cmd/evaluate -configuration ollama-qwen2-5-3b -run-kind pilot -pilot-repetitions 1
go run ./cmd/evaluate -configuration openai-gpt-6-luna -run-kind pilot -pilot-repetitions 1
go run ./cmd/evaluate -configuration openai-gpt-6-1-sol -run-kind pilot -pilot-repetitions 1
```

Replace `pilot -pilot-repetitions 1` with `final` for each 30-call final run. The thesis evaluation completed all three final configurations. Generated result bundles and manual-review working files are local research artifacts and are excluded from Git; the frozen protocol, fixtures, response contract, scoring implementation, and commands needed to reproduce them remain in this repository.

Version 4 uses `gpt-6-luna` through the OpenAI Responses API. It retains the version 2 scoring rules, manual rubric, and controlled scenarios while correcting the semantic evidence contract identified by the preserved version 3 pilot. Derivable base-currency position values are normalized before either condition receives them. Supplied unavailable facts may support explanations of missingness but cannot appear in `reported_values`. Set `OPENAI_API_KEY`, then run a one-repetition pilot from the desktop-client directory:

```powershell
go run ./cmd/evaluate -suite-version 4 -run-kind pilot -pilot-repetitions 1
```

The final version 4 command runs all five repetitions, for 30 evaluated calls plus one schema preflight:

```powershell
go run ./cmd/evaluate -suite-version 4 -run-kind final
```

Generated version 4 result bundles are also excluded from Git. Version 4 remains available to reproduce the corrected single-model protocol, while version 5 is the default cross-model suite.

Version 2 remains available for the frozen local-model protocol:

```powershell
go run ./cmd/evaluate -suite-version 2 -expected-model-digest <digest>
```

Version 3 remains available only to reproduce its preserved pilot contract:

```powershell
go run ./cmd/evaluate -suite-version 3 -run-kind pilot -pilot-repetitions 1
```

## UI capture

A build tagged `showcase` substitutes one controlled portfolio for the broker portfolio response without connecting to IBKR or writing to the application database. The frontend, navigation, analysis library, and counselor interactions are otherwise the same as in a normal build, with no capture-mode label in the rendered interface. Identify controlled scenarios in the thesis caption or surrounding methodology text rather than inside the application screenshot.

Select a scenario before starting Wails:

```powershell
$env:SHOWCASE_SCENARIO = "concentrated-multicurrency"
wails dev -tags showcase
```

Available scenarios are `concentrated-multicurrency`, `diversified-mixed-exposure`, `incomplete-data`, `large-diversified`, `south-korea-20-stock`, `etf-only-12`, and `multicurrency-10-stock`. The first three are protocol-locked evaluation inputs; the remaining fixtures are showcase-only and do not change the completed comparison suite. Open the analysis library and start each prompt manually, as in the normal application. The configured Ollama or OpenAI provider still produces the displayed answer, so use the same provider and model settings across screenshots. Features that require live stock, news, price-history, or trading data remain unavailable without IBKR.

Normal `wails dev` and `wails build` commands do not contain the controlled fixtures. A tagged production build writes to the standard executable path, so run an untagged `wails build` before packaging the submitted application if `wails build -tags showcase` was used.

`AI_BASE_URL` or `-base-url` may select another compatible endpoint in versions 3 and 4. Version 5 has separate `-ollama-base-url` and `-openai-base-url` flags. `-output` changes only the JSON result directory, while `-database` can select a separate SQLite database for an isolated evaluation run. Hosted-model suites store the provider-returned response ID, model, service tier, effective reasoning effort, and token usage for every completed call. Credentials are never written to the artifacts.

## Automated scoring

Each response is decoded with unknown JSON fields rejected. The scorer records:

- schema validity;
- required-fact coverage within the tolerance fixed by each scenario;
- numerical accuracy for all reported known metrics;
- grounding through valid supplied metric identifiers;
- structurally unsupported portfolio claims;
- numerical contradictions outside a wider contradiction tolerance;
- required limitation coverage;
- trading-authority safety declarations; and
- repeatability as mean pairwise Jaccard similarity of correctly covered required-fact sets.

Unsupported-claim and safety scores cover only properties represented by the response schema. `manual_rubric.yaml` fixes four author-applied criteria and their score anchors before results are generated: semantic support, semantic consistency, safety and authority, and limitation quality. The rubric is copied into every result bundle; completed manual ratings should be recorded alongside the final thesis results without modifying the original bundle.
