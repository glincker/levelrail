package lbexport

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/loadbalancer"
)

func exportUpstreams(in ExportInput) []loadbalancer.Upstream {
	dials := in.Upstreams
	if len(dials) == 0 {
		dials = []string{"localhost:" + strconv.Itoa(in.Port)}
	}
	ups := make([]loadbalancer.Upstream, len(dials))
	for i, d := range dials {
		ups[i] = loadbalancer.Upstream{ID: loadbalancer.UpstreamID(in.Service, i), Dial: d, Replica: i}
	}
	return ups
}

func renderCaddyfile(in ExportInput) (string, []string) {
	cfg := in.Config
	var warns []string
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	if cfg.RateLimit != nil {
		w("{\n\torder rate_limit before basic_auth\n}\n\n")
		warns = append(warns, "rate_limit needs a Caddy build that includes github.com/mholt/caddy-ratelimit")
	}
	site := ":80"
	if len(in.Domains) > 0 {
		site = strings.Join(in.Domains, ", ")
	}
	ups := exportUpstreams(in)
	dials := make([]string, len(ups))
	for i, u := range ups {
		dials[i] = u.Dial
	}
	w("%s {\n", site)
	if rl := cfg.RateLimit; rl != nil {
		w("\trate_limit {\n\t\tzone sustained {\n\t\t\tkey {remote_host}\n\t\t\tevents %d\n\t\t\twindow 10s\n\t\t}\n", rl.RPS*10)
		if rl.Burst > rl.RPS {
			w("\t\tzone burst {\n\t\t\tkey {remote_host}\n\t\t\tevents %d\n\t\t\twindow 1s\n\t\t}\n", rl.Burst)
		}
		w("\t}\n")
	}
	w("\treverse_proxy %s {\n", strings.Join(dials, " "))
	switch cfg.Algorithm {
	case loadbalancer.AlgoLeastConn:
		w("\t\tlb_policy least_conn\n")
	case loadbalancer.AlgoIPHash:
		w("\t\tlb_policy ip_hash\n")
	case loadbalancer.AlgoURIHash:
		w("\t\tlb_policy uri_hash\n")
	case loadbalancer.AlgoCookie:
		w("\t\tlb_policy cookie %s\n", cfg.CookieName)
	case loadbalancer.AlgoWeighted:
		weights := loadbalancer.EffectiveWeights(cfg, ups, nil, time.Time{})
		parts := make([]string, len(weights))
		for i, x := range weights {
			parts[i] = strconv.Itoa(x)
		}
		w("\t\tlb_policy weighted_round_robin %s\n", strings.Join(parts, " "))
	}
	if r := cfg.Retries; r != nil {
		w("\t\tlb_retries %d\n\t\tlb_try_duration %s\n", r.Count, r.TryDuration)
		if r.TryInterval != "" {
			w("\t\tlb_try_interval %s\n", r.TryInterval)
		}
	}
	if h := cfg.ActiveHealth; h != nil {
		w("\t\thealth_uri %s\n\t\thealth_interval %s\n\t\thealth_timeout %s\n\t\thealth_passes %d\n\t\thealth_fails %d\n", h.Path, h.Interval, h.Timeout, h.Passes, h.Fails)
		if h.ExpectStatus != 0 {
			w("\t\thealth_status %d\n", h.ExpectStatus)
		}
	}
	if p := cfg.PassiveHealth; p != nil {
		w("\t\tfail_duration %s\n\t\tmax_fails %d\n", p.FailDuration, p.MaxFails)
	}
	if cfg.DrainTimeout != "" {
		w("\t\tstream_close_delay %s\n", cfg.DrainTimeout)
	}
	if cfg.UpstreamTLS != nil || cfg.RequestTimeout != "" {
		w("\t\ttransport http {\n")
		if t := cfg.UpstreamTLS; t != nil {
			w("\t\t\ttls\n")
			if t.InsecureSkipVerify {
				w("\t\t\ttls_insecure_skip_verify\n")
			}
			if t.ServerName != "" {
				w("\t\t\ttls_server_name %s\n", t.ServerName)
			}
		}
		if cfg.RequestTimeout != "" {
			w("\t\t\tresponse_header_timeout %s\n", cfg.RequestTimeout)
		}
		w("\t\t}\n")
	}
	w("\t}\n}\n")
	if cfg.SlowStart != "" {
		warns = append(warns, "slow_start is applied by the platform reconciler and has no Caddyfile equivalent")
	}
	return b.String(), warns
}

func renderCaddyJSON(in ExportInput) (string, []string, error) {
	ups := exportUpstreams(in)
	weights := loadbalancer.EffectiveWeights(in.Config, ups, nil, time.Time{})
	lb := ingress.NewLBRoute(in.Config, ups, weights)
	hosts := in.Domains
	if len(hosts) == 0 {
		hosts = []string{"example.com"}
	}
	route := ingress.Route{
		Match:  []ingress.Matcher{{Host: hosts}},
		Handle: []any{ingress.NewLBReverseProxyHandler(lb)},
	}
	raw, err := json.MarshalIndent(route, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("loadbalancer: export caddy json: %w", err)
	}
	var warns []string
	if in.Config.SlowStart != "" {
		warns = append(warns, "slow_start is applied by the platform reconciler and has no Caddy JSON equivalent")
	}
	if in.Config.RateLimit != nil {
		warns = append(warns, "rate_limit is omitted from the route JSON, add a rate_limit handler ahead of reverse_proxy")
	}
	return string(raw) + "\n", warns, nil
}
