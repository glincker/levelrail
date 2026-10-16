package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/store"
)

// StaticPublishRecorder stamps a static deploy's attempt with what a container
// deploy gets from the application controller: a content digest, a gate
// timeline and a serving rollout state. *store.DB satisfies it.
type StaticPublishRecorder interface {
	SetDeployAttemptDigest(ctx context.Context, id, digest, reason string) error
	SetDeployAttemptRollout(ctx context.Context, id, state string) error
	RecordGateEvent(ctx context.Context, e store.DeployGateEvent) error
}

// WithStaticPublishRecorder enables digest, gate and rollout recording for
// build.type: static deploys.
func WithStaticPublishRecorder(r StaticPublishRecorder) Option {
	return func(p *Pipeline) { p.staticRecorder = r }
}

// StaticTreeStats is what hashing a published tree found.
type StaticTreeStats struct {
	Digest string
	Files  int
	Bytes  int64
}

// HashStaticTree returns a stable digest of every regular file's path and
// content under dir, so the same tree always hashes the same.
func HashStaticTree(dir string) (StaticTreeStats, error) {
	var st StaticTreeStats
	h := sha256.New()
	fsys := os.DirFS(dir)
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		f, err := fsys.Open(p)
		if err != nil {
			return err
		}
		defer f.Close() //nolint:errcheck // read-only handle
		fh := sha256.New()
		n, err := io.Copy(fh, f)
		if err != nil {
			return fmt.Errorf("hash %q: %w", p, err)
		}
		// WalkDir is lexical, so the stream order is deterministic.
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00%x\n", path.Clean(p), n, fh.Sum(nil))
		st.Files++
		st.Bytes += n
		return nil
	})
	if err != nil {
		return st, fmt.Errorf("hash static tree %q: %w", dir, err)
	}
	st.Digest = "sha256:" + hex.EncodeToString(h.Sum(nil))
	return st, nil
}

// emitStatic reports one fetch or publish step to the build log, so the Build
// tab shows what a static deploy did even though no image is built.
func emitStatic(progress func(build.ProgressEvent), step, line string, done bool) {
	if progress == nil {
		return
	}
	if line != "" {
		progress(build.ProgressEvent{Step: step, Log: line, Stream: "stdout"})
	}
	progress(build.ProgressEvent{Step: step, Completed: done})
}

// recordStaticPublished marks the attempt served: digest, the cutover and
// watch gate steps (there is no container to gate or watch), and the serving
// rollout state the CLI wait and the outcome endpoint settle on.
func (p *Pipeline) recordStaticPublished(ctx context.Context, req Request, st StaticTreeStats, took time.Duration) {
	if p.staticRecorder == nil || req.AttemptID == "" {
		return
	}
	warn := func(what string, err error) {
		if err != nil {
			p.logger.Warn("deploy: record static publish failed",
				slog.String("step", what), slog.String("attempt_id", req.AttemptID), slog.String("error", err.Error()))
		}
	}
	warn("digest", p.staticRecorder.SetDeployAttemptDigest(ctx, req.AttemptID, st.Digest, store.DigestReasonStaticTree))
	msg := fmt.Sprintf("Published %d files (%d bytes) to the embedded file server in %s", st.Files, st.Bytes, took.Round(time.Millisecond))
	for _, e := range []store.DeployGateEvent{
		{Phase: deploysafety.PhaseCutover, Status: deploysafety.StatusPassed, Reason: "StaticSitePublished", Message: msg},
		{Phase: deploysafety.PhaseWatch, Status: deploysafety.StatusPassed, Reason: "StaticSiteNoContainer", Message: "Static site: no container to watch"},
	} {
		e.DeployAttemptID, e.ServiceName = req.AttemptID, req.ServiceName
		warn("gate "+e.Phase, p.staticRecorder.RecordGateEvent(ctx, e))
	}
	warn("rollout", p.staticRecorder.SetDeployAttemptRollout(ctx, req.AttemptID, store.RolloutStateServing))
}
