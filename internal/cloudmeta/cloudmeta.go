// Package cloudmeta reads the provider, region and zone a machine runs in
// from the link-local metadata service of the cloud it sits on (AWS IMDSv2,
// Google Cloud, Azure, Hetzner, DigitalOcean). Detection is best effort: a
// machine with no metadata service (a home server) yields no result and the
// caller falls back to labels typed by the operator.
package cloudmeta

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/hostprovider"
)

// Environment variables read by FromEnv.
const (
	EnvTimeout  = "APP_AGENT_METADATA_TIMEOUT"
	EnvDisable  = "APP_AGENT_METADATA_DISABLE"
	EnvProvider = "APP_NODE_PROVIDER"
	EnvRegion   = "APP_NODE_REGION"
	EnvZone     = "APP_NODE_ZONE"
)

// Sources of a detected Info.
const (
	SourceMetadata = "metadata"
	SourceEnv      = "env"
)

const (
	defaultTimeout = time.Second
	maxBody        = 16 << 10
	linkLocal      = "http://169.254.169.254"
	gcpHost        = "http://metadata.google.internal"
)

// Info is where a machine runs. Empty fields were not reported.
type Info struct {
	Provider   string
	Region     string
	Zone       string
	InstanceID string
	Source     string
}

// Found reports whether anything was detected.
func (i Info) Found() bool { return i.Provider != "" || i.Region != "" || i.Zone != "" }

// Bases are the metadata service roots, overridable so tests can point each
// provider at a fake server.
type Bases struct {
	AWS, GCP, Azure, Hetzner, DigitalOcean string
}

// DefaultBases are the real link-local endpoints.
func DefaultBases() Bases {
	return Bases{AWS: linkLocal, GCP: gcpHost, Azure: linkLocal, Hetzner: linkLocal, DigitalOcean: linkLocal}
}

// Config drives Detect.
type Config struct {
	Client  *http.Client
	Bases   Bases
	Timeout time.Duration
	// Override wins over anything detected and is how an operator labels a
	// machine that has no metadata service.
	Override Info
	Disabled bool
}

// FromEnv builds a Config from the process environment.
func FromEnv(lookup func(string) (string, bool)) Config {
	cfg := Config{Bases: DefaultBases(), Timeout: defaultTimeout}
	if v, ok := lookup(EnvTimeout); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Timeout = d
		}
	}
	if v, ok := lookup(EnvDisable); ok && (v == "1" || strings.EqualFold(v, "true")) {
		cfg.Disabled = true
	}
	get := func(k string) string { v, _ := lookup(k); return LabelValue(v) }
	cfg.Override = Info{Provider: get(EnvProvider), Region: get(EnvRegion), Zone: get(EnvZone)}
	return cfg
}

// SystemConfig is FromEnv over os.LookupEnv.
func SystemConfig() Config { return FromEnv(os.LookupEnv) }

var invalidLabel = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// LabelValue makes s a valid node label value, or "" when nothing usable remains.
func LabelValue(s string) string {
	s = strings.Trim(invalidLabel.ReplaceAllString(strings.TrimSpace(s), "-"), "-._")
	if len(s) > 63 {
		s = strings.Trim(s[:63], "-._")
	}
	return s
}

type prober struct {
	client  *http.Client
	timeout time.Duration
}

func (p prober) do(ctx context.Context, method, url string, header map[string]string) (int, http.Header, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := p.client.Do(req) //nolint:gosec // url is built from a fixed metadata base
	if err != nil {
		return 0, nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	return resp.StatusCode, resp.Header, b, err
}

func (p prober) text(ctx context.Context, url string, header map[string]string) (string, bool) {
	code, _, b, err := p.do(ctx, http.MethodGet, url, header)
	if err != nil || code != http.StatusOK {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// Detect applies the operator override on top of the first provider whose
// metadata service answers. Providers are probed concurrently but resolved in
// a fixed order, so a host that answers on more than one gives a stable result.
func Detect(ctx context.Context, cfg Config) Info {
	var found Info
	if !cfg.Disabled {
		found = probeAll(ctx, cfg)
	}
	return merge(found, cfg.Override)
}

func merge(found, override Info) Info {
	out := found
	applied := false
	set := func(dst *string, v string) {
		if v != "" {
			*dst, applied = v, true
		}
	}
	set(&out.Provider, override.Provider)
	set(&out.Region, override.Region)
	set(&out.Zone, override.Zone)
	if applied {
		out.Source = SourceEnv
	}
	return out
}

func probeAll(ctx context.Context, cfg Config) Info {
	p := prober{client: cfg.Client, timeout: cfg.Timeout}
	if p.client == nil {
		p.client = &http.Client{}
	}
	if p.timeout <= 0 {
		p.timeout = defaultTimeout
	}
	b := cfg.Bases
	fns := []func(context.Context, prober, string) (Info, bool){probeAWS, probeGCP, probeAzure, probeHetzner, probeDigitalOcean}
	bases := []string{b.AWS, b.GCP, b.Azure, b.Hetzner, b.DigitalOcean}
	results := make([]Info, len(fns))
	oks := make([]bool, len(fns))
	var wg sync.WaitGroup
	for i := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], oks[i] = fns[i](ctx, p, strings.TrimRight(bases[i], "/"))
		}()
	}
	wg.Wait()
	for i, ok := range oks {
		if ok {
			results[i].Source = SourceMetadata
			return results[i]
		}
	}
	return Info{}
}

