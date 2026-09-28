# Portfolio prompt evaluation

This package compares two fixed conditions with the same Ollama model and generation settings:

- `baseline` receives normalized positions and account-relative exchange rates.
- `proposed` receives the same normalized input plus deterministic backend metrics, stable metric identifiers, and grounding instructions.

Web research is disabled for both conditions so changing external evidence cannot affect the comparison. The conditions use the same strict JSON response contract. Five repetitions are run for each of three controlled sanitized scenarios. Condition order alternates between repetitions to reduce order effects.

The embedded scenarios cover concentration, diversification, multiple currencies, long and short exposure, positive and negative unrealized results, and missing data. They are controlled evaluation fixtures and are not represented as captured customer accounts.

## Run

Make sure the model declared in `manifest.yaml` is installed in Ollama, then run from the desktop-client directory:

```powershell
go run ./cmd/evaluate
```

For a final recorded run, first copy the model digest from a preliminary result bundle and require it explicitly:

```powershell
go run ./cmd/evaluate -expected-model-digest <digest>
```

`AI_BASE_URL` or `-base-url` may select another local Ollama endpoint without changing the fixed model or generation settings. `-output` changes only the JSON result directory, while `-database` can select a separate SQLite database for an isolated evaluation run.

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
