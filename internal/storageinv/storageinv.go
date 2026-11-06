// Package storageinv builds the volume and stopped-container inventory the
// Storage page shows, and owns the rule that keeps database data from being
// offered for deletion.
package storageinv

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

const (
	// containerAppLabel mirrors spec.AppLabelKey without importing spec.
	containerAppLabel = "platform-reserved.app"
	dbPrefix          = "db-"
)

// Source is the read side of a Docker daemon. *docker.Client satisfies it.
type Source interface {
	ListAllVolumes(ctx context.Context) ([]docker.InventoryVolume, error)
	ListStoppedContainers(ctx context.Context) ([]docker.InventoryContainer, error)
}

// Volume is one inventory row.
type Volume struct {
	Name      string            `json:"name"`
	SizeBytes *int64            `json:"size_bytes,omitempty"`
	CreatedAt string            `json:"created_at,omitempty"`
	InUse     bool              `json:"in_use"`
	Labels    map[string]string `json:"labels,omitempty"`
	OwnerApp  string            `json:"owner_app,omitempty"`
	// Database is the database resource owning the volume, when one does.
	Database   string `json:"database,omitempty"`
	IsDatabase bool   `json:"is_database"`
	Orphaned   bool   `json:"orphaned"`
	Protected  bool   `json:"protected"`
	// ProtectReason is plain text for the Reason column, empty when unprotected.
	ProtectReason string `json:"protect_reason,omitempty"`
}

// Container is one stopped container row.
type Container struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	SizeBytes *int64     `json:"size_bytes,omitempty"`
	OwnerApp  string     `json:"owner_app,omitempty"`
	ExitedAt  *time.Time `json:"exited_at,omitempty"`
}

// Owners is the desired-state knowledge ownership is resolved from.
type Owners struct {
	// VolumeApp maps a Docker volume name to the app declaring it.
	VolumeApp map[string]string
	// Databases are the names of every managed database resource.
	Databases map[string]bool
	// Referenced holds every volume name desired state still references.
	Referenced map[string]bool
	// ContainerApp resolves an app from a container name, or "".
	ContainerApp func(name string) string
}

// IsProtectedVolume reports whether v must never be offered for deletion by
// any cleanup path: a db- name, or a volume a database resource owns.
func IsProtectedVolume(v Volume) (bool, string) {
	if v.IsDatabase || v.Database != "" {
		return true, "holds database data, which is never removed automatically"
	}
	if strings.HasPrefix(v.Name, dbPrefix) {
		return true, "named db-, which marks database data that is never removed automatically"
	}
	return false, ""
}

// IsProtectedVolumeName is the name-only form of IsProtectedVolume for delete
// paths that hold a volume name and nothing else.
func IsProtectedVolumeName(name string) (bool, string) {
	return IsProtectedVolume(Volume{Name: name})
}

// databaseOf finds the database a db-<name>-<role> volume belongs to,
// preferring the longest known name so dashed database names resolve.
func databaseOf(name string, dbs map[string]bool) string {
	rest, ok := strings.CutPrefix(name, dbPrefix)
	if !ok {
		return ""
	}
	best := ""
	for db := range dbs {
		if (rest == db || strings.HasPrefix(rest, db+"-")) && len(db) > len(best) {
			best = db
		}
	}
	return best
}

// BuildVolumes joins raw Docker volumes with ownership. A volume is orphaned
// when nothing in desired state references it, no container mounts it and no
// database owns it.
func BuildVolumes(raw []docker.InventoryVolume, o Owners) []Volume {
	out := make([]Volume, 0, len(raw))
	for _, r := range raw {
		v := Volume{Name: r.Name, CreatedAt: r.CreatedAt, InUse: r.InUse, Labels: r.Labels}
		if r.SizeBytes >= 0 {
			size := r.SizeBytes
			v.SizeBytes = &size
		}
		v.Database = databaseOf(r.Name, o.Databases)
		v.IsDatabase = v.Database != ""
		if app := o.VolumeApp[r.Name]; app != "" {
			v.OwnerApp = app
		} else if app := r.Labels[docker.VolumeOwnerAppLabel]; app != "" {
			v.OwnerApp = app
		}
		v.Orphaned = !r.InUse && !o.Referenced[r.Name] && v.Database == ""
		v.Protected, v.ProtectReason = IsProtectedVolume(v)
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// BuildContainers joins stopped containers with their owning app, taken from
// the Levelrail label first and the container name second.
func BuildContainers(raw []docker.InventoryContainer, o Owners) []Container {
	out := make([]Container, 0, len(raw))
	for _, r := range raw {
		c := Container{ID: r.ID, Name: r.Name}
		if r.SizeBytes >= 0 {
			size := r.SizeBytes
			c.SizeBytes = &size
		}
		if !r.ExitedAt.IsZero() {
			t := r.ExitedAt
			c.ExitedAt = &t
		}
		if app := r.Labels[containerAppLabel]; app != "" {
			c.OwnerApp = app
		} else if o.ContainerApp != nil {
			c.OwnerApp = o.ContainerApp(r.Name)
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
