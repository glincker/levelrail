package database

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbprovision"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Environment variables for the waiting steps of creating a database.
const (
	EnvDockerWaitInterval  = "APP_DB_DOCKER_WAIT_INTERVAL"
	EnvReadyProbeInterval  = "APP_DB_READY_PROBE_INTERVAL"
	EnvReadyTimeout        = "APP_DB_READY_TIMEOUT"
	defaultDockerWait      = 3 * time.Second
	defaultReadyProbe      = 2 * time.Second
	defaultReadyTimeout    = 2 * time.Minute
	reasonWaitingForDocker = "WaitingForDocker"
	reasonNotAccepting     = "NotAcceptingConnections"
	reasonVolumeFailed     = "VolumeFailed"
	probeDialTimeout       = time.Second
)

func durationFromEnv(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return def
}

// volumeAdmin is what the local Docker client offers for finding and removing
// a leftover volume; runtimes without it (remote nodes) skip that check.
type volumeAdmin interface {
	ListNamedVolumes(ctx context.Context) ([]docker.NamedVolume, error)
	RemoveVolume(ctx context.Context, name string) error
}

type volumeResolver interface {
	SetDatabaseVolumeResolved(ctx context.Context, name string) error
}

// waitForDocker reports a Docker that does not answer as a waiting state, not a
// failure: the engine asks for another pass and backs off by itself.
func (c *Controller) waitForDocker(err error) reconcile.Result {
	c.prov.Start(dbprovision.StepDockerCheck, "Waiting for Docker: "+err.Error())
	res := notReady(reasonWaitingForDocker, err)
	res.RequeueAfter = durationFromEnv(EnvDockerWaitInterval, defaultDockerWait)
	return res
}

// settleVolume decides what to do with a data volume that already exists when a
// brand new database is created. stop is true when this pass must end with res.
func (c *Controller) settleVolume(ctx context.Context, desired *store.DesiredDatabase, volName string, fresh bool) (res reconcile.Result, stop bool) {
	if !fresh || desired.VolumeResolved {
		return reconcile.Result{}, false
	}
	adm, ok := c.runtime.(volumeAdmin)
	if !ok {
		return reconcile.Result{}, false
	}
	vols, err := adm.ListNamedVolumes(ctx)
	if err != nil {
		if c.prov != nil {
			return c.waitForDocker(err), true
		}
		return notReady(reasonVolumeFailed, err), true
	}
	var found *docker.NamedVolume
	for i := range vols {
		if vols[i].Name == volName {
			found = &vols[i]
		}
	}
	if found == nil {
		return reconcile.Result{}, false
	}
	switch desired.ExistingVolume {
	case "reuse":
		return reconcile.Result{}, false
	case "discard":
		if found.Mounted {
			return notReady(reasonVolumeFailed, fmt.Errorf("volume %s is still attached to a container; wait for the old database to stop and retry", volName)), true
		}
		if err := adm.RemoveVolume(ctx, volName); err != nil {
			return notReady(reasonVolumeFailed, fmt.Errorf("remove leftover volume %s: %w", volName, err)), true
		}
		return reconcile.Result{}, false
	}
	return notReady(dbprovision.ReasonLeftoverVolume, &dbprovision.LeftoverVolumeError{Name: volName, SizeBytes: found.SizeBytes}), true
}

func (c *Controller) markVolumeResolved(ctx context.Context, desired *store.DesiredDatabase, fresh bool) {
	if !fresh || desired.VolumeResolved {
		return
	}
	if r, ok := c.store.(volumeResolver); ok {
		_ = r.SetDatabaseVolumeResolved(ctx, c.dbName)
	}
}

// WithReadyProbe replaces the TCP connect probe, for tests.
func WithReadyProbe(fn func(ctx context.Context, addr string) error) Option {
	return func(c *Controller) { c.probe = fn }
}

func dialProbe(ctx context.Context, addr string) error {
	d := net.Dialer{Timeout: probeDialTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("connect %s: %w", addr, err)
	}
	return conn.Close()
}

// probeReady checks that the engine port accepts connections on the container
// address. ok is false when the pass must end with res (still starting, or timed out).
func (c *Controller) probeReady(ctx context.Context, state *docker.ContainerState, port int) (res reconcile.Result, ok bool) {
	if c.prov == nil || state == nil {
		return reconcile.Result{}, true
	}
	ip := ""
	for _, n := range state.Networks {
		if n.IPAddress != "" {
			ip = n.IPAddress
			break
		}
	}
	// Docker Desktop does not route container IPs to the host, so a host-side
	// dial can never succeed there.
	if ip == "" || (c.probe == nil && runtime.GOOS != "linux") {
		c.prov.Done(dbprovision.StepWaitReady, "container is running")
		return reconcile.Result{}, true
	}
	probe := c.probe
	if probe == nil {
		probe = dialProbe
	}
	addr := net.JoinHostPort(ip, strconv.Itoa(port))
	err := probe(ctx, addr)
	if err == nil {
		c.prov.Done(dbprovision.StepWaitReady, "accepting connections on port "+strconv.Itoa(port))
		return reconcile.Result{}, true
	}
	if !state.Created.IsZero() && time.Since(state.Created) > durationFromEnv(EnvReadyTimeout, defaultReadyTimeout) {
		return notReady(reasonNotAccepting, fmt.Errorf("the database did not accept connections on port %d in time: %w", port, err)), false
	}
	if res, loop := c.crashLoopResult(ctx, containerName(c.dbName)); loop {
		return res, false
	}
	c.prov.Start(dbprovision.StepWaitReady, "waiting for port "+strconv.Itoa(port)+" to accept connections")
	res = notReady("Starting", errors.New("waiting for the database to accept connections"))
	res.RequeueAfter = durationFromEnv(EnvReadyProbeInterval, defaultReadyProbe)
	return res, false
}

func settingsSummary(d *store.DesiredDatabase) string {
	size := "default size"
	if d.Resources != nil && d.Resources.MemoryBytes > 0 {
		size = fmt.Sprintf("%d MiB memory", d.Resources.MemoryBytes>>20)
	}
	if d.BackupSchedule != "" {
		return size + ", backups " + d.BackupSchedule
	}
	return size + ", no scheduled backups"
}
