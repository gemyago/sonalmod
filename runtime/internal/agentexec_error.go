package internal

import "fmt"

// AgentExecErrorKind classifies agent execution dispatch failures.
type AgentExecErrorKind string

const (
	// AgentExecErrorKindValidation indicates invalid input for a dispatch operation.
	AgentExecErrorKindValidation AgentExecErrorKind = "validation"
	// AgentExecErrorKindNotFound indicates the requested profile does not exist.
	AgentExecErrorKindNotFound AgentExecErrorKind = "not-found"
	// AgentExecErrorKindUnsupported indicates the selected execution mode is not supported.
	AgentExecErrorKindUnsupported AgentExecErrorKind = "unsupported"
	// AgentExecErrorKindExecution indicates a lower-level dependency failed during dispatch.
	AgentExecErrorKindExecution AgentExecErrorKind = "execution"
)

// AgentExecError wraps agent execution dispatch failures with a stable kind and operation.
type AgentExecError struct {
	Kind AgentExecErrorKind
	Op   string
	Err  error
}

func (e *AgentExecError) Error() string {
	return fmt.Sprintf("agent execution %s (%s): %v", e.Op, e.Kind, e.Err)
}

func (e *AgentExecError) Unwrap() error {
	return e.Err
}

// WrapAgentExecError wraps err with a stable kind and operation. Returns nil when err is nil.
func WrapAgentExecError(kind AgentExecErrorKind, op string, err error) error {
	if err == nil {
		return nil
	}

	return &AgentExecError{
		Kind: kind,
		Op:   op,
		Err:  err,
	}
}
