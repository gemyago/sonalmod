## ADDED Requirements

### Requirement: ACP stdio execution uses a single internal boundary
The system SHALL keep generic ACP stdio request mapping, launch execution, result translation, and session recording behind one ACP-focused internal boundary instead of splitting those concerns across sibling generic internal packages.

#### Scenario: Standard run delegates through one ACP boundary
- **WHEN** a standard run request selects a profile whose execution mode is `acp-stdio`
- **THEN** the standard agent-run path delegates ACP request mapping, command execution, result translation, and session recording through one ACP-focused internal boundary

#### Scenario: Generic ACP internals are not split across sibling packages
- **WHEN** maintainers inspect the generic ACP stdio request mapper, executor, result, and session-recording code
- **THEN** those generic ACP concepts are grouped under the ACP stdio internal boundary instead of spread across sibling generic packages

### Requirement: Standard run profile-selection failures use stable public problem details
The system SHALL map profile-selection and execution-mode dispatch failures to stable public problem responses that do not expose wrapped internal dependency or implementation error details.

#### Scenario: Missing selected profile returns stable not-found detail
- **WHEN** a client sends a standard run request with a `profileName` that does not identify a saved profile
- **THEN** the system responds with a not-found problem detail indicating that the agent profile was not found without exposing lower-level wrapped error text

#### Scenario: Invalid or unsupported profile selection returns stable bad-request detail
- **WHEN** a client sends a standard run request whose selected profile is invalid for dispatch, including unsupported execution mode
- **THEN** the system responds with a bad-request problem detail for invalid profile selection without exposing lower-level wrapped error text

#### Scenario: Internal dispatch failure returns generic run-failed detail
- **WHEN** profile lookup or execution setup fails for an internal execution reason
- **THEN** the system responds with the standard agent-run failure problem detail instead of returning raw wrapped internal error text
