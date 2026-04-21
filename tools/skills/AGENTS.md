# Skills tools (`tools/skills`)

Go module that exposes a **skill discovery and activation toolset** for the Sonalmod agent runtime. Skills are discovered from configured directories containing `SKILL.md` files; agents receive compact metadata at startup and load full skill instructions on demand via tool calls.

## Registration contract

Integrators use a single type, **`Skills`**, built with **`New(roots, opts...)`** and two methods:

1. **`agent.WithSystemPromptFragments(skillSet.BuildSystemPromptFragments()...)`** — optional variadic spread into runner options (Section and Content are set by the method; empty catalog yields no fragments).
2. **`skillSet.RegisterTools(registry)`** — register `skills_list` and `skills_read` on the agent tools registry.

Optional **`Option`** helpers: **`WithLogger`**, **`WithMaxSkillBytes`**, **`WithMaxCatalogEntries`**. Omitting **`WithLogger`** uses [slog.Default](https://pkg.go.dev/log/slog#Default) for catalog warnings and stored logger defaults.

Catalog construction and types live in **`internal/skills`**; the public surface is **`New`**, **`Skills`**, **`RegisterTools`**, and **`BuildSystemPromptFragments`**.

In this flow:

- Each path is scanned for subdirectories containing a `SKILL.md` file. Non-existent paths are skipped with a warning; they do not cause catalog creation to fail.
- Duplicate skill names are resolved by configured root order: first discovered name wins; later duplicates are skipped with a warning.
- **`skills_list`** returns name and description per discovered skill in catalog order; it does not expose host filesystem paths.
- **`skills_read`** returns the full `SKILL.md` body for a named skill. Only skills in the catalog are accessible; arbitrary path input is not accepted.

## Public Contract

Public contract stays minimal and tight. Public contract is anything exported from the module (uppercase and non-internal).

Before extending the public contract (e.g. new exported types or methods):

- Prefer unexported helpers or internal packages.
- If export seems necessary, reconsider; unexport if there is any doubt.
- Only export after that second pass, and keep the API minimal.

Rules for doc comments on public contract types and methods:

- Docs should not expose internal implementation details or underlying frameworks used.
- Docs should be concise and to the point.

## Security and boundaries

- Skills are **read-only** in this phase. No script execution.
- Skill loading is restricted to configured catalog directories only.
- File size and catalog size limits are enforced (see `internal/skills` package).
- Absolute host paths are never returned in tool responses or error messages.
- Catalog is built once at startup; filesystem changes after startup are not reflected until process restart.

## Default limits

| Limit | Where |
|-------|-------|
| Max bytes per skill file | `internal/skills.DefaultMaxSkillBytes` (override with **`WithMaxSkillBytes`**) |
| Max catalog entries | `internal/skills.DefaultMaxCatalogEntries` (override with **`WithMaxCatalogEntries`**) |

## Module Rules and Conventions

This section defines module-specific rules and conventions. Project-level rules and conventions must also be followed.

Use gopher skill as your primary source of golang coding conventions and best practices.

The rules are:

- Update module rules and conventions when user corrects the behavior of AI.
- JSON uses camelCase for property names and identifiers in tool request/response structs.
- **`internal/skills`:** JSON request/response structs for a tool live in the same `.go` file as the service methods for that tool.

## Commands

```sh
# Run tests with coverage
make test

# Run linter
make lint
```

## Task Completion Protocol

Repository level task completion protocol **MUST ALWAYS** be followed. If you didn't follow it, this means task is not complete.
