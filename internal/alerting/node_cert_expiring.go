package alerting

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Defaults for the node certificate thresholds. Healthy agents renew with
// a third of a 90 day lifetime left, so a certificate inside 21 days means
// renewal has been failing for over a week.
const (
	DefaultNodeCertWarning  = 21 * 24 * time.Hour
	DefaultNodeCertCritical = 7 * 24 * time.Hour
)

// NodeCertState summarizes a node's agent certificate.
type NodeCertState string

// Node certificate states, from healthy to unusable.
const (
	NodeCertOK       NodeCertState = "ok"
	NodeCertExpiring NodeCertState = "expiring"
	NodeCertCritical NodeCertState = "critical"
	NodeCertExpired  NodeCertState = "expired"
	NodeCertRevoked  NodeCertState = "revoked"
	NodeCertUnknown  NodeCertState = "unknown"
)

// ClassifyNodeCert returns n's certificate state at now and the time left
// before it expires (negative once expired).
func ClassifyNodeCert(n store.Node, warning, critical time.Duration, now time.Time) (NodeCertState, time.Duration) {
	if n.CertRevokedAt != nil {
		return NodeCertRevoked, 0
	}
	if n.CertNotAfter == nil {
		return NodeCertUnknown, 0
	}
	left := n.CertNotAfter.Sub(now)
	switch {
	case left <= 0:
		return NodeCertExpired, left
	case left <= critical:
		return NodeCertCritical, left
	case left <= warning:
		return NodeCertExpiring, left
	}
	return NodeCertOK, left
}

// EvaluateNodeCertExpiring runs one KindNodeCertExpiring rule: it fires
// while any node's agent certificate is inside the warning window (the
// rule's ForDuration, else warning) or already expired. Revoked nodes are
// left out: that was an operator's decision, not a failure. LastValue is
// the fewest days left across the flagged nodes.
func EvaluateNodeCertExpiring(ctx context.Context, nodes NodeSource, r Rule, warning, critical time.Duration, now time.Time) (Rule, []string, error) {
	if r.ForDuration > 0 {
		warning = r.ForDuration
	}
	all, err := nodes.ListNodes(ctx)
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: list nodes: %w", r.ID, err)
	}

	var notices []string
	var minLeft *time.Duration
	for _, n := range all {
		state, left := ClassifyNodeCert(n, warning, critical, now)
		if state != NodeCertExpiring && state != NodeCertCritical && state != NodeCertExpired {
			continue
		}
		if minLeft == nil || left < *minLeft {
			l := left
			minLeft = &l
		}
		notices = append(notices, nodeCertNotice(n, state, left))
	}

	next := r
	next.LastEvaluatedAt = &now
	if minLeft != nil {
		v := minLeft.Hours() / 24
		next.LastValue = &v
	} else {
		next.LastValue = nil
	}
	return advanceState(next, r, len(notices) > 0, 0, now), notices, nil
}

func nodeCertNotice(n store.Node, state NodeCertState, left time.Duration) string {
	name := n.Name
	if name == "" {
		name = n.ID
	}
	if state == NodeCertExpired {
		return fmt.Sprintf("%s: agent certificate expired %s ago, re-enroll the node", name, (-left).Round(time.Hour))
	}
	return fmt.Sprintf("%s: agent certificate expires in %s, renewal is not succeeding", name, left.Round(time.Hour))
}
