# Test: Multiple Sequential Tool Calls

## Purpose

Verify the agent calls the same tool twice in one interaction and compares both results (no short-circuit).

## Prerequisites

- Shell with `bash`.
- Working directory: `tests/agent/integration-cli`.
- `<MODEL>` from `go run . list-models`.

## Commands

```bash
cd tests/agent/integration-cli

export MODEL='<provider/model>'
export SESSION_ID="test-multi-tool-$(date +%s)"

go run . run \
  --model "$MODEL" \
  --session "$SESSION_ID" \
  --prompt "$(cat <<'PROMPT'
You have tools available.

1. Call the test_get_location tool to get the current location. Note the result.
2. Call the test_get_location tool a second time. Note the result.
3. Compare the two results and report whether they are the same or different.
4. Include both raw results in your response.
PROMPT
)"
```

Expect exit code **0**.

## Expected output

`test_get_location` returns deterministic data every call: city `New York`, country `US`, latitude `40.7128`, longitude `-74.0060`.

- Stdout should show evidence of **two** tool uses or two distinct location payloads (not a single inferred answer without a second call).
- Agent should state that both results match (same city/country/coordinates).

## Success criteria

- Exit code 0.
- Output includes both results or clear duplication of the same tool output twice.
- Agent concludes the two results are the same (consistent with deterministic tool).

## Failure indicators

- Only one tool invocation implied.
- Second result skipped with wording like “already know” without two calls.
- Non-zero exit code.
