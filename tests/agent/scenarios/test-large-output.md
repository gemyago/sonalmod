# Test: Large Output

## Purpose

Verify the agent can generate a large streamed response without truncation or corruption (numbers 1–200, one per line).

## Prerequisites

- Shell with `bash`.
- Working directory: `tests/agent/integration-cli`.
- `<MODEL>` from `go run . list-models` as in [tests/AGENTS.md](../../AGENTS.md).

## Commands

```bash
cd tests/agent/integration-cli

export MODEL='<provider/model>'
export SESSION_ID="test-large-output-$(date +%s)"

go run . run \
  --model "$MODEL" \
  --session "$SESSION_ID" \
  --prompt "$(cat <<'PROMPT'
Generate a numbered list from 1 to 200. Each number must be on its own line in this exact format:

1
2
3
...
200

Output only the numbers, one per line. No header, no footer, no explanation.
PROMPT
)"
```

Expect exit code **0**.

## Expected output

- Stdout contains every integer from **1** through **200** inclusive.
- Each number appears on its own line (allow leading/trailing whitespace on lines; numbers must be present).
- Output is not truncated before `200`.

**Practical check:** After the run, you may pipe or save stdout and verify programmatically that all integers 1–200 appear (e.g. count distinct lines matching `^[[:space:]]*[0-9]+[[:space:]]*$` and coverage); or confirm visually that the stream ends at 200 with no gap in sequence.

## Success criteria

- Exit code 0.
- All numbers 1–200 present; none missing or duplicated; ends at 200.

## Failure indicators

- Any number in 1–200 absent.
- Truncation (e.g. stops before 200).
- Numbers merged on one line such that required lines are missing.
