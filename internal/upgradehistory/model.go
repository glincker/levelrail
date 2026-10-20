// Package upgradehistory records every control plane version transition at
// boot, however the binary was replaced, and builds the links and context
// the Updates page shows for it.
package upgradehistory

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/kit/semver"
)

// Transition kinds.
const (
	KindInstalled   = "installed"
	KindAdopted     = "adopted"
	KindUpgraded    = "upgraded"
	KindRolledBack  = "rolled_back"
	KindRebuilt     = "rebuilt"
	KindDevelopment = "development"
	KindChanged     = "changed"
)

// Initiator and health values when nothing reported them.
const (
	InitiatorUnknown = "unknown"
	// HealthBooted: the version started and its migrations applied. Recording
	// happens right after, so this is the only health fact known at that time.
	HealthBooted = "booted"
)

// Audit action names, written into the audit log path as "<action>: detail".
const (
	AuditActionRecorded = "upgrade_history.recorded"
	AuditActionAcked    = "upgrade_history.acknowledged"
)

const (
	devVersion      = "dev"
	develVersion    = "(devel)"
	maxFieldRunes   = 120
	releasePathTag  = "/releases/tag/"
	comparePathFrom = "/compare/"
)

var (
	releaseTagPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$`)
	pseudoPattern     = regexp.MustCompile(`\d{14}-[0-9a-f]{12}(\+.*)?$`)
)

// IsRelease reports whether v looks like a published release tag such as
// v1.2.3 or v1.2.3-beta.4. Dev builds and pseudo-versions are not.
func IsRelease(v string) bool {
	return releaseTagPattern.MatchString(v) && !pseudoPattern.MatchString(v)
}

// IsDevelopment reports whether v is a local or pseudo-version build.
func IsDevelopment(v string) bool {
	return v == devVersion || v == develVersion || pseudoPattern.MatchString(v) || strings.HasSuffix(v, "+dirty")
}

// ChannelOf names the release channel a tag belongs to, "" for non-releases.
func ChannelOf(v string) string {
	switch {
	case !IsRelease(v):
		return ""
	case strings.Contains(v, "-"):
		return "beta"
	default:
		return "stable"
	}
}

// Classify decides the kind of a transition from prev to cur. A marker
// method of rollback wins when the versions cannot be ordered.
func Classify(prev, cur string, hasPrev, markerRollback bool) string {
	switch {
	case !hasPrev:
		return KindInstalled
	case IsDevelopment(cur):
		return KindDevelopment
	}
	cmp, ok := semver.Compare(cur, prev)
	switch {
	case ok && cmp > 0:
		return KindUpgraded
	case ok && cmp < 0:
		return KindRolledBack
	case ok:
		return KindRebuilt
	case markerRollback:
		return KindRolledBack
	default:
		return KindChanged
	}
}

// Links are the public pages for a transition. Empty fields mean the version
// is not a published release, so no link is invented.
type Links struct {
	ReleaseURL string
	CompareURL string
}

// BuildLinks derives release and compare URLs from the repository URL.
func BuildLinks(repoURL, from, to string) Links {
	base := strings.TrimSuffix(strings.TrimRight(repoURL, "/"), ".git")
	if base == "" || !strings.HasPrefix(base, "https://") || !IsRelease(to) {
		return Links{}
	}
	l := Links{ReleaseURL: base + releasePathTag + url.PathEscape(to)}
	if IsRelease(from) {
		l.CompareURL = base + comparePathFrom + url.PathEscape(from) + "..." + url.PathEscape(to)
	}
	return l
}

// clean trims, drops control characters and caps a free-text field.
func clean(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < 0x20 || r == 0x7f {
			continue
		}
		if n == maxFieldRunes {
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}
