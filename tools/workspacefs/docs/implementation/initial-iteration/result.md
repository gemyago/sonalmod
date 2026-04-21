# Deep research: Scoped filesystem tools for LLM agents and a Go host

## Executive summary

This synthesis answers how to define a **minimal viable set** of scoped filesystem tools for coding agents, how to **shape APIs** so models use them efficiently, and how a **Go host** should confine paths and test behavior. There is **no single industry-wide “five tools” standard**: the Model Context Protocol (MCP) specifies **wire behavior** (`tools/list`, JSON Schema parameters, `tools/call`, structured results with `isError`) and **Roots** for scoping, while concrete tool names come from implementations—most authoritatively the **reference `@modelcontextprotocol/server-filesystem`**, which uses `read_text_file`, `write_file`, `edit_file`, `list_directory`, `search_files`, `directory_tree`, and related helpers rather than literal `grep`/`glob`/`list_dir` names. **Bounded reads** (`head`/`tail` line windows, mutually exclusive), **batch reads** with partial failure, and **dry-run before edit** are recurring good patterns; **pagination** applies to tool *listing*, while large *file* outputs need **tool-level** limits—industry discussions (e.g. xagent#213) stress caps, streaming, and truncation notices but are **not** normative. For **editing**, OpenAI’s Agents SDK centers **diff-oriented** `ApplyPatchTool` with a host-implemented `ApplyPatchEditor`; the MCP reference server uses **`oldText`/`newText`** edits with Git-style diff output and recommends **dry run first**—MCP does **not** mandate unified-diff input. For **Go**, **`os.Root` / `OpenInRoot`** and **`Root.FS()`** (not `DirFS`) are the primary confinement story; **`filepath.IsLocal`/`Localize`** are lexical only. **CVE-2025-22873** (trailing `../` opening the parent) is fixed in **Go 1.24.3** per **golang-announce**; pin **≥ 1.24.3** for `os.Root` security. **Testing** should combine table-driven path cases, fuzzing of sanitization, UTF-8/binary contracts, MCP contract tests (`tools/list`/`tools/call`, protocol vs `isError`), and **gosec** (G304-style) as a complement—not a substitute—for tests.

## Key findings

1. **MCP interoperability essentials** — Clients discover tools via **`tools/list`** with optional **cursor pagination**; each tool has **`name`**, **`description`**, **`inputSchema`**, optional **`annotations`**; invocation is **`tools/call`**; execution failures can use **`isError: true`** while still returning structured content ([MCP server tools spec](https://modelcontextprotocol.io/specification/2025-03-26/server/tools), [MCP pagination](https://modelcontextprotocol.io/specification/2025-03-26/server/utilities/pagination)).

2. **Scoping is a protocol concern** — **Roots** use **`file://` URIs**; clients must validate roots; servers should validate paths against roots; the reference filesystem server combines **CLI allowlists** with **Roots** and exposes **`list_allowed_directories`** ([MCP client roots](https://modelcontextprotocol.io/specification/2025-06-18/client/roots), [filesystem server README](https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/filesystem/README.md)).

3. **Reference tool surface (not `grep` by name)** — The authoritative README maps exploration to **`read_text_file`** (optional **`head`/`tail`**, UTF-8), **`search_files`** (glob-style), **`directory_tree`**, **`list_directory`**, **`get_file_info`**, **`read_multiple_files`** (failed reads do not abort the batch), and writes/edits via **`write_file`** and **`edit_file`** with **`dryRun`** ([filesystem server README](https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/filesystem/README.md)).

4. **Annotations help UIs/models** — The reference server sets **readOnly / idempotent / destructive** hints per tool; the MCP spec notes annotations must be treated as **untrusted** unless the server is trusted ([filesystem server README](https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/filesystem/README.md), [MCP server tools spec](https://modelcontextprotocol.io/specification/2025-03-26/server/tools)).

5. **OpenAI Agents SDK differs in taxonomy** — Local/runtime tools include **`ApplyPatchTool`** (requires host **`ApplyPatchEditor`**), **`ShellTool`**, etc.; filesystem-shaped work is not enumerated as a fixed `read_file`/`grep` matrix; **function tools** can implement custom read patterns ([OpenAI Agents SDK — Tools](https://openai.github.io/openai-agents-python/tools/)).

6. **Structured errors** — Distinguish **JSON-RPC protocol errors** (e.g. unknown tool, bad cursor → **-32602**) from **tool execution** failures surfaced with **`isError: true`** ([MCP server tools spec](https://modelcontextprotocol.io/specification/2025-03-26/server/tools)).

7. **Truncation and output size** — Reference server: line windows via **`head`/`tail`**, **`dryRun`** for edits; MCP spec does not define a global **truncation marker string**. xagent#213 discusses unified output limits and streaming as **ecosystem pain**, not a standard ([filesystem server README](https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/filesystem/README.md), [xagent#213](https://github.com/xorbitsai/xagent/issues/213)).

8. **Patch vs replace** — **Agents SDK**: `ApplyPatchTool` is **diff-oriented** (`ApplyPatchOperation` includes **`diff`** on updates); **not** interchangeable with ad hoc tools without changing the wire contract. **MCP reference**: **`edit_file`** uses **`oldText`/`newText`**, documents Git-style diff output, recommends **dry run first**; **replaceable in principle** by read+write **at the cost** of bundled semantics ([OpenAI Agents SDK — Tools](https://openai.github.io/openai-agents-python/tools/), [Editor reference](https://openai.github.io/openai-agents-python/ref/editor/), [filesystem server README](https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/filesystem/README.md)).

9. **Go confinement** — **`OpenRoot`/`OpenInRoot`**, **`Root`** methods block paths outside the root **including via symlinks** (with documented platform caveats); **`Root.FS()`** is preferred over **`DirFS`** for symlink-safe `fs.FS`; **`filepath.IsLocal`** does not account for symlinks ([pkg.go.dev/os#Root](https://pkg.go.dev/os#Root), [go.dev/blog/osroot](https://go.dev/blog/osroot), [Go 1.24 release notes](https://go.dev/doc/go1.24)).

10. **CVE-2025-22873** — Parent directory reachable via names ending in **`"../"`**; **fixed in Go 1.24.3**; fix **only applies to 1.24.x**; **1.23.x not affected** in the same way per announcement ([golang-announce](https://groups.google.com/g/golang-announce/c/UZoIkUT367A), [issue #73555](https://go.dev/issue/73555)).

11. **Testing** — Table-driven path cases (absolute, `..`, symlinks, Windows nuances), **fuzz** `string`/`[]byte` targets for sanitization, UTF-8/binary contracts, MCP **`tools/list`/`tools/call`** and **`isError`** behavior, **gosec** G304-style hints toward **`os.Root`** ([go.dev/doc/security/fuzz](https://go.dev/doc/security/fuzz/), [go.dev/blog/osroot](https://go.dev/blog/osroot), [MCP server tools spec](https://modelcontextprotocol.io/specification/2025-03-26/server/tools), [gosec#1297](https://github.com/securego/gosec/issues/1297)).

## Detailed sections

### 1. Smallest tool set and competing views

The **MCP specification** standardizes **how** tools are listed and called, not a single minimal FS tool set. The **reference filesystem server** is the most concrete cross-implementation inventory in this research: text read (with optional line windows), batch read, write, substring-based edit with dry-run, directory list, glob-style search, tree, metadata, and allowlist introspection ([filesystem server README](https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/filesystem/README.md)). **OpenAI Agents SDK** documentation emphasizes **categories** (hosted vs local vs MCP) and **`ApplyPatchTool`** for disk edits rather than a parallel grep/glob/read matrix ([OpenAI Agents SDK — Tools](https://openai.github.io/openai-agents-python/tools/)).

**Inference:** For a **coding** agent, a practical minimum often includes **read (bounded)**, **write or edit**, **list or tree**, and **search by name/path pattern**—aligned with the reference server’s split—but exact names and optional tools remain **host/server choices** as long as schemas and Roots are coherent.

### 2. Essential vs nice-to-have

| Layer | Treated as essential (per cited docs) | Commonly optional |
| --- | --- | --- |
| MCP wire | `tools/list` (+ pagination), `tools/call`, JSON Schema per tool, structured results + `isError` | `listChanged`, annotations |
| Scoping | At least one allowed directory; path checks vs roots | Dynamic roots vs CLI-only |
| Reference FS server | Path parameters; UTF-8 text read; `head`/`tail` rules; `dryRun` for edits (recommended) | `sortBy`, `excludePatterns`, media reads, batch reads |
| Agents SDK | Deployment choice among hosted/local/MCP/function tools | `ApplyPatchTool` vs shell vs MCP |

**Nice-to-have** examples from the same sources: **`directory_tree`** for structure snapshots, **`read_multiple_files`** for batching, **`list_directory_with_sizes`**, and **tool search / small namespaces** in Agents SDK to limit schema token load ([filesystem server README](https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/filesystem/README.md), [OpenAI Agents SDK — Tools](https://openai.github.io/openai-agents-python/tools/)).

### 3. Return shapes, limits, and pagination

- **Tool listing:** Opaque **`cursor`** / **`nextCursor`**; page size server-defined; clients must not parse or persist cursors across sessions ([MCP pagination](https://modelcontextprotocol.io/specification/2025-03-26/server/utilities/pagination)).
- **File reads:** **`head`** OR **`tail`** (not both) for line windows; default README wording still allows full file read—bounded reads are **opt-in** ([filesystem server README](https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/filesystem/README.md)).
- **Batch reads:** Partial success—failed paths do not abort the whole batch ([filesystem server README](https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/filesystem/README.md)).
- **Numeric caps:** Not specified in the MCP tools spec or README excerpts reviewed; xagent#213 suggests **order-of-magnitude** file read bounds and unified filters as **non-normative** ([xagent#213](https://github.com/xorbitsai/xagent/issues/213)).

### 4. Errors and edge cases for model recovery

- **Protocol vs tool:** Use JSON-RPC errors for bad tool names/args/cursors; use **`isError: true`** for logical failures (rate limits, FS errors as “soft” failures) ([MCP server tools spec](https://modelcontextprotocol.io/specification/2025-03-26/server/tools)).
- **Paths:** Roots + server allowlists + **`list_allowed_directories`** reduce ambiguous paths ([MCP client roots](https://modelcontextprotocol.io/specification/2025-06-18/client/roots), [filesystem server README](https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/filesystem/README.md)).
- **Security section:** Servers **MUST** validate inputs, enforce access controls, and **sanitize outputs** ([MCP server tools spec](https://modelcontextprotocol.io/specification/2025-03-26/server/tools)).

### 5. Diff, patch, and replace

- **OpenAI Agents SDK:** Updates are **diff-centric** in the `ApplyPatchTool` model; **`ApplyPatchEditor`** is **required** to apply operations on disk ([OpenAI Agents SDK — Tools](https://openai.github.io/openai-agents-python/tools/), [Editor reference](https://openai.github.io/openai-agents-python/ref/editor/)).
- **MCP reference server:** **`edit_file`** is **`oldText`/`newText`**, not unified-diff input; **dry run first** is documented best practice; MCP **does not** mandate a unified-diff tool at protocol level ([filesystem server README](https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/filesystem/README.md)).

### 6. Go `os.Root`, `fs.FS`, and path escape

- Use **`os.OpenRoot` / `Root` methods** or **`os.OpenInRoot`** for untrusted path components instead of **`filepath.Join` + `os.Open`** when confinement matters ([go.dev/blog/osroot](https://go.dev/blog/osroot), [pkg.go.dev/os#Root](https://pkg.go.dev/os#Root)).
- Expose **`fs.FS` via `Root.FS()`** rather than **`DirFS`** when symlink escape is in scope ([pkg.go.dev/os#DirFS](https://pkg.go.dev/os#DirFS)).
- **`filepath.IsLocal` / `Localize`:** lexical validation; **do not** replace **`Root`** for symlink/attacker threat models ([pkg.go.dev/path/filepath#IsLocal](https://pkg.go.dev/path/filepath#IsLocal)).
- **Platform caveats** (Unix `openat`, Windows handles, **WASI**, **`GOOS=js` TOCTOU**, **bind mounts** not blocked like symlinks, **`Root.Symlink`** not validating `oldname`) are documented on **pkg.go.dev** and the **osroot blog** ([go.dev/blog/osroot](https://go.dev/blog/osroot), [pkg.go.dev/os#Root.Symlink](https://pkg.go.dev/os#Root.Symlink)).
- **Pin Go ≥ 1.24.3** for **CVE-2025-22873** fix scope described in **golang-announce** ([golang-announce](https://groups.google.com/g/golang-announce/c/UZoIkUT367A)).
- **#67002** tracks additional “safer open” work beyond 1.24 **`os.Root`** ([issue #67002](https://go.dev/issue/67002)).

### 7. Testing and validation

- **Table-driven tests:** `..` variants, absolute paths, Windows quirks, in-root `..` that stays inside root, symlinks inside vs outside root ([go.dev/blog/osroot](https://go.dev/blog/osroot), [OWASP Path Traversal](https://owasp.org/www-community/attacks/Path_Traversal) as **seed ideas** for fuzzing).
- **Fuzzing:** Official **`FuzzXxx`** with **`string`/`[]byte`**; **`testdata/fuzz`** corpora ([go.dev/doc/security/fuzz](https://go.dev/doc/security/fuzz/)).
- **Encoding:** Define behavior for **invalid UTF-8** on text tools; round-trip **bytes** where writes exist ([filesystem server README](https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/filesystem/README.md) context in T5 notes).
- **MCP contracts:** Assert **`tools/list`** shape and pagination; **`tools/call`** content + **`isError`**; invalid inputs → correct error channel ([MCP server tools spec](https://modelcontextprotocol.io/specification/2025-03-26/server/tools)).
- **Static analysis:** **gosec** issue **#1297** / PR **#1386** direction—**G304**-style remediation hints for **`os.Root`** ([gosec#1297](https://github.com/securego/gosec/issues/1297)); verify against **current gosec** release when pinning CI (**inference**: pin version in CI).

## Source list

- https://modelcontextprotocol.io/specification/2025-03-26/server/tools — MCP Server Tools (listing, call, `inputSchema`, `isError`, security). Accessed **2026-04-01**. Supported tool wire protocol and error split.

- https://modelcontextprotocol.io/specification/2025-03-26/server/utilities/pagination — MCP Pagination (opaque cursors, `tools/list`). Accessed **2026-04-01**. Supported pagination rules.

- https://modelcontextprotocol.io/specification/2025-06-18/client/roots — MCP Client Roots (`file://`, validation). Accessed **2026-04-01**. Supported filesystem boundary semantics.

- https://modelcontextprotocol.io/docs/develop/connect-local-servers — MCP: Connect to local servers. Accessed **2026-04-01**. Supported high-level filesystem server narrative (approval, scope).

- https://raw.githubusercontent.com/modelcontextprotocol/servers/main/src/filesystem/README.md — Reference filesystem MCP server README. Accessed **2026-04-01**. Supported tool names, parameters, annotations, `head`/`tail`, `dryRun`.

- https://www.npmjs.com/package/@modelcontextprotocol/server-filesystem — npm package entry. Accessed **2026-04-01**. Supported parity with reference server surface.

- https://openai.github.io/openai-agents-python/tools/ — OpenAI Agents SDK: Tools. Accessed **2026-04-01**. Supported `ApplyPatchTool`, tool categories, function-tool examples.

- https://openai.github.io/openai-agents-python/ref/editor/ — OpenAI Agents SDK: Editor / `ApplyPatchEditor`. Accessed **2026-04-01**. Supported diff-oriented edit protocol.

- https://raw.githubusercontent.com/openai/openai-agents-python/main/src/agents/editor.py — `editor.py` source. Accessed **2026-04-01**. Supported implementation detail for patch operations.

- https://developers.openai.com/api/docs/guides/tools — OpenAI API: Tools overview. Accessed **2026-04-01**. Supported general tools guidance (secondary to Agents SDK for patch detail per T3).

- https://github.com/xorbitsai/xagent/issues/213 — xagent: unified tool output limits discussion. Accessed **2026-04-01**. Supported non-normative industry pain points on limits/streaming.

- https://go.dev/blog/osroot — Go blog: Traversal-resistant file APIs. Accessed **2026-04-01**. Supported `os.Root` narrative, sanitization vs confinement, platform notes.

- https://go.dev/doc/go1.24 — Go 1.24 release notes. Accessed **2026-04-01**. Supported `Root` / directory-limited access summary.

- https://pkg.go.dev/os — Package `os` (`Root`, `OpenRoot`, `OpenInRoot`, `DirFS`, etc.). Accessed **2026-04-01**. Supported API semantics and symlink behavior.

- https://pkg.go.dev/path/filepath — Package `path/filepath` (`IsLocal`, `Localize`). Accessed **2026-04-01**. Supported lexical path validation limits.

- https://groups.google.com/g/golang-announce/c/UZoIkUT367A — golang-announce: Go 1.24.3 / 1.23.9 security. Accessed **2026-04-01**. Supported CVE-2025-22873 fix scope and versions.

- https://go.dev/issue/73555 — Go issue #73555 (CVE-2025-22873). Accessed **2026-04-01**. Supported technical description of parent-directory issue.

- https://go.dev/doc/devel/release — Go release history. Accessed **2026-04-01**. Supported release line context for 1.24.3.

- https://go.dev/issue/67002 — Go issue #67002 (safer file opens). Accessed **2026-04-01**. Supported roadmap beyond `os.Root`.

- https://go.dev/doc/security/fuzz/ — Go fuzzing documentation. Accessed **2026-04-01**. Supported fuzzing API and corpora.

- https://github.com/securego/gosec/issues/1297 — gosec: suggest `os.Root` (G304 context). Accessed **2026-04-01**. Supported static analysis complement.

- https://snyk.io/articles/safe-path-handling/ — Snyk: safe path handling. Accessed **2026-04-01**. Supported secondary framing (TOCTOU, prefix checks)—not normative protocol text.

- https://snyk.io/articles/preventing-path-traversal-vulnerabilities-in-mcp-server-function-handlers/ — Snyk: MCP path traversal handlers. Accessed **2026-04-01**. Supported secondary MCP handler test ideas.

- https://owasp.org/www-community/attacks/Path_Traversal — OWASP: Path Traversal. Accessed **2026-04-01**. Supported encoding/obfuscation ideas as fuzz seeds (adapted to JSON tool args per T5).

## Gaps and limitations

- **Anthropic** public documentation and **Cursor** product docs were **not** retrieved for explicit patch vs `str_replace` guidance; **no Anthropic/Cursor URLs** appear as primary evidence in **T3** (search failures / timeout per task notes).

- **MCP spec versioning:** Task notes used **2025-03-26**; a **`/specification/latest/`** path exists—incremental normative diffs were not fully diffed.

- **Numeric output caps** (max lines, max bytes) are **not** standardized in the MCP spec + reference README excerpts; **xagent#213** is **one project’s issue**, not a standard.

- **Reference server README** does not document **full-text in-file grep** as a separate tool; **`search_files`** is **glob-style** path discovery in the README reviewed (**T1**).

- **gosec PR #1386** behavior should be verified against **current gosec release notes** when locking CI rules (**T5**).

- **Secondary sources** (Snyk, OWASP) are useful for test design but are **not** substitutes for Go or MCP primary text.

## Confidence

- **High** — MCP tool listing/call/`isError`, pagination cursor rules, reference filesystem tool names/parameters, Go **`os.Root`** vs **`DirFS`**, **`IsLocal`** limits, **CVE-2025-22873** fix version from **golang-announce**, Agents SDK **`ApplyPatchTool`/`ApplyPatchEditor`** requirement.

- **Medium** — “Minimal” tool set for all coding agents (**inference** bridges gaps); **OpenAI tools overview** depth vs Agents SDK; **pkg.go.dev** method availability across **1.24 vs 1.25** (verify against your `go` directive).

- **Low** — Universal truncation marker strings (not in spec/README); **full** cross-host consensus on patch vs replace without Anthropic/Cursor primary docs.

## Suggested follow-ups

- Retrieve **Anthropic** and **Cursor** primary documentation on **`apply_patch`**, **`search_replace`**, or equivalent, to close the **T3** gap.

- Diff **MCP `latest`** vs **2025-03-26** for tools/pagination if normative wording must be pinned.

- Inspect **reference server source** or changelog for **numeric limits** (max lines, size caps) if implementation parity is required.

- Confirm **gosec** release containing **#1386** remediation and enable in CI with a pinned version.
