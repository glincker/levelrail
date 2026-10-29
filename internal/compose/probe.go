package compose

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// resolveServiceHealth prefers a declared x-levelrail-healthcheck over the
// compose healthcheck:, so a template can keep its docker-compose check for
// plain Compose users while this platform probes from outside.
func resolveServiceHealth(serviceKey string, hc *Healthcheck, pp *spec.PlatformProbe, mainPort int) (*composeHealth, string, error) {
	if pp == nil {
		return resolveHealthcheck(serviceKey, hc)
	}
	h, err := resolvePlatformProbe(serviceKey, pp, mainPort)
	if err != nil {
		return nil, "", err
	}
	return h, "", nil
}

// resolvePlatformProbe maps pp onto a readiness probe against the
// service's published port. Redirects are never followed, so a probe can
// not be bounced to another host; a 3xx is judged by expect_status.
func resolvePlatformProbe(serviceKey string, pp *spec.PlatformProbe, mainPort int) (*composeHealth, error) {
	d, err := pp.Validate()
	if err != nil {
		return nil, fmt.Errorf("service %q: x-levelrail-healthcheck: %w", serviceKey, err)
	}
	if mainPort == 0 {
		return nil, fmt.Errorf("service %q: x-levelrail-healthcheck needs a tcp port under ports: to probe", serviceKey)
	}
	if pp.Port != 0 && pp.Port != mainPort {
		return nil, fmt.Errorf("service %q: x-levelrail-healthcheck port %d is not the published port %d; only the first tcp port is reachable from outside", serviceKey, pp.Port, mainPort)
	}

	p := store.ServiceProbe{
		Platform: true,
		Interval: d.Interval,
		Timeout:  d.Timeout,
		Failures: pp.Retries,
	}
	if pp.Kind == spec.PlatformProbeTCP {
		p.TCP = true
	} else {
		noFollow := false
		p.Path, p.ExpectedStatus, p.FollowRedirects = pp.ProbePath(), string(pp.ExpectStatus), &noFollow
	}
	out := &composeHealth{ServiceProbe: p}
	if d.StartPeriod > 0 {
		out.ReadyTimeout = d.StartPeriod + d.Interval*time.Duration(max(pp.Retries, 1))
	}
	return out, nil
}
