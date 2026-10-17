package pipeline

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
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

func artifactHandler(buildExit int) func(string) execResult {
	return func(s string) execResult {
		if strings.Contains(s, "make") {
			return execResult{Output: "build output\n", Exit: buildExit}
		}
		return execResult{Output: "ok\n"}
	}
}

func goodTar(t *testing.T) []byte {
	return dockerTar(t, []tarEntry{{name: "dist/", typ: tar.TypeDir}, {name: "dist/app", body: "binary"}})
}

func serveTar(rt *fakeRuntime, b []byte) {
	rt.copyOut = func(_ context.Context, _, _ string, _ int64) (io.ReadCloser, docker.PathStat, error) {
		return io.NopCloser(bytes.NewReader(b)), docker.PathStat{Name: "dist", Mode: os.ModeDir | 0o755}, nil
	}
}

func TestArtifactHandoffBetweenJobs(t *testing.T) {
	t.Parallel()
	arts := newArtifactStoreT(t, ArtifactLimits{})
	h := newHarnessWithArtifacts(t, artifactHandler(0), arts)
	serveTar(h.rt, goodTar(t))
	run := h.start(h.save(artifactFlow), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s %q\n%s", got.Status, got.Reason, h.dump(run.ID))
	}
	rows, err := h.db.ListPipelineArtifacts(context.Background(), run.ID)
	if err != nil || len(rows) != 1 || rows[0].JobKey != "build" || len(rows[0].SHA256) != 64 {
		t.Fatalf("artifact rows = %+v, %v", rows, err)
	}
	if len(h.rt.restores) != 1 {
		t.Fatalf("restore ran %d times, want 1 (only the job that needs build)", len(h.rt.restores))
	}
	stored, _ := os.ReadFile(filepath.Join(arts.dir, filepath.FromSlash(rows[0].StoragePath)))
	if !bytes.Equal(h.rt.restores[0], stored) {
		t.Error("restored bytes differ from the stored archive")
	}
	if !strings.Contains(h.logs(run.ID), "restored artifacts of job build") {
		t.Errorf("logs missing restore line:\n%s", h.logs(run.ID))
	}
	if h.rt.ran("base64") != 0 || h.rt.ran("tar ") != 0 {
		t.Error("artifact transfer shelled out inside the job container")
	}
	if names := storedNames(t, stored); strings.Join(names, ",") != "dist/,dist/app" {
		t.Errorf("stored entries = %v", names)
	}
}

func storedNames(t *testing.T, gzTar []byte) []string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(gzTar))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return names
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
}

