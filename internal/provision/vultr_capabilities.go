package provision

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

const vultrCapProbeTimeout = 8 * time.Second

// vultrKeyNeeds is the one requirement Vultr has: its keys are not scoped,
// so access is the key plus an access control list that admits this server.
const vultrKeyNeeds = "an API key whose access control list allows this server"

type vultrCapEntry struct {
	id, group, path string
	usedBy          []string
}

// vultrCapabilityTable lists what a Vultr key can do. Reads are probed with
// GET only, writes stay unknown until a real write teaches the result.
var vultrCapabilityTable = []vultrCapEntry{
	{"account.read", "account", "/account", []string{"account status"}},
	{"regions.read", "compute", "/regions?per_page=1", []string{"provision form"}},
	{"sizes.read", "compute", "/plans?per_page=1", []string{"provision form", "price guard"}},
	{"instances.read", "compute", "/instances?per_page=1", []string{"provision status"}},
	{"instances.create", "compute", "", []string{"provision server"}},
	{"instances.delete", "compute", "", []string{"destroy server"}},
}

func vultrCapabilityScope(id string) (string, bool) {
	for _, e := range vultrCapabilityTable {
		if e.id == id {
			return vultrKeyNeeds, true
		}
	}
	return "", false
}

// ProbeCapabilities runs one read-only GET per read capability, at most
// concurrency at once. A 401 or 403 is missing, a 2xx is granted, anything
// else stays unknown and never reads as missing.
func (v *Vultr) ProbeCapabilities(ctx context.Context, concurrency int) ProviderCapabilities {
	if concurrency < 1 {
		concurrency = 1
	}
	now := time.Now().UTC()
	out := ProviderCapabilities{
		Provider: "vultr", AccountID: "vultr-default", Method: "probe",
		CheckedAt: now, Capabilities: make([]Capability, len(vultrCapabilityTable)),
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := false
	for i, e := range vultrCapabilityTable {
		c := Capability{ID: e.id, Group: e.group, Needs: vultrKeyNeeds, UsedBy: e.usedBy, ObservedAt: now,
			State: CapabilityUnknown, Evidence: EvidenceNotProbed}
		if e.path == "" {
			out.Capabilities[i] = c
			continue
		}
		wg.Add(1)
		go func(i int, e vultrCapEntry, c Capability) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pctx, cancel := context.WithTimeout(ctx, vultrCapProbeTimeout)
			defer cancel()
			err := v.client.do(pctx, http.MethodGet, e.path, nil, nil)
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
