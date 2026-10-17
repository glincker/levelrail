package ingress

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// SnapshotStatus is what the driver knows about its route snapshot.
type SnapshotStatus struct {
	Enabled bool
	// SavedAt is the last successful write or the age of the restored file.
	SavedAt time.Time
	// Restored is true when this process started serving from the snapshot.
	Restored bool
	// RestoreError is why a present snapshot was not used; empty when it was
	// used or there was none.
	RestoreError string
	// SaveError is the last write failure, cleared by the next success.
	SaveError string
}

// SetSnapshot makes Apply persist every config it loads and enables
// RestoreSnapshot. fingerprint is InheritedSockets.Fingerprint.
func (d *Driver) SetSnapshot(store *SnapshotStore, fingerprint string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.snapshot = store
	d.snapshotFingerprint = fingerprint
}

// SnapshotStatus reports the snapshot state for doctor.
func (d *Driver) SnapshotStatus() SnapshotStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	st := d.snapStatus
	st.Enabled = d.snapshot != nil
	return st
}

// RestoreSnapshot loads the saved config into Caddy so the listeners serve
// before the first reconcile. A missing, corrupt or stale snapshot is not an
// error for the caller: it returns false and the normal reconcile path runs.
func (d *Driver) RestoreSnapshot(ctx context.Context) (bool, error) {
	d.mu.Lock()
	store, fp := d.snapshot, d.snapshotFingerprint
	d.mu.Unlock()
	if store == nil {
		return false, nil
	}
	data, err := store.Load(fp)
	if errors.Is(err, ErrNoSnapshot) {
		return false, nil
	}
	if err != nil {
		d.noteRestoreFailure(err)
		return false, err
	}
	if err := loadConfig(data.Config, true); err != nil {
		err = fmt.Errorf("ingress: restore snapshot: load config: %w", err)
		d.noteRestoreFailure(err)
		return false, err
	}
	d.mu.Lock()
	d.lastApplied = append([]byte(nil), data.Config...)
	d.listenPorts = make(map[int]bool, len(data.Ports))
	for _, p := range data.Ports {
		d.listenPorts[p] = true
	}
	d.snapStatus.Restored = true
	d.snapStatus.SavedAt = data.SavedAt
	d.snapStatus.RestoreError = ""
	d.mu.Unlock()
	d.logger.InfoContext(ctx, "ingress serving from route snapshot before the first reconcile",
		"config_bytes", len(data.Config), "saved_at", data.SavedAt)
	return true, nil
}

func (d *Driver) noteRestoreFailure(err error) {
	d.mu.Lock()
	d.snapStatus.RestoreError = err.Error()
	d.mu.Unlock()
}

func (d *Driver) saveSnapshot(payload []byte) {
	d.mu.Lock()
	store, fp := d.snapshot, d.snapshotFingerprint
	ports := make([]int, 0, len(d.listenPorts))
	for p := range d.listenPorts {
		ports = append(ports, p)
	}
	d.mu.Unlock()
	if store == nil {
		return
	}
	sort.Ints(ports)
	now := time.Now().UTC()
	err := store.Save(SnapshotData{SavedAt: now, Fingerprint: fp, Ports: ports, Config: payload})
	d.mu.Lock()
	if err != nil {
		d.snapStatus.SaveError = err.Error()
	} else {
		d.snapStatus.SaveError = ""
		d.snapStatus.SavedAt = now
	}
	d.mu.Unlock()
	if err != nil {
		d.logger.Warn("ingress route snapshot not saved, a restart will wait for the first reconcile", "error", err)
	}
}