func TestArtifactUploadFailureFailsJobAndSkipsDownstream(t *testing.T) {
	t.Parallel()
	boom := errors.New("connection reset")
	tests := []struct {
		name  string
		lim   ArtifactLimits
		serve func(t *testing.T, rt *fakeRuntime)
		want  string
	}{
		{"over the size cap", ArtifactLimits{MaxBytes: 20}, func(t *testing.T, rt *fakeRuntime) {
			serveTar(rt, dockerTar(t, []tarEntry{{name: "dist/big", body: strings.Repeat("abcdefghij", 5000)}}))
		}, "larger than the 20 byte limit"},
		{"unpacked cap on the raw stream", ArtifactLimits{MaxUnpackedBytes: 10, MaxEntries: 1}, func(t *testing.T, rt *fakeRuntime) {
			serveTar(rt, dockerTar(t, []tarEntry{{name: "dist/big", body: strings.Repeat("a", 20000)}}))
		}, "expands past the 10 byte limit"},
		{"too many files", ArtifactLimits{MaxEntries: 1}, func(t *testing.T, rt *fakeRuntime) { serveTar(rt, goodTar(t)) }, "more than 1 files"},
		{"symlink escape", ArtifactLimits{}, func(t *testing.T, rt *fakeRuntime) {
			serveTar(rt, dockerTar(t, []tarEntry{{name: "dist/link", typ: tar.TypeSymlink, target: "../../../etc/shadow"}}))
		}, "points outside the workspace"},
		{"path traversal entry", ArtifactLimits{}, func(t *testing.T, rt *fakeRuntime) {
			serveTar(rt, dockerTar(t, []tarEntry{{name: "../../etc/cron.d/x", body: "x"}}))
		}, "leaves the workspace"},
		{"device entry", ArtifactLimits{}, func(t *testing.T, rt *fakeRuntime) {
			serveTar(rt, dockerTar(t, []tarEntry{{name: "dist/null", typ: tar.TypeChar}}))
		}, "unsupported type"},
		{"path not found", ArtifactLimits{}, func(*testing.T, *fakeRuntime) {}, "path not found: dist"},
		{"stream dies mid entry", ArtifactLimits{}, func(t *testing.T, rt *fakeRuntime) {
			full := dockerTar(t, []tarEntry{{name: "dist/app", body: strings.Repeat("z", 4000)}})
			rt.copyOut = func(context.Context, string, string, int64) (io.ReadCloser, docker.PathStat, error) {
				return io.NopCloser(io.MultiReader(bytes.NewReader(full[:1200]), &errReader{boom})), docker.PathStat{}, nil
			}
		}, "connection reset"},
		{"stream ends short", ArtifactLimits{}, func(t *testing.T, rt *fakeRuntime) {
			full := dockerTar(t, []tarEntry{{name: "dist/app", body: strings.Repeat("z", 4000)}})
			serveTar(rt, full[:1500])
		}, "unexpected EOF"},
		{"archive api refused", ArtifactLimits{}, func(_ *testing.T, rt *fakeRuntime) {
			rt.copyOut = func(context.Context, string, string, int64) (io.ReadCloser, docker.PathStat, error) {
				return nil, docker.PathStat{}, docker.ErrArchiveUnsupported
			}
		}, "does not support archive copy"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			arts := newArtifactStoreT(t, tc.lim)
			h := newHarnessWithArtifacts(t, artifactHandler(0), arts)
			tc.serve(t, h.rt)
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
	h := newHarnessWithArtifacts(t, artifactHandler(2), arts)
	serveTar(h.rt, goodTar(t))
	run := h.start(h.save(artifactFlow), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusFailed {
		t.Fatalf("run = %s", got.Status)
	}
	jobs := h.jobs(run.ID)
	if jobs["test"].Reason != "needed job build failed" {
		t.Errorf("test reason = %q", jobs["test"].Reason)
	}
	if len(h.rt.restores) != 0 {
		t.Error("artifacts were restored from a failed job")
	}
}

func TestArtifactStorageNotConfigured(t *testing.T) {
	t.Parallel()
	h := newHarness(t, artifactHandler(0))
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
	h := newHarnessWithArtifacts(t, artifactHandler(0), arts)
	serveTar(h.rt, dockerTar(t, []tarEntry{{name: "dist/env", body: "TOKEN=s3cr3t-value"}}))
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
	h := newHarnessWithArtifacts(t, artifactHandler(0), arts)
	serveTar(h.rt, goodTar(t))
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
	if r := h.jobs(run.ID)["test"].Reason; !strings.Contains(r, "failed its sha256 check while restoring") {
		t.Errorf("test reason = %q", r)
	}
	if h.rt.ran("ls dist") != 0 {
		t.Error("steps ran against an unverified artifact")
	}
}

func TestArtifactCancelDuringUploadLeavesNothing(t *testing.T) {
	t.Parallel()
	arts := newArtifactStoreT(t, ArtifactLimits{})
	h := newHarnessWithArtifacts(t, artifactHandler(0), arts)
	started := make(chan struct{})
	var once sync.Once
	h.rt.copyOut = func(ctx context.Context, _, _ string, _ int64) (io.ReadCloser, docker.PathStat, error) {
		once.Do(func() { close(started) })
		return &blockingReader{ctx: ctx}, docker.PathStat{}, nil
	}
	run := h.start(h.save(artifactFlow), StartOptions{})
	h.until(run.ID, func(store.PipelineRun) bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	})
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

func TestArtifactRestoreFailureKeepsStoredArtifact(t *testing.T) {
	t.Parallel()
	arts := newArtifactStoreT(t, ArtifactLimits{})
	h := newHarnessWithArtifacts(t, artifactHandler(0), arts)
	serveTar(h.rt, goodTar(t))
	h.rt.copyIn = func(_ string, r io.Reader) error {
		_, _ = io.CopyN(io.Discard, r, 10)
		return errors.New("daemon closed the connection")
	}
	run := h.start(h.save(artifactFlow), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusFailed {
		t.Fatalf("run = %s", got.Status)
	}
	if r := h.jobs(run.ID)["test"].Reason; !strings.Contains(r, "restoring artifacts of job build") || !strings.Contains(r, "daemon closed") {
		t.Errorf("test reason = %q", r)
	}
	if h.jobs(run.ID)["build"].Status != store.PipelineStatusSucceeded {
		t.Errorf("build = %s, an upload that worked must stay succeeded", h.jobs(run.ID)["build"].Status)
	}
	rows, _ := h.db.ListPipelineArtifacts(context.Background(), run.ID)
	if len(rows) != 1 {
		t.Fatalf("artifact rows = %+v", rows)
	}
	f, err := arts.Open(rows[0])
	if err != nil {
		t.Fatalf("stored artifact damaged by a failed restore: %v", err)
	}
	_ = f.Close()
}

const parallelArtifactFlow = `
version: 1
jobs:
  a:
    image: alpine
    artifacts: [dist]
    steps:
      - run: make
  b:
    image: alpine
    artifacts: [dist]
    steps:
      - run: make
  join:
    needs: [a, b]
    image: alpine
    steps:
      - run: ls dist
`

func TestArtifactConcurrentCollection(t *testing.T) {
	t.Parallel()
	arts := newArtifactStoreT(t, ArtifactLimits{})
	h := newHarnessWithArtifacts(t, artifactHandler(0), arts)
	h.rt.copyOut = func(_ context.Context, cid, _ string, _ int64) (io.ReadCloser, docker.PathStat, error) {
		b := dockerTar(t, []tarEntry{{name: "dist/app", body: "built in " + cid + strings.Repeat("x", 100000)}})
		return io.NopCloser(bytes.NewReader(b)), docker.PathStat{}, nil
	}
	run := h.start(h.save(parallelArtifactFlow), StartOptions{})
	if got := h.done(run.ID); got.Status != store.PipelineStatusSucceeded {
		t.Fatalf("run = %s %q\n%s", got.Status, got.Reason, h.dump(run.ID))
	}
	rows, _ := h.db.ListPipelineArtifacts(context.Background(), run.ID)
	if len(rows) != 2 || rows[0].SHA256 == rows[1].SHA256 || rows[0].StoragePath == rows[1].StoragePath {
		t.Fatalf("rows = %+v, want two distinct artifacts", rows)
	}
	if len(h.rt.restores) != 2 {
		t.Errorf("restores = %d, want 2", len(h.rt.restores))
	}
	for _, r := range rows {
		f, err := arts.Open(r)
		if err != nil {
			t.Errorf("artifact %s: %v", r.ID, err)
			continue
		}
		_ = f.Close()
	}
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
