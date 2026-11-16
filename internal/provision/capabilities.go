package provision

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Capability states. granted needs evidence, missing needs a 401 or 403 for
// that exact probe, unknown covers everything a safe read cannot prove.
const (
	CapabilityGranted = "granted"
	CapabilityMissing = "missing"
	CapabilityUnknown = "unknown"
)

// Evidence values say how a state was reached.
const (
	EvidenceProbeOK    = "probe_ok"
	EvidenceProbe403   = "probe_403"
	EvidenceProbe404   = "probe_404"
	EvidenceLearnedOK  = "learned_ok"
	Evidence403Learned = "learned_403"
	EvidenceNotProbed  = "not_probed"
	EvidenceProbeError = "probe_error"
)

// Capability is one provider permission as the control plane understands it.
type Capability struct {
	ID         string    `json:"id"`
	Group      string    `json:"group"`
	State      string    `json:"state"`
	Evidence   string    `json:"evidence"`
	Needs      string    `json:"needs"`
	UsedBy     []string  `json:"used_by"`
	ObservedAt time.Time `json:"observed_at"`
}

// ProviderCapabilities is the provider-neutral result of a capability check.
type ProviderCapabilities struct {
	Provider       string       `json:"provider"`
	AccountID      string       `json:"account_id"`
	Method         string       `json:"method"`
	CheckedAt      time.Time    `json:"checked_at"`
	Cached         bool         `json:"cached"`
	NextCheckAt    *time.Time   `json:"next_check_at,omitempty"`
	LiveCheckError string       `json:"live_check_error,omitempty"`
	Capabilities   []Capability `json:"capabilities"`
}

// CapabilityProber is optional on a provisioner: only providers with a
// safe read-only way to check a credential implement it.
type CapabilityProber interface {
	ProbeCapabilities(ctx context.Context, concurrency int) ProviderCapabilities
}

// ScopeMissingError reports a provider 401 or 403 mapped to the scope the
// operation needs. It carries fixed text only, never the provider body.
type ScopeMissingError struct {
	Provider   string
	Capability string
	Needs      string
}

func (e *ScopeMissingError) Error() string {
	return "provision: " + e.Provider + " token needs scope " + e.Needs
}

// ScopeFromError returns a ScopeMissingError when err is a provider 401 or
// 403 for a known capability of provider, so callers can say which scope to
// add instead of a bare 403.
func ScopeFromError(provider, capabilityID string, err error) (*ScopeMissingError, bool) {
	var pe *ProviderError
	if !errors.As(err, &pe) || (pe.Status != http.StatusForbidden && pe.Status != http.StatusUnauthorized) {
		return nil, false
	}
	needs, ok := capabilityScope(provider, capabilityID)
	if !ok {
		return nil, false
	}
	return &ScopeMissingError{Provider: provider, Capability: capabilityID, Needs: needs}, true
}

func capabilityScope(provider, id string) (string, bool) {
	if provider != "digitalocean" {
		return "", false
	}
	for _, p := range digitaloceanCapabilityTable {
		if p.id == id {
			return p.needs, true
		}
	}
	return "", false
}
