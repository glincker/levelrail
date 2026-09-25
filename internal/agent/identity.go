package agent

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/version"
)

// Leaf parses the identity's client certificate.
func (id *Identity) Leaf() (*x509.Certificate, error) {
	return parseCertPEM(id.ClientCertPEM)
}

// Expired reports whether the identity's certificate is past its NotAfter,
// treating an unparseable certificate as expired.
func (id *Identity) Expired(now time.Time) bool {
	leaf, err := id.Leaf()
	return err != nil || !now.Before(leaf.NotAfter)
}

// validateIssued checks a certificate the control plane just issued: it
// names nodeID, is for keyPEM's key, and chains to caPEM for client auth.
func validateIssued(nodeID string, certPEM, keyPEM, caPEM []byte, now time.Time) error {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("agent: issued certificate does not match the local key: %w", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return fmt.Errorf("agent: parse issued certificate: %w", err)
	}
	if nodeID != "" && leaf.Subject.CommonName != nodeID {
		return fmt.Errorf("agent: issued certificate names %q, want %q", leaf.Subject.CommonName, nodeID)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return fmt.Errorf("agent: no valid CA certificate to verify the issued certificate")
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		return fmt.Errorf("agent: issued certificate does not verify against the CA: %w", err)
	}
	return nil
}

// IdentityHolder is the agent's in-memory current identity, swapped after a
// renewal so the next connection presents the new certificate without a
// restart.
type IdentityHolder struct {
	mu sync.RWMutex
	id *Identity
}

// NewIdentityHolder wraps id.
func NewIdentityHolder(id *Identity) *IdentityHolder {
	return &IdentityHolder{id: id}
}

// Current returns the identity in use.
func (h *IdentityHolder) Current() *Identity {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.id
}

// Set replaces the identity in use.
func (h *IdentityHolder) Set(id *Identity) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.id = id
}

// LocalAgentInfo describes this agent binary and the machine it runs on.
func LocalAgentInfo() *agentpb.AgentInfo {
	return &agentpb.AgentInfo{Version: version.Version, Commit: buildCommit(), Os: runtime.GOOS, Arch: runtime.GOARCH}
}

func buildCommit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return ""
}
