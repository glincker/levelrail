package cpbackup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Continuous replica: an opt-in, off-by-default loop that re-ships an encrypted snapshot of the
// control plane database whenever it changed, to the same destination the scheduled backups use.
const (
	EnvReplicaInterval = "APP_CONTROL_PLANE_REPLICA_INTERVAL"
	ReplicaPrefix      = "cp-replica"
	minReplicaInterval = 5 * time.Second
)

var replicaSlots = [2]string{"a", "b"}

// ReplicaInterval parses EnvReplicaInterval. Zero means off; a value under the minimum is raised to it.
func ReplicaInterval(lookup func(string) (string, bool)) time.Duration {
	v, ok := lookup(EnvReplicaInterval)
	if !ok {
		return 0
	}
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil || d <= 0 {
		return 0
	}
	return max(d, minReplicaInterval)
}

// ReplicaKey is the ciphertext key of one of the two alternating slots. Two slots keep a
// restorable copy in place while the other is overwritten, so an interrupted ship never leaves none.
func ReplicaKey(installID, slot string) string {
	return ReplicaPrefix + "/" + installID + "/slot-" + slot + dataSuffix
}

// ReplicaStatus is the replica loop's state for the UI and CLI.
type ReplicaStatus struct {
	Enabled         bool       `json:"enabled"`
	IntervalSeconds int        `json:"interval_seconds"`
	LastShipAt      *time.Time `json:"last_ship_at,omitempty"`
	LastShipMs      int64      `json:"last_ship_ms,omitempty"`
	LastSizeBytes   int64      `json:"last_size_bytes,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	// PendingSince is set while the database has changes the replica does not hold yet.
	PendingSince *time.Time `json:"pending_since,omitempty"`
}

// Replicator ships changed snapshots on an interval.
type Replicator struct {
	Svc      *Service
	DBPath   string
	Interval time.Duration

	mu          sync.Mutex
	status      ReplicaStatus
	lastToken   string
	shipped     string
	pendingFrom time.Time
	nextSlot    int
	lastPlain   string
}

// Status returns a copy of the current state.
func (r *Replicator) Status() ReplicaStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.status
	st.Enabled = r.Interval > 0
	st.IntervalSeconds = int(r.Interval / time.Second)
	if r.lastToken != r.shipped && !r.pendingFrom.IsZero() {
		t := r.pendingFrom
		st.PendingSince = &t
	}
	return st
}

// changeToken is size and mtime of the database and its WAL: cheap, and any commit moves it.
func (r *Replicator) changeToken() (string, error) {
	var b strings.Builder
	for _, p := range []string{r.DBPath, r.DBPath + "-wal"} {
		fi, err := os.Stat(p)
		if errors.Is(err, os.ErrNotExist) {
			b.WriteString("-;")
			continue
		}
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", filepath.Base(p), err)
		}
		b.WriteString(strconv.FormatInt(fi.Size(), 10) + ":" + strconv.FormatInt(fi.ModTime().UnixNano(), 10) + ";")
	}
	return b.String(), nil
}

// Run ships on every tick until ctx ends. It does nothing when Interval is zero.
func (r *Replicator) Run(ctx context.Context) {
	if r.Interval <= 0 {
		return
	}
	t := time.NewTicker(r.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := r.Tick(ctx); err != nil && !errors.Is(err, ErrNotConfigured) && !errors.Is(err, context.Canceled) {
				r.Svc.logger().Warn("control plane replica ship failed", slog.String("error", err.Error()))
			}
		}
	}
}

// Tick ships once if the database changed since the last successful ship. It reports whether it shipped.
func (r *Replicator) Tick(ctx context.Context) (bool, error) {
	tok, err := r.changeToken()
	if err != nil {
		return false, r.fail(err)
	}
	r.mu.Lock()
	now := r.Svc.now()
	if tok != r.shipped && r.lastToken != tok {
		r.lastToken = tok
		if r.pendingFrom.IsZero() {
			r.pendingFrom = now
		}
	}
	unchanged := tok == r.shipped
	r.mu.Unlock()
	if unchanged {
		return false, nil
	}
	if !r.Svc.backupMu.TryLock() {
		return false, nil
	}
	defer r.Svc.backupMu.Unlock()
	started := time.Now()
	size, uploaded, err := r.ship(ctx)
	if err != nil {
		return false, r.fail(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shipped = tok
	r.pendingFrom = time.Time{}
	if !uploaded {
		return false, nil
	}
	at := r.Svc.now().UTC()
	r.status.LastShipAt, r.status.LastShipMs, r.status.LastSizeBytes, r.status.LastError = &at, time.Since(started).Milliseconds(), size, ""
	return true, nil
}

func (r *Replicator) fail(err error) error {
	r.mu.Lock()
	r.status.LastError = err.Error()
	r.mu.Unlock()
	return err
}

// ship uploads a fresh snapshot unless its content equals the last upload (reading the database moves file times too).
func (r *Replicator) ship(ctx context.Context) (size int64, uploaded bool, err error) {
	s := r.Svc
	var cfg store.CPDRSettings
	cfg, err = s.Store.GetCPDRSettings(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("load settings: %w", err)
	}
	if !cfg.Enabled || cfg.TargetID == "" || len(cfg.Recipients) == 0 {
		return 0, false, ErrNotConfigured
	}
	recipients, err := ParseRecipients(s.recipients(cfg))
	if err != nil {
		return 0, false, fmt.Errorf("recipients: %w", err)
	}
	installID, err := s.installID(ctx, cfg)
	if err != nil {
		return 0, false, err
	}
	bucket, _, err := s.Dest.Open(ctx, cfg.TargetID)
	if err != nil {
		return 0, false, fmt.Errorf("open destination: %w", err)
	}
	tmp, err := s.tempDir()
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	snap := filepath.Join(tmp, "snapshot.db")
	if _, _, err := s.DB.SnapshotTo(ctx, snap); err != nil {
		return 0, false, fmt.Errorf("snapshot: %w", err)
	}
	_, plain, err := store.FileSHA256(snap)
	if err != nil {
		return 0, false, err
	}
	r.mu.Lock()
	same := plain == r.lastPlain
	r.mu.Unlock()
	if same {
		return 0, false, nil
	}
	version, err := store.InspectSnapshot(ctx, snap)
	if err != nil {
		return 0, false, fmt.Errorf("inspect snapshot: %w", err)
	}
	migrations, err := inspectMigrationCount(ctx, snap)
	if err != nil {
		return 0, false, err
	}
	cipher := filepath.Join(tmp, "snapshot.db.age")
	sl, err := seal(snap, cipher, recipients)
	if err != nil {
		return 0, false, err
	}
	r.mu.Lock()
	slot := replicaSlots[r.nextSlot%2]
	r.mu.Unlock()
	key := ReplicaKey(installID, slot)
	m := Manifest{
		ManifestVersion: manifestFormat, InstallID: installID, Key: key, CreatedAt: s.now().UTC().Truncate(time.Second),
		BinaryVersion: s.BinaryVersion, SchemaVersion: version, MigrationsApplied: migrations,
		SHA256: sl.SHA256, SizeBytes: sl.Size, PlainSHA256: sl.PlainSHA256, PlainSizeBytes: sl.PlainSize,
		Compression: "gzip", Encryption: "age", RecipientCount: len(recipients),
		ContainsWrappedSecrets: true, IncludesMasterKey: false,
	}
	// The slot's old manifest goes first so a half-written ciphertext is never described by a stale one.
	_ = bucket.Delete(ctx, ManifestKey(key))
	if err := upload(ctx, bucket, key, cipher, m); err != nil {
		return 0, false, err
	}
	r.mu.Lock()
	r.nextSlot++
	r.lastPlain = plain
	r.mu.Unlock()
	return sl.Size, true, nil
}

// ErrNoReplica means no verified replica slot exists at the destination.
var ErrNoReplica = errors.New("no replica exists at the destination yet")

// RestoreLatestReplica restores the newest replica slot that verifies, falling back to the other one.
func RestoreLatestReplica(ctx context.Context, b Bucket, installID string, ids []age.Identity, opts RestoreOptions) (RestoreReport, error) {
	type slot struct {
		key string
		m   Manifest
	}
	var found []slot
	for _, sl := range replicaSlots {
		key := ReplicaKey(installID, sl)
		raw, err := readAll(ctx, b, ManifestKey(key))
		if err != nil {
			continue
		}
		var m Manifest
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		found = append(found, slot{key, m})
	}
	if len(found) == 0 {
		return RestoreReport{}, ErrNoReplica
	}
	sort.Slice(found, func(i, j int) bool { return found[i].m.CreatedAt.After(found[j].m.CreatedAt) })
	opts.Identities = ids
	var last error
	for _, f := range found {
		rep, err := Restore(ctx, NewBucketSource(b, f.key), opts)
		if err == nil {
			return rep, nil
		}
		last = err
	}
	return RestoreReport{}, last
}

func readAll(ctx context.Context, b Bucket, key string) ([]byte, error) {
	rc, _, err := b.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(rc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
