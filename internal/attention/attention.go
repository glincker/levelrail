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

// Severity levels, critical first.
const (
	Critical = "critical"
	Warning  = "warning"
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
	Now       time.Time
}

// Item kinds that block on a person, ranked ahead of other items of the
// same severity.
const (
	KindDeviceLogin = "device_login"
	KindApproval    = "approval"
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
		items = append(items, Item{Warning, KindDeviceLogin, subject, detail, false})
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
		items = append(items, Item{Warning, KindApproval, a.ServiceName, a.Action + " of " + a.Image + " requested by " + by + " awaits approval", false})
	}
	return items
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
		return Item{Critical, "disk", "data dir", detail, false}, true
	case pct < diskWarnFreePercent:
		return Item{Warning, "disk", "data dir", detail, false}, true
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
			items = append(items, Item{Critical, "node_cert", n.Name, "agent certificate expired " + expires + ": re-enroll the node", false})
		case "critical":
			items = append(items, Item{Critical, "node_cert", n.Name, "agent certificate expires " + expires + ", renewal is failing", false})
		case "expiring":
			items = append(items, Item{Warning, "node_cert", n.Name, "agent certificate expires " + expires + ", renewal is failing", false})
		case "revoked":
			items = append(items, Item{Warning, "node_cert", n.Name, "agent certificate revoked: re-enroll or delete the node", false})
		}
	}
	if a := n.Agent; a != nil && a.Outdated {
		v := a.Version
		if v == "" {
			v = "unknown version"
		}
		items = append(items, Item{Warning, "node_agent", n.Name, "agent " + v + " is older than the minimum " + a.MinVersion, false})
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
		items = append(items, Item{Critical, "deploy", f.ServiceName, detail, false})
	}
	for _, a := range in.Apps {
		if a.Status.Variant == "destructive" {
			items = append(items, Item{Critical, "app", a.Name, a.Status.Label, false})
		}
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	items = append(items, deviceLoginItems(in.Devices, now)...)
	items = append(items, approvalItems(in.Approvals)...)
	for _, n := range in.Nodes {
		if n.StatusReason == "enrolled_never_connected" {
			items = append(items, Item{Warning, "node_enroll", n.Name, "joined but never connected: check the agent's logs and that it can reach the control plane", false})
		}
		if n.Status == "offline" {
			detail := "never reported in"
			if n.LastSeenAt != nil {
				detail = "last seen " + n.LastSeenAt.Format("2006-01-02 15:04 MST")
			}
			items = append(items, Item{Critical, "node", n.Name, detail, false})
		}
		items = append(items, nodeAgentItems(n)...)
	}
	for _, c := range in.Certs {
		switch c.Status {
		case "expired":
			items = append(items, Item{Critical, "certificate", c.Domain, "expired " + c.NotAfter.Format("2006-01-02"), false})
		case "expiring_soon":
			items = append(items, Item{Warning, "certificate", c.Domain, "expires " + c.NotAfter.Format("2006-01-02"), false})
		}
		if c.Renewal == "stalled" && c.Status != "expired" {
			items = append(items, Item{Warning, "cert_renewal", c.Domain, "renewal looks stalled, certificate expires " + c.NotAfter.Format("2006-01-02"), false})
		}
	}
	for _, c := range in.Doctor.Checks {
		switch c.Status {
		case "fail":
			items = append(items, Item{Critical, "doctor", c.Name, c.Message, false})
		case "warn":
			items = append(items, Item{Warning, "doctor", c.Name, c.Message, false})
		}
	}
	if in.Updates.UpdateAvailable {
		latest := "a newer release"
		if in.Updates.LatestVersion != nil {
			latest = *in.Updates.LatestVersion
		}
		items = append(items, Item{Warning, "update", "control plane", latest + " is available (running " + in.Updates.CurrentVersion + ")", false})
	}
	sort.SliceStable(items, func(i, j int) bool {
		ci, cj := items[i].Severity == Critical, items[j].Severity == Critical
		if ci != cj {
			return ci
		}
		return kindRank(items[i].Kind) < kindRank(items[j].Kind)
	})
	return items
}

// Collect fetches app statuses, nodes, certificates and the doctor report
// (required) plus failed deploys and system status (optional: a failure
// drops only its own items) and returns the attention items.
func Collect(ctx context.Context, client *apiclient.Client) ([]Item, error) {
	apps, err := client.ListAppStatuses(ctx)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	nodes, err := client.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	certs, err := client.ListCertificates(ctx)
	if err != nil {
		return nil, fmt.Errorf("list certificates: %w", err)
	}
	doctor, err := client.GetSystemDoctor(ctx)
	if err != nil {
		return nil, fmt.Errorf("get system doctor report: %w", err)
	}
	failed, _ := client.ListFailedDeploys(ctx, "24h")
	status, _ := client.GetSystemStatus(ctx)
	updates, _ := client.GetUpdates(ctx)
	devices, _ := client.ListPendingDeviceLogins(ctx)
	approvals, _ := client.ListDeployApprovals(ctx, "pending", "")
	items := Build(Input{Apps: apps, Nodes: nodes, Certs: certs, Doctor: doctor, Failed: failed, Status: status, Updates: updates, Devices: devices, Approvals: approvals})
	markFixable(ctx, client, items)
	return items, nil
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
