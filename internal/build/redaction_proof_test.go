package build

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

const proofCanary = "canary-build-secret-4417" //nolint:gosec // synthetic test value

// Every field a build progress event can log (step name, error, output) must be masked.
func TestBuildProgressRedactionNeverLogsRegisteredSecret(t *testing.T) {
	secretmask.Add(proofCanary)
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	progress := SlogProgress(log)

	progress(ProgressEvent{Step: "RUN export TOKEN=" + proofCanary})
	progress(ProgressEvent{Step: "RUN export TOKEN=" + proofCanary, Completed: true})
	progress(ProgressEvent{Step: "[2/4] RUN x", Error: "failed with " + proofCanary, Completed: true})
	progress(ProgressEvent{Step: "[2/4] RUN x", Log: "TOKEN=" + proofCanary, Stream: "stderr"})

	if buf.Len() == 0 {
		t.Fatal("no build output was logged")
	}
	if strings.Contains(buf.String(), proofCanary) {
		t.Errorf("build log output holds the secret: %s", buf.String())
	}
}
