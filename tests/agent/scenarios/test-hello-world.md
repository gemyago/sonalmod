# Test: Hello World

## Purpose

Verify the agent can follow a simple, exact-output instruction. Baseline smoke test — if this fails, nothing else will work.

## Prerequisites

- Shell with `bash` (for heredoc prompts below).
- Working directory for all commands: `tests/agent/integration-cli` (see [tests/AGENTS.md](../../AGENTS.md)).
- `<MODEL>`: one `provider/model` from `go run . list-models` (lines look like `* provider/model`; use the name **without** the `* `).

## Commands

From repository root:

```bash
cd tests/agent/integration-cli

# Optional: print models; pick <MODEL> from output.
go run . list-models

export MODEL='<provider/model>'   # replace with a real name from list-models
export SESSION_ID="test-hello-world-$(date +%s)"

go run . run \
  --model "$MODEL" \
  --session "$SESSION_ID" \
  --prompt "$(cat <<'PROMPT'
Respond with exactly the following string on its own line:

HELLO_WORLD_OK

Do not add any other text, explanation, or formatting. Output only: HELLO_WORLD_OK
PROMPT
)"
```

Expect exit code **0**.

## Expected output

- Stdout (agent response text) contains the exact substring `HELLO_WORLD_OK`.
- Preamble/extra lines are acceptable as long as `HELLO_WORLD_OK` appears.

## Success criteria

- Last command exits with code 0.
- Output contains `HELLO_WORLD_OK`.

## Failure indicators

- Exit code non-zero.
- `HELLO_WORLD_OK` missing from output.
- Process crash or empty response with no substring match.
