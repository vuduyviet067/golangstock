package dto

// HTTPError encapsulates an HTTP status code, a safe public client message,
// optional field-level validation errors, and the underlying root cause error.
//
// It implements error and supports errors.Unwrap. Mapping errors to an
// HTTPError and writing it to a response remain in the HTTP handler package.
type HTTPError struct {
	StatusCode int
	Message    string
	Fields     map[string]string // nil if not a field-level validation error
	Cause      error
}

func (e *HTTPError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *HTTPError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
