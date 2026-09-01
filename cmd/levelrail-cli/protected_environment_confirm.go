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

// protectedEnvironmentError reports whether err is the 409
// internal/api's requireEnvironmentConfirmation/environmentNeedsConfirmation
// return when a deploy, rollback, or promote targets a protected
// environment with confirm not already true: the only 409 those three
// endpoints ever return, so the status code alone disambiguates it from
// a validation or not-found failure.
func protectedEnvironmentError(err error) (*apiclient.APIError, bool) {
	var apiErr *apiclient.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
		return apiErr, true
	}
	return nil, false
}

// resolveProtectedEnvironmentConfirmation prompts on stderr and reads
// one line from stdin, mirroring resolveRestoreConfirmation's own
// EOF-refuses-cleanly behavior (backups_restore.go) for a script with no
// terminal attached: it must fail closed, not hang or silently proceed.
func resolveProtectedEnvironmentConfirmation(message string, stdin io.Reader, stderr io.Writer) (bool, error) {
	_, _ = fmt.Fprintf(stderr, "%s\nType \"yes\" to proceed: ", message)
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		return false, newValidationError("read confirmation: %v", err)
	}
	return strings.TrimSpace(line) == "yes", nil
}

// confirmProtectedEnvironment runs attempt(confirm) once. If it fails
// because the target is tagged with a protected environment and confirm
// was already false, it prompts interactively and retries once with
// confirm=true when the operator confirms; otherwise it returns
// attempt's own result and error unchanged. Shared by "apps
// deploy"/"apps rollback"/"apps promote", which all hit this same
// protected-environment gate through a different underlying call.
func confirmProtectedEnvironment(confirm bool, stdin io.Reader, stderr io.Writer, attempt func(confirm bool) (appResource, error)) (appResource, error) {
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
