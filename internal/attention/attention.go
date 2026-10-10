// Package attention builds the list of things an operator should look at
// right now, shared by the CLI and the MCP server.
package attention

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Severity levels, critical first. Info is for resolved items that need no
// action, such as a login that expired.
const (
	Critical = "critical"
	Warning  = "warning"
	Info     = "info"
)

// Disk thresholds mirror web/src/lib/diskPressure.ts.
const (
	diskWarnFreePercent     = 10.0
	diskCriticalFreePercent = 5.0
)

// Item is one thing that needs an operator's attention.
type Item struct {
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Subject  string `json:"subject"`
	Detail   string `json:"detail"`
	// Fixable is set on deploy and app items whose diagnosis carries a
	// one-click fix.
	Fixable bool `json:"fixable,omitempty"`
	// ID is stable across polls for the same condition. Title is one plain
	// line, Action the next step, Link a dashboard path to do it in.
	ID     string            `json:"id"`
	Title  string            `json:"title"`
	Action string            `json:"action"`
	Link   string            `json:"link"`
	Params map[string]string `json:"params,omitempty"`
}

// Input is everything Build reads; any field may be zero when its
// endpoint was unavailable.
type Input struct {
	Apps    []apiclient.AppStatusEntry
	Nodes   []apiclient.NodeResource
	Certs   []apiclient.CertificateResource
	Doctor  apiclient.SystemDoctorResource
	Failed  []apiclient.FailedDeployResource
	Status  apiclient.SystemStatusResource
	Updates apiclient.UpdatesResource
	// Devices are CLI logins waiting for approval, Approvals are pending
	// deploy approvals. Now defaults to time.Now when zero.
	Devices   []apiclient.DevicePendingLogin
	Approvals []apiclient.DeployApprovalResource
	// Resolved are recent expired or denied CLI logins the caller has not
	// dismissed. Feed holds the server-built items for tokens, data copies,
	// backups and invitations, already limited to what the caller may see.
	Resolved []apiclient.DeviceActivity
	Feed     []apiclient.AttentionFeedItem
	Now      time.Time
}

// Item kinds that block on a person, ranked ahead of other items of the
// same severity.
const (
	KindDeviceLogin         = "device_login"
	KindDeviceLoginResolved = "device_login_resolved"
	KindApproval            = "approval"
	KindTokenExpiring       = "token_expiring" //nolint:gosec // item kind name, not a credential
	KindTokenExpired        = "token_expired"  //nolint:gosec // item kind name, not a credential
	KindDataCopy            = "data_copy"
	KindBackupOverdue       = "backup_overdue"
	KindInvites             = "invites"
)

func deviceLoginItems(devices []apiclient.DevicePendingLogin, now time.Time) []Item {
	items := make([]Item, 0, len(devices))
	for _, d := range devices {
		if !d.ExpiresAt.IsZero() && !d.ExpiresAt.After(now) {
			continue
		}
		subject := d.ClientName
		if subject == "" {
			subject = "unnamed client"
		}
		detail := "a CLI login is waiting for approval"
		if d.RequesterIP != "" {
			detail += " from " + d.RequesterIP
		}
		if !d.ExpiresAt.IsZero() {
			detail += fmt.Sprintf(", expires in %d min", int(d.ExpiresAt.Sub(now).Minutes())+1)
		}
		detail += ". A signed-in operator must approve it in the dashboard under Settings > CLI access"
		it := newItem(Warning, KindDeviceLogin, subject, detail)
		it.ID += ":" + strconv.FormatInt(d.CreatedAt.Unix(), 10)
		items = append(items, it)
	}
	return items
}

func approvalItems(approvals []apiclient.DeployApprovalResource) []Item {
	items := make([]Item, 0, len(approvals))
	for _, a := range approvals {
		by := a.RequestedByName
		if by == "" {
			by = a.RequestedBy
		}
		items = append(items, newItem(Warning, KindApproval, a.ServiceName, a.Action+" of "+a.Image+" requested by "+by+" awaits approval"))
	}
	return items
}

// severityRank orders critical, then warning, then info.
func severityRank(s string) int {
	switch s {
	case Critical:
		return 0
	case Info:
		return 2
	}
	return 1
}

// acmeReason appends the CA's failure reason to a certificate detail.
func acmeReason(c apiclient.CertificateResource) string {
	if c.ACMEFailure == nil || c.ACMEFailure.Reason == "" {
		return ""
	}
	return ". Last CA error: " + c.ACMEFailure.Reason
}

