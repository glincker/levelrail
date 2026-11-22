// Package tui is the interactive terminal UI behind the CLI's "tui"
// command: a live, keyboard-driven view of apps, databases, deployments,
// nodes, domains, alerts and backups over the public HTTP API only.
package tui

import (
	"context"
	"strings"
	"time"
)

// Tone is the dashboard's status colour vocabulary (web/src/components/kit/tone.ts).
type Tone int

// Tones, in the dashboard's own order.
const (
	ToneNeutral Tone = iota
	ToneSuccess
	ToneWarning
	ToneDanger
	ToneInfo
)

// AppRow is one app in the apps view.
type AppRow struct {
	Name     string
	Status   string
	Tone     Tone
	Image    string
	Strategy string
	Node     string
	Replicas int
	Domains  []string
}

// DBRow is one database in the databases view.
type DBRow struct {
	Name     string
	Engine   string
	Version  string
	Node     string
	Schedule string
	Retain   int
	Public   bool
	Stopped  bool
}

// DeployRow is one deployment in the deployments view.
type DeployRow struct {
	ID         string
	App        string
	Status     string
	Trigger    string
	Env        string
	Commit     string
	Branch     string
	StartedAt  time.Time
	DurationMS int64
	IsLive     bool
}

// NodeRow is one node in the nodes view.
type NodeRow struct {
	Name        string
	Address     string
	Status      string
	Schedulable bool
	Local       bool
	LastSeen    time.Time
}

// DomainRow is one domain in the domains view.
type DomainRow struct {
	Domain string
	App    string
	TLS    string
	Kind   string
}

// AttentionRow is one item of the alerts/attention feed.
type AttentionRow struct {
	Severity string
	Kind     string
	Subject  string
	Title    string
	Detail   string
}

// BackupRow is one backup attempt in the backups view.
type BackupRow struct {
	Resource  string
	Kind      string
	Status    string
	SizeBytes int64
	StartedAt time.Time
}

// MeshInfo is the header's mesh summary.
type MeshInfo struct {
	Enabled bool
	Peers   int
	Healthy int
}

// ServerInfo is the header's control plane summary.
type ServerInfo struct {
	Version         string
	UpdateAvailable bool
}

// Section names used as keys of Fast.Errs and Slow.Errs.
const (
	SecApps      = "apps"
	SecDeploys   = "deployments"
	SecAttention = "alerts"
	SecDatabases = "databases"
	SecNodes     = "nodes"
	SecDomains   = "domains"
	SecBackups   = "backups"
	SecMesh      = "mesh"
	SecServer    = "server"
)

// Fast is the data refreshed on every tick. A section present in Errs
// failed and its field is ignored, so stale data survives a blip.
type Fast struct {
	Apps      []AppRow
	Deploys   []DeployRow
	Attention []AttentionRow
	Errs      map[string]string
}

// Slow is the data refreshed every few ticks.
type Slow struct {
	Databases []DBRow
	Nodes     []NodeRow
	Domains   []DomainRow
	Backups   []BackupRow
	Mesh      MeshInfo
	Server    ServerInfo
	Errs      map[string]string
}

// LogKind selects which resource a log stream tails.
type LogKind int

// Log stream sources.
const (
	LogApp LogKind = iota
	LogDatabase
)

// LogLine is one streamed log line.
type LogLine struct {
	Text   string
	Stream string
}

// API is everything the UI needs from the control plane, so tests can fake it.
type API interface {
	Fast(ctx context.Context) Fast
	Slow(ctx context.Context) Slow
	Restart(ctx context.Context, app string) error
	Start(ctx context.Context, app string) error
	Stop(ctx context.Context, app string) error
	Deploy(ctx context.Context, app, image string) error
	Rollback(ctx context.Context, app, deployID string) error
	Scale(ctx context.Context, app string, replicas int) error
	SetEnv(ctx context.Context, app, key, value string) error
	Logs(ctx context.Context, kind LogKind, name string, onLine func(LogLine) error) error
}

// jumpTarget is one object the command bar can jump to by fuzzy name.
type jumpTarget struct {
	view  viewID
	kind  string
	name  string
	verbs []string
}

// fuzzyScore ranks name against query: 0 means no match, higher is better.
// A substring beats a scattered subsequence; shorter names win ties.
func fuzzyScore(query, name string) int {
	q, n := strings.ToLower(query), strings.ToLower(name)
	if q == "" || n == "" {
		return 0
	}
	if n == q {
		return 1000
	}
	if i := strings.Index(n, q); i >= 0 {
		return 500 - i - len(n)
	}
	qi := 0
	qr := []rune(q)
	for _, c := range n {
		if qi < len(qr) && c == qr[qi] {
			qi++
		}
	}
	if qi < len(qr) {
		return 0
	}
	return 100 - len(n)
}

// jumpTargets lists every app, node and domain with the CLI verbs for it.
func (m Model) jumpTargets() []jumpTarget {
	var out []jumpTarget
	for _, a := range m.apps {
		out = append(out, jumpTarget{viewApps, "app", a.Name, []string{"apps restart", "apps scale", "apps stop", "apps start", "apps logs", "apps env"}})
	}
	for _, n := range m.nodes {
		out = append(out, jumpTarget{viewNodes, "node", n.Name, []string{"nodes get", "nodes cordon", "nodes uncordon", "nodes drain", "nodes health"}})
	}
	for _, d := range m.domains {
		out = append(out, jumpTarget{viewDomains, "domain", d.Domain, []string{"apps domains"}})
	}
	return out
}

// bestJump picks the highest scoring target for query.
func (m Model) bestJump(query string) (jumpTarget, bool) {
	best, top := jumpTarget{}, 0
	for _, t := range m.jumpTargets() {
		if sc := fuzzyScore(query, t.name); sc > top {
			best, top = t, sc
		}
	}
	return best, top > 0
}
