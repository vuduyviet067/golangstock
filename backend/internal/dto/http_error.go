package dto

type HTTPError struct {
	StatusCode int
	// Code is the UPPER_SNAKE error code written in the response body. An
	// empty Code is replaced by the general code of StatusCode.
	Code    string
	Message string
	Fields  map[string]string
	Cause   error
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
