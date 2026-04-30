## MODIFIED Requirements

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

#### Scenario: ACP stdio profile uses ACP-specific profile runner
- **WHEN** a standard run selects a profile whose execution mode is `acp-stdio`
- **THEN** the system executes the run through an internal ACP-specific profile runner that launches the configured ACP command and owns ACP session recorder initialization

#### Scenario: ACP stdio model override is ignored
- **WHEN** a standard run selects a profile whose execution mode is `acp-stdio` and also includes request-level `model`
- **THEN** the system still executes the run through the configured ACP stdio command without changing the ACP process model selection behavior

#### Scenario: ACP stdio launch failure uses standard stream error
- **WHEN** an ACP stdio command cannot be launched or returns a protocol error
- **THEN** the standard run response surfaces the failure through the standard stream error contract
