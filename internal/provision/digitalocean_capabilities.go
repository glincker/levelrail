package provision

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

const digitaloceanCapProbeTimeout = 8 * time.Second

// doCapEntry is one capability row. path is empty for writes: a GET cannot
// prove them, so they stay unknown until a real write teaches the result.
type doCapEntry struct {
	id, group, needs string
	path             string
	usedBy           []string
}

// digitaloceanCapabilityTable lists the scopes by DigitalOcean custom scope
// name. Reads are probed with per_page=1, writes are never probed.
var digitaloceanCapabilityTable = []doCapEntry{
	{"regions.read", "compute", "regions:read", "/regions?per_page=1", []string{"provision form"}},
	{"sizes.read", "compute", "sizes:read", "/sizes?per_page=1", []string{"provision form", "price guard"}},
	{"instances.read", "compute", "droplet:read", "/droplets?per_page=1", []string{"provision status", "discover droplets"}},
	{"instances.create", "compute", "droplet:create", "", []string{"provision server"}},
	{"instances.delete", "compute", "droplet:delete", "", []string{"destroy server"}},
	{"instances.power", "compute", "droplet:update", "", []string{"power actions"}},
	{"ssh_keys.read", "compute", "ssh_key:read", "/account/keys?per_page=1", []string{"ssh key listing"}},
	{"tags.write", "compute", "tag:create", "", []string{"managed droplet tags"}},
	{"firewalls.read", "network", "firewall:read", "/firewalls?per_page=1", []string{"firewall view"}},
	{"networks.read", "network", "vpc:read", "/vpcs?per_page=1", []string{"network view"}},
	{"volumes.read", "storage", "block_storage:read", "/volumes?per_page=1", []string{"volume view"}},
	{"projects.read", "account", "project:read", "/projects?per_page=1", []string{"project grouping"}},
	{"kubernetes.read", "kubernetes", "kubernetes:read", "/kubernetes/clusters?per_page=1", []string{"cluster connect"}},
	{"account.read", "account", "account:read", "/account", []string{"account status"}},
	{"billing.balance", "billing", "billing:read", "/customers/my/balance", []string{"spend and budget"}},
}

// ProbeCapabilities runs one read-only GET per read capability, at most
// concurrency at once. A 401 or 403 is missing, a 2xx is granted, anything
// else (404, 5xx, network) stays unknown and never reads as missing.
func (d *DigitalOcean) ProbeCapabilities(ctx context.Context, concurrency int) ProviderCapabilities {
	if concurrency < 1 {
		concurrency = 1
	}
	now := time.Now().UTC()
	out := ProviderCapabilities{
		Provider: "digitalocean", AccountID: "digitalocean-default", Method: "probe",
		CheckedAt: now, Capabilities: make([]Capability, len(digitaloceanCapabilityTable)),
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := false
	for i, e := range digitaloceanCapabilityTable {
		c := Capability{ID: e.id, Group: e.group, Needs: e.needs, UsedBy: e.usedBy, ObservedAt: now,
			State: CapabilityUnknown, Evidence: EvidenceNotProbed}
		if e.path == "" {
			out.Capabilities[i] = c
			continue
		}
		wg.Add(1)
		go func(i int, e doCapEntry, c Capability) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pctx, cancel := context.WithTimeout(ctx, digitaloceanCapProbeTimeout)
			defer cancel()
			err := d.client.do(pctx, http.MethodGet, e.path, nil, nil)
			var pe *ProviderError
			switch {
			case err == nil:
				c.State, c.Evidence = CapabilityGranted, EvidenceProbeOK
			case errors.As(err, &pe) && (pe.Status == http.StatusForbidden || pe.Status == http.StatusUnauthorized):
				c.State, c.Evidence = CapabilityMissing, EvidenceProbe403
			case errors.As(err, &pe) && pe.Status == http.StatusNotFound:
				c.Evidence = EvidenceProbe404
			default:
				c.Evidence = EvidenceProbeError
				mu.Lock()
				failed = true
				mu.Unlock()
			}
			out.Capabilities[i] = c
		}(i, e, c)
	}
	wg.Wait()
	if failed {
		out.LiveCheckError = "some probes could not reach the provider"
	}
	return out
}
