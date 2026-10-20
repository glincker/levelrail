package main

import (
	"errors"
	"io"
	"net/http"
	"time"
)

// Machine-readable error categories in --json error output.
const (
	errCodeValidation  = "validation"
	errCodeNetwork     = "network"
	errCodeAuth        = "unauthorized"
	errCodeForbidden   = "forbidden"
	errCodeNotFound    = "not_found"
	errCodeConflict    = "conflict"
	errCodeRateLimited = "rate_limited"
	errCodeInvalid     = "invalid_request"
	errCodeServer      = "server_error"
	errCodeAPI         = "api_error"
)

// jsonError is --json mode's error shape. Error is the original field;
// the rest are additive so scripts can branch without parsing the message.
type jsonError struct {
	Error      string `json:"error"`
	Code       string `json:"code"`
	ExitCode   int    `json:"exit_code"`
	HTTPStatus int    `json:"http_status,omitempty"`
	RetryAfter string `json:"retry_after,omitempty"`
	Hint       string `json:"hint,omitempty"`
}

func errorCodeFor(status int) (code, hint string) {
	switch {
	case status == http.StatusUnauthorized:
		return errCodeAuth, "the token is missing, expired or revoked: run auth login or pass --token"
	case status == http.StatusForbidden:
		return errCodeForbidden, "the token lacks the required ability: see tokens create --abilities"
	case status == http.StatusNotFound:
		return errCodeNotFound, "check the resource name with the matching list command"
	case status == http.StatusConflict, status == http.StatusPreconditionFailed, status == http.StatusPreconditionRequired:
		return errCodeConflict, "the resource is in a state that blocks this change: read the message, fix it, retry"
	case status == http.StatusTooManyRequests:
		return errCodeRateLimited, "wait for retry_after, then retry"
	case status >= 500:
		return errCodeServer, "the control plane failed: check its logs and doctor output"
	case status >= 400:
		return errCodeInvalid, ""
	default:
		return errCodeAPI, ""
	}
}

func newJSONError(err error) jsonError {
	out := jsonError{Error: err.Error(), ExitCode: exitCodeForError(err)}
	var apiErr *apiError
	var valErr *validationError
	switch {
	case errors.As(err, &apiErr):
		out.HTTPStatus = apiErr.StatusCode
		out.Code, out.Hint = errorCodeFor(apiErr.StatusCode)
		if apiErr.RetryAfter > 0 {
			out.RetryAfter = apiErr.RetryAfter.Round(time.Second).String()
		}
	case errors.As(err, &valErr):
		out.Code = errCodeValidation
	default:
		out.Code = errCodeNetwork
		out.Hint = "could not reach the control plane: check --api-url and that it is running"
	}
	return out
}

// writeJSONError writes --json mode's error object to out. The original
// "error" field is unchanged.
func writeJSONError(out io.Writer, err error) error {
	return writeJSONValue(out, newJSONError(err))
}
