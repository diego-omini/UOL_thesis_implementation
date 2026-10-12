# Evaluation output

The evaluation command writes timestamped JSON bundles and blind-review files to this directory by default. Generated outputs are intentionally ignored by Git because they can contain large model responses, provider metadata, and machine-specific run details.

Run the fixed version 5 configurations from `desktop-client`:

```powershell
go run ./cmd/evaluate -configuration ollama-qwen2-5-3b -run-kind pilot -pilot-repetitions 1
go run ./cmd/evaluate -configuration openai-gpt-6-luna -run-kind pilot -pilot-repetitions 1
go run ./cmd/evaluate -configuration openai-gpt-6-1-sol -run-kind pilot -pilot-repetitions 1
```

Use `-run-kind final` for a complete fixed-configuration run. Use `-output` to write bundles to another local directory and `-database` to select an isolated SQLite database. See [`../README.md`](../README.md) for protocol and scoring details.
