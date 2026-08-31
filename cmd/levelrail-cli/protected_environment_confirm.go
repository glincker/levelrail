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
