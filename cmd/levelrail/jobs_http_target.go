package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/scheduledtask"
	"github.com/GLINCKER/levelrail/internal/store"
)

// jobHTTPPoll is how often a waking app is probed for its published port.
const jobHTTPPoll = 250 * time.Millisecond

type jobSleepStore interface {
	appsleep.Apps
	SetAppSleeping(ctx context.Context, serviceName string, sleeping bool, now time.Time) error
	GetAppSleep(ctx context.Context, serviceName string) (store.AppSleep, error)
}

type jobNodeResolver interface {
	UpstreamHost(ctx context.Context, nodeID string) (docker.Runtime, string, error)
}

// jobHTTPTarget wakes a sleeping app and finds the host port its first replica
// publishes, holding the caller (bounded by ctx) until it accepts connections.
type jobHTTPTarget struct {
	db     jobSleepStore
	nudger appsleep.Nudger
	local  docker.Runtime
	nodes  jobNodeResolver
}

var _ scheduledtask.HTTPTarget = jobHTTPTarget{}

// Prepare implements scheduledtask.HTTPTarget.
func (t jobHTTPTarget) Prepare(ctx context.Context, svc *store.DesiredService) (string, error) {
	if _, err := appsleep.Wake(ctx, t.db, t.db, t.nudger, svc.Name, time.Now()); err != nil {
		return "", fmt.Errorf("wake %q: %w", svc.Name, err)
	}
	rt, host := t.local, "127.0.0.1"
	if svc.NodeID != "" && t.nodes != nil {
		var err error
		if rt, host, err = t.nodes.UpstreamHost(ctx, svc.NodeID); err != nil {
			return "", err
		}
	}
	name := application.ReplicaContainerName(svc.Name, application.NameImage(*svc), svc.RestartNonce, 0)

	tick := time.NewTicker(jobHTTPPoll)
	defer tick.Stop()
	var last error
	for {
		addr, err := publishedAddr(ctx, rt, name, host)
		if err == nil {
			d := net.Dialer{Timeout: time.Second}
			conn, derr := d.DialContext(ctx, "tcp", addr)
			if derr == nil {
				_ = conn.Close()
				return "http://" + addr, nil
			}
			err = derr
		}
		last = err
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("%q not reachable: %w", svc.Name, errors.Join(ctx.Err(), last))
		case <-tick.C:
		}
	}
}

func publishedAddr(ctx context.Context, rt docker.Runtime, container, host string) (string, error) {
	state, err := rt.InspectByName(ctx, container)
	if err != nil {
		return "", fmt.Errorf("inspect %q: %w", container, err)
	}
	if state == nil || !state.Running {
		return "", errors.New("container not running")
	}
	if len(state.Ports) == 0 {
		return "", errors.New("container publishes no port")
	}
	return net.JoinHostPort(host, strconv.Itoa(state.Ports[0].HostPort)), nil
}
