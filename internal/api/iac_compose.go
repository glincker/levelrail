package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/iac"
)

// iacComposeKey marks a compose deploy sent by the IaC executor through the
// loopback doer; only in-process code can attach it to a request context.
type iacComposeKey struct{}

type iacComposeInput struct {
	env map[string]string
}

// strictIaCCompose re-validates a referenced compose file on the server, so a
// plan built elsewhere is never trusted. Refusals carry codes and lines only.
func strictIaCCompose(w http.ResponseWriter, body []byte, env map[string]string) bool {
	if !iac.ComposeRefsEnabled() {
		writeCodedError(w, http.StatusConflict, iac.CodeUnavailable, "compose references are not enabled (setting iac.compose_refs)")
		return false
	}
	declared := make(map[string]string, len(env))
	for k := range env {
		declared[k] = "set"
	}
	if _, err := iac.StrictComposeCheck(body, "compose", declared); err != nil {
		issues := iac.ComposeStrictIssues("compose", err)
		writeCodedError(w, http.StatusBadRequest, issues[0].Code, issues[0].String())
		return false
	}
	return true
}
