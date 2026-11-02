package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Parked domain DNS states.
const (
	parkedDNSPointing    = "pointing"
	parkedDNSNotPointing = "not_pointing"
	parkedDNSUnknown     = "unknown"
)

// Parked domain certificate states.
const (
	parkedTLSIssued  = "issued"
	parkedTLSPending = "pending"
	parkedTLSNone    = "none"
)

// parkedLookupConcurrency bounds simultaneous DNS checks so a long list stays polite to resolvers.
const parkedLookupConcurrency = 8

// parkedLookupTimeout is a variable only so tests can shrink it.
var parkedLookupTimeout = domainCheckLookupTimeout

type parkedDNSState struct {
	Status    string `json:"status"`
	CheckedAt string `json:"checked_at,omitempty"`
}

type parkedTLSState struct {
	State    string     `json:"state"`
	NotAfter *time.Time `json:"not_after,omitempty"`
	DaysLeft *int       `json:"days_left,omitempty"`
}

// parkedDomainState is the live DNS and certificate picture of one parked domain.
type parkedDomainState struct {
	DNS parkedDNSState `json:"dns"`
	TLS parkedTLSState `json:"tls"`
}

// parkedDNSFromCheck maps the shared domain check verdict to the parked wording.
func parkedDNSFromCheck(status string) string {
	switch status {
	case domainCheckStatusConnected, domainCheckStatusPropagating:
		return parkedDNSPointing
	case domainCheckStatusNotResolving, domainCheckStatusResolvesElsewhere:
		return parkedDNSNotPointing
	default:
		return parkedDNSUnknown
	}
}

// parkedStateFor looks up DNS and certificate state for one domain. A lookup
// that overruns parkedLookupTimeout reports unknown instead of blocking.
func (rt *Router) parkedStateFor(ctx context.Context, r *http.Request, domain string) parkedDomainState {
	st := parkedDomainState{
		DNS: parkedDNSState{Status: parkedDNSUnknown, CheckedAt: time.Now().UTC().Format(time.RFC3339)},
		TLS: parkedTLSState{State: parkedTLSNone},
	}
	if cert, err := rt.domainCertStatus(ctx, domain); err == nil {
		st.TLS = parkedTLSFromCert(cert)
	}

	expected, inferred := advertisedHost(r, rt.advertisedPublicHost(ctx))
	dctx, cancel := context.WithTimeout(ctx, parkedLookupTimeout)
	defer cancel()
	done := make(chan domainCheckResponse, 1)
	go func() { done <- rt.runDomainCheckOpts(dctx, domain, expected, inferred, false) }()
	select {
	case resp := <-done:
		st.DNS.Status = parkedDNSFromCheck(resp.Status)
	case <-dctx.Done():
	}
	return st
}

func parkedTLSFromCert(c domainCertStatusResource) parkedTLSState {
	out := parkedTLSState{State: parkedTLSNone}
	switch {
	case c.NotAfter != nil && c.DaysLeft != nil:
		// An expired certificate stays "issued" with negative days_left so the UI can say so.
		out.State, out.NotAfter, out.DaysLeft = parkedTLSIssued, c.NotAfter, c.DaysLeft
	case c.State == certFlowPending:
		out.State = parkedTLSPending
	}
	return out
}

// parkedStates resolves every domain concurrently, keyed by domain.
func (rt *Router) parkedStates(ctx context.Context, r *http.Request, domains []string) map[string]parkedDomainState {
	out := make(map[string]parkedDomainState, len(domains))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, parkedLookupConcurrency)
	for _, d := range domains {
		d = strings.ToLower(d)
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			st := rt.parkedStateFor(ctx, r, d)
			mu.Lock()
			out[d] = st
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}
