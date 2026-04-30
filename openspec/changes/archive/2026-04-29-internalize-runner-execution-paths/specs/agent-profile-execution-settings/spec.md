## ADDED Requirements

### Requirement: Standard run path orchestration stays internal
The system SHALL keep standard run path selection and mode-specific run setup inside runtime internal code, while the public `agent.Runner` surface remains a thin orchestrator over exported dependencies and helpers.

#### Scenario: Direct built-in runs delegate through an internal execution runner
- **WHEN** a standard run request omits `profileName`
- **THEN** the public runner delegates direct-run execution-path selection and built-in run preparation to an internal runtime runner instead of owning mode-specific branching itself

#### Scenario: Profile-backed runs delegate through an internal execution runner
- **WHEN** a standard run request includes `profileName`
- **THEN** the public runner delegates profile lookup, execution-mode dispatch, and effective model resolution to an internal runtime runner instead of performing those steps in the public package

#### Scenario: ACP stdio remains a delegated internal execution path
- **WHEN** the internal runtime runner resolves a profile whose execution mode is `acp-stdio`
- **THEN** it delegates execution to ACP-specific internal code while keeping execution-path selection outside the public runner layer
