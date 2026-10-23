package migrateverify

import (
	"fmt"
	"strconv"
	"time"
)

// Thresholds classify probe results. All come from env, with the defaults above.
type Thresholds struct {
	TLSWarnDays    int
	LatencyFactor  float64
	LatencyFloorMS int64
}

// ThresholdsFromEnv reads the thresholds through lookup.
func ThresholdsFromEnv(lookup func(string) (string, bool)) Thresholds {
	t := Thresholds{TLSWarnDays: defaultTLSWarnDays, LatencyFactor: defaultLatencyFactor, LatencyFloorMS: defaultLatencyFloorMS}
	if v, ok := lookup(EnvTLSWarnDays); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			t.TLSWarnDays = n
		}
	}
	if v, ok := lookup(EnvLatencyFactor); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 1 {
			t.LatencyFactor = f
		}
	}
	if v, ok := lookup(EnvLatencyFloorMS); ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			t.LatencyFloorMS = n
		}
	}
	return t
}

// ProbeOptionsFromEnv reads the probe timeout and concurrency through lookup.
func ProbeOptionsFromEnv(lookup func(string) (string, bool)) ProbeOptions {
	o := ProbeOptions{Timeout: DefaultProbeTimeout, Concurrency: DefaultProbeConcurrency}
	if v, ok := lookup(EnvProbeTimeout); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			o.Timeout = d
		}
	}
	if v, ok := lookup(EnvProbeConcurrency); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			o.Concurrency = n
		}
	}
	return ClampProbeOptions(o)
}

const probeFixReach = "check the app is running and routed here; probe the ingress directly with --target-ip"

// CompareProbes classifies the live probe results of every domain.
func CompareProbes(probes []DomainProbe, th Thresholds) []Difference {
	var out []Difference
	add := func(p DomainProbe, sev Severity, detail, fix string) {
		out = append(out, Difference{Category: CatProbes, Severity: sev, App: p.App, Subject: p.Domain, Detail: detail, Fix: fix})
	}
	for _, p := range probes {
		s, t := p.Source, p.Target
		switch {
		case t.Error != "" && s.Error != "":
			add(p, Warning, "neither side answered: "+t.Error, "check the domain's DNS and that the app is running")
			continue
		case t.Error != "":
			add(p, Blocking, "this instance did not answer: "+t.Error, probeFixReach)
			continue
		case s.Error != "":
			add(p, Info, "the source did not answer, nothing to compare: "+s.Error, "nothing to do if the source is already stopped")
			continue
		}
		if s.Status != t.Status {
			sev := Warning
			if t.Status >= 500 || (s.Status < 400 && t.Status >= 400) {
				sev = Blocking
			}
			add(p, sev, fmt.Sprintf("status differs (source %d, here %d)", s.Status, t.Status), "check the app logs and health on this instance")
		}
		if s.TLS && !t.TLS {
			add(p, Warning, "source serves https, this instance answered over plain http", "check the certificate was issued for the domain")
		}
		if t.TLS && !t.TLSValid {
			sev := Blocking
			if p.Staged {
				sev = Warning
			}
			add(p, sev, "certificate is not valid: "+t.TLSError, "issue the certificate (DNS challenge before cutover, or after DNS moves)")
		} else if t.TLS && t.TLSDaysLeft < th.TLSWarnDays {
			add(p, Warning, fmt.Sprintf("certificate expires in %d days", t.TLSDaysLeft), "renew the certificate before cutover")
		}
		if s.RedirectTarget != t.RedirectTarget {
			add(p, Warning, fmt.Sprintf("redirect differs (source %q, here %q)", s.RedirectTarget, t.RedirectTarget), "match the redirect in the domain settings")
		}
		if t.LatencyMS > th.LatencyFloorMS && float64(t.LatencyMS) > th.LatencyFactor*float64(s.LatencyMS) {
			add(p, Warning, fmt.Sprintf("slower here (source %d ms, here %d ms)", s.LatencyMS, t.LatencyMS), "check resources and cold starts on this instance")
		}
	}
	return out
}
