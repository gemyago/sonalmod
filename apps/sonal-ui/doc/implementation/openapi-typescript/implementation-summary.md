# Implementation Summary: OpenAPI TypeScript + openapi-fetch (sonal-ui)

**Plan:** [plan-openapi-typescript.md](./plan-openapi-typescript.md)

## Overview

The UI now depends on **openapi-fetch** and **openapi-typescript**, with Makefile-only **`make generate-api`** / **`make check-api`** against `runtime/internal/agentapi/openapi.yaml`. Generated **`agentapi.generated.ts`** drives typed HTTP via **`createAgentApiClient`**, while SSE still uses raw **`Response`** streams. **`StreamEvent`** and related schemas come from codegen end-to-end; **`make lint`** runs **`check-api`** so the spec and generated types stay aligned.

## Tasks

### Task 1.1: Dependencies
Added **openapi-fetch** and **openapi-typescript** to `package.json` and refreshed **`package-lock.json`**. No codegen npm scripts—Makefile-only per plan.

### Task 1.2: Makefile targets
**`generate-api`** and **`check-api`** in **`apps/sonal-ui/Makefile`**; **`AGENTS.md`** documents paths and usage. Local **`make generate-api`** / **`check-api`** verified.

### Task 1.3: Initial codegen commit
Ran codegen and **`check-api`** successfully; generated artifact was already committed and matched the spec, so no extra commit was needed for this step alone.

### Task 2.1: Typed client module
**openapi-fetch** client with generated **`paths`**, **`startAgentRun`** / **`continueAgentRun`** using **`parseAs: 'stream'`** and returning **`Response`** for SSE. Vitest + MSW + faker coverage; **`Chat.test.ts`** updated for openapi-fetch **`Request`** URLs.

### Task 2.2: Wire SSE to generated `StreamEvent`
**`sse.ts`** and **`types.ts`** use generated **`StreamEvent`**; **`isStreamEvent()`** guard; **`Chat.svelte`** and **`sse.test.ts`** updated.

### Task 3.1: Lint pipeline and env typing
**`make lint`** in sonal-ui runs **`npm run lint`** then **`check-api`**. **`vite-env.d.ts`** documents **`VITE_*`** on **`ImportMetaEnv`**.

### Task 3.2: Documentation and wireframe
**`ui-wireframe.md`** unchanged—no user-visible behavior or in-app configuration change.

### Task 4.1: Completion protocol
Root **`make lint`** and **`make test`** passed; **`AGENTS.md`** already reflected Task 3.1 workflow.

## Deviations & notes

- **Task 1.2 / 1.3:** Generated file committed under Task 1.2; Task 1.3 reruns produced no diff—commit step satisfied by existing artifact.
- **Task 2.1:** Tests use absolute base URL for Node/Vitest + openapi-fetch; on non-2xx, openapi-fetch surfaces **`error`** and the **`Response`** body may not be re-readable (aligned with **`Chat.svelte`** checking **`ok`** / status).

## Completion

- Lint: ✓
- Type check: ✓
- Tests: ✓
