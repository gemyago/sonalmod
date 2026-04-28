package profilerun

import "fmt"

// ErrorKind classifies profile run failures.
type ErrorKind string

const (
	// ErrorKindValidation indicates invalid profile run input.
	ErrorKindValidation ErrorKind = "validation"
	// ErrorKindNotFound indicates the requested profile does not exist.
	ErrorKindNotFound ErrorKind = "not-found"
	// ErrorKindUnsupported indicates the selected execution mode is not wired yet.
	ErrorKindUnsupported ErrorKind = "unsupported"
	// ErrorKindExecution indicates a lower-level dependency failed during profile run dispatch.
	ErrorKindExecution ErrorKind = "execution"
)

// Error wraps profile run failures with a stable kind and operation.
type Error struct {
	Kind ErrorKind
	Op   string
	Err  error
}

func (e *Error) Error() string {
	return fmt.Sprintf("profile execution %s (%s): %v", e.Op, e.Kind, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}

// WrapError wraps err with a stable kind and operation.
func WrapError(kind ErrorKind, op string, err error) error {
	if err == nil {
		return nil
	}

	return &Error{
		Kind: kind,
		Op:   op,
		Err:  err,
	}
}
