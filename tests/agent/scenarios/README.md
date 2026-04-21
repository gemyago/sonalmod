# Agent integration scenarios

Each `test-*.md` file is a **self-contained** recipe: one sub-agent opens **only** that file, runs the listed shell commands against `tests/agent/integration-cli`, and decides PASS/FAIL from **Expected output** and success criteria in the same file.

Orchestration (who runs which file, in what order) lives in [master.md](./master.md). The master does **not** read these bodies—only paths.

## Scenario file template

Use this section order so every scenario stays executable without extra interpretation:

1. **Purpose** — What capability is under test.
2. **Prerequisites** — Working directory, provider setup (see [tests/AGENTS.md](../../AGENTS.md)); do not read gitignored provider files.
3. **Commands** — Numbered steps: `cd`, `go run . list-models` (when needed), `go run . run ...` with `--model`, `--prompt`, and `--session` as required. Use placeholders `<MODEL>`, `<SESSION_ID>` where the executor fills values.
4. **Expected output** — What MUST appear in integration-cli stdout (and exit code if relevant) for each command.
5. **Success criteria** — Checklist tying back to **Expected output**.
6. **Failure indicators** — Clear FAIL conditions.

Flags supported by the CLI today: `run` requires `--model` and `--prompt`; optional `--session` (`-s`). Use `list-models` to obtain `provider/model` names. There is **no** `--providers-config` flag—configuration is via the integration-cli data layout described in [tests/README.md](../../README.md).
