package awsx

import (
	"errors"
	"fmt"
	"strings"
)

// Class groups AWS error codes into the few outcomes callers act on.
type Class string

// Error classes.
const (
	ClassThrottle Class = "throttle"
	ClassAuth     Class = "auth"
	ClassNotFound Class = "not_found"
	ClassConflict Class = "conflict"
	ClassInvalid  Class = "invalid"
	ClassServer   Class = "server"
	ClassOther    Class = "other"
)

// Error is one AWS service error. Error() omits Message on purpose, because
// some services echo request values back.
type Error struct {
	Service string
	Op      string
	Code    string
	Message string
	Status  int
	Class   Class
}

func (e *Error) Error() string {
	return fmt.Sprintf("awsx: %s %s: %s (%d)", e.Service, e.Op, e.Code, e.Status)
}

// CodeOf returns the AWS error code of err, or empty.
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// ClassOf returns the class of err, or ClassOther.
func ClassOf(err error) Class {
	var e *Error
	if errors.As(err, &e) {
		return e.Class
	}
	return ClassOther
}

var throttleCodes = map[string]bool{
	"Throttling": true, "ThrottlingException": true, "ThrottledException": true,
	"RequestLimitExceeded": true, "TooManyRequestsException": true,
	"ProvisionedThroughputExceededException": true, "RequestThrottled": true,
	"SlowDown": true, "PriorRequestNotComplete": true, "LimitExceededException": true,
}

var authCodes = map[string]bool{
	"AccessDenied": true, "AccessDeniedException": true, "UnauthorizedOperation": true,
	"UnrecognizedClientException": true, "InvalidClientTokenId": true, "ExpiredToken": true,
	"ExpiredTokenException": true, "SignatureDoesNotMatch": true, "AuthFailure": true,
	"InvalidSignatureException": true, "MissingAuthenticationToken": true, "OptInRequired": true,
}

// classify maps a code and HTTP status to a Class.
func classify(code string, status int) Class {
	switch {
	case throttleCodes[code] || status == 429:
		return ClassThrottle
	case authCodes[code] || status == 401 || status == 403:
		return ClassAuth
	case strings.Contains(code, "NotFound") || status == 404:
		return ClassNotFound
	case strings.Contains(code, "AlreadyExists") || strings.Contains(code, "Conflict") ||
		strings.Contains(code, "InUse") || strings.Contains(code, "ResourceExists"):
		return ClassConflict
	case status >= 500 || code == "InternalFailure" || code == "ServiceUnavailable" || code == "InternalServerError":
		return ClassServer
	case strings.HasPrefix(code, "Validation") || strings.Contains(code, "Invalid") || status == 400:
		return ClassInvalid
	}
	return ClassOther
}

func retryable(c Class) bool { return c == ClassThrottle || c == ClassServer }