// kindRank sorts items that wait on a person ahead of the rest.
func kindRank(kind string) int {
	if kind == KindDeviceLogin || kind == KindApproval {
		return 0
	}
	return 1
}

func diskItem(s apiclient.SystemStatusResource) (Item, bool) {
	if s.DataDirTotalBytes <= 0 {
		return Item{}, false
	}
	pct := float64(s.DataDirFreeBytes) / float64(s.DataDirTotalBytes) * 100
	detail := fmt.Sprintf("%.1f%% free (%d of %d bytes)", pct, s.DataDirFreeBytes, s.DataDirTotalBytes)
	switch {
	case pct < diskCriticalFreePercent:
		return newItem(Critical, "disk", "data dir", detail), true
	case pct < diskWarnFreePercent:
		return newItem(Warning, "disk", "data dir", detail), true
	}
	return Item{}, false
}

// nodeAgentItems flags a node's agent certificate (ADR 021) and an agent
// older than the control plane's minimum version.
func nodeAgentItems(n apiclient.NodeResource) []Item {
	var items []Item
	if c := n.Cert; c != nil {
		expires := ""
		if c.NotAfter != nil {
			expires = c.NotAfter.Format("2006-01-02")
		}
		switch c.State {
		case "expired":
			items = append(items, newItem(Critical, "node_cert", n.Name, "agent certificate expired "+expires+": re-enroll the node"))
		case "critical":
			items = append(items, newItem(Critical, "node_cert", n.Name, "agent certificate expires "+expires+", renewal is failing"))
		case "expiring":
			items = append(items, newItem(Warning, "node_cert", n.Name, "agent certificate expires "+expires+", renewal is failing"))
		case "revoked":
			items = append(items, newItem(Warning, "node_cert", n.Name, "agent certificate revoked: re-enroll or delete the node"))
		}
	}
	if a := n.Agent; a != nil && a.Outdated {
		v := a.Version
		if v == "" {
			v = "unknown version"
		}
		items = append(items, newItem(Warning, "node_agent", n.Name, "agent "+v+" is older than the minimum "+a.MinVersion))
	}
	return items
}

