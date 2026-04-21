# Implementation Summary: Workspace identifiers instead of absolute roots in `tools/workspacefs`

**Plan:** [plan-workspace-identifiers.md](./plan-workspace-identifiers.md)

## Overview

The module now registers workspaces via `WithWorkspaces` and `WorkspaceConfig` (identifier, description, internal path). The service indexes `os.Root` instances by workspace id; tools require a `workspace` field on every request; introspection is `workspacefs_list_workspaces` (identifier + description only). Tests and docs were updated end-to-end, with explicit checks that model-visible errors and responses do not leak configured host paths.

## Tasks

### Task 1.1: identifier-based workspace configuration in registration

Replaced `WithRoots([]string)` with `WithWorkspaces([]WorkspaceConfig)` and `WorkspaceConfig` (identifier, description, path), added validation and `RegisterTools` wiring that resolves paths to the internal service, and added tests for failure cases and successful single- and multi-workspace registration.

### Task 1.2: service workspace-indexed root selection (TDD)

The service was refactored from parallel path-based roots to ordered workspace entries (identifier, description, internal path, `os.Root`) with `pickWorkspace` validation and `ListWorkspaces` returning descriptors; registration and tests were updated so tools use workspace ids in the JSON `root` field until Task 2.2.

### Task 2.1: replace introspection tool with `workspacefs_list_workspaces` (TDD)

Replaced `workspacefs_list_allowed_directories` with `workspacefs_list_workspaces`, wired to `Service.ListWorkspaces()` and a new `ListWorkspacesResponse` (identifier + description, no host paths). Tests were updated (`TestListWorkspacesTool`, service tests); `ListAllowedDirectories` and its response type were removed.

### Task 2.2: replace request field `root` with required `workspace` across all tool models (TDD)

Request structs now use a required `workspace` field instead of `root`; handlers, tests, JSON round-trip checks, and agent tool docs were updated. `DirectoryTreeResponse.Root` (the tree node) was left unchanged.

### Task 3.1: operation paths use workspace selection; behavior preserved (TDD)

No production changes were needed because every operation already calls `pickWorkspace` before path work. Tests were extended so each tool surface explicitly covers empty or missing `workspace`, unknown identifiers where applicable, while existing success and edge-case coverage remains.

### Task 3.2: no absolute-path leakage in model-visible responses and errors (TDD)

Registration and service paths now return stable messages that cite workspace identifiers only—no `%w` of OS errors that embedded configured directories. Regression tests cover registration, service, and agent tool surfaces so model-visible errors never leak host paths.

### Task 4.1: module docs and conventions (workspace identifiers)

Updated `AGENTS.md` and `doc.go` for the workspace-identifier contract and `os.Root` confinement; aligned internal docstrings and path/glob validation messages to workspace-relative wording and trimmed an obsolete comment in `skeleton.go`.

## Deviations & notes

- **Task 1.2 → 2.2:** Between 1.2 and 2.2, tools temporarily used workspace ids in the JSON `root` field; Task 2.2 completed the rename to `workspace`.
- **Task 3.1:** Implementation was already satisfied by `pickWorkspace`; the task focused on extending test coverage.

## Completion

- Lint: ✓
- Type check: ✓
- Tests: ✓
