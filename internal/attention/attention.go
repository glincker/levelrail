// Package attention builds the list of things an operator should look at
// right now, shared by the CLI and the MCP server.
package attention

import (
	"context"
	"fmt"
	"sort"

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
}

// Input is everything Build reads; any field may be zero when its
// endpoint was unavailable.
type Input struct {
	Apps   []apiclient.AppStatusEntry
	Nodes  []apiclient.NodeResource
	Certs  []apiclient.CertificateResource
	Doctor apiclient.SystemDoctorResource
	Failed []apiclient.FailedDeployResource
	Status apiclient.SystemStatusResource
}

func diskItem(s apiclient.SystemStatusResource) (Item, bool) {
	if s.DataDirTotalBytes <= 0 {
		return Item{}, false
	}
	pct := float64(s.DataDirFreeBytes) / float64(s.DataDirTotalBytes) * 100
	detail := fmt.Sprintf("%.1f%% free (%d of %d bytes)", pct, s.DataDirFreeBytes, s.DataDirTotalBytes)
	switch {
	case pct < diskCriticalFreePercent:
		return Item{Critical, "disk", "data dir", detail}, true
	case pct < diskWarnFreePercent:
		return Item{Warning, "disk", "data dir", detail}, true
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
			items = append(items, Item{Critical, "node_cert", n.Name, "agent certificate expired " + expires + ": re-enroll the node"})
		case "critical":
			items = append(items, Item{Critical, "node_cert", n.Name, "agent certificate expires " + expires + ", renewal is failing"})
		case "expiring":
			items = append(items, Item{Warning, "node_cert", n.Name, "agent certificate expires " + expires + ", renewal is failing"})
		case "revoked":
			items = append(items, Item{Warning, "node_cert", n.Name, "agent certificate revoked: re-enroll or delete the node"})
		}
	}
	if a := n.Agent; a != nil && a.Outdated {
		v := a.Version
		if v == "" {
			v = "unknown version"
		}
		items = append(items, Item{Warning, "node_agent", n.Name, "agent " + v + " is older than the minimum " + a.MinVersion})
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
		items = append(items, Item{Critical, "deploy", f.ServiceName, detail})
	}
	for _, a := range in.Apps {
		if a.Status.Variant == "destructive" {
			items = append(items, Item{Critical, "app", a.Name, a.Status.Label})
		}
	}
	for _, n := range in.Nodes {
		if n.Status == "offline" {
			detail := "never reported in"
			if n.LastSeenAt != nil {
				detail = "last seen " + n.LastSeenAt.Format("2006-01-02 15:04 MST")
			}
			items = append(items, Item{Critical, "node", n.Name, detail})
		}
		items = append(items, nodeAgentItems(n)...)
	}
	for _, c := range in.Certs {
		switch c.Status {
		case "expired":
			items = append(items, Item{Critical, "certificate", c.Domain, "expired " + c.NotAfter.Format("2006-01-02")})
		case "expiring_soon":
			items = append(items, Item{Warning, "certificate", c.Domain, "expires " + c.NotAfter.Format("2006-01-02")})
		}
	}
	for _, c := range in.Doctor.Checks {
		switch c.Status {
		case "fail":
			items = append(items, Item{Critical, "doctor", c.Name, c.Message})
		case "warn":
			items = append(items, Item{Warning, "doctor", c.Name, c.Message})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Severity == Critical && items[j].Severity != Critical
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
	return Build(Input{Apps: apps, Nodes: nodes, Certs: certs, Doctor: doctor, Failed: failed, Status: status}), nil
}
