package main

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
)

// meshHubSyncer keeps the hub's observed endpoint equal to what the advertise
// host resolves to, so a changed DNS answer reaches agents on the next reconcile.
type meshHubSyncer struct {
	resolver *network.HubHostResolver
	rec      endpointRecorder
	nodeID   string
	logger   *slog.Logger
	last     string
}

// Sync resolves host and records the endpoint when it changed. A failed
// lookup keeps the last good endpoint so a working handshake is never torn down.
func (s *meshHubSyncer) Sync(ctx context.Context, host string) {
	endpoint := ""
	if meshHubHostNeedsIP(host) {
		res, err := s.resolver.Resolve(ctx, host)
		if err != nil {
			s.logger.Warn("mesh hub hostname did not resolve; fix the DNS record or save a public IP as the advertise address",
				slog.String("host", host), slog.String("error", err.Error()), slog.Bool("kept_last_good", res.IP.IsValid()))
		}
		if res.IP.IsValid() {
			endpoint = net.JoinHostPort(res.IP.String(), strconv.Itoa(network.DefaultListenPort))
		}
	} else {
		endpoint = meshHubEndpoint(host, network.DefaultListenPort)
		if endpoint == "" {
			s.logger.Warn("mesh hub endpoint unknown: set the advertise address to this server's public host so agents can reach it on UDP " + strconv.Itoa(network.DefaultListenPort))
		}
	}
	if endpoint == "" && s.last != "" {
		// A cleared host or an expired last-good IP must not leave a stale endpoint behind.
		if err := s.rec.SetObservedEndpoint(s.nodeID, ""); err == nil {
			s.last = ""
		}
		return
	}
	if endpoint == "" || endpoint == s.last {
		return
	}
	if err := s.rec.SetObservedEndpoint(s.nodeID, endpoint); err != nil {
		s.logger.Warn("mesh hub endpoint not recorded", slog.String("endpoint", endpoint), slog.String("error", err.Error()))
		return
	}
	if ap, err := netip.ParseAddrPort(endpoint); err == nil && meshHubHostNeedsIP(host) {
		s.logger.Info("mesh hub hostname resolved for the mesh endpoint", slog.String("host", host), slog.String("ip", ap.Addr().String()))
	}
	s.last = endpoint
}

// runMeshHubRefresh calls refresh every interval until ctx ends. The interval is
// read each round so the runtime setting applies live.
func runMeshHubRefresh(ctx context.Context, logger *slog.Logger, interval func() time.Duration, refresh func(context.Context)) {
	for {
		d := interval()
		if d <= 0 {
			d = time.Minute
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
			guardedHubRefresh(ctx, logger, refresh)
		}
	}
}

// guardedHubRefresh keeps a panic in one refresh from taking the control plane down.
func guardedHubRefresh(ctx context.Context, logger *slog.Logger, refresh func(context.Context)) {
	defer func() {
		if p := recover(); p != nil {
			logger.Error("mesh hub refresh panicked, will retry", slog.Any("panic", p))
		}
	}()
	refresh(ctx)
}
