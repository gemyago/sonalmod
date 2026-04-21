# Integration and e2e tests

## Running agent integration tests

Agent integration tests require a local providers config file with real LLM credentials. This file is gitignored and must be created manually.

### Manual Setup

Copy the example and fill in your credentials:

```bash
cp tests/agent/integration-cli/data/providers/provider.example.yaml \
   tests/agent/integration-cli/data/providers/provider.yaml
```
Name the file against provider name.

Update `tests/agent/integration-cli/data/providers/provider.yaml` with provider specific details, like models, base URL e.t.c.

**NOTE for AI**: AI must not attempt to read actual provider files, never!!!

### Run tests

Run agent that supports sub-agents (do we have any that doesn't?) and run the prompt like this: "Please run agent integration tests".