package deploylog

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/secretmask"
)

const proofCanary = "canary-build-secret-7731" //nolint:gosec // synthetic test value

// Build output is read live (SSE subscribers), from the in-memory replay and from the store; none may carry a secret.
func TestBuildOutputNeverCarriesRegisteredSecret(t *testing.T) {
	secretmask.Add(proofCanary)
	st := &fakeStore{}
	r := NewRecorder(st, discardLogger())
	r.Start("dep_proof")
	_, live, unsub, ok := r.Snapshot("dep_proof")
	if !ok {
		t.Fatal("attempt not active")
	}
	defer unsub()

	r.Progress("dep_proof")(build.ProgressEvent{Log: "RUN export TOKEN=" + proofCanary, Stream: "stderr"})

	if got := (<-live).Line; strings.Contains(got, proofCanary) {
		t.Errorf("live subscriber saw the secret: %q", got)
	}
	replay, _, unsub2, _ := r.Snapshot("dep_proof")
	defer unsub2()
	for _, e := range replay {
		if strings.Contains(e.Line, proofCanary) {
			t.Errorf("in-memory replay holds the secret: %q", e.Line)
		}
	}
	r.Finish(t.Context(), "dep_proof")
	for _, e := range st.snapshot() {
		if strings.Contains(e.Message, proofCanary) {
			t.Errorf("stored build log holds the secret: %q", e.Message)
		}
	}
}
