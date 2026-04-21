# Workspacefs tools (`tools/workspacefs`)

Go module that exposes a **scoped filesystem and shell-execution toolset** for the Sonalmod agent runtime: file operations are confined with `os.Root` under each **configured workspace**. At registration, callers supply **workspace identifiers** mapped to host directory paths; models and JSON tools use **identifiers and relative paths only**—not absolute host directories as selectors.

## Workspace identifiers and model-visible contract

- **Registration:** Use **`WithWorkspaces`** with **`WorkspaceConfig`** entries: **`Identifier`** and **`Description`** are model-visible; **`Path`** is the host directory (validated, normalized internally, **never** returned in tool responses or `workspacefs_list_workspaces`).
- **Selection:** Every filesystem tool request includes required JSON field **`workspace`** (camelCase in OpenAPI), matching a configured identifier—including when only one workspace exists.
- **Discovery:** **`workspacefs_list_workspaces`** returns identifier and short description per entry in configuration order; it does not expose filesystem paths.
- **Paths:** Request path fields are **relative** to the selected workspace (no absolute paths, no `..` escape), enforced via sanitization and `os.Root` confinement.
- **Errors and docs:** Use **workspace** terminology for tool-facing messages (for example unknown or missing workspace); avoid implying models should pass absolute roots. Path validation errors refer to the **`path`** input without echoing configured host directories.

## Public Contract

Public contract stays minimal and tight. Public contract is anything that is exported from the module (e.g uppercase and non internal)

Before extending the public contract (e.g. new exported types or methods e.t.c):
- Prefer unexported helpers or internal packages.
- If export seems necessary, reconsider; unexport if there is any doubt.
- Only export after that second pass, and keep the API minimal.

Rules for doc comments on public contract types and methods:
- Docs should not expose internal implementation details or underlying frameworks used (e.g ADK, genkit e.t.c)
- Docs should be concise and to the point.

## Security and scoping

- Enforcement uses **`os.OpenRoot` / `os.Root`**: relative tool paths never resolve outside the opened directory for the **selected workspace**. Do not replace this with `filepath.Join` + `os.Open` for untrusted paths.
- **Symlinks and mounts:** Behavior is defined by **`os.Root`** and the Go version in use; exotic layouts (e.g. bind mounts) may have platform-specific caveats—see Go documentation.

## Default limits

| Limit | Where |
|-------|--------|
| Max bytes per single read | **`internal/workspacefs.DefaultMaxReadBytes`** (1 MiB); configurable via **`WithMaxReadBytes`** on **`NewService`** |
| Max entries (list / tree / search matches) | **`internal/workspacefs.DefaultMaxListEntries`**; **`WithMaxListEntries`** on **`NewService`** |
| Directory tree depth when omitted | **`internal/workspacefs.DefaultMaxTreeDepth`**; request **`max_depth`** minimum 1, values **> 100** clamped |

## Exec tools contract

Exec tools are **opt-in**: they are not registered by default. Enable them at registration with **`WithExec(ExecOptions{...})`**.

### Tools

| Tool | Description |
|------|-------------|
| `workspacefs_exec_command` | Execute a shell command within a workspace. Set `background: true` for long-running commands; returns a `jobId`. |
| `workspacefs_exec_job_output` | Poll output and status for a background job by `jobId`. |
| `workspacefs_exec_kill_job` | Terminate an active background job by `jobId`. |

### Request contract

- **`workspace`** is required on every exec tool call (same contract as filesystem tools).
- **`workingDirectory`** is optional and must be relative to the workspace root; absolute paths and `..` escapes are rejected.
- **`command`** is passed to the OS shell (`sh -c` on Unix, `cmd /c` on Windows); no path confinement applies to the command itself (see security boundary note below).

### Response contract

| Field | When present |
|-------|-------------|
| `exitCode` | Always; 0 on success, non-zero on failure, -1 when unavailable. |
| `stdout` / `stderr` | Captured output (possibly truncated; see `truncated` flag). |
| `truncated` | True when stdout or stderr was cut at the output byte cap. |
| `timedOut` | True when the foreground command exceeded the configured timeout. |
| `jobId` | Background mode only; stable UUID for subsequent poll/kill calls. |
| `running` | Background start response only (`true`). |
| `elapsed` | Job output response only; present when job is no longer running. |

### Security boundary

`os.Root` confines **filesystem tools only**. Shell commands invoked via exec tools run as the host process user with **no filesystem sandbox**. Exec is therefore disabled by default.

Two complementary controls narrow the risk:

1. **Command denylist** — applied before process start. When `ExecOptions.BlockedCommands` is nil, the built-in denylist blocks common network exfiltration tools: `curl`, `wget`, `nc`, `netcat`, `ncat`, `ssh`, `scp`, `sftp`, `ftp`, `telnet`, `rsync`. Path prefixes are stripped (e.g. `/usr/bin/curl` → `curl`); matching is case-insensitive.
2. **Resource limits** — timeout, max output bytes, max concurrent background jobs — prevent runaway processes.

### Exec default limits

| Limit | Default constant | `ExecOptions` field |
|-------|-----------------|---------------------|
| Max output bytes per stream | **`DefaultExecMaxOutputBytes`** (1 MiB) | `MaxOutputBytes` (0 → default) |
| Per-command timeout | **`DefaultExecTimeout`** (30 s) | `DefaultTimeout` (0 → default) |
| Max concurrent background jobs | **`DefaultExecMaxConcurrentJobs`** (10) | `MaxConcurrentJobs` (0 → default) |

### Job lifecycle

On Unix, the shell process is started in its own process group so cancellation, timeouts, and kill apply to child processes as well (for example `sleep` under `sh -c`). On Windows, termination targets the shell process only.

1. Call `workspacefs_exec_command` with `background: true` → receive `jobId`.
2. Poll `workspacefs_exec_job_output` with `jobId` until `running` is false.
3. Optionally call `workspacefs_exec_kill_job` to terminate early.
4. On `Service.Close()`, all active background jobs are cancelled and their resources released.

## Module Rules and Conventions

This section defines module-specific rules and conventions. Project-level rules and conventions must also be followed.

Use gopher skill as your primary source of golang coding conventions and best practices.

The rules are:
- Update module rules and conventions when user corrects the behavior of AI.
- OpenAPI JSON uses camelCase for property names or any other identifiers or keys; regenerate after spec edits.
- **`internal/workspacefs`:** JSON request/response structs for a tool live in the same `.go` file as the `Service` methods for that tool, not in separate `models_*.go` files.
- Tests: For assertions that depend on lexicographic order of generated names (e.g. directory listings), use deterministic prefixes (`a-` / `m-` / `z-`) with UUID suffixes, or sort both sides before comparing.

## Task Completion Protocol

Repository level task completion protocol **MUST ALWAYS** be followed. If you didn't follow it, this means task is not complete.