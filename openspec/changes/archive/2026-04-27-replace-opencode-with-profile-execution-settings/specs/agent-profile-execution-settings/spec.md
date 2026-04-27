## ADDED Requirements

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
The system SHALL use `executionSettings.defaultModel` as the model for profiles whose execution mode is omitted or `regular`.

#### Scenario: Regular profile stores default model
- **WHEN** a regular profile is saved with `defaultModel` set to a fully-qualified provider model
- **THEN** subsequent runs for that profile use that model for the built-in runner

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
The system SHALL require standard agent run requests to identify the agent profile used for execution.

#### Scenario: Start run selects profile
- **WHEN** a client starts a new run with `POST /agent-runs` and a valid `profileName`
- **THEN** the system loads that profile and uses its execution settings to execute the run

#### Scenario: Continue run selects profile
- **WHEN** a client continues a session with `POST /sessions/{sessionId}/agent-runs` and a valid `profileName`
- **THEN** the system loads that profile and uses its execution settings to execute the run in the requested session

#### Scenario: Missing profile selection is rejected
- **WHEN** a client sends a standard run request without `profileName`
- **THEN** the system rejects the request with a validation error

#### Scenario: Missing selected profile is rejected
- **WHEN** a client sends a standard run request with a `profileName` that does not identify a saved profile
- **THEN** the system rejects the request with a not-found error

### Requirement: Standard runs dispatch by profile execution mode
The system SHALL execute standard agent runs through the selected profile's execution mode.

#### Scenario: Regular profile uses built-in runner
- **WHEN** a standard run selects a profile whose execution mode is omitted or `regular`
- **THEN** the system executes the run through the built-in agent runner using the profile's `defaultModel`

#### Scenario: ACP stdio profile uses internal ACP executor
- **WHEN** a standard run selects a profile whose execution mode is `acp-stdio`
- **THEN** the system launches the configured ACP command through an internal executor and streams the result through the standard SSE run contract

#### Scenario: ACP stdio launch failure uses standard stream error
- **WHEN** an ACP stdio command cannot be launched or returns a protocol error
- **THEN** the standard run response surfaces the failure through the standard stream error contract

### Requirement: Standard session behavior is preserved for ACP stdio runs
The system SHALL preserve standard session identity, streaming, and read-back behavior for ACP stdio runs.

#### Scenario: ACP stdio run returns standard session-bound stream
- **WHEN** a client starts an ACP stdio profile run
- **THEN** the stream begins with the server-assigned Sonalmod `sessionBound` event and ends with a `done` event

#### Scenario: ACP stdio run history can be read
- **WHEN** an ACP stdio profile run has completed successfully
- **THEN** `GET /sessions/{sessionId}` replays the session events through the standard session read stream

### Requirement: OpenCode is not exposed as a public runtime surface
The system SHALL keep OpenCode-specific implementation details internal, except that users may configure an ACP stdio command whose executable is OpenCode.

#### Scenario: OpenCode endpoints are removed
- **WHEN** clients inspect the runtime OpenAPI contract
- **THEN** the contract does not expose `/opencode-bindings`, `/opencode-bindings/{bindingName}`, or `/opencode-launches` paths

#### Scenario: OpenCode public aliases are removed
- **WHEN** Go consumers import the public `runtime/agent` package
- **THEN** they cannot construct or depend on OpenCode binding services or OpenCode launchers through exported public APIs

#### Scenario: Runtime HTTP handler does not require OpenCode dependencies
- **WHEN** Go consumers construct `runtime/httpapi.NewHandler`
- **THEN** they are not required to provide OpenCode binding services or OpenCode launcher dependencies

#### Scenario: Generic internals are not named after OpenCode
- **WHEN** maintainers inspect runtime domain, dispatcher, persistence, handler wiring, and public package boundaries
- **THEN** surviving generic ACP stdio concepts use generic names instead of OpenCode-specific names
