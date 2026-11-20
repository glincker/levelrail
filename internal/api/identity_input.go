package api

import (
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/identity"
)

// cleanNameInput validates a user-supplied display name and answers 400 itself on failure.
func cleanNameInput(w http.ResponseWriter, label, s string) (string, bool) {
	n, err := identity.ValidateName(label, s)
	if err != nil {
		writeError(w, http.StatusBadRequest, identityMessage(err))
		return "", false
	}
	return n, true
}

// cleanEmailInput validates and canonicalizes a user-supplied email or username and answers 400 itself on failure.
func cleanEmailInput(w http.ResponseWriter, s string) (string, bool) {
	n, err := identity.ValidateEmail(s)
	if err != nil {
		writeError(w, http.StatusBadRequest, identityMessage(err))
		return "", false
	}
	return n, true
}

// identityMessage drops the sentinel prefix so clients see only the reason.
func identityMessage(err error) string {
	return strings.TrimPrefix(err.Error(), identity.ErrInvalid.Error()+": ")
}
