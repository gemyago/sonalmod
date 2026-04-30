## ADDED Requirements

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

## MODIFIED Requirements

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
