package pipeline

import (
	"archive/tar"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const artifactFlow = `
version: 1
jobs:
  build:
    image: alpine
    artifacts: [dist]
    steps:
      - run: make
  test:
    needs: build
    image: alpine
    steps:
      - run: ls dist
  report:
    needs: test
    image: alpine
    steps:
      - run: echo done
`

func newArtifactStoreT(t *testing.T, lim ArtifactLimits) *ArtifactStore {
	t.Helper()
	if lim.Retention == 0 {
		lim.Retention = time.Hour
	}
	s, err := NewArtifactStore(t.TempDir(), lim)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func artifactHandler(archive []byte, buildExit int) func(string) execResult {
	return func(s string) execResult {
		switch {
		case strings.Contains(s, collectBegin):
			return execResult{Output: collectOutput(archive)}
		case strings.Contains(s, "make"):
			return execResult{Output: "build output\n", Exit: buildExit}
		}
		return execResult{Output: "ok\n"}
	}
}

func goodArchive(t *testing.T) []byte {
	return makeTarGz(t, []tarEntry{{name: "dist/", typ: tar.TypeDir}, {name: "dist/app", body: "binary"}})
}

func TestArtifactHandoffBetweenJobs(t *testing.T) {
	t.Parallel()
	arts := newArtifactStoreT(t, ArtifactLimits{})
	h := newHarnessWithArtifacts(t, artifactHandler(goodArchive(t), 0), arts)
	run := h.start(h.save(artifactFlow), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s %q\n%s", got.Status, got.Reason, h.dump(run.ID))
	}
	rows, err := h.db.ListPipelineArtifacts(context.Background(), run.ID)
	if err != nil || len(rows) != 1 || rows[0].JobKey != "build" || len(rows[0].SHA256) != 64 {
		t.Fatalf("artifact rows = %+v, %v", rows, err)
	}
	if n := h.rt.ran("base64 -d <<'" + restoreData + "'"); n != 1 {
		t.Errorf("restore ran %d times, want 1 (only the job that needs build)", n)
	}
	if !strings.Contains(h.logs(run.ID), "restored artifacts of job build") {
		t.Errorf("logs missing restore line:\n%s", h.logs(run.ID))
	}
	f, err := arts.Open(rows[0])
	if err != nil {
		t.Fatalf("stored archive fails its own hash: %v", err)
	}
	_ = f.Close()
}

func TestArtifactUploadFailureFailsJobAndSkipsDownstream(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		lim     ArtifactLimits
		archive func(*testing.T) []byte
		want    string
	}{
		{"over the size cap", ArtifactLimits{MaxBytes: 20}, goodArchive, "larger than the 20 byte limit"},
		{"symlink escape", ArtifactLimits{}, func(t *testing.T) []byte {
			return makeTarGz(t, []tarEntry{{name: "dist/link", typ: tar.TypeSymlink, target: "../../../etc/shadow"}})
		}, "points outside the workspace"},
		{"path traversal entry", ArtifactLimits{}, func(t *testing.T) []byte {
			return makeTarGz(t, []tarEntry{{name: "../../etc/cron.d/x", body: "x"}})
		}, "leaves the workspace"},
		{"archive not gzip", ArtifactLimits{}, func(*testing.T) []byte { return []byte("plain text") }, "not a gzip archive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			arts := newArtifactStoreT(t, tc.lim)
			h := newHarnessWithArtifacts(t, artifactHandler(tc.archive(t), 0), arts)
			run := h.start(h.save(artifactFlow), StartOptions{})
			got := h.done(run.ID)
			if got.Status != store.PipelineStatusFailed {
				t.Fatalf("run = %s, want failed (a job with a rejected artifact must not report ok)", got.Status)
			}
			jobs := h.jobs(run.ID)
			if jobs["build"].Status != store.PipelineStatusFailed || !strings.Contains(jobs["build"].Reason, tc.want) {
				t.Errorf("build = %s %q, want failed containing %q", jobs["build"].Status, jobs["build"].Reason, tc.want)
			}
			if jobs["test"].Status != store.PipelineStatusSkipped || !strings.Contains(jobs["test"].Reason, "needed job build failed") {
				t.Errorf("test = %s %q, want skipped naming build", jobs["test"].Status, jobs["test"].Reason)
			}
			if jobs["report"].Status != store.PipelineStatusSkipped {
				t.Errorf("report = %s, want skipped", jobs["report"].Status)
			}
			if rows, _ := h.db.ListPipelineArtifacts(context.Background(), run.ID); len(rows) != 0 {
				t.Errorf("rejected artifact was indexed: %+v", rows)
			}
			assertNoStoredFiles(t, arts)
		})
	}
}

func assertNoStoredFiles(t *testing.T, s *ArtifactStore) {
	t.Helper()
	_ = filepath.WalkDir(s.dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			t.Errorf("leftover file in artifact store: %s", p)
		}
		return nil
	})
}

