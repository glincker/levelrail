package api

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

const apiCanary = "canary-api-secret-4420" //nolint:gosec // synthetic test value

// Every live log line goes through writeSSEEvent, so one test pins the wire-level guarantee for
// container tails, deploy logs and last-output replays.
func TestSSELogEventMasksRegisteredSecret(t *testing.T) {
	secretmask.Add(apiCanary)
	rec := httptest.NewRecorder()
	writeSSEEvent(rec, sseLogEvent{Line: "boot DB_PASSWORD=" + apiCanary, Stream: "stdout"})
	if strings.Contains(rec.Body.String(), apiCanary) {
		t.Fatalf("SSE frame carries the secret: %q", rec.Body.String())
	}
}

// No key= prefix on purpose: the regex redactor must not be what saves this input.
func TestDeployErrorMessageMasksRegisteredSecret(t *testing.T) {
	secretmask.Add(apiCanary)
	err := errors.New("build failed: host " + apiCanary + " unreachable")
	if got := deployErrorMessage(err); strings.Contains(got, apiCanary) {
		t.Errorf("stored deploy error carries the secret: %q", got)
	}
}
