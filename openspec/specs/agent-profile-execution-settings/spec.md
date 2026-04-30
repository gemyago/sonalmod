# agent-profile-execution-settings Specification

## Purpose
TBD - created by archiving change replace-opencode-with-profile-execution-settings. Update Purpose after archive.
## Requirements
### Requirement: Agent profiles declare optional execution mode
The system SHALL allow every saved agent profile to declare execution settings with an optional mode. Omitted mode SHALL be treated as `regular`; supported explicit modes SHALL be `regular` and `acp-stdio`.

#### Scenario: Omitted mode defaults to regular
- **WHEN** a client creates or updates an agent profile without `executionSettings.mode` and with a non-empty `defaultModel`
- **THEN** the profile is saved with regular execution settings

#### Scenario: Explicit regular profile is accepted
- **WHEN** a client creates or updates an agent profile with `executionSettings.mode` set to `regular` and a non-empty `defaultModel`
- **THEN** the profile is saved with regular execution settings

#### Scenario: ACP stdio profile is accepted
- **WHEN** a client creates or updates an agent profile with `executionSettings.mode` set to `acp-stdio` and valid ACP command settings
- **THEN** the profile is saved with ACP stdio execution settings

#### Scenario: Unsupported execution mode is rejected
- **WHEN** a client creates or updates an agent profile with an unsupported `executionSettings.mode`
- **THEN** the system rejects the request with a validation error

### Requirement: Regular execution settings select the built-in runner model
The system SHALL use `executionSettings.defaultModel` as the default built-in runner model for profiles whose execution mode is omitted or `regular`, unless a standard run request provides a request-level `model` override.

#### Scenario: Regular profile stores default model
- **WHEN** a regular profile is saved with `defaultModel` set to a fully-qualified provider model
- **THEN** subsequent regular runs for that profile use that model when no request-level override is provided

#### Scenario: Request model overrides regular profile default
- **WHEN** a standard run request selects a profile whose execution mode is omitted or `regular` and includes a request-level `model`
- **THEN** the built-in runner executes the run with the request-level `model` instead of the profile's `defaultModel`

#### Scenario: Regular profile without model is rejected
- **WHEN** a client creates or updates a regular profile without a non-empty `defaultModel`
- **THEN** the system rejects the request with a validation error

### Requirement: ACP stdio execution settings store process configuration
The system SHALL store ACP command, arguments, and optional working directory in `AgentProfile.executionSettings` for profiles whose execution mode is `acp-stdio`.

#### Scenario: ACP stdio settings contain launch defaults
- **WHEN** an ACP stdio profile is saved
- **THEN** the saved profile includes the command executable, command arguments, and optional working directory needed to launch the external agent over stdio

#### Scenario: Invalid ACP stdio command is rejected
- **WHEN** a client creates or updates an ACP stdio profile with an empty command, empty argument, duplicate argument, or control characters
- **THEN** the system rejects the request with a validation error

#### Scenario: ACP stdio settings do not require default model
- **WHEN** a client creates or updates an ACP stdio profile without `defaultModel`
- **THEN** the profile is valid when the ACP command settings are valid

### Requirement: Standard agent runs select a profile
The system SHALL allow standard agent run requests to optionally identify the agent profile used to resolve execution defaults or alternate execution mode.

#### Scenario: Start run selects profile
- **WHEN** a client starts a new run with `POST /agent-runs` and a valid `profileName`
- **THEN** the system loads that profile and uses its execution settings to execute the run

#### Scenario: Continue run selects profile
- **WHEN** a client continues a session with `POST /sessions/{sessionId}/agent-runs` and a valid `profileName`
- **THEN** the system loads that profile and uses its execution settings to execute the run in the requested session

#### Scenario: Standard run without profile uses request model
- **WHEN** a client sends a standard run request without `profileName` and with a valid request-level `model`
- **THEN** the system executes the run through the built-in runner using that request-level `model`

#### Scenario: Missing selected profile is rejected
- **WHEN** a client sends a standard run request with a `profileName` that does not identify a saved profile
- **THEN** the system rejects the request with a not-found error

#### Scenario: Missing profile and model is rejected
- **WHEN** a client sends a standard run request without `profileName` and without a request-level `model`
- **THEN** the system rejects the request with a validation error

