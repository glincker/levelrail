package pipeline

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
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
	if err := jr.packPaths(ctx, cid, paths, w, st.Limits()); err != nil {
		w.Abort()
		return jr.collectFailure(ctx, err, st.Limits())
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

// packPaths copies each declared path out of the container through the
// archive API and writes one gzip tar to w as it streams, so nothing is
// buffered and a cap hit stops the copy early.
func (jr *jobRun) packPaths(ctx context.Context, cid string, paths []string, w *ArtifactWriter, lim ArtifactLimits) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	pl := packLimits{MaxUnpacked: lim.MaxUnpackedBytes, MaxEntries: lim.MaxEntries, Secrets: jr.mask.values()}
	pk := newPacker(tw, pl)
	for _, p := range paths {
		if err := jr.packOne(ctx, cid, p, pk, pl.streamCap()); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func (jr *jobRun) packOne(ctx context.Context, cid, p string, pk *packer, streamCap int64) error {
	rc, stat, err := jr.rt.CopyFromContainer(ctx, cid, path.Join(workspaceDir, p), streamCap)
	if err != nil {
		if errors.Is(err, docker.ErrArchivePathNotFound) || strings.Contains(err.Error(), docker.ErrArchivePathNotFound.Error()) {
			return fmt.Errorf("path not found: %s", p)
		}
		return err
	}
	defer func() { _ = rc.Close() }()
	return pk.add(p, rc, stat.Mode, stat.LinkTarget)
}

func (jr *jobRun) collectFailure(ctx context.Context, err error, lim ArtifactLimits) error {
	if ctx.Err() != nil {
		jr.e.cfg.Logger.Warn("pipeline: artifact upload cancelled", slog.String("run_id", jr.run.ID), slog.String("job_id", jr.row.ID))
		return ctx.Err()
	}
	jr.e.cfg.Logger.Warn("pipeline: artifact upload failed", slog.String("run_id", jr.run.ID), slog.String("job_id", jr.row.ID), slog.String("error", err.Error()))
	switch {
	case errors.Is(err, ErrArtifactTooLarge):
		return fmt.Errorf("artifact upload failed: archive is larger than the %d byte limit", lim.MaxBytes)
	case errors.Is(err, docker.ErrArchiveTooLarge):
		return fmt.Errorf("artifact upload failed: files expand past the %d byte limit", lim.MaxUnpackedBytes)
	}
	var secret errSecretInArtifact
	if errors.As(err, &secret) {
		return fmt.Errorf("artifact rejected: %w", err)
	}
	return fmt.Errorf("artifact upload failed: %w", err)
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
	f, err := jr.e.cfg.Artifacts.OpenVerified(meta)
	if err != nil {
		return fmt.Errorf("artifact of job %s is unavailable: %w", meta.JobKey, err)
	}
	defer func() { _ = f.Close() }()
	copyErr := jr.rt.CopyToContainer(ctx, cid, workspaceDir, f, meta.SizeBytes)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if verr := f.Verify(); errors.Is(verr, ErrArtifactCorrupt) {
		jr.e.cfg.Logger.Error("pipeline: artifact failed its sha256 check during restore", slog.String("run_id", jr.run.ID), slog.String("job_id", jr.row.ID), slog.String("artifact_id", meta.ID))
		return fmt.Errorf("artifact of job %s failed its sha256 check while restoring, the job was stopped", meta.JobKey)
	}
	if copyErr != nil {
		return fmt.Errorf("restoring artifacts of job %s: %w", meta.JobKey, copyErr)
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
