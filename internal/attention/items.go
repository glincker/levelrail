package attention

import (
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Dashboard paths an item links to.
const (
	linkStatus   = "/status"
	linkApps     = "/apps/"
	linkNodes    = "/nodes"
	linkDomains  = "/domains"
	linkApproval = "/approvals"
	linkCLI      = "/settings/cli-access"
	linkTokens   = "/settings/tokens"
	linkUsers    = "/settings/users"
	linkUpdates  = "/settings/updates"
	linkDBs      = "/databases/"
	linkSecurity = "/settings/security"
)

type kindInfo struct {
	title  string
	action string
	link   string
	// linkSubject appends the subject to link.
	linkSubject bool
}

var kindInfos = map[string]kindInfo{
	"disk":                  {"Disk space is low on the data directory", "Free space or grow the disk", linkStatus, false},
	"deploy":                {"Deploy of %s failed", "Open the app and read the deploy error", linkApps, true},
	"app":                   {"%s is not healthy", "Open the app and check its status and logs", linkApps, true},
	"node":                  {"Node %s is offline", "Check the machine and its agent", linkNodes, false},
	"node_cert":             {"Agent certificate of %s needs attention", "Re-enroll the node or fix renewal", linkNodes, false},
	"node_agent":            {"Agent on %s is out of date", "Update the agent on that node", linkNodes, false},
	"node_enroll":           {"Node %s joined but never connected", "Check the agent logs and the network path", linkNodes, false},
	"certificate":           {"Certificate for %s needs attention", "Open Domains and check the certificate", linkDomains, false},
	"cert_renewal":          {"Certificate renewal for %s looks stalled", "Open Domains and read the CA error", linkDomains, false},
	"doctor":                {"Check failed: %s", "Open Status and read the check", linkStatus, false},
	"update":                {"A new version is available", "Review the release and update", linkUpdates, false},
	KindDeviceLogin:         {"CLI login from %s is waiting", "Compare the code and approve or deny it", linkCLI, false},
	KindDeviceLoginResolved: {"CLI login from %s", "Run the CLI login again if you still need it", linkCLI, false},
	KindApproval:            {"Deploy of %s awaits approval", "Open Approvals and approve or reject it", linkApproval, false},
	KindTokenExpiring:       {"API token %s expires soon", "Create a replacement token and revoke this one", linkTokens, false},
	KindTokenExpired:        {"API token %s has expired", "Create a replacement token if it is still in use", linkTokens, false},
	KindDataCopy:            {"Data copy into %s needs attention", "Open the database and retry the copy", linkDBs, true},
	KindBackupOverdue:       {"Backup of %s is overdue", "Open the database and run a backup", linkDBs, true},
	KindInvites:             {"Invitations are waiting", "Open Users and resend or revoke them", linkUsers, false},
	KindLoginCode:           {"Sign-in code requested from %s", "Show the code only if you asked for it", linkSecurity, false},
	KindLoginApproval:       {"New browser sign-in from %s is waiting", "Approve it only if it is you, otherwise deny it", linkSecurity, false},
	KindDBSecurityUpdates:   {"Databases have security updates available", "Open each database's Upgrades tab and apply the patch", linkDBs, false},
	KindDBEOL:               {"%s runs a release past its end of life", "Plan a major upgrade or restore into a supported version", linkDBs, true},
	KindDBUpgradeFailed:     {"Upgrade of %s did not complete", "Open the database's Upgrades tab and read the reason", linkDBs, true},
	KindDockerGuard:         {"Docker API guard needs a decision", "Review the would-be denials and switch the guard to enforce", linkSecurity, false},
}

// newItem builds an item with its stable id, one-line title, next action and
// dashboard link filled in from its kind.
func newItem(severity, kind, subject, detail string) Item {
	it := Item{Severity: severity, Kind: kind, Subject: subject, Detail: detail, ID: kind + ":" + subject}
	decorate(&it)
	return it
}

// NewItem is newItem for callers outside this package, such as the server's
// attention feed.
func NewItem(severity, kind, subject, detail string) Item {
	return newItem(severity, kind, subject, detail)
}

func decorate(it *Item) {
	info, ok := kindInfos[it.Kind]
	if !ok {
		return
	}
	if it.Title == "" {
		it.Title = info.title
		if strings.Contains(info.title, "%s") {
			it.Title = fmt.Sprintf(info.title, it.Subject)
		}
	}
	if it.Action == "" {
		it.Action = info.action
	}
	if it.Link == "" {
		it.Link = info.link
		if info.linkSubject {
			it.Link += it.Subject
		}
	}
}

func fromFeed(f apiclient.AttentionFeedItem) Item {
	it := Item{
		Severity: f.Severity, Kind: f.Kind, Subject: f.Subject, Detail: f.Detail,
		ID: f.ID, Title: f.Title, Action: f.Action, Link: f.Link, Params: f.Params,
	}
	decorate(&it)
	return it
}

// deviceLoginResolvedItems lists recent expired and denied CLI logins as
// informational items, skipping anything dismissed.
func deviceLoginResolvedItems(resolved []apiclient.DeviceActivity, now time.Time) []Item {
	var items []Item
	for _, d := range resolved {
		if d.Dismissed || (d.State != apiclient.DeviceLoginExpired && d.State != apiclient.DeviceLoginDenied) {
			continue
		}
		name := d.ClientName
		if name == "" {
			name = "an unnamed client"
		}
		detail := "a CLI login from " + name
		if d.RequesterIP != "" {
			detail += " (" + d.RequesterIP + ")"
		}
		if d.State == apiclient.DeviceLoginExpired {
			detail += " expired before it was approved"
		} else {
			detail += " was denied"
		}
		detail += ", " + ago(now.Sub(d.ExpiresAt)) + " ago. Run levelrail-cli auth login --device again if you still need it"
		it := newItem(Info, KindDeviceLoginResolved, name, detail)
		it.ID = KindDeviceLoginResolved + ":" + d.ID + ":" + d.State
		it.Params = map[string]string{"state": d.State, "audit_path": d.AuditPath}
		items = append(items, it)
	}
	return items
}

func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	return fmt.Sprintf("%d h", int(d.Hours()))
}
