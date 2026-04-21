# Test: Session Awareness

## Purpose

Verify conversation context persists across **two** sequential `run` invocations with the **same** `--session` value.

## Prerequisites

- Shell with `bash`.
- Working directory: `tests/agent/integration-cli`.
- `<MODEL>` from `go run . list-models`.
- Pick **one** session id for both runs, e.g. `export SESSION_ID="test-session-awareness-$(date +%s)"` — **reuse exactly** for command 2.

## Commands

### Command 1 — store phrase

```bash
cd tests/agent/integration-cli

export MODEL='<provider/model>'
export SESSION_ID="test-session-awareness-$(date +%s)"

go run . run \
  --model "$MODEL" \
  --session "$SESSION_ID" \
  --prompt "$(cat <<'PROMPT'
I will tell you a memorable phrase: ZEBRA_42_ALPHA

Please confirm you understood by responding with: PHRASE_STORED
PROMPT
)"
```

Expect exit code **0**. Note: stdout should contain `PHRASE_STORED`.

### Command 2 — recall phrase (same session)

Use the **same** `$SESSION_ID` as command 1 (re-export if you opened a new shell):

```bash
cd tests/agent/integration-cli

export MODEL='<provider/model>'
# SESSION_ID must match command 1 exactly:
export SESSION_ID='<same-as-command-1>'

go run . run \
  --model "$MODEL" \
  --session "$SESSION_ID" \
  --prompt "What memorable phrase did I share with you earlier in this conversation?"
```

Expect exit code **0**.

## Expected output

- After command 1: substring `PHRASE_STORED` in stdout.
- After command 2: substring `ZEBRA_42_ALPHA` in stdout.

## Success criteria

- Both commands exit 0.
- First output contains `PHRASE_STORED`.
- Second output contains `ZEBRA_42_ALPHA`.

## Failure indicators

- Second output does not contain `ZEBRA_42_ALPHA`.
- Agent claims no memory of a prior turn when `SESSION_ID` was reused.
- Different `--session` values used across the two runs (invalidates test).
