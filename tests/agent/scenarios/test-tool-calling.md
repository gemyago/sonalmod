# Test: Tool Calling

## Purpose

Verify the agent invokes tools, chains results (location → weather), and reports tool-returned data.

## Prerequisites

- Shell with `bash`.
- Working directory: `tests/agent/integration-cli`.
- `<MODEL>` from `go run . list-models`.

## Commands

```bash
cd tests/agent/integration-cli

export MODEL='<provider/model>'
export SESSION_ID="test-tool-calling-$(date +%s)"

go run . run \
  --model "$MODEL" \
  --session "$SESSION_ID" \
  --prompt "$(cat <<'PROMPT'
You have tools available. Do the following:

1. Call the test_get_location tool to get the current location.
2. Use the location returned by test_get_location as input to the test_get_weather tool.
3. Report both results clearly, including city and country from the location tool, and temperature, conditions, and humidity from the weather tool.
PROMPT
)"
```

Expect exit code **0**.

## Expected output

Deterministic tool data (integration-cli test tools):

- `test_get_location`: city `New York`, country `US` (and fixed lat/lon in tool result).
- `test_get_weather`: temperature `22.5`, unit `celsius`, conditions `Partly Cloudy`, humidity `65`.

Stdout should reflect that the model used the tools (not invented unrelated cities) and should include at least:

- `New York` or `US`
- `22.5` and/or `Partly Cloudy` (or equivalent weather fields from the tool)

## Success criteria

- Exit code 0.
- Output references location data consistent with New York / US.
- Output references weather data consistent with 22.5 °C / Partly Cloudy (or clear paraphrase of the same tool payload).

## Failure indicators

- No tool usage (fabricated data with no connection to tool returns).
- Missing location or weather substance.
- Non-zero exit code.
