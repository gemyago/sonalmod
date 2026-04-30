## ADDED Requirements

### Requirement: Runtime runner requires profile service
The system SHALL require an agent profiles service when constructing the runtime runner.

#### Scenario: Runner construction without profile service is rejected
- **WHEN** a caller constructs the runtime runner without `AgentProfilesService`
- **THEN** construction fails with a configuration error

#### Scenario: Runner construction with profile service is accepted
- **WHEN** a caller constructs the runtime runner with both `ProvidersConfigService` and `AgentProfilesService`
- **THEN** construction succeeds when the remaining runner configuration is valid

### Requirement: Runner owns profile-backed run orchestration
The runtime runner SHALL own standard run orchestration for direct built-in runs, regular profile-backed runs, and ACP stdio profile-backed runs. Generic profile execution wrappers MUST NOT be required for regular profile execution.

#### Scenario: Direct model run stays runner-owned
- **WHEN** a standard run request omits `profileName` and includes a valid request-level `model`
- **THEN** the runtime runner executes the run through its built-in runner path

#### Scenario: Regular profile run stays runner-owned
- **WHEN** a standard run request selects a profile whose execution mode is omitted or `regular`
- **THEN** the runtime runner resolves the effective model and executes the run through its built-in runner path using the selected profile identity and instructions

#### Scenario: ACP stdio profile delegates only ACP-specific execution
- **WHEN** a standard run request selects a profile whose execution mode is `acp-stdio`
- **THEN** the runtime runner delegates ACP command execution, ACP result mapping, and ACP session recording to ACP-specific internal logic

## MODIFIED Requirements

### Requirement: Standard runs dispatch by profile execution mode
The runtime runner SHALL dispatch standard agent runs according to the optional `profileName` and the effective built-in runner model.

#### Scenario: Regular run without profile uses request model
- **WHEN** a standard run request omits `profileName` and includes a valid request-level `model`
- **THEN** the system executes the run through the built-in agent runner using that request-level `model`

#### Scenario: Regular profile uses built-in runner default when no override is provided
- **WHEN** a standard run selects a profile whose execution mode is omitted or `regular` and omits request-level `model`
- **THEN** the runtime runner executes the run through the built-in agent runner using the profile's `defaultModel`

#### Scenario: Regular profile uses request-level model override
- **WHEN** a standard run selects a profile whose execution mode is omitted or `regular` and includes request-level `model`
- **THEN** the runtime runner executes the run through the built-in agent runner using the request-level `model`

#### Scenario: ACP stdio profile uses internal ACP executor
- **WHEN** a standard run selects a profile whose execution mode is `acp-stdio`
- **THEN** the runtime runner launches the configured ACP command through ACP-specific internal execution logic and streams the result through the standard SSE run contract

#### Scenario: ACP stdio model override is ignored
- **WHEN** a standard run selects a profile whose execution mode is `acp-stdio` and also includes request-level `model`
- **THEN** the system still executes the run through the configured ACP stdio command without changing the ACP process model selection behavior

#### Scenario: ACP stdio launch failure uses standard stream error
- **WHEN** an ACP stdio command cannot be launched or returns a protocol error
- **THEN** the standard run response surfaces the failure through the standard stream error contract
