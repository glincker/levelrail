package api

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/autoscale"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// appGauge scrapes the app's own metrics path for the policy gauge. Any failure is an unknown reading, never zero.
func (g autoscaleSignals) appGauge(ctx context.Context, p autoscale.Policy) (autoscale.Reading, error) {
	rt := g.rt
	target, _, msg := rt.resolveAppTunnelTarget(ctx, p.App, "")
	if target == nil {
		return autoscale.Reading{Note: msg}, nil
	}
	if target.NodeID != "" && !rt.isLocalNode(target.NodeID) {
		return autoscale.Reading{Note: "gauge scraping reaches only apps on the control plane node"}, nil
	}
	nodeRuntime, err := rt.execRuntime(target.NodeID)
	if err != nil {
		return autoscale.Reading{Note: "the app's node is not reachable"}, nil
	}
	ictx, cancel := context.WithTimeout(ctx, dockerInspectTimeout)
	state, err := nodeRuntime.InspectByName(ictx, application.ContainerName(target.Name, application.NameImage(*target), target.RestartNonce))
	cancel()
	if err != nil || state == nil || !state.Running {
		return autoscale.Reading{Note: "the app has no running container to scrape"}, nil
	}
	ip := ""
	for _, n := range state.Networks {
		if n.IPAddress != "" {
			ip = n.IPAddress
			break
		}
	}
	if ip == "" || target.Port <= 0 {
		return autoscale.Reading{Note: "the app has no reachable address or port to scrape"}, nil
	}
	timeout := g.gaugeTimeout(ctx)
	sctx, scancel := context.WithTimeout(ctx, timeout)
	defer scancel()
	base := "http://" + net.JoinHostPort(ip, strconv.Itoa(target.Port))
	v, err := autoscale.ScrapeGauge(sctx, &http.Client{Timeout: timeout}, base, autoscale.GaugePathFromEnv(os.Getenv), autoscale.GaugeName(p.Metric), g.gaugeMaxBody(ctx))
	if err != nil {
		return autoscale.Reading{Note: err.Error()}, nil
	}
	return autoscale.Reading{Value: v, Known: true}, nil
}

// gaugeTimeout reads the runtime setting; unset or invalid values fall back to the built-in default.
func (g autoscaleSignals) gaugeTimeout(ctx context.Context) time.Duration {
	if rs := g.rt.RuntimeSettings(); rs != nil {
		if d := rs.Duration(ctx, runtimesettings.KeyAutoscaleGaugeTimeout); d > 0 {
			return d
		}
	}
	return autoscale.DefaultGaugeTimeout
}

func (g autoscaleSignals) gaugeMaxBody(ctx context.Context) int64 {
	if rs := g.rt.RuntimeSettings(); rs != nil {
		if n := rs.Limit(ctx, runtimesettings.KeyAutoscaleGaugeMaxBytes); n > 0 {
			return int64(n)
		}
	}
	return autoscale.DefaultGaugeMaxBody
}
