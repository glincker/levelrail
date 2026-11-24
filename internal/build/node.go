package build

// This file: the node-selection piece that routes a deploy's build step
// to a build-capable node, using the same "" == local-node convention
// migrations/0009_node_placement.sql established for service and
// database placement. Router (router.go) is what acts on the answer.

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// NodeInfo is the node shape build routing needs. Deliberately not
// store.Node: internal/build has no dependency on internal/store, the
// same consumer-defined interface convention the other package
// boundaries follow.
type NodeInfo struct {
	// ID is the node's identifier, matching store.Node.ID. "" is the
	// control plane's own node, listed for display but never a candidate.
	ID   string
	Name string
	// Local marks the control plane's own node.
	Local bool
	// AcceptsBuildWorkloads mirrors store.Node.AcceptsBuildWorkloads.
	AcceptsBuildWorkloads bool
	// Online reports whether the node is reachable through the agent
	// transport right now.
	Online bool
	// Draining is true for a cordoned node: it takes no new build work.
	Draining bool
	// LastSeen is the last heartbeat, zero when unknown.
	LastSeen time.Time
	// Arch is "os/arch" such as linux/amd64, empty when the agent has not reported it.
	Arch   string
	Labels map[string]string
	// Pressure is the host pressure level (ok, warn, critical, unknown).
	Pressure   string
	CPUCores   int
	SlotsUsed  int
	SlotsTotal int
	Waiting    int
	// CacheBytes is the node's build cache size when known.
	CacheBytes int64
	// LastBuildAt is when the node last finished a build, zero when never.
	LastBuildAt time.Time
	// Trust is the builder tier (server, managed, personal); empty counts as server.
	Trust string
	// DiskFreeBytes is the free disk the node last reported, zero when unknown.
	DiskFreeBytes int64
	// OwnerID is the user a personal builder belongs to, TeamID the team it serves; empty when unrecorded.
	OwnerID string
	TeamID  string
}

// Pressure levels a NodeInfo reports.
const (
	PressureOK       = "ok"
	PressureWarn     = "warn"
	PressureCritical = "critical"
	PressureUnknown  = "unknown"
)

// Strategy is how a build node is chosen among the usable ones.
type Strategy string

// Strategies. First keeps the original behavior: the usable node with the smallest ID.
const (
	StrategyFirst       Strategy = "first"
	StrategyLeastLoaded Strategy = "least_loaded"
	StrategySticky      Strategy = "sticky_per_app"
)

// ParseStrategy maps a stored value to a Strategy, defaulting to First.
func ParseStrategy(v string) Strategy {
	switch Strategy(v) {
	case StrategyLeastLoaded, StrategySticky:
		return Strategy(v)
	}
	return StrategyFirst
}

// Policy carries the live routing settings.
type Policy struct {
	Strategy Strategy
	// PressureHold skips nodes under critical host pressure (not used by StrategyFirst).
	PressureHold bool
	// StaleAfter is the heartbeat age past which a node is skipped; zero disables the check.
	StaleAfter time.Duration
	// MinDiskFreeBytes skips a node reporting less free disk than this; zero disables the gate.
	MinDiskFreeBytes int64
	Now              time.Time
	// PersonalInProduction lets personal builders build production environments that have no explicit allow-list.
	PersonalInProduction bool
	// PersonalScope is ScopeOwner, ScopeTeam or ScopeAll; empty means owner.
	PersonalScope string
}

// Personal builder scopes: whose apps a personal builder may receive.
const (
	ScopeOwner = "owner"
	ScopeTeam  = "team"
	ScopeAll   = "all"
)

// Build placement modes an app can choose.
const (
	ModeAuto         = "auto"
	ModeControlPlane = "control_plane"
	ModeNode         = "node"
)

// Query describes the build being placed.
type Query struct {
	App string
	// Mode control_plane keeps the build on the control plane, whatever runners exist.
	Mode string
	// TargetArch is the "os/arch" the image will run on, empty when unknown.
	TargetArch string
	// Labels every chosen runner must carry; an empty value only requires the key.
	Labels map[string]string
	// Preferred is a soft preference, Pinned a hard one: a pinned runner that is
	// unusable sends the build to the control plane, never to another runner.
	Preferred string
	Pinned    string
	// LastNode is the runner that built this app last, used by StrategySticky.
	LastNode string
	// EnvKind is the kind of the app's environment, "production" restricts personal builders.
	EnvKind string
	// AllowedBuilders is the environment's explicit allow-list of tiers or node IDs; it beats the default.
	AllowedBuilders []string
	// AppTeamID and AppTeamMembers say who the app belongs to, for the personal builder scope.
	AppTeamID      string
	AppTeamMembers []string
}

// Builder trust tiers, mirroring the store values.
const (
	TrustServer   = "server"
	TrustManaged  = "managed"
	TrustPersonal = "personal"
)

func (n NodeInfo) tier() string {
	if n.Trust == "" {
		return TrustServer
	}
	return n.Trust
}

