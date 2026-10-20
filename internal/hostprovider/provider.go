// Package hostprovider names the cloud or VPS provider this server runs on,
// from DMI strings first and a short link-local metadata probe second, and
// holds the provider-specific steps for opening web ports in a firewall the
// control plane cannot change.
package hostprovider

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ID names a provider.
type ID string

// Provider IDs.
const (
	DigitalOcean ID = "digitalocean"
	AWS          ID = "aws"
	GCP          ID = "gcp"
	Hetzner      ID = "hetzner"
	Azure        ID = "azure"
	Oracle       ID = "oracle"
	Vultr        ID = "vultr"
	Linode       ID = "linode"
)

// Detection sources.
const (
	SourceDMI      = "dmi"
	SourceMetadata = "metadata"
)

// Deps are the host seams; tests replace both.
type Deps struct {
	ReadFile func(path string) ([]byte, error)
	// Get performs one GET and returns the status code and a bounded body.
	Get func(ctx context.Context, url string, header http.Header) (int, []byte, error)
}

// SystemDeps reads the real host and uses a one second metadata timeout.
func SystemDeps() Deps {
	client := &http.Client{Timeout: time.Second}
	return Deps{
		ReadFile: os.ReadFile,
		Get: func(ctx context.Context, url string, header http.Header) (int, []byte, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return 0, nil, err
			}
			req.Header = header
			resp, err := client.Do(req) //nolint:gosec // url is one of the fixed metadata endpoints below
			if err != nil {
				return 0, nil, err
			}
			defer func() { _ = resp.Body.Close() }()
			b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return resp.StatusCode, b, err
		},
	}
}

var dmiFiles = []string{"sys_vendor", "product_name", "board_vendor", "bios_vendor", "chassis_asset_tag", "product_version"}

func dmiText(d Deps) string {
	var parts []string
	for _, f := range dmiFiles {
		if b, err := d.ReadFile("/sys/class/dmi/id/" + f); err == nil {
			parts = append(parts, strings.ToLower(strings.TrimSpace(string(b))))
		}
	}
	return strings.Join(parts, "|")
}

func fromDMI(text string) (ID, bool) {
	rules := []struct {
		needle string
		id     ID
	}{
		{"digitalocean", DigitalOcean}, {"amazon ec2", AWS}, {"amazon", AWS}, {"google", GCP}, {"hetzner", Hetzner},
		{"7783-7084-3265-9085-8269-3286-77", Azure}, {"microsoft corporation", Azure}, {"oraclecloud.com", Oracle},
		{"vultr", Vultr}, {"linode", Linode}, {"akamai", Linode},
	}
	for _, r := range rules {
		if strings.Contains(text, r.needle) {
			return r.id, true
		}
	}
	return "", false
}

type probe struct {
	id     ID
	url    string
	header http.Header
}

func probes() []probe {
	return []probe{
		{DigitalOcean, "http://169.254.169.254/metadata/v1/id", nil},
		{Hetzner, "http://169.254.169.254/hetzner/v1/metadata", nil},
		{GCP, "http://metadata.google.internal/computeMetadata/v1/instance/id", http.Header{"Metadata-Flavor": {"Google"}}},
		{Azure, "http://169.254.169.254/metadata/instance?api-version=2021-02-01", http.Header{"Metadata": {"true"}}},
		{Oracle, "http://169.254.169.254/opc/v2/instance/id", http.Header{"Authorization": {"Bearer Oracle"}}},
		{Vultr, "http://169.254.169.254/v1.json", nil},
		{AWS, "http://169.254.169.254/latest/meta-data/instance-id", nil},
	}
}

// Detect returns the provider guidance for this host, or false when none is recognised.
func Detect(ctx context.Context, d Deps) (Guidance, bool) {
	if id, ok := fromDMI(dmiText(d)); ok {
		return For(id, SourceDMI), true
	}
	for _, p := range probes() {
		pctx, cancel := context.WithTimeout(ctx, time.Second)
		status, body, err := d.Get(pctx, p.url, p.header)
		cancel()
		if err != nil || status != http.StatusOK || len(strings.TrimSpace(string(body))) == 0 {
			continue
		}
		if p.id == Vultr && !strings.Contains(string(body), "instanceid") && !strings.Contains(string(body), "instance-v2-id") {
			continue
		}
		return For(p.id, SourceMetadata), true
	}
	return Guidance{}, false
}
