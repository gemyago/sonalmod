# Test: Error Handling

## Purpose

Verify tool errors are surfaced cleanly: agent does not crash; user-visible text describes the failure (e.g. empty location).

## Prerequisites

- Shell with `bash`.
- Working directory: `tests/agent/integration-cli`.
- `<MODEL>` from `go run . list-models`.

## Commands

```bash
cd tests/agent/integration-cli

export MODEL='<provider/model>'
export SESSION_ID="test-error-handling-$(date +%s)"

go run . run \
  --model "$MODEL" \
  --session "$SESSION_ID" \
  --prompt "$(cat <<'PROMPT'
Call the test_get_weather tool with an empty string as the location value.

Report what happened: did the tool return an error? What was the error message or behaviour?
PROMPT
)"
```

Expect exit code **0** (the CLI should still exit successfully; the tool error is part of the agent/tool protocol, not necessarily a process crash).

## Expected output

- Agent output describes that the tool failed, returned an error, or could not get weather for an empty location — not fabricated weather numbers.
- Substrings that often appear: `error`, `failed`, `empty`, or similar plain-language description.

## Success criteria

- Exit code 0 from `go run . run`.
- Output mentions failure/error/empty location behaviour (semantic match OK).
- No fabricated plausible weather **without** acknowledging the tool error path.

## Failure indicators

- Agent refuses to call the tool and never attempts the scenario.
- Process panic / non-zero exit without user-visible explanation.
- Silent success with made-up temperature/humidity as if the call succeeded.
- Output claims perfect weather data with no mention of error when empty location was requested.
