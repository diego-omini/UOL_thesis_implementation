# Evaluation results

Run the fixed evaluation suite from the desktop-client directory:

```powershell
go run ./cmd/evaluate
```

Each execution writes one timestamped JSON bundle containing the suite manifest, frozen scenarios, condition definitions, runtime and model identity, raw responses, parsed responses, per-run scores, and aggregate scores. The same bundle is also stored in the application's local SQLite database.

Use `-expected-model-digest` for final thesis runs so execution stops if the installed Ollama model differs from the recorded model build. Result files should be retained unchanged once they are included in the evaluation dataset.
