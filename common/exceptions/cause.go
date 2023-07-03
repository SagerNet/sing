package exceptions

type causeError struct {
	message string
	cause   error
}

func (e *causeError) Error() string {
	return e.message + ": " + e.cause.Error()
}

func (e *causeError) Unwrap() error {
	return e.cause
}
