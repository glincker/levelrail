package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestNewJSONError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		code     string
		exit     int
		status   int
		hint     bool
		retryFor string
	}{
		{"not found", fmt.Errorf("get app: %w", &apiError{StatusCode: 404, Message: "no such app"}), errCodeNotFound, exitAPIError, 404, true, ""},
		{"unauthorized", &apiError{StatusCode: 401, Message: "bad token"}, errCodeAuth, exitAPIError, 401, true, ""},
		{"forbidden", &apiError{StatusCode: 403, Message: "lacks deploy"}, errCodeForbidden, exitAPIError, 403, true, ""},
		{"conflict", &apiError{StatusCode: 409, Message: "protected"}, errCodeConflict, exitAPIError, 409, true, ""},
		{"precondition", &apiError{StatusCode: 412, Message: "stale"}, errCodeConflict, exitAPIError, 412, true, ""},
		{"rate limited", &apiError{StatusCode: 429, Message: "slow down", RetryAfter: 30 * time.Second}, errCodeRateLimited, exitAPIError, 429, true, "30s"},
		{"bad request", &apiError{StatusCode: 400, Message: "bad"}, errCodeInvalid, exitAPIError, 400, false, ""},
		{"server", &apiError{StatusCode: 502, Message: "boom"}, errCodeServer, exitAPIError, 502, true, ""},
		{"validation", newValidationError("--name is required"), errCodeValidation, exitValidation, 0, false, ""},
		{"network", errors.New("dial tcp: refused"), errCodeNetwork, exitNetwork, 0, true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := newJSONError(tc.err)
			if got.Code != tc.code || got.ExitCode != tc.exit || got.HTTPStatus != tc.status || got.RetryAfter != tc.retryFor || (got.Hint != "") != tc.hint {
				t.Errorf("newJSONError = %+v", got)
			}
			if got.Error != tc.err.Error() {
				t.Errorf("error message changed: %q", got.Error)
			}
		})
	}
}

func TestWriteJSONErrorKeepsErrorField(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSONError(&buf, &apiError{StatusCode: 404, Message: "gone"}); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["error"] == "" || got["code"] != errCodeNotFound || got["exit_code"] != float64(exitAPIError) || got["http_status"] != float64(404) {
		t.Errorf("got %v", got)
	}
}
