// Package imagelife builds the image inventory of a node, decides which
// images retention may free, and verifies image signatures. Nothing here
// talks to Docker: callers pass records in and execute the plans.
package imagelife

import (
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Roles an app can have for an image.
const (
	RoleLive           = "live"
	RoleRollbackTarget = "rollback_target"
	RoleRecent         = "recent"
	RoleHistory        = "history"
)

const succeededStatus = "succeeded"

// Attempt is the slice of a deploy attempt the inventory needs.
type Attempt struct {
	ID        string
	Image     string
	Digest    string
	Source    string
	CommitSHA string
	Branch    string
	Status    string
	StartedAt time.Time
}

// Pin is a rollback pin: an image kept alive as a rollback target.
type Pin struct {
	DeployID string
	Image    string
	ImageID  string
	PinRef   string
}

// AppState is what one app contributes to the inventory.
type AppState struct {
	Name        string
	LiveImage   string
	LiveImageID string
	// Attempts is newest first.
	Attempts []Attempt
	Pins     []Pin
}

// AppUse says how an app relates to one image.
type AppUse struct {
	App       string    `json:"app"`
	Role      string    `json:"role"`
	DeployID  string    `json:"deploy_id,omitempty"`
	Source    string    `json:"source,omitempty"`
	CommitSHA string    `json:"commit_sha,omitempty"`
	Branch    string    `json:"branch,omitempty"`
	At        time.Time `json:"at,omitempty"`
}

// Entry is one image with its owners and retention status.
type Entry struct {
	ID            string    `json:"id"`
	Digest        string    `json:"digest,omitempty"`
	Tags          []string  `json:"tags"`
	SizeBytes     int64     `json:"size_bytes"`
	SharedBytes   int64     `json:"shared_bytes"`
	Containers    int64     `json:"containers"`
	CreatedAt     time.Time `json:"created_at"`
	Apps          []AppUse  `json:"apps"`
	Protected     bool      `json:"protected"`
	ProtectReason string    `json:"protect_reason,omitempty"`
}

// DigestOfRecord returns the registry digest of r, or "" for a local build.
func DigestOfRecord(r docker.ImageRecord) string {
	for _, rd := range r.RepoDigests {
		if i := strings.Index(rd, "@"); i >= 0 {
			return rd[i+1:]
		}
	}
	return ""
}

func (e Entry) hasRef(ref string) bool {
	if ref == "" {
		return false
	}
	for _, t := range e.Tags {
		if t == ref {
			return true
		}
	}
	return false
}

func (e Entry) matchesDigest(d string) bool {
	return d != "" && (d == e.ID || d == e.Digest)
}

func (e Entry) matchesAttempt(a Attempt) bool {
	return e.matchesDigest(a.Digest) || e.hasRef(a.Image) || e.matchesDigest(docker.ImageDigestOf(a.Image))
}

// Build joins image records with app deploy history. keepPerApp is how many
// newest succeeded deploys per app count as "recent" and stay protected.
// Entries come back biggest first.
func Build(records []docker.ImageRecord, apps []AppState, keepPerApp int, minAge time.Duration, now time.Time) []Entry {
	entries := make([]Entry, 0, len(records))
	for _, r := range records {
		tags := make([]string, 0, len(r.Tags))
		for _, t := range r.Tags {
			if t != "<none>:<none>" {
				tags = append(tags, t)
			}
		}
		entries = append(entries, Entry{
			ID: r.ID, Digest: DigestOfRecord(r), Tags: tags, SizeBytes: r.SizeBytes,
			SharedBytes: r.SharedBytes, Containers: r.Containers, CreatedAt: r.CreatedAt, Apps: []AppUse{},
		})
	}
	for _, app := range apps {
		attachApp(entries, app, keepPerApp)
	}
	for i := range entries {
		markProtection(&entries[i], minAge, now)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].SizeBytes > entries[j].SizeBytes })
	return entries
}

func attachApp(entries []Entry, app AppState, keepPerApp int) {
	for i := range entries {
		e := &entries[i]
		if e.hasRef(app.LiveImage) || e.matchesDigest(docker.ImageDigestOf(app.LiveImage)) || (app.LiveImageID != "" && app.LiveImageID == e.ID) {
			e.Apps = append(e.Apps, AppUse{App: app.Name, Role: RoleLive})
		}
		for _, p := range app.Pins {
			if (p.ImageID != "" && p.ImageID == e.ID) || e.hasRef(p.PinRef) {
				e.Apps = append(e.Apps, AppUse{App: app.Name, Role: RoleRollbackTarget, DeployID: p.DeployID})
			}
		}
	}
	succeeded := 0
	for _, a := range app.Attempts {
		role := RoleHistory
		if a.Status == succeededStatus {
			if succeeded < keepPerApp {
				role = RoleRecent
			}
			succeeded++
		}
		for i := range entries {
			if entries[i].matchesAttempt(a) {
				entries[i].Apps = append(entries[i].Apps, AppUse{
					App: app.Name, Role: role, DeployID: a.ID, Source: a.Source,
					CommitSHA: a.CommitSHA, Branch: a.Branch, At: a.StartedAt,
				})
			}
		}
	}
}

func markProtection(e *Entry, minAge time.Duration, now time.Time) {
	switch {
	case e.Containers > 0:
		e.Protected, e.ProtectReason = true, "used by a running container"
		return
	case hasRole(e.Apps, RoleLive):
		e.Protected, e.ProtectReason = true, "live image of an app"
		return
	case hasRole(e.Apps, RoleRollbackTarget):
		e.Protected, e.ProtectReason = true, "pinned rollback target"
		return
	case hasRole(e.Apps, RoleRecent):
		e.Protected, e.ProtectReason = true, "one of the newest successful deploys"
		return
	case len(e.Apps) == 0:
		e.Protected, e.ProtectReason = true, "not built or deployed by this platform"
		return
	case minAge > 0 && now.Sub(e.CreatedAt) < minAge:
		e.Protected, e.ProtectReason = true, "younger than the minimum retention age"
	}
}

func hasRole(uses []AppUse, role string) bool {
	for _, u := range uses {
		if u.Role == role {
			return true
		}
	}
	return false
}

// Plan is the outcome of retention: what would be freed and what is kept.
type Plan struct {
	Remove       []Entry `json:"remove"`
	ReclaimBytes int64   `json:"reclaim_bytes"`
	KeptCount    int     `json:"kept_count"`
}

// PlanGC selects every unprotected entry. Protected images are never in
// Remove, whatever the policy says.
func PlanGC(entries []Entry) Plan {
	p := Plan{Remove: []Entry{}}
	for _, e := range entries {
		if e.Protected {
			p.KeptCount++
			continue
		}
		p.Remove = append(p.Remove, e)
		p.ReclaimBytes += e.SizeBytes - e.SharedBytes
	}
	return p
}

// RemoveRefs returns the references to delete, in order, to free e: each tag,
// or the ID for an untagged image.
func RemoveRefs(e Entry) []string {
	if len(e.Tags) == 0 {
		return []string{e.ID}
	}
	return append([]string(nil), e.Tags...)
}