### Requirement: Standard agent run path owns built-in profile execution
The system SHALL execute direct runs and regular profile-backed runs through the standard internal agent run path. A separate generic profile execution wrapper MUST NOT be required for those execution modes.

#### Scenario: Direct run uses standard agent run path
- **WHEN** a standard run request omits `profileName` and includes a valid request-level `model`
- **THEN** the system executes the run through the standard internal agent run path using that request-level `model`

#### Scenario: Regular profile run uses standard agent run path
- **WHEN** a standard run request selects a profile whose execution mode is omitted or `regular`
- **THEN** the system resolves the effective model and executes the run through the standard internal agent run path using the selected profile identity and instructions

#### Scenario: ACP stdio remains mode-specific
- **WHEN** a standard run request selects a profile whose execution mode is `acp-stdio`
- **THEN** the standard internal agent run path delegates only ACP-specific execution, result mapping, and session recording to ACP-specific internal logic

### Requirement: Standard runs dispatch by profile execution mode
The system SHALL dispatch standard agent runs according to the optional `profileName` and the effective built-in runner model, using the standard internal agent run path for direct and regular profile execution and delegating only ACP-specific execution to ACP-specific internals.

#### Scenario: Regular run without profile uses request model
- **WHEN** a standard run request omits `profileName` and includes a valid request-level `model`
- **THEN** the system executes the run through the standard internal agent run path using that request-level `model`

#### Scenario: Regular profile uses built-in runner default when no override is provided
- **WHEN** a standard run selects a profile whose execution mode is omitted or `regular` and omits request-level `model`
- **THEN** the system executes the run through the standard internal agent run path using the profile's `defaultModel`

#### Scenario: Regular profile uses request-level model override
- **WHEN** a standard run selects a profile whose execution mode is omitted or `regular` and includes request-level `model`
- **THEN** the system executes the run through the standard internal agent run path using the request-level `model`

#### Scenario: ACP stdio profile delegates only ACP-specific execution
- **WHEN** a standard run selects a profile whose execution mode is `acp-stdio`
- **THEN** the standard internal agent run path delegates ACP command execution, ACP result mapping, and ACP session recording to ACP-specific internal logic and returns the standard SSE run contract

#### Scenario: ACP stdio model override is ignored
- **WHEN** a standard run selects a profile whose execution mode is `acp-stdio` and also includes request-level `model`
- **THEN** the system still executes the run through the configured ACP stdio command without changing the ACP process model selection behavior

#### Scenario: ACP stdio launch failure uses standard stream error
- **WHEN** an ACP stdio command cannot be launched or returns a protocol error
- **THEN** the standard run response surfaces the failure through the standard stream error contract

### Requirement: OpenCode is not exposed as a public runtime surface
The system SHALL keep OpenCode-specific implementation details and profile-dispatch plumbing internal, except that users may configure an ACP stdio command whose executable is OpenCode.

#### Scenario: OpenCode endpoints are removed
- **WHEN** clients inspect the runtime OpenAPI contract
- **THEN** the contract does not expose `/opencode-bindings`, `/opencode-bindings/{bindingName}`, or `/opencode-launches` paths

#### Scenario: OpenCode public aliases are removed
- **WHEN** Go consumers import the public `runtime/agent` package
- **THEN** they cannot construct or depend on OpenCode binding services or OpenCode launchers through exported public APIs

#### Scenario: Runtime HTTP handler does not require OpenCode dependencies
- **WHEN** Go consumers construct `runtime/httpapi.NewHandler`
- **THEN** they are not required to provide OpenCode binding services or OpenCode launcher dependencies

#### Scenario: Runtime public APIs do not expose profile dispatch internals
- **WHEN** Go consumers construct the public runtime runner and HTTP handler
- **THEN** they are not required to construct, pass, or depend on an exported `ProfileRunDispatcher`-style abstraction

#### Scenario: Generic internals are not named after OpenCode
- **WHEN** maintainers inspect runtime domain, dispatcher, persistence, handler wiring, and public package boundaries
- **THEN** surviving generic ACP stdio concepts use generic names instead of OpenCode-specific names

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