// trustAllows reports whether n may build for the query's environment. An explicit
// allow-list decides alone; otherwise personal builders are kept off production.
func (q Query) trustAllows(n NodeInfo, p Policy) bool {
	if len(q.AllowedBuilders) > 0 {
		for _, a := range q.AllowedBuilders {
			if a == n.ID {
				return true
			}
			// A tier word admits the tier, but a personal builder still keeps to its scope.
			if a == n.tier() {
				return n.tier() != TrustPersonal || q.personalScopeAllows(n, p.PersonalScope)
			}
		}
		return false
	}
	if q.EnvKind == "" {
		// No known environment (unknown app): only server builders qualify.
		return n.tier() == TrustServer
	}
	if n.tier() == TrustPersonal {
		if q.EnvKind == "production" && !p.PersonalInProduction {
			return false
		}
		return q.personalScopeAllows(n, p.PersonalScope)
	}
	return true
}

// personalScopeAllows keeps a personal builder to its owner's or team's apps; an unrecorded owner fails closed.
func (q Query) personalScopeAllows(n NodeInfo, scope string) bool {
	if scope == ScopeAll {
		return true
	}
	if n.OwnerID != "" && slices.Contains(q.AppTeamMembers, n.OwnerID) {
		return true
	}
	return scope == ScopeTeam && n.TeamID != "" && n.TeamID == q.AppTeamID
}

// Decision is where a build runs and why.
type Decision struct {
	// Node is "" for the control plane.
	Node string
	// Trust is the chosen node's builder tier, empty for the control plane.
	Trust  string
	Reason string
}

// ErrNoBuildNodeAvailable is no longer returned by SelectBuildNode: the
// build-node-routing initiative made falling back to the local node the
// default instead (SelectBuildNode's own doc comment). Kept as an
// exported sentinel in case anything outside this package still
// references it.
var ErrNoBuildNodeAvailable = errors.New("build: no build-capable node is currently online")

