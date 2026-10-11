# Portfolio prompt evaluation

This package compares two fixed prompt conditions with the same model and provider configuration:

- `baseline` receives normalized positions and account-relative exchange rates.
- `proposed` receives the same normalized input plus deterministic backend metrics, stable metric identifiers, and grounding instructions.

Web research is disabled for both conditions so changing external evidence cannot affect the comparison. The conditions use the same strict JSON response contract. Five repetitions are planned for each of three controlled sanitized scenarios. Condition order alternates between repetitions to reduce order effects.

The embedded scenarios cover concentration, diversification, multiple currencies, long and short exposure, positive and negative unrealized results, and missing data. They are controlled evaluation fixtures and are not represented as captured customer accounts.

## Run

Version 4 uses `gpt-6-luna` through the OpenAI Responses API. It retains the version 2 scoring rules, manual rubric, and controlled scenarios while correcting the semantic evidence contract identified by the preserved version 3 pilot. Derivable base-currency position values are normalized before either condition receives them. Supplied unavailable facts may support explanations of missingness but cannot appear in `reported_values`. Set `OPENAI_API_KEY`, then run a one-repetition pilot from the desktop-client directory:

```powershell
go run ./cmd/evaluate -suite-version 4 -run-kind pilot -pilot-repetitions 1
```

The final version 4 command runs all five repetitions, for 30 evaluated calls plus one schema preflight:

```powershell
go run ./cmd/evaluate -suite-version 4 -run-kind final
```

The locked final run completed on 10 October 2026. All 15 proposed responses and 10 of 15 baseline responses passed semantic validation; all five baseline failures occurred in the concentrated-multicurrency scenario. The run had no provider, timeout, truncation, or malformed-output failures. The timestamped result bundle and its blind-review files are retained in `evaluation/results`; manual scoring under the frozen rubric remains separate from the automated scores.

Version 2 remains available for the frozen local-model protocol:

```powershell
go run ./cmd/evaluate -suite-version 2 -expected-model-digest <digest>
```

Version 3 remains available only to reproduce its preserved pilot contract:

```powershell
go run ./cmd/evaluate -suite-version 3 -run-kind pilot -pilot-repetitions 1
```

`AI_BASE_URL` or `-base-url` may select another compatible endpoint. Leave it unset for the default OpenAI endpoint in versions 3 and 4. `-output` changes only the JSON result directory, while `-database` can select a separate SQLite database for an isolated evaluation run. Hosted-model suites store the provider-returned response ID, model, service tier, effective reasoning effort, and token usage for every completed call. Credentials are never written to the artifacts.

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
