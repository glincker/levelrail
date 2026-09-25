package loadbalancer

import (
	"fmt"
	"strings"
)

// Export formats accepted by Export.
const (
	FormatTerraform      = "terraform"
	FormatCDK            = "cdk"
	FormatCloudFormation = "cloudformation"
	FormatCaddy          = "caddy"
	FormatCaddyJSON      = "caddy-json"
)

// ExportFormats lists every supported format.
func ExportFormats() []string {
	return []string{FormatTerraform, FormatCDK, FormatCloudFormation, FormatCaddy, FormatCaddyJSON}
}

// ExportInput is everything a generator needs about one service.
type ExportInput struct {
	Service string
	Port    int
	Domains []string
	Config  Config
	// Upstreams are the current dial addresses; used by the Caddy formats only.
	Upstreams []string
}

// Artifact is one generated file.
type Artifact struct {
	Format      string   `json:"format"`
	Filename    string   `json:"filename"`
	ContentType string   `json:"content_type"`
	Body        string   `json:"body"`
	Warnings    []string `json:"warnings,omitempty"`
}

// Export renders in as the requested format. It never contacts any cloud API.
func Export(format string, in ExportInput) (Artifact, error) {
	if in.Service == "" {
		return Artifact{}, fmt.Errorf("loadbalancer: export: service name is required")
	}
	if err := in.Config.Validate(); err != nil {
		return Artifact{}, fmt.Errorf("loadbalancer: export: %w", err)
	}
	in.Config = in.Config.Defaults()
	if in.Port == 0 {
		in.Port = 80
	}
	name := sanitizeName(in.Service)
	switch format {
	case FormatTerraform:
		body, warns := renderTerraform(name, in)
		return Artifact{Format: format, Filename: name + "-lb.tf", ContentType: "text/plain", Body: body, Warnings: warns}, nil
	case FormatCDK:
		body, warns := renderCDK(name, in)
		return Artifact{Format: format, Filename: name + "-lb-stack.ts", ContentType: "text/plain", Body: body, Warnings: warns}, nil
	case FormatCloudFormation:
		body, warns := renderCloudFormation(name, in)
		return Artifact{Format: format, Filename: name + "-lb.yaml", ContentType: "text/yaml", Body: body, Warnings: warns}, nil
	case FormatCaddy:
		body, warns := renderCaddyfile(in)
		return Artifact{Format: format, Filename: "Caddyfile", ContentType: "text/plain", Body: body, Warnings: warns}, nil
	case FormatCaddyJSON:
		body, warns, err := renderCaddyJSON(in)
		if err != nil {
			return Artifact{}, err
		}
		return Artifact{Format: format, Filename: name + "-lb.json", ContentType: "application/json", Body: body, Warnings: warns}, nil
	default:
		return Artifact{}, fmt.Errorf("loadbalancer: export: unknown format %q, want one of %s", format, strings.Join(ExportFormats(), ", "))
	}
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "service"
	}
	return out
}

// alb is the AWS Application Load Balancer equivalent of a Config. ALB has no
// ip_hash, uri_hash, retries, passive health or rate limiting, so those map to
// the closest behaviour and a warning.
type alb struct {
	Name, TGName string
	Port         int
	Protocol     string
	Algorithm    string
	Sticky       bool
	StickySecs   int
	HealthPath   string
	Interval     int
	Timeout      int
	Healthy      int
	Unhealthy    int
	Matcher      string
	DrainSecs    int
	SlowSecs     int
	IdleSecs     int
	Warnings     []string
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func secs(s string) int { return int(ParseDuration(s).Seconds()) }

func buildALB(name string, in ExportInput) alb {
	cfg := in.Config
	a := alb{Name: name, TGName: truncate(name, 32), Port: in.Port, Protocol: "HTTP", Algorithm: "round_robin",
		HealthPath: "/", Interval: 30, Timeout: 5, Healthy: 5, Unhealthy: 2, Matcher: "200-399"}
	warn := func(format string, args ...any) { a.Warnings = append(a.Warnings, fmt.Sprintf(format, args...)) }

	switch cfg.Algorithm {
	case AlgoLeastConn:
		a.Algorithm = "least_outstanding_requests"
	case AlgoWeighted:
		a.Algorithm = "weighted_random"
		warn("weighted: ALB target group weights are set on the listener forward action or via weighted_random, per-target weights %v are not exported", cfg.Weights)
	case AlgoCookie:
		a.Sticky, a.StickySecs = true, 86400
	case AlgoIPHash, AlgoURIHash:
		warn("%s has no ALB equivalent, exported as round_robin", cfg.Algorithm)
	}
	if cfg.UpstreamTLS != nil {
		a.Protocol = "HTTPS"
	}
	if h := cfg.ActiveHealth; h != nil {
		a.HealthPath = h.Path
		a.Interval = clamp(secs(h.Interval), 5, 300)
		a.Timeout = clamp(secs(h.Timeout), 2, 120)
		if a.Timeout >= a.Interval {
			a.Timeout = a.Interval - 1
		}
		a.Healthy = clamp(h.Passes, 2, 10)
		a.Unhealthy = clamp(h.Fails, 2, 10)
		if h.ExpectStatus != 0 {
			a.Matcher = fmt.Sprint(h.ExpectStatus)
		}
	}
	a.DrainSecs = -1
	if cfg.DrainTimeout != "" {
		a.DrainSecs = clamp(secs(cfg.DrainTimeout), 0, 3600)
	}
	if cfg.SlowStart != "" {
		a.SlowSecs = clamp(secs(cfg.SlowStart), 30, 900)
		if a.Algorithm == "weighted_random" {
			warn("slow_start is not supported together with weighted_random on ALB, dropped")
			a.SlowSecs = 0
		}
	}
	if cfg.RequestTimeout != "" {
		a.IdleSecs = clamp(secs(cfg.RequestTimeout), 1, 4000)
	}
	if cfg.PassiveHealth != nil {
		warn("passive_health has no ALB equivalent, ALB only uses active health checks")
	}
	if cfg.Retries != nil {
		warn("retries have no ALB equivalent")
	}
	if cfg.RateLimit != nil {
		warn("rate_limit has no ALB equivalent, use an AWS WAF rate-based rule")
	}
	return a
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimRight(s[:n], "-")
}
