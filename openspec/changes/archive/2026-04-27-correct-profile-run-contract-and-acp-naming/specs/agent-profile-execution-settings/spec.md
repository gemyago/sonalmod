## MODIFIED Requirements

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

## ADDED Requirements

### Requirement: Selected regular profiles contribute built-in runner identity and instructions
The system SHALL use selected regular profiles as built-in runner configuration, not only as model lookup records.

#### Scenario: Selected regular profile sets built-in agent identity
- **WHEN** a standard run selects a profile whose execution mode is omitted or `regular`
- **THEN** the built-in runner uses the saved profile `name` as the agent identity for that run

#### Scenario: Selected regular profile appends profile instructions
- **WHEN** a standard run selects a profile whose execution mode is omitted or `regular`
- **THEN** the built-in runner appends the saved profile `instructions` to the system instruction chain for that run

#### Scenario: Run without selected profile has no profile-scoped instructions
- **WHEN** a standard run request omits `profileName`
- **THEN** built-in execution uses no profile-scoped instruction fragment

## MODIFIED Requirements

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

### Requirement: Standard runs dispatch by profile execution mode
The system SHALL dispatch standard agent runs according to the optional `profileName` and the effective built-in runner model.

#### Scenario: Regular run without profile uses request model
- **WHEN** a standard run request omits `profileName` and includes a valid request-level `model`
- **THEN** the system executes the run through the built-in agent runner using that request-level `model`

#### Scenario: Regular profile uses built-in runner default when no override is provided
- **WHEN** a standard run selects a profile whose execution mode is omitted or `regular` and omits request-level `model`
- **THEN** the system executes the run through the built-in agent runner using the profile's `defaultModel`

#### Scenario: Regular profile uses request-level model override
- **WHEN** a standard run selects a profile whose execution mode is omitted or `regular` and includes request-level `model`
- **THEN** the system executes the run through the built-in agent runner using the request-level `model`

#### Scenario: ACP stdio profile uses internal ACP executor
- **WHEN** a standard run selects a profile whose execution mode is `acp-stdio`
- **THEN** the system launches the configured ACP command through an internal executor and streams the result through the standard SSE run contract

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
