package upgradehistory

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// MarkerFile is the context file the installer and the host rollback command
// leave in the data dir for the next boot.
const MarkerFile = "upgrade-context.json"

// MarkerMaxAge bounds how long an unconsumed marker is believed.
const MarkerMaxAge = 24 * time.Hour

const markerMaxBytes = 4096

// Marker methods.
const (
	MethodInstallScript = "install.sh"
	MethodRollback      = "rollback"
	MethodSelfUpgrade   = "self-upgrade"
	MethodManual        = "manual"
	MethodPackage       = "package"
	MethodCI            = "ci"
)

// Marker is who changed the binary and how. It never holds secrets.
type Marker struct {
	ToVersion  string    `json:"to_version"`
	Initiator  string    `json:"initiator"`
	Method     string    `json:"method"`
	Channel    string    `json:"channel,omitempty"`
	BackupName string    `json:"backup_name,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	WrittenAt  time.Time `json:"written_at"`
}

// WriteMarker stores m in dir, replacing any earlier one, with owner-only
// permissions.
func WriteMarker(dir string, m Marker) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode upgrade marker: %w", err)
	}
	tmp := filepath.Join(dir, MarkerFile+".tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write upgrade marker: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, MarkerFile)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("install upgrade marker: %w", err)
	}
	return nil
}

// ClearMarker removes the marker, ignoring a missing one.
func ClearMarker(dir string) {
	_ = os.Remove(filepath.Join(dir, MarkerFile))
}

// ConsumeMarker reads and deletes the marker when it describes a transition
// to running. A marker for another version is left in place (an old binary
// restarting must not eat it) unless it has outlived MarkerMaxAge. A
// malformed marker is deleted and reported as absent.
func ConsumeMarker(dir, running string, now time.Time) (Marker, bool) {
	path := filepath.Join(dir, MarkerFile)
	claimed := path + ".claimed"
	f, err := os.Open(path) //nolint:gosec // fixed file name in the operator's data dir
	if err != nil {
		return Marker{}, false
	}
	var m Marker
	decodeErr := json.NewDecoder(io.LimitReader(f, markerMaxBytes)).Decode(&m)
	_ = f.Close()
	if decodeErr != nil || now.Sub(m.WrittenAt) > MarkerMaxAge {
		ClearMarker(dir)
		return Marker{}, false
	}
	if m.ToVersion != running {
		return Marker{}, false
	}
	if err := os.Rename(path, claimed); err != nil {
		return Marker{}, false
	}
	_ = os.Remove(claimed)
	m.Initiator, m.Method, m.Channel = clean(m.Initiator), clean(m.Method), clean(m.Channel)
	m.BackupName, m.Reason = clean(m.BackupName), clean(m.Reason)
	return m, true
}
