package attention

import (
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const keyOriginReenroll = "reenroll_required"

// nodeReenrollItem flags a node whose key an early beta made on the control
// plane. Severity follows the time left to expiry using the node's own
// warning and critical windows.
func nodeReenrollItem(n apiclient.NodeResource, now time.Time) Item {
	c := n.Cert
	sev, class := Info, ClassRecommendation
	detail := "Its key was made by the control plane in an early beta. It cannot renew its certificate."
	if c.NotAfter != nil {
		left := c.NotAfter.Sub(now)
		date := c.NotAfter.Format("2006-01-02")
		switch {
		case left <= 0:
			sev, class = Critical, ClassIncident
			detail = "Its key was made by the control plane in an early beta. Its certificate expired on " + date + " and it cannot renew."
		default:
			detail = "Its key was made by the control plane in an early beta. It keeps working until its certificate expires on " + date + ", then it cannot renew."
			switch {
			case left <= days(c.CriticalDays):
				sev, class = Critical, ClassIncident
			case left <= days(c.WarningDays):
				sev, class = Warning, ClassIncident
			}
		}
	}
	it := newItem(sev, KindNodeReenroll, n.Name, detail)
	it.ID = KindNodeReenroll + ":" + n.ID
	it.Class = class
	it.Link = linkNodes + "/" + n.ID + "?reenroll=1"
	it.Params = map[string]string{"node_id": n.ID}
	if c.NotAfter != nil {
		it.Params["expires_at"] = c.NotAfter.UTC().Format(time.RFC3339)
	}
	return it
}

func days(d int) time.Duration { return time.Duration(d) * 24 * time.Hour }

// renewalDetail appends the agent's last renewal error and next retry, both
// bounded and stripped of anything that looks like key material.
func renewalDetail(r *apiclient.NodeCertRenewal) string {
	if r == nil || r.LastError == "" {
		return ""
	}
	out := ". Last error: " + SafeErrorText(r.LastError)
	if r.NextRetryAt != nil {
		out += ". Next retry " + r.NextRetryAt.UTC().Format("2006-01-02 15:04 MST")
	}
	return out
}

// nodeFailureItem flags a node the failure policy has acted on or is waiting on.
func nodeFailureItem(n apiclient.NodeResource, f *apiclient.NodeFailureSummary) Item {
	detail := "unreachable, " + strconv.Itoa(f.Moved) + " resource(s) moved to healthy nodes"
	sev := Warning
	if f.NeedsYou > 0 {
		sev = Critical
		detail += ", " + strconv.Itoa(f.NeedsYou) + " with node-local data need your decision"
	}
	it := newItem(sev, KindNodeFailure, n.Name, detail)
	it.ID = KindNodeFailure + ":" + n.ID
	it.Link = linkNodes + "/" + n.ID
	it.Params = map[string]string{"node_id": n.ID}
	return it
}
