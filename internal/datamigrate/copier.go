package datamigrate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
)

// Restorer replays a dump into a running database container.
// internal/backup's ContainerRestorer satisfies it and replaces the target's
// contents, so a repeated copy is idempotent.
type Restorer interface {
	Restore(ctx context.Context, engine, containerName string, dump io.Reader) error
}

// Copier moves a source database into a managed database through a helper
// container on the target's node. It only uses the Docker Engine API.
type Copier struct {
	Runtime  docker.Runtime
	Restorer Restorer
	Logger   *slog.Logger
	// VerifyAttempts and VerifyInterval retry the target count while the
	// engine is still loading the restored data. Zero values use defaults.
	// HelperNetwork is a Docker network the helper joins instead of the
	// default bridge, so a source container on it is reachable.
	HelperNetwork  string
	VerifyAttempts int
	VerifyInterval time.Duration
}

// Target names the managed database receiving the data.
type Target struct {
	Name    string
	Engine  string
	Version string
}

// Copy runs the full copy and verification. Every error is scrubbed of the
// source password. The helper container is always removed.
func (c *Copier) Copy(ctx context.Context, t Target, src Source) (Verification, error) {
	v, err := c.copy(ctx, t, src)
	if err != nil {
		return v, errors.New(src.Scrub(err.Error()))
	}
	return v, nil
}

func (c *Copier) copy(ctx context.Context, t Target, src Source) (Verification, error) {
	log := c.logger().With(slog.String("database", t.Name), slog.String("engine", t.Engine))
	dump, err := DumpCommand(t.Engine)
	if err != nil {
		return Verification{}, err
	}
	targetName := database.ContainerName(t.Name)
	state, err := c.Runtime.InspectByName(ctx, targetName)
	if err != nil {
		return Verification{}, fmt.Errorf("inspect target database: %w", err)
	}
	if state == nil || !state.Running {
		return Verification{}, errors.New("the managed database is not running yet, wait for it to become ready and retry")
	}

	helper, err := c.startHelper(ctx, t, src)
	if err != nil {
		return Verification{}, err
	}
	defer func() {
		rmCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if rmErr := c.Runtime.Remove(rmCtx, helper, true); rmErr != nil {
			log.Warn("datamigrate: remove helper container failed", slog.String("error", src.Scrub(rmErr.Error())))
		}
	}()

	log.Info("datamigrate: streaming dump into target", slog.String("source_host", src.Host))
	rc, err := c.Runtime.Exec(ctx, helper, dump)
	if err != nil {
		return Verification{}, fmt.Errorf("start dump of the source: %w", err)
	}
	tail := &errTail{r: rc}
	restoreErr := c.Restorer.Restore(ctx, t.Engine, targetName, tail)
	_ = rc.Close()
	if tail.err != nil && !errors.Is(tail.err, io.EOF) {
		return Verification{}, fmt.Errorf("dump of the source failed: %w", tail.err)
	}
	if restoreErr != nil {
		return Verification{}, fmt.Errorf("restore into the managed database: %w", restoreErr)
	}
	return c.verify(ctx, t, helper, targetName)
}

func (c *Copier) startHelper(ctx context.Context, t Target, src Source) (string, error) {
	return startHelperContainer(ctx, c.Runtime, database.ImageRef(t.Engine, t.Version), src, t.Name, c.HelperNetwork)
}

// startHelperContainer starts a sleeping container from image whose only
// inputs are the SRC_* variables, and returns its id.
func startHelperContainer(ctx context.Context, rt docker.Runtime, image string, src Source, label, network string) (string, error) {
	var attach *docker.NetworkAttachment
	if network != "" && network != defaultBridgeNetwork {
		attach = &docker.NetworkAttachment{Name: network}
	}
	return startHelperOn(ctx, rt, image, src, label, attach)
}

func startHelperOn(ctx context.Context, rt docker.Runtime, image string, src Source, label string, attach *docker.NetworkAttachment) (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate helper name: %w", err)
	}
	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:       "dbmigrate-" + hex.EncodeToString(buf),
		Image:      image,
		Entrypoint: []string{"sleep"},
		Command:    []string{"86400"},
		Env:        helperEnv(src),
		Labels:     map[string]string{"data-copy": label},
		Network:    attach,
	})
	if err != nil {
		return "", fmt.Errorf("create helper container: %w", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		_ = rt.Remove(context.Background(), id, true)
		return "", fmt.Errorf("start helper container: %w", err)
	}
	return id, nil
}

func (c *Copier) verify(ctx context.Context, t Target, helper, targetName string) (Verification, error) {
	srcCmd, err := SourceCountCommand(t.Engine)
	if err != nil {
		return Verification{}, err
	}
	tgtCmd, err := TargetCountCommand(t.Engine)
	if err != nil {
		return Verification{}, err
	}
	srcOut, err := c.runCapture(ctx, helper, srcCmd)
	if err != nil {
		return Verification{}, fmt.Errorf("count source rows: %w", err)
	}
	source := ParseCounts(srcOut)
	if t.Engine == EngineRedis && len(source) == 0 {
		return Verification{}, errors.New("count source keys: no result")
	}

	attempts, interval := c.VerifyAttempts, c.VerifyInterval
	if attempts <= 0 {
		attempts = 6
	}
	if interval <= 0 {
		interval = 3 * time.Second
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return Verification{}, ctx.Err()
			case <-time.After(interval):
			}
		}
		out, err := c.runCapture(ctx, targetName, tgtCmd)
		if err != nil {
			lastErr = fmt.Errorf("count target rows: %w", err)
			continue
		}
		v := Compare(source, ParseCounts(out))
		if v.Mismatched > 0 {
			return v, fmt.Errorf("verification failed, %s", v.Failure(10))
		}
		return v, nil
	}
	return Verification{}, lastErr
}

func (c *Copier) runCapture(ctx context.Context, container string, cmd []string) (string, error) {
	rc, err := c.Runtime.Exec(ctx, container, cmd)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	var sb strings.Builder
	if _, err := io.Copy(&sb, io.LimitReader(rc, 8<<20)); err != nil {
		return "", err
	}
	return sb.String(), nil
}

func (c *Copier) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// errTail remembers the first non-EOF error its source returned, so a dump
// that died midway is reported even when the restore tool saw a clean EOF.
type errTail struct {
	r   io.Reader
	err error
}

func (e *errTail) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err != nil && e.err == nil {
		e.err = err
	}
	return n, err
}