func TestArtifactFailedUpstreamSkipsDownstreamWithReason(t *testing.T) {
	t.Parallel()
	arts := newArtifactStoreT(t, ArtifactLimits{})
	h := newHarnessWithArtifacts(t, artifactHandler(goodArchive(t), 2), arts)
	run := h.start(h.save(artifactFlow), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusFailed {
		t.Fatalf("run = %s", got.Status)
	}
	jobs := h.jobs(run.ID)
	if jobs["test"].Reason != "needed job build failed" {
		t.Errorf("test reason = %q", jobs["test"].Reason)
	}
	if h.rt.ran(collectBegin) != 0 {
		t.Error("artifacts were collected from a failed job")
	}
}

func TestArtifactStorageNotConfigured(t *testing.T) {
	t.Parallel()
	h := newHarness(t, artifactHandler(nil, 0))
	run := h.start(h.save(artifactFlow), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusFailed {
		t.Fatalf("run = %s", got.Status)
	}
	if r := h.jobs(run.ID)["build"].Reason; !strings.Contains(r, "artifact storage is not configured") {
		t.Errorf("build reason = %q", r)
	}
}

func TestArtifactSecretInArchiveIsRefused(t *testing.T) {
	t.Parallel()
	arts := newArtifactStoreT(t, ArtifactLimits{})
	archive := makeTarGz(t, []tarEntry{{name: "dist/env", body: "TOKEN=s3cr3t-value"}})
	h := newHarnessWithArtifacts(t, artifactHandler(archive, 0), arts)
	yaml := strings.Replace(artifactFlow, "      - run: make\n", "      - run: make\n        secrets: [TOKEN]\n", 1)
	run := h.start(h.save(yaml), StartOptions{})
	h.done(run.ID)
	if r := h.jobs(run.ID)["build"].Reason; !strings.Contains(r, "contains a secret value") {
		t.Errorf("build reason = %q, want secret refusal\n%s", r, h.dump(run.ID))
	}
	if strings.Contains(h.logs(run.ID)+h.dump(run.ID), "s3cr3t-value") {
		t.Error("secret value leaked into logs or reasons")
	}
}

func TestArtifactHashMismatchBlocksRestore(t *testing.T) {
	t.Parallel()
	arts := newArtifactStoreT(t, ArtifactLimits{})
	h := newHarnessWithArtifacts(t, artifactHandler(goodArchive(t), 0), arts)
	yaml := strings.Replace(artifactFlow, "      - run: ls dist\n", "      - uses: approval\n        with: { message: wait }\n      - run: ls dist\n", 1)
	run := h.start(h.save(yaml), StartOptions{})
	h.until(run.ID, func(store.PipelineRun) bool {
		return h.jobs(run.ID)["test"].Status == store.PipelineStatusWaitingApproval
	})
	rows, _ := h.db.ListPipelineArtifacts(context.Background(), run.ID)
	if len(rows) != 1 {
		t.Fatalf("artifacts = %+v", rows)
	}
	full, _ := arts.resolve(rows[0].StoragePath)
	if err := os.WriteFile(full, []byte("corrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	approvals, _ := h.db.ListPipelineApprovals(context.Background(), run.ID)
	_, _ = h.db.DecidePipelineApproval(context.Background(), approvals[0].ID, "approved", "me", "", time.Now())
	if got := h.done(run.ID); got.Status != store.PipelineStatusFailed {
		t.Fatalf("run = %s", got.Status)
	}
	if r := h.jobs(run.ID)["test"].Reason; !strings.Contains(r, "failed its sha256 check") {
		t.Errorf("test reason = %q", r)
	}
	if h.rt.ran("ls dist") != 0 {
		t.Error("steps ran against an unverified artifact")
	}
}

func TestArtifactCancelDuringUploadLeavesNothing(t *testing.T) {
	t.Parallel()
	arts := newArtifactStoreT(t, ArtifactLimits{})
	h := newHarnessWithArtifacts(t, func(s string) execResult {
		if strings.Contains(s, collectBegin) {
			return execResult{Block: true}
		}
		return execResult{Output: "ok\n"}
	}, arts)
	run := h.start(h.save(artifactFlow), StartOptions{})
	h.until(run.ID, func(store.PipelineRun) bool { return h.rt.ran(collectBegin) > 0 })
	if err := h.e.Cancel(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	if got := h.done(run.ID); got.Status != store.PipelineStatusCancelled {
		t.Fatalf("run = %s %q", got.Status, got.Reason)
	}
	if rows, _ := h.db.ListPipelineArtifacts(context.Background(), run.ID); len(rows) != 0 {
		t.Errorf("cancelled upload was indexed: %+v", rows)
	}
	h.e.Wait()
	assertNoStoredFiles(t, arts)
}

func TestArtifactValidationRejectsBadDeclarations(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, artifacts, steps, want string }{
		{"traversal", "[../secrets]", "- run: x", "leaves the workspace"},
		{"absolute", "[/etc]", "- run: x", "relative to the workspace"},
		{"glob", "['dist/*']", "- run: x", "globs are not supported"},
		{"no container step", "[dist]", "- uses: notify\n        with: { message: hi }", "at least one step that runs in the job container"},
	}
	for _, tc := range tests {
		yaml := "version: 1\njobs:\n  a:\n    image: x\n    artifacts: " + tc.artifacts + "\n    steps:\n      " + tc.steps + "\n"
		_, issues := Validate([]byte(yaml))
		if len(issues) == 0 || !strings.Contains(issues[0].Message, tc.want) {
			t.Errorf("%s: issues = %+v, want %q", tc.name, issues, tc.want)
		}
	}
	if _, issues := Validate([]byte(artifactFlow)); len(issues) != 0 {
		t.Errorf("valid artifact flow rejected: %+v", issues)
	}
}

func TestArtifactStepPathsStayInWorkspace(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	for _, p := range []string{"../../etc/passwd", "/etc/shadow"} {
		jr := &jobRun{e: h.e}
		if _, err := jr.stepScript(KindArtifactUpload, Step{With: map[string]string{"name": "x", "path": p}}); err == nil {
			t.Errorf("upload path %q accepted", p)
		}
		if _, err := jr.stepScript(KindArtifactDownload, Step{With: map[string]string{"name": "x", "path": p}}); err == nil {
			t.Errorf("download path %q accepted", p)
		}
	}
}
