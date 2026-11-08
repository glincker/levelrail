package backup

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrComposeContainerNotRunning means no running container backs the app service on this node.
var ErrComposeContainerNotRunning = errors.New("backup: compose service has no running container")

// ComposeContainerResolver finds the running container behind a compose app service.
type ComposeContainerResolver interface {
	ResolveComposeContainer(ctx context.Context, app, service string) (string, error)
}

// ComposeTargetResolver is the node-aware form: it also reports which node runs the container.
type ComposeTargetResolver interface {
	ResolveComposeTarget(ctx context.Context, app, service string) (container, nodeID string, err error)
}

type composeNodeKey struct{}

// WithComposeNode carries the node of the resolved container to the dumper and restorer.
func WithComposeNode(ctx context.Context, nodeID string) context.Context {
	return context.WithValue(ctx, composeNodeKey{}, nodeID)
}

func composeNodeFrom(ctx context.Context) string {
	id, _ := ctx.Value(composeNodeKey{}).(string)
	return id
}

// resolveComposeWithNode resolves the container and returns a context bound to its node.
func resolveComposeWithNode(ctx context.Context, r ComposeContainerResolver, app, service string) (context.Context, string, error) {
	if tr, ok := r.(ComposeTargetResolver); ok {
		container, nodeID, err := tr.ResolveComposeTarget(ctx, app, service)
		if err != nil {
			return ctx, "", err
		}
		return WithComposeNode(ctx, nodeID), container, nil
	}
	container, err := r.ResolveComposeContainer(ctx, app, service)
	return ctx, container, err
}

// ComposeRunStore records compose service backup attempts.
type ComposeRunStore interface {
	StartComposeServiceBackupRun(ctx context.Context, r store.ComposeServiceBackupRun) error
	FinishComposeServiceBackupRun(ctx context.Context, id, status string, sizeBytes int64, checksum, errMsg, finishedAt string) error
}

// ComposeServiceRunner dumps a compose database service through the shared Runner,
// so destination, encryption and upload behave like every other backup.
type ComposeServiceRunner struct {
	Runner     *Runner
	Runs       ComposeRunStore
	Containers ComposeContainerResolver
	Dumper     Dumper
	Now        func() time.Time
}

func (c *ComposeServiceRunner) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// RunComposeServiceBackup dumps app/service to targetID and records the attempt in the
// compose run history. Every failure after the row exists finishes it as failed.
func (c *ComposeServiceRunner) RunComposeServiceBackup(ctx context.Context, runID, app, service, engine, targetID string) error {
	if err := checkDiskSpace(resolveWorkDir(c.Runner.WorkDir)); err != nil {
		return err
	}
	startedAt := c.now()
	sealer, sealErr := c.Runner.dumpSealer(ctx, targetID)
	objectKey := ""
	if sealErr == nil {
		objectKey = fmt.Sprintf("compose/%s/%s/%s-%s-%s.dump%s", app, service, service, startedAt.UTC().Format("20060102T150405Z"), runID, CodecSuffix(sealer.Codec()))
	}
	if err := c.Runs.StartComposeServiceBackupRun(ctx, store.ComposeServiceBackupRun{
		ID: runID, App: app, Service: service, TargetID: targetID, ObjectKey: objectKey,
		StartedAt: startedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("backup: start compose run %q: %w", runID, err)
	}

	var (
		size     int64
		checksum string
		runErr   = sealErr
	)
	if runErr == nil {
		size, checksum, runErr = c.dump(ctx, app, service, engine, targetID, objectKey, sealer)
	}

	status, errMsg := store.BackupStatusSucceeded, ""
	if runErr != nil {
		status, errMsg = store.BackupStatusFailed, runErr.Error()
	}
	finishedAt := c.now().UTC().Format(time.RFC3339)
	if err := finishHistoryWithRetry(ctx, func() error {
		return c.Runs.FinishComposeServiceBackupRun(ctx, runID, status, size, checksum, errMsg, finishedAt)
	}); err != nil {
		return fmt.Errorf("backup: finish compose run %q: %w", runID, err)
	}
	return runErr
}

func (c *ComposeServiceRunner) dump(ctx context.Context, app, service, engine, targetID, objectKey string, sealer *Sealer) (int64, string, error) {
	ctx, container, err := resolveComposeWithNode(ctx, c.Containers, app, service)
	if err != nil {
		return 0, "", fmt.Errorf("resolve container for %s/%s: %w", app, service, err)
	}
	// A copy so the shared Runner keeps its database Dumper; only the dump source differs here.
	r := *c.Runner
	r.Dumper = c.Dumper
	size, checksum, _, err := r.runDumpAndUpload(ctx, app+"/"+service, engine, container, targetID, objectKey, sealer)
	return size, checksum, err
}

// RandomComposeRunID returns a new compose run id.
func RandomComposeRunID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("backup: generate compose run id: %w", err)
	}
	return "csr_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// ComposeServiceLister lists the desired services of one app.
type ComposeServiceLister interface {
	ListServicesByApp(ctx context.Context, appID string) ([]store.DesiredService, error)
}

// DesiredComposeResolver resolves a compose service to its running container on this node
// using the same name rules the reconciler uses to own containers.
type DesiredComposeResolver struct {
	Services ComposeServiceLister
	Runtime  docker.Runtime
	// Runtimes picks the runtime of the node a service is placed on; "" is the control plane.
	// When nil, Runtime serves every node.
	Runtimes func(nodeID string) (docker.Runtime, error)
	// Owns reports whether containerName belongs to desired service serviceName.
	Owns func(serviceName, containerName string) bool
}

func (d *DesiredComposeResolver) runtimeFor(nodeID string) (docker.Runtime, error) {
	if d.Runtimes != nil {
		return d.Runtimes(nodeID)
	}
	return d.Runtime, nil
}

// ResolveComposeContainer implements ComposeContainerResolver.
func (d *DesiredComposeResolver) ResolveComposeContainer(ctx context.Context, app, service string) (string, error) {
	c, _, err := d.ResolveComposeTarget(ctx, app, service)
	return c, err
}

// ResolveComposeTarget implements ComposeTargetResolver, looking on the node the service is placed on.
func (d *DesiredComposeResolver) ResolveComposeTarget(ctx context.Context, app, service string) (string, string, error) {
	svcs, err := d.Services.ListServicesByApp(ctx, app)
	if err != nil {
		return "", "", fmt.Errorf("list services of %q: %w", app, err)
	}
	full := app + "-" + service
	for _, s := range svcs {
		if s.Name != full && s.Name != service {
			continue
		}
		rt, err := d.runtimeFor(s.NodeID)
		if err != nil {
			return "", "", fmt.Errorf("runtime for node %q: %w", s.NodeID, err)
		}
		states, err := rt.ListByPrefix(ctx, s.Name+"-")
		if err != nil {
			return "", "", fmt.Errorf("list containers of %q: %w", s.Name, err)
		}
		for _, st := range states {
			if st.Running && d.Owns != nil && d.Owns(s.Name, st.Name) {
				return st.Name, s.NodeID, nil
			}
		}
		return "", "", ErrComposeContainerNotRunning
	}
	return "", "", fmt.Errorf("service %q not found in app %q", service, app)
}
