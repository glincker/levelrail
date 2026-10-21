package migrateagent

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/migrationpb"
)

// InventoryDoc is the stored and served form of one discovery.
type InventoryDoc struct {
	DockerVersion string         `json:"docker_version"`
	Containers    []ContainerDoc `json:"containers"`
	Volumes       []VolumeDoc    `json:"volumes"`
	NetworkCount  int            `json:"network_count"`
	CollectedAt   time.Time      `json:"collected_at"`
}

// ContainerDoc is one container on the source. EnvKeys are names only.
type ContainerDoc struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Image           string     `json:"image"`
	State           string     `json:"state"`
	Status          string     `json:"status"`
	EnvKeys         []string   `json:"env_keys,omitempty"`
	Mounts          []MountDoc `json:"mounts,omitempty"`
	Ports           []string   `json:"ports,omitempty"`
	DatabaseEngine  string     `json:"database_engine,omitempty"`
	DatabaseVersion string     `json:"database_version,omitempty"`
	// Unlisted is true when no imported app matches this container, so a
	// platform API read would not have shown it.
	Unlisted bool `json:"unlisted"`
}

// MountDoc is a container mount.
type MountDoc struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"read_only"`
}

// VolumeDoc is a named volume on the source.
type VolumeDoc struct {
	Name   string `json:"name"`
	Driver string `json:"driver"`
}

// StatDoc is one scanned target.
type StatDoc struct {
	Target            string `json:"target"`
	Files             int64  `json:"files"`
	Bytes             int64  `json:"bytes"`
	ChangedDuringScan bool   `json:"changed_during_scan,omitempty"`
	Error             string `json:"error,omitempty"`
}

// ScanDoc is the latest volume scan.
type ScanDoc struct {
	At    time.Time `json:"at"`
	Stats []StatDoc `json:"stats"`
}

// InventoryFromPB converts the wire inventory. The agent-reported mountpoint
// of a volume is dropped: operators only need names.
func InventoryFromPB(in *migrationpb.Inventory) InventoryDoc {
	doc := InventoryDoc{DockerVersion: in.GetDockerVersion(), NetworkCount: int(in.GetNetworkCount()),
		CollectedAt: in.GetCollectedAt().AsTime(), Containers: []ContainerDoc{}, Volumes: []VolumeDoc{}}
	for _, c := range in.GetContainers() {
		cd := ContainerDoc{ID: c.GetId(), Name: c.GetName(), Image: c.GetImage(), State: c.GetState(), Status: c.GetStatus(),
			EnvKeys: c.GetEnvKeys(), Ports: c.GetPorts(), DatabaseEngine: c.GetDatabaseEngine(), DatabaseVersion: c.GetDatabaseVersion()}
		for _, m := range c.GetMounts() {
			cd.Mounts = append(cd.Mounts, MountDoc{Type: m.GetType(), Name: m.GetName(), Source: m.GetSource(), Destination: m.GetDestination(), ReadOnly: m.GetReadOnly()})
		}
		doc.Containers = append(doc.Containers, cd)
	}
	for _, v := range in.GetVolumes() {
		doc.Volumes = append(doc.Volumes, VolumeDoc{Name: v.GetName(), Driver: v.GetDriver()})
	}
	return doc
}

// ScanFromPB converts the wire scan result.
func ScanFromPB(in *migrationpb.ScanResult, at time.Time) ScanDoc {
	doc := ScanDoc{At: at, Stats: []StatDoc{}}
	for _, s := range in.GetStats() {
		doc.Stats = append(doc.Stats, StatDoc{Target: s.GetTarget(), Files: s.GetFiles(), Bytes: s.GetBytes(),
			ChangedDuringScan: s.GetChangedDuringScan(), Error: s.GetError()})
	}
	return doc
}

// ParseInventory decodes a stored inventory, or the zero value when none is stored.
func ParseInventory(raw string) InventoryDoc {
	var doc InventoryDoc
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &doc)
	}
	if doc.Containers == nil {
		doc.Containers = []ContainerDoc{}
	}
	if doc.Volumes == nil {
		doc.Volumes = []VolumeDoc{}
	}
	return doc
}

// ParseScan decodes a stored scan.
func ParseScan(raw string) ScanDoc {
	var doc ScanDoc
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &doc)
	}
	if doc.Stats == nil {
		doc.Stats = []StatDoc{}
	}
	return doc
}

// ImportedRef identifies an app that was imported from the source platform.
type ImportedRef struct {
	App      string
	SourceID string
}

// MarkUnlisted flags every container that matches no imported app by name or
// source id. Matching is a substring test because platforms prefix or suffix
// container names, so a flag means "worth a look", not proof.
func MarkUnlisted(inv *InventoryDoc, refs []ImportedRef) {
	for i := range inv.Containers {
		name := strings.ToLower(inv.Containers[i].Name)
		inv.Containers[i].Unlisted = true
		for _, r := range refs {
			for _, key := range []string{r.App, r.SourceID} {
				if key != "" && strings.Contains(name, strings.ToLower(key)) {
					inv.Containers[i].Unlisted = false
				}
			}
		}
	}
}
