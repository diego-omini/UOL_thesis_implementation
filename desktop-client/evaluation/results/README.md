# Evaluation results

Run the fixed evaluation suite from the desktop-client directory:

```powershell
go run ./cmd/evaluate -suite-version 4 -run-kind pilot -pilot-repetitions 1
```

Each execution writes one timestamped JSON bundle containing the suite manifest, frozen scenarios, condition definitions, runtime and model identity, raw responses, parsed responses, per-run scores, and aggregate scores. The same bundle is also stored in the application's local SQLite database.

Use `-run-kind final` for the complete version 4 protocol. Versions 2 and 3 remain explicitly selectable; version 2 still accepts `-expected-model-digest` so execution stops if the installed Ollama model differs from its recorded build. Result files should be retained unchanged once they are included in the evaluation dataset.

The completed version 4 final dataset is `portfolio-prompt-comparison-v4-final-20261010T194424Z.json`, with matching `-blind-review.json` and `-blind-map.json` files. The schema preflight and all 30 calls completed. Proposed passed semantic validation in 15/15 runs and baseline in 10/15; the blind-review artifact is ready for the separately scored manual rubric.