// SelectBuildNode picks which node a build should run on, given every
// known node. AcceptsBuildWorkloads is a routing preference, not a hard
// requirement: a build is never failed just because the node an operator.
func SelectBuildNode(nodes []NodeInfo) string {
	candidates := make([]NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		if n.AcceptsBuildWorkloads {
			candidates = append(candidates, n)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	for _, n := range candidates {
		if n.Online && !n.Draining {
			return n.ID
		}
	}
	return ""
}

func (n NodeInfo) usable(p Policy) bool {
	if !n.AcceptsBuildWorkloads || n.Local || n.ID == "" || !n.Online || n.Draining {
		return false
	}
	if p.StaleAfter > 0 && !n.LastSeen.IsZero() && !p.Now.IsZero() && p.Now.Sub(n.LastSeen) > p.StaleAfter {
		return false
	}
	return n.diskGate(p) == ""
}

// diskGate names why a node is held back for low disk, empty when it passes or the disk is unknown.
func (n NodeInfo) diskGate(p Policy) string {
	if p.MinDiskFreeBytes > 0 && n.DiskFreeBytes > 0 && n.DiskFreeBytes < p.MinDiskFreeBytes {
		return fmt.Sprintf("disk low: %d MiB free, needs %d MiB", n.DiskFreeBytes>>20, p.MinDiskFreeBytes>>20)
	}
	return ""
}

// SkipReason says why routing skips a runner, empty when it is usable now.
func (n NodeInfo) SkipReason(p Policy) string {
	if n.Local || n.ID == "" || !n.AcceptsBuildWorkloads {
		return ""
	}
	if !n.usable(p) {
		if why := n.diskGate(p); why != "" {
			return why
		}
		return unusableWhy(n)
	}
	return ""
}

func (n NodeInfo) hasLabels(want map[string]string) bool {
	for k, v := range want {
		got, ok := n.Labels[k]
		if !ok || (v != "" && got != v) {
			return false
		}
	}
	return true
}

func (n NodeInfo) freeSlots() int {
	if n.SlotsTotal <= 0 {
		return 1
	}
	return max(n.SlotsTotal-n.SlotsUsed-n.Waiting, 0)
}

func (n NodeInfo) label() string {
	if n.Name != "" {
		return n.Name
	}
	return n.ID
}

// ChooseBuildNode places one build. Nodes that are draining, offline, stale or
// missing a required label are never chosen; when no runner qualifies the
// answer is the control plane, with the reason, so a build never fails for
// want of a runner.
func ChooseBuildNode(nodes []NodeInfo, q Query, p Policy) Decision {
	if q.Mode == ModeControlPlane {
		return Decision{Reason: "control plane: chosen for this app"}
	}
	var all []NodeInfo
	for _, n := range nodes {
		if n.AcceptsBuildWorkloads && !n.Local && n.ID != "" {
			all = append(all, n)
		}
	}
	if len(all) == 0 {
		return Decision{Reason: "control plane: no build node is registered"}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	if q.Pinned != "" {
		for _, n := range all {
			if n.ID != q.Pinned {
				continue
			}
			if !q.trustAllows(n, p) {
				return Decision{Reason: "control plane: pinned runner " + n.label() + " is not allowed to build for this environment"}
			}
			if n.usable(p) {
				return Decision{Node: n.ID, Trust: n.tier(), Reason: "pinned runner " + n.label()}
			}
			return Decision{Reason: "control plane: pinned runner " + n.label() + " is " + n.SkipReason(p)}
		}
		return Decision{Reason: "control plane: pinned runner " + q.Pinned + " is not a build node"}
	}

	var live []NodeInfo
	for _, n := range all {
		if n.usable(p) && q.trustAllows(n, p) {
			live = append(live, n)
		}
	}
	if len(live) == 0 {
		for _, n := range all {
			if n.tier() == TrustPersonal && n.OwnerID == "" && n.usable(p) {
				return Decision{Reason: "control plane: personal builder " + n.label() + " has no builder.owner label"}
			}
		}
		return Decision{Reason: "control plane: no build node is online and accepting work"}
	}
	if p.Strategy == StrategyFirst || p.Strategy == "" {
		if len(q.Labels) > 0 {
			live = filterNodes(live, func(n NodeInfo) bool { return n.hasLabels(q.Labels) })
			if len(live) == 0 {
				return Decision{Reason: "control plane: no runner carries labels " + labelString(q.Labels)}
			}
		}
		return Decision{Node: live[0].ID, Trust: live[0].tier(), Reason: "first online build node " + live[0].label()}
	}

	if len(q.Labels) > 0 {
		live = filterNodes(live, func(n NodeInfo) bool { return n.hasLabels(q.Labels) })
		if len(live) == 0 {
			return Decision{Reason: "control plane: no runner carries labels " + labelString(q.Labels)}
		}
	}
	if q.TargetArch != "" {
		live = filterNodes(live, func(n NodeInfo) bool { return n.Arch == "" || n.Arch == q.TargetArch })
		if len(live) == 0 {
			return Decision{Reason: "control plane: no runner builds for " + q.TargetArch}
		}
	}
	if p.PressureHold {
		live = filterNodes(live, func(n NodeInfo) bool { return n.Pressure != PressureCritical })
		if len(live) == 0 {
			return Decision{Reason: "control plane: every runner is under critical host pressure"}
		}
	}

	best, bestScore := live[0], scoreNode(live[0], q, p)
	for _, n := range live[1:] {
		if s := scoreNode(n, q, p); s > bestScore {
			best, bestScore = n, s
		}
	}
	return Decision{Node: best.ID, Trust: best.tier(), Reason: explainChoice(best, q, p)}
}

func filterNodes(in []NodeInfo, keep func(NodeInfo) bool) []NodeInfo {
	out := in[:0:0]
	for _, n := range in {
		if keep(n) {
			out = append(out, n)
		}
	}
	return out
}

// Score weights: free capacity dominates, then a reported warn pressure, then affinity.
const (
	scoreFreePerSlotPct = 100
	scoreNoFreeSlot     = 60
	scoreWaiter         = 20
	scorePressureWarn   = 25
	scoreArchUnknown    = 10
	scorePreferred      = 30
	scoreAffinity       = 60
)

func scoreNode(n NodeInfo, q Query, p Policy) int {
	score := 0
	if n.SlotsTotal > 0 {
		score += scoreFreePerSlotPct * n.freeSlots() / n.SlotsTotal
	} else {
		score += scoreFreePerSlotPct
	}
	if n.freeSlots() == 0 {
		score -= scoreNoFreeSlot
	}
	score -= scoreWaiter * n.Waiting
	if n.Pressure == PressureWarn {
		score -= scorePressureWarn
	}
	if q.TargetArch != "" && n.Arch == "" {
		score -= scoreArchUnknown
	}
	if q.Preferred != "" && (n.ID == q.Preferred || n.Name == q.Preferred) {
		score += scorePreferred
	}
	if p.Strategy == StrategySticky && q.LastNode != "" && n.ID == q.LastNode && n.freeSlots() > 0 {
		score += scoreAffinity
	}
	return score
}

func explainChoice(n NodeInfo, q Query, p Policy) string {
	var b strings.Builder
	switch {
	case p.Strategy == StrategySticky && q.LastNode == n.ID:
		b.WriteString("same runner as the last build of this app, ")
	case q.Preferred != "" && (n.ID == q.Preferred || n.Name == q.Preferred):
		b.WriteString("preferred runner, ")
	default:
		b.WriteString("least loaded runner, ")
	}
	b.WriteString(n.label())
	if n.SlotsTotal > 0 {
		fmt.Fprintf(&b, ": %d of %d slots free", n.freeSlots(), n.SlotsTotal)
	}
	if n.Pressure != "" && n.Pressure != PressureUnknown {
		fmt.Fprintf(&b, ", pressure %s", n.Pressure)
	}
	return b.String()
}

func unusableWhy(n NodeInfo) string {
	switch {
	case n.Draining:
		return "draining"
	case !n.Online:
		return "offline"
	default:
		return "not reporting"
	}
}

func labelString(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if m[k] == "" {
			parts = append(parts, k)
		} else {
			parts = append(parts, k+"="+m[k])
		}
	}
	return strings.Join(parts, ",")
}