// Build mirrors web/src/lib/attention.ts: disk pressure, failing apps,
// recent failed deploys, offline nodes, bad certificates, and doctor
// warnings or failures, critical first.
func Build(in Input) []Item {
	items := []Item{}
	if it, ok := diskItem(in.Status); ok {
		items = append(items, it)
	}
	for _, f := range in.Failed {
		detail := f.Error
		if detail == "" {
			detail = "deploy failed"
		}
		items = append(items, newItem(Critical, "deploy", f.ServiceName, detail))
	}
	for _, a := range in.Apps {
		if a.Status.Variant == "destructive" {
			items = append(items, newItem(Critical, "app", a.Name, a.Status.Label))
		}
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	items = append(items, deviceLoginItems(in.Devices, now)...)
	items = append(items, approvalItems(in.Approvals)...)
	items = append(items, deviceLoginResolvedItems(in.Resolved, now)...)
	for _, f := range in.Feed {
		items = append(items, fromFeed(f))
	}
	for _, n := range in.Nodes {
		if n.StatusReason == "enrolled_never_connected" {
			items = append(items, newItem(Warning, "node_enroll", n.Name, "joined but never connected: check the agent's logs and that it can reach the control plane"))
		}
		if n.Status == "offline" {
			detail := "never reported in"
			if n.LastSeenAt != nil {
				detail = "last seen " + n.LastSeenAt.Format("2006-01-02 15:04 MST")
			}
			items = append(items, newItem(Critical, "node", n.Name, detail))
		}
		items = append(items, nodeAgentItems(n)...)
	}
	for _, c := range in.Certs {
		switch c.Status {
		case "expired":
			items = append(items, newItem(Critical, "certificate", c.Domain, "expired "+c.NotAfter.Format("2006-01-02")+acmeReason(c)))
		case "expiring_soon":
			items = append(items, newItem(Warning, "certificate", c.Domain, "expires "+c.NotAfter.Format("2006-01-02")+acmeReason(c)))
		}
		if c.Renewal == "stalled" && c.Status != "expired" {
			items = append(items, newItem(Warning, "cert_renewal", c.Domain, "renewal looks stalled, certificate expires "+c.NotAfter.Format("2006-01-02")+acmeReason(c)))
		}
	}
	for _, c := range in.Doctor.Checks {
		switch c.Status {
		case "fail":
			items = append(items, newItem(Critical, "doctor", c.Name, c.Message))
		case "warn":
			items = append(items, newItem(Warning, "doctor", c.Name, c.Message))
		}
	}
	if in.Updates.UpdateAvailable {
		latest := "a newer release"
		if in.Updates.LatestVersion != nil {
			latest = *in.Updates.LatestVersion
		}
		items = append(items, newItem(Warning, "update", "control plane", latest+" is available (running "+in.Updates.CurrentVersion+")"))
	}
	sort.SliceStable(items, func(i, j int) bool {
		si, sj := severityRank(items[i].Severity), severityRank(items[j].Severity)
		if si != sj {
			return si < sj
		}
		return kindRank(items[i].Kind) < kindRank(items[j].Kind)
	})
	return items
}

// Collect gathers every source the caller's token may read and returns the
// attention items. A source that fails or is forbidden drops only its own
// items; it fails only when no source answered at all.
func Collect(ctx context.Context, client *apiclient.Client) ([]Item, error) {
	var in Input
	answered := 0
	var firstErr error
	note := func(what string, err error) {
		if err == nil {
			answered++
		} else if firstErr == nil {
			firstErr = fmt.Errorf("%s: %w", what, err)
		}
	}
	var err error
	in.Apps, err = client.ListAppStatuses(ctx)
	note("list apps", err)
	in.Nodes, err = client.ListNodes(ctx)
	note("list nodes", err)
	in.Certs, err = client.ListCertificates(ctx)
	note("list certificates", err)
	in.Doctor, err = client.GetSystemDoctor(ctx)
	note("get system doctor report", err)
	in.Feed, err = client.GetAttentionFeed(ctx)
	note("get attention feed", err)
	if answered == 0 {
		return nil, firstErr
	}
	in.Failed, _ = client.ListFailedDeploys(ctx, "24h")
	in.Status, _ = client.GetSystemStatus(ctx)
	in.Updates, _ = client.GetUpdates(ctx)
	in.Approvals, _ = client.ListDeployApprovals(ctx, "pending", "")
	if activity, aerr := client.ListDeviceActivity(ctx); aerr == nil {
		in.Devices, in.Resolved = splitActivity(activity)
	} else {
		in.Devices, _ = client.ListPendingDeviceLogins(ctx)
	}
	items := Build(in)
	markFixable(ctx, client, items)
	return items, nil
}

func splitActivity(activity []apiclient.DeviceActivity) (waiting []apiclient.DevicePendingLogin, resolved []apiclient.DeviceActivity) {
	for _, a := range activity {
		if a.State == apiclient.DeviceLoginWaiting {
			waiting = append(waiting, apiclient.DevicePendingLogin{
				ClientName: a.ClientName, RequesterIP: a.RequesterIP, UserAgent: a.UserAgent,
				CreatedAt: a.CreatedAt, ExpiresAt: a.ExpiresAt,
			})
			continue
		}
		resolved = append(resolved, a)
	}
	return waiting, resolved
}

const (
	envDiagnoseLimit     = "APP_ATTENTION_DIAGNOSE_LIMIT"
	defaultDiagnoseLimit = 10
)

// markFixable flags deploy and app items whose diagnosis has a patch fix.
// It asks about at most APP_ATTENTION_DIAGNOSE_LIMIT distinct apps, and a
// failed lookup just leaves the item unflagged.
func markFixable(ctx context.Context, client *apiclient.Client, items []Item) {
	limit := defaultDiagnoseLimit
	if n, err := strconv.Atoi(os.Getenv(envDiagnoseLimit)); err == nil && n >= 0 {
		limit = n
	}
	fixable := map[string]bool{}
	asked := map[string]bool{}
	for i := range items {
		it := &items[i]
		if it.Kind != "deploy" && it.Kind != "app" {
			continue
		}
		if !asked[it.Subject] {
			if len(asked) >= limit {
				continue
			}
			asked[it.Subject] = true
			if d, err := client.DiagnoseApp(ctx, it.Subject, ""); err == nil {
				fixable[it.Subject] = d.Fixable
			}
		}
		it.Fixable = fixable[it.Subject]
	}
}
