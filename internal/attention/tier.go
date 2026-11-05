package attention

import "strings"

// Tier is how loudly an item is shown. Only critical and needs_action count
// toward the header chip.
const (
	TierCritical     = "critical"
	TierNeedsAction  = "needs_action"
	TierAdvisory     = "advisory"
	TierAutoResolved = "auto_resolved"
)

// Group keys for rows that share one root cause.
const (
	GroupDockerAccess = "docker_access"
	GroupDisk         = "disk"
	GroupCertificates = "certificates"
	GroupNodes        = "nodes"
	GroupTokens       = "tokens"
	GroupBackups      = "backups"
	GroupExposure     = "exposure"
)

// Signal is everything the tier and group mapping reads. For a doctor row
// Kind is the row code and Status is ok, warn or fail; for a feed item Kind
// is the item kind and Severity is critical, warning or info.
type Signal struct {
	Kind     string
	Subject  string
	Severity string
	Status   string
	Class    string
	Message  string
}

type tierKey struct{ level, class string }

// tierTable is the one severity model: level (critical, warning, info) by
// class. A class left empty is read as an incident.
var tierTable = map[tierKey]string{
	{Critical, ClassIncident}:       TierCritical,
	{Critical, ClassRecommendation}: TierCritical,
	{Warning, ClassIncident}:        TierNeedsAction,
	{Warning, ClassRecommendation}:  TierAdvisory,
	{Info, ClassIncident}:           TierAdvisory,
	{Info, ClassRecommendation}:     TierAdvisory,
}

// autoResolvedKinds are items the platform already handled.
var autoResolvedKinds = map[string]bool{
	KindDeviceLoginResolved: true,
}

// advisoryPrefixes are message prefixes that cap a row at advisory, such as
// low and info exposure findings.
var advisoryPrefixes = []string{"[low]", "[info]"}

type groupRule struct {
	match     string
	prefix    bool
	group     string
	bySubject bool
}

// groupRules is checked in order; the first match wins.
var groupRules = []groupRule{
	{match: "docker_guard", group: GroupDockerAccess},
	{match: "docker_privilege", group: GroupDockerAccess},
	{match: "disk", group: GroupDisk},
	{match: "disk_space", group: GroupDisk},
	{match: "docker_root_disk", group: GroupDisk},
	{match: "disk_hygiene", group: GroupDisk},
	{match: "inodes", group: GroupDisk},
	{match: "disk_io_latency", group: GroupDisk},
	{match: "certificates", group: GroupCertificates},
	{match: "certificate", group: GroupCertificates},
	{match: "certificate_budget", group: GroupCertificates},
	{match: "cert_renewal", group: GroupCertificates},
	{match: "nodes", group: GroupNodes},
	{match: "node", group: GroupNodes},
	{match: "node_", prefix: true, group: GroupNodes},
	{match: "agent_version_skew", group: GroupNodes},
	{match: "token_", prefix: true, group: GroupTokens},
	{match: "legacy_tokens", group: GroupTokens},
	{match: "backup_", prefix: true, group: GroupBackups},
	{match: "control_plane_backup", group: GroupBackups},
	{match: "control_plane_dr", group: GroupBackups},
	{match: "schedule_never_runs", group: GroupBackups},
	{match: "exposure", prefix: true, group: GroupExposure, bySubject: true},
}

// Assess returns the tier and group of one signal; tier is empty for a
// doctor row that is not warn or fail.
func Assess(s Signal) (tier, group string) {
	return tierOf(s), groupOf(s)
}

func tierOf(s Signal) string {
	level := s.Severity
	switch s.Status {
	case "":
	case "fail":
		level = Critical
	case "warn":
		level = Warning
	default:
		return ""
	}
	if autoResolvedKinds[s.Kind] {
		return TierAutoResolved
	}
	class := s.Class
	if class == "" {
		class = ClassIncident
	}
	tier, ok := tierTable[tierKey{level, class}]
	if !ok {
		tier = TierNeedsAction
	}
	if tier == TierNeedsAction {
		for _, p := range advisoryPrefixes {
			if strings.HasPrefix(s.Message, p) {
				return TierAdvisory
			}
		}
	}
	return tier
}

func groupOf(s Signal) string {
	for _, r := range groupRules {
		hit := s.Kind == r.match || (r.prefix && strings.HasPrefix(s.Kind, r.match))
		if !hit {
			continue
		}
		if r.bySubject && s.Subject != "" {
			return r.group + ":" + s.Subject
		}
		return r.group
	}
	return ""
}

// CountsTowardHeader is the header count rule.
func CountsTowardHeader(tier string) bool {
	return tier == TierCritical || tier == TierNeedsAction
}

// HeaderCount is how many items the header chip shows.
func HeaderCount(items []Item) int {
	n := 0
	for _, it := range items {
		if CountsTowardHeader(it.Tier) {
			n++
		}
	}
	return n
}

// stamp fills Tier and Group on an item that does not carry them yet.
func stamp(it *Item) {
	if it.Tier != "" {
		return
	}
	it.Tier, it.Group = Assess(Signal{
		Kind: it.Kind, Subject: it.Subject, Severity: it.Severity, Class: it.Class, Message: it.Detail,
	})
}
