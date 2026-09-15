package ard

import (
	"errors"
	"fmt"
	"net/http"
)

// Error codes of appendix B.
const (
	CodeInvalidArgument   = "INVALID_ARGUMENT"
	CodeUnauthenticated   = "UNAUTHENTICATED"
	CodeNotFound          = "NOT_FOUND"
	CodeRateLimitExceeded = "RATE_LIMIT_EXCEEDED"
	CodeInternalError     = "INTERNAL_ERROR"
	CodeNotImplemented    = "NOT_IMPLEMENTED"
)

// ErrNotImplemented reports that a registry does not implement an optional endpoint.
// Section 5.3.3 defines the 501 answer for Explore.
var ErrNotImplemented = errors.New("ard: endpoint not implemented by this registry")

// APIError is a registry answer that is not a success. See appendix B.
type APIError struct {
	HTTPStatus int
	Code       string
	Message    string
}

// NewAPIError builds an error for one HTTP status. The code follows appendix B when the
// caller gives none.
func NewAPIError(status int, code, message string) *APIError {
	if code == "" {
		code = CodeForStatus(status)
	}
	return &APIError{HTTPStatus: status, Code: code, Message: message}
}

func (e *APIError) Error() string {
	return fmt.Sprintf("ard: %s (HTTP %d): %s", e.Code, e.HTTPStatus, e.Message)
}

// Unwrap reports ErrNotImplemented for a 501 answer, so that errors.Is finds it.
func (e *APIError) Unwrap() error {
	if e.HTTPStatus == http.StatusNotImplemented {
		return ErrNotImplemented
	}
	return nil
}

// Body gives the wire form of the error.
func (e *APIError) Body() ErrorBody {
	return ErrorBody{ErrorCode: e.Code, Message: e.Message}
}

// CodeForStatus maps an HTTP status to the error code of appendix B.
func CodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return CodeInvalidArgument
	case http.StatusUnauthorized:
		return CodeUnauthenticated
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusTooManyRequests:
		return CodeRateLimitExceeded
	case http.StatusNotImplemented:
		return CodeNotImplemented
	default:
		return CodeInternalError
	}
}

// StatusForCode maps an error code of appendix B to its HTTP status.
func StatusForCode(code string) int {
	switch code {
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeUnauthenticated:
		return http.StatusUnauthorized
	case CodeNotFound:
		return http.StatusNotFound
	case CodeRateLimitExceeded:
		return http.StatusTooManyRequests
	case CodeNotImplemented:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}
