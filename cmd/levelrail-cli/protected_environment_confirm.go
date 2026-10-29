package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// A 409 from these endpoints only ever means the protected-environment gate, so the status alone identifies it.
func protectedEnvironmentError(err error) (*apiclient.APIError, bool) {
	var apiErr *apiclient.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict && strings.Contains(apiErr.Message, "set confirm") {
		return apiErr, true
	}
	return nil, false
}

// Fails closed on EOF so a script with no terminal never proceeds or hangs.
func resolveProtectedEnvironmentConfirmation(message string, stdin io.Reader, stderr io.Writer) (bool, error) {
	_, _ = fmt.Fprintf(stderr, "%s\nType \"yes\" to proceed: ", message)
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		return false, newValidationError("read confirmation: %v", err)
	}
	return strings.TrimSpace(line) == "yes", nil
}

// PendingApproval can come back even after a confirmed retry, so callers must check it.
func confirmProtectedEnvironment(confirm bool, stdin io.Reader, stderr io.Writer, attempt func(confirm bool) (deployTriggerResult, error)) (deployTriggerResult, error) {
	result, err := attempt(confirm)
	if confirm || err == nil {
		return result, err
	}
	apiErr, ok := protectedEnvironmentError(err)
	if !ok {
		return result, err
	}
	confirmed, cerr := resolveProtectedEnvironmentConfirmation(apiErr.Message, stdin, stderr)
	if cerr != nil {
		return result, cerr
	}
	if !confirmed {
		return result, err
	}
	return attempt(true)
}