func probeAWS(ctx context.Context, p prober, base string) (Info, bool) {
	code, _, tok, err := p.do(ctx, http.MethodPut, base+"/latest/api/token", map[string]string{"X-aws-ec2-metadata-token-ttl-seconds": "60"})
	if err != nil || code != http.StatusOK || len(tok) == 0 {
		return Info{}, false
	}
	h := map[string]string{"X-aws-ec2-metadata-token": strings.TrimSpace(string(tok))}
	zone, ok := p.text(ctx, base+"/latest/meta-data/placement/availability-zone", h)
	if !ok {
		return Info{}, false
	}
	region, _ := p.text(ctx, base+"/latest/meta-data/placement/region", h)
	if region == "" {
		region = strings.TrimRight(zone, "abcdefghijklmnopqrstuvwxyz")
	}
	id, _ := p.text(ctx, base+"/latest/meta-data/instance-id", h)
	return Info{Provider: string(hostprovider.AWS), Region: LabelValue(region), Zone: LabelValue(zone), InstanceID: id}, true
}

func probeGCP(ctx context.Context, p prober, base string) (Info, bool) {
	code, hdr, b, err := p.do(ctx, http.MethodGet, base+"/computeMetadata/v1/instance/zone", map[string]string{"Metadata-Flavor": "Google"})
	if err != nil || code != http.StatusOK || hdr.Get("Metadata-Flavor") != "Google" {
		return Info{}, false
	}
	zone := lastSegment(strings.TrimSpace(string(b)))
	if zone == "" {
		return Info{}, false
	}
	region := zone
	if i := strings.LastIndex(zone, "-"); i > 0 {
		region = zone[:i]
	}
	id, _ := p.text(ctx, base+"/computeMetadata/v1/instance/id", map[string]string{"Metadata-Flavor": "Google"})
	return Info{Provider: string(hostprovider.GCP), Region: LabelValue(region), Zone: LabelValue(zone), InstanceID: id}, true
}

func probeAzure(ctx context.Context, p prober, base string) (Info, bool) {
	code, _, b, err := p.do(ctx, http.MethodGet, base+"/metadata/instance/compute?api-version=2021-02-01&format=json", map[string]string{"Metadata": "true"})
	if err != nil || code != http.StatusOK {
		return Info{}, false
	}
	var doc struct {
		Location string `json:"location"`
		Zone     string `json:"zone"`
		VMID     string `json:"vmId"`
	}
	if json.Unmarshal(b, &doc) != nil || doc.Location == "" {
		return Info{}, false
	}
	zone := ""
	if doc.Zone != "" {
		zone = doc.Location + "-" + doc.Zone
	}
	return Info{Provider: string(hostprovider.Azure), Region: LabelValue(doc.Location), Zone: LabelValue(zone), InstanceID: doc.VMID}, true
}

func probeHetzner(ctx context.Context, p prober, base string) (Info, bool) {
	id, ok := p.text(ctx, base+"/hetzner/v1/metadata/instance-id", nil)
	if !ok || id == "" {
		return Info{}, false
	}
	region, _ := p.text(ctx, base+"/hetzner/v1/metadata/region", nil)
	zone, _ := p.text(ctx, base+"/hetzner/v1/metadata/availability-zone", nil)
	return Info{Provider: string(hostprovider.Hetzner), Region: LabelValue(region), Zone: LabelValue(zone), InstanceID: id}, true
}

// DigitalOcean has regions but no zones.
func probeDigitalOcean(ctx context.Context, p prober, base string) (Info, bool) {
	region, ok := p.text(ctx, base+"/metadata/v1/region", nil)
	if !ok || region == "" {
		return Info{}, false
	}
	id, _ := p.text(ctx, base+"/metadata/v1/id", nil)
	return Info{Provider: string(hostprovider.DigitalOcean), Region: LabelValue(region), InstanceID: id}, true
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}
