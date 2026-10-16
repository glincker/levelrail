package pipeline

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func (e *Engine) artifactIndex() ArtifactIndex {
	idx, _ := e.cfg.Store.(ArtifactIndex)
	return idx
}

func (jr *jobRun) artifactsReady() (ArtifactIndex, error) {
	idx := jr.e.artifactIndex()
	if jr.e.cfg.Artifacts == nil || idx == nil {
		return nil, errors.New("artifact storage is not configured on this control plane")
	}
	return idx, nil
}

// collectArtifacts archives the job's declared paths out of the container
// into the artifact store. Any failure fails the job: a job that "succeeded"
// without its artifacts would hand an empty workspace to the next job.
func (jr *jobRun) collectArtifacts(ctx context.Context, k int) error {
	idx, err := jr.artifactsReady()
	if err != nil {
		return err
	}
	paths, err := cleanArtifactPaths(jr.jd.Artifacts)
	if err != nil {
		return err
	}
	cid, err := jr.ensureContainer(ctx)
	if err != nil {
		return err
	}
	st := jr.e.cfg.Artifacts
	existing, err := idx.ListPipelineArtifacts(ctx, jr.run.ID)
	if err != nil {
		return fmt.Errorf("list stored artifacts: %w", err)
	}
	var used int64
	for _, a := range existing {
		if a.JobKey != jr.row.Key {
			used += a.SizeBytes
		}
	}
	id := jr.e.cfg.NewID()
	w, err := st.Begin(jr.run.ID, id, used)
	if err != nil {
		return err
	}
	rc, err := jr.rt.ExecWithInput(ctx, cid, []string{"sh", "-s"}, strings.NewReader(collectScript(paths)))
	if err != nil {
		w.Abort()
		return fmt.Errorf("exec: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = rc.Close() })
	diag, rerr := readCollected(rc, w)
	stop()
	_ = rc.Close()
	if rerr != nil {
		w.Abort()
		return jr.collectFailure(ctx, rerr, diag, st.Limits())
	}
	if err := jr.verifyArchive(w); err != nil {
		w.Abort()
		return err
	}
	rel, size, sum, err := w.Commit()
	if err != nil {
		return err
	}
	now := jr.e.cfg.Now()
	meta := store.PipelineArtifact{
		ID: id, RunID: jr.run.ID, JobKey: jr.row.Key, Name: jr.row.Key, SizeBytes: size, SHA256: sum,
		StoragePath: rel, CreatedAt: now, ExpiresAt: now.Add(st.Limits().Retention),
	}
	if err := idx.SavePipelineArtifact(context.WithoutCancel(ctx), meta); err != nil {
		_ = st.Remove(rel)
		return fmt.Errorf("record artifact: %w", err)
	}
	jr.sink.Line(k, "stdout", fmt.Sprintf("stored artifact %s (%d bytes, sha256 %s)", jr.row.Key, size, sum))
	return nil
}

func (jr *jobRun) collectFailure(ctx context.Context, rerr error, diag []string, lim ArtifactLimits) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, d := range diag {
		jr.sink.Line(0, "stderr", "[artifacts] "+d)
	}
	switch {
	case errors.Is(rerr, ErrArtifactTooLarge):
		return fmt.Errorf("artifact upload failed: archive is larger than the %d byte limit", lim.MaxBytes)
	}
	var ee *docker.ExecExitError
	if errors.As(rerr, &ee) && len(diag) > 0 {
		return fmt.Errorf("artifact upload failed: %s", diag[0])
	}
	return fmt.Errorf("artifact upload failed: %w", rerr)
}

func (jr *jobRun) verifyArchive(w *ArtifactWriter) error {
	f, err := openRead(w.Path())
	if err != nil {
		return fmt.Errorf("artifact check failed: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := validateArchive(f, jr.mask.values(), jr.e.cfg.Artifacts.Limits().MaxUnpackedBytes); err != nil {
		return fmt.Errorf("artifact rejected: %w", err)
	}
	return nil
}

// restoreArtifacts unpacks the artifacts of every needed job that stored
// some into this job's workspace.
func (jr *jobRun) restoreArtifacts(ctx context.Context, k int) error {
	idx, err := jr.artifactsReady()
	if err != nil {
		return err
	}
	stored, err := idx.ListPipelineArtifacts(ctx, jr.run.ID)
	if err != nil {
		return fmt.Errorf("list stored artifacts: %w", err)
	}
	cid, err := jr.ensureContainer(ctx)
	if err != nil {
		return err
	}
	needed := neededArtifactJobs(jr.def, baseJobName(jr.row.Key), jr.jd)
	for _, o := range jr.all {
		if !slices.Contains(needed, baseJobName(o.Key)) {
			continue
		}
		meta, ok := findArtifact(stored, o.Key)
		if !ok {
			if o.Status == store.PipelineStatusSucceeded {
				return fmt.Errorf("artifacts of job %s are missing (expired or never stored)", o.Key)
			}
			jr.sink.Line(k, "stdout", fmt.Sprintf("no artifacts from job %s (it ended %s)", o.Key, o.Status))
			continue
		}
		if err := jr.restoreOne(ctx, k, cid, meta); err != nil {
			return err
		}
	}
	return nil
}

func findArtifact(list []store.PipelineArtifact, jobKey string) (store.PipelineArtifact, bool) {
	for _, a := range list {
		if a.JobKey == jobKey {
			return a, true
		}
	}
	return store.PipelineArtifact{}, false
}

func (jr *jobRun) restoreOne(ctx context.Context, k int, cid string, meta store.PipelineArtifact) error {
	f, err := jr.e.cfg.Artifacts.Open(meta)
	if errors.Is(err, ErrArtifactCorrupt) {
		return fmt.Errorf("artifact of job %s failed its sha256 check, refusing to restore it", meta.JobKey)
	}
	if err != nil {
		return fmt.Errorf("artifact of job %s is unavailable: %w", meta.JobKey, err)
	}
	defer func() { _ = f.Close() }()
	in := restoreStream(f)
	defer func() { _ = in.Close() }()
	rc, err := jr.rt.ExecWithInput(ctx, cid, []string{"sh", "-s"}, in)
	if err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = rc.Close() })
	defer func() {
		stop()
		_ = rc.Close()
	}()
	readErr := readLines(rc, func(line string) { jr.sink.Line(k, "stdout", strings.TrimPrefix(line, collectErr)) })
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if readErr != nil {
		var ee *docker.ExecExitError
		if errors.As(readErr, &ee) {
			return fmt.Errorf("restoring artifacts of job %s exited with code %d", meta.JobKey, ee.ExitCode)
		}
		return fmt.Errorf("restoring artifacts of job %s: %w", meta.JobKey, readErr)
	}
	jr.sink.Line(k, "stdout", fmt.Sprintf("restored artifacts of job %s (%d bytes, sha256 %s)", meta.JobKey, meta.SizeBytes, meta.SHA256))
	return nil
}

// sweepArtifacts removes expired archives and orphaned run directories.
func (e *Engine) sweepArtifacts(ctx context.Context) error {
	idx := e.artifactIndex()
	if e.cfg.Artifacts == nil || idx == nil {
		return nil
	}
	_, err := e.cfg.Artifacts.Sweep(ctx, idx, func(ctx context.Context, runID string) bool {
		_, gerr := e.cfg.Store.GetPipelineRun(ctx, runID)
		return !errors.Is(gerr, store.ErrPipelineRunNotFound)
	})
	return err
}
