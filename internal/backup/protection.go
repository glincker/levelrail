package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// Resource health states, worst first.
const (
	HealthFailing     = "failing"
	HealthUnverified  = "unverified"
	HealthWarning     = "warning"
	HealthHealthy     = "healthy"
	HealthUnprotected = "unprotected"
)

// ProtectionStore is the store surface the protection service reads and
// writes.
type ProtectionStore interface {
	backupResolver
	ListSucceededBackups(ctx context.Context) ([]store.BackupHistory, error)
	ListAllBackupHistory(ctx context.Context, limit int, before *time.Time) ([]store.BackupHistory, error)
	ListScheduledServiceVolumes(ctx context.Context) ([]store.ServiceVolumeBackupConfig, error)
	ListScheduledDatabases(ctx context.Context) ([]store.DesiredDatabase, error)
	ListBackupDrills(ctx context.Context, serviceName, volumeName, databaseName string, limit int) ([]store.BackupDrill, error)
	ListBackupTargetProtection(ctx context.Context) ([]store.BackupTargetProtection, error)
	SetBackupTargetProtection(ctx context.Context, p store.BackupTargetProtection) error
	GetVolumeBackupPolicy(ctx context.Context, serviceName, volumeName string) (store.VolumeBackupPolicy, error)
	SetVolumeBackupPolicy(ctx context.Context, p store.VolumeBackupPolicy) error
}

// HealthBackup is one backup in a health row.
type HealthBackup struct {
	ID        string
	At        string
	Status    string
	SizeBytes int64
	Codec     string
	Error     string
}

// DrillSummary is one drill in a health row.
type DrillSummary struct {
	ID      string
	At      string
	Status  string
	Stage   string
	Error   string
	Trigger string
	Files   int64
}

// ResourceHealth is the protection state of one database or app volume.
type ResourceHealth struct {
	Kind         string
	AppName      string
	ResourceName string
	TargetID     string
	Schedule     string
	NextRun      string
	BackupCount  int
	TotalBytes   int64
	LastAttempt  *HealthBackup
	LastBackup   *HealthBackup
	// LastVerifiedRestore is the newest passed drill: proof a restore
	// worked, not just that a file exists.
	LastVerifiedRestore *DrillSummary
	LastDrill           *DrillSummary
	// Encrypted is true when the newest backup is client side encrypted.
	Encrypted       bool
	State           string
	StateReason     string
	ProtectionLevel string
	Warning         string
}

// Protection reports backup health and starts drills.
type Protection struct {
	Store  ProtectionStore
	Drills *DrillRunner
	Prober ProtectionProber
	// Secrets resolves target credentials for probes and restores.
	Secrets    SecretsResolver
	Downloader Downloader
	// RestoreStore records restore-to-new-volume attempts.
	RestoreStore VolumeCloneRestoreStore
	Sealer       *Sealer
	// DrillInterval is the configured drill period, used to flag a resource
	// whose last verified restore is older than two periods.
	DrillInterval time.Duration
	Logger        *slog.Logger
	Now           func() time.Time
}

func (p *Protection) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// EncryptionStatus describes client side encryption of new volume backups.
type EncryptionStatus struct {
	Enabled   bool
	Recipient string
	Codec     string
}

// Encryption reports whether new volume backups are encrypted.
func (p *Protection) Encryption() EncryptionStatus {
	if p.Sealer == nil {
		return EncryptionStatus{}
	}
	st := EncryptionStatus{Codec: p.Sealer.Codec(), Enabled: len(p.Sealer.Recipients) > 0}
	if st.Enabled {
		if s, ok := p.Sealer.Recipients[0].(interface{ String() string }); ok {
			st.Recipient = s.String()
		}
	}
	return st
}

// Health builds one row per backed up or scheduled resource.
func (p *Protection) Health(ctx context.Context) ([]ResourceHealth, error) {
	attempts, err := p.Store.ListAllBackupHistory(ctx, 5000, nil)
	if err != nil {
		return nil, fmt.Errorf("list backup history: %w", err)
	}
	vols, err := p.Store.ListScheduledServiceVolumes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list scheduled volumes: %w", err)
	}
	dbs, err := p.Store.ListScheduledDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list scheduled databases: %w", err)
	}
	drills, err := p.Store.ListBackupDrills(ctx, "", "", "", 2000)
	if err != nil {
		return nil, fmt.Errorf("list drills: %w", err)
	}
	prot, err := p.Store.ListBackupTargetProtection(ctx)
	if err != nil {
		return nil, fmt.Errorf("list target protection: %w", err)
	}
	protByTarget := map[string]store.BackupTargetProtection{}
	for _, t := range prot {
		protByTarget[t.TargetID] = t
	}

	rows := map[drillKey]*ResourceHealth{}
	get := func(k drillKey) *ResourceHealth {
		if r, ok := rows[k]; ok {
			return r
		}
		r := &ResourceHealth{Kind: k.kind}
		if k.kind == store.BackupResourceKindVolume {
			r.AppName, r.ResourceName = k.a, k.b
		} else {
			r.ResourceName = k.a
		}
		rows[k] = r
		return r
	}

	for _, v := range vols {
		r := get(drillKey{store.BackupResourceKindVolume, v.ServiceName, v.VolumeName})
		r.Schedule, r.TargetID = v.BackupSchedule, v.BackupTargetID
	}
	for _, d := range dbs {
		r := get(drillKey{store.BackupResourceKindDatabase, d.Name, ""})
		r.Schedule, r.TargetID = d.BackupSchedule, d.BackupTargetID
	}
	// attempts are newest first.
	for _, h := range attempts {
		r := get(keyOf(h))
		if r.TargetID == "" {
			r.TargetID = h.TargetID
		}
		s := summarizeBackup(h)
		if r.LastAttempt == nil {
			r.LastAttempt = &s
		}
		if h.Status == store.BackupStatusSucceeded {
			r.BackupCount++
			r.TotalBytes += h.SizeBytes
			if r.LastBackup == nil {
				r.LastBackup = &s
				r.Encrypted = h.Codec == CodecZstdAge
			}
		}
	}
	for _, d := range drills {
		k := drillKey{d.ResourceKind, d.DatabaseName, ""}
		if d.ResourceKind == store.BackupResourceKindVolume {
			k = drillKey{d.ResourceKind, d.ServiceName, d.VolumeName}
		}
		r, ok := rows[k]
		if !ok {
			continue
		}
		s := DrillSummary{ID: d.ID, At: d.StartedAt, Status: d.Status, Stage: d.Stage, Error: d.Error, Trigger: d.Trigger, Files: d.Files}
		if d.Status == store.DrillStatusRunning {
			continue
		}
		if r.LastDrill == nil {
			r.LastDrill = &s
		}
		if d.Status == store.DrillStatusPassed && r.LastVerifiedRestore == nil {
			r.LastVerifiedRestore = &s
		}
	}

	now := p.now()
	out := make([]ResourceHealth, 0, len(rows))
	for _, r := range rows {
		if r.Schedule != "" {
			if sched, err := cronexpr.Parse(r.Schedule); err == nil {
				r.NextRun = sched.Next(now).UTC().Format(time.RFC3339)
			}
		}
		if pr, ok := protByTarget[r.TargetID]; ok && pr.ProbeError == "" {
			bp := BucketProtection{ObjectLock: pr.ObjectLock, LockMode: pr.LockMode, Versioning: pr.Versioning, CanDelete: pr.CanDelete}
			r.ProtectionLevel, r.Warning = bp.Level(), bp.Warning()
		}
		r.State, r.StateReason = p.classify(r, now)
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Kind != b.Kind {
			return a.Kind > b.Kind
		}
		if a.AppName != b.AppName {
			return a.AppName < b.AppName
		}
		return a.ResourceName < b.ResourceName
	})
	return out, nil
}

func summarizeBackup(h store.BackupHistory) HealthBackup {
	return HealthBackup{ID: h.ID, At: h.StartedAt, Status: h.Status, SizeBytes: h.SizeBytes, Codec: h.Codec, Error: h.Error}
}

func (p *Protection) classify(r *ResourceHealth, now time.Time) (state, reason string) {
	switch {
	case r.LastBackup == nil && r.LastAttempt != nil && r.LastAttempt.Status == store.BackupStatusFailed:
		return HealthFailing, "every backup attempt has failed"
	case r.LastBackup == nil:
		return HealthUnprotected, "no successful backup yet"
	case r.LastDrill != nil && r.LastDrill.Status == store.DrillStatusFailed:
		return HealthFailing, "the last restore drill failed"
	case r.LastAttempt != nil && r.LastAttempt.Status == store.BackupStatusFailed && r.LastAttempt.At > r.LastBackup.At:
		return HealthWarning, "the latest backup attempt failed"
	case r.LastVerifiedRestore == nil:
		return HealthUnverified, "no restore has been proven to work yet"
	}
	if p.DrillInterval > 0 {
		if at, err := time.Parse(time.RFC3339, r.LastVerifiedRestore.At); err == nil && now.Sub(at) > 2*p.DrillInterval {
			return HealthWarning, "the last verified restore is overdue"
		}
	}
	if r.Warning != "" && r.ProtectionLevel == ProtectionOpen {
		return HealthWarning, "the backup bucket can be overwritten by the same key"
	}
	return HealthHealthy, ""
}

// FailedDrill is a resource whose latest drill failed.
type FailedDrill struct {
	Resource string
	Reason   string
	At       string
}

// FailedDrills returns one entry per resource whose most recent finished
// drill failed. A later passing drill clears it.
func (p *Protection) FailedDrills(ctx context.Context) ([]FailedDrill, error) {
	drills, err := p.Store.ListBackupDrills(ctx, "", "", "", 1000)
	if err != nil {
		return nil, fmt.Errorf("list drills: %w", err)
	}
	seen := map[drillKey]bool{}
	var out []FailedDrill
	for _, d := range drills {
		if d.Status == store.DrillStatusRunning {
			continue
		}
		k := drillKey{d.ResourceKind, d.DatabaseName, ""}
		name := d.DatabaseName
		if d.ResourceKind == store.BackupResourceKindVolume {
			k = drillKey{d.ResourceKind, d.ServiceName, d.VolumeName}
			name = d.ServiceName + "/" + d.VolumeName
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		if d.Status == store.DrillStatusFailed {
			reason := d.Error
			if d.Stage != "" {
				reason = "at " + d.Stage + ": " + reason
			}
			out = append(out, FailedDrill{Resource: name, Reason: reason, At: d.StartedAt})
		}
	}
	return out, nil
}

// StartDrill validates backupID and runs a manual drill in the background,
// returning the drill ID to poll.
func (p *Protection) StartDrill(ctx context.Context, backupID string) (string, error) {
	if p.Drills == nil {
		return "", errors.New("restore drills are not configured on this control plane")
	}
	bh, err := p.Store.GetBackupHistory(ctx, backupID)
	if err != nil {
		return "", fmt.Errorf("get backup %q: %w", backupID, err)
	}
	if bh.Status != store.BackupStatusSucceeded {
		return "", fmt.Errorf("backup %q has status %q: only a succeeded backup can be drilled", backupID, bh.Status)
	}
	id, err := NewDrillID()
	if err != nil {
		return "", err
	}
	go func() { //nolint:gosec // deliberately not ctx: a drill outlives the request that started it
		if _, err := p.Drills.RunDrill(context.Background(), id, backupID, store.DrillTriggerManual); err != nil && p.Logger != nil {
			p.Logger.Error("backup: manual restore drill failed", slog.String("drill_id", id), slog.String("error", err.Error()))
		}
	}()
	return id, nil
}

// RefreshProtection re-probes every named target's bucket (all when targetIDs
// is empty) and returns the stored results.
func (p *Protection) RefreshProtection(ctx context.Context, targetIDs []string) ([]store.BackupTargetProtection, error) {
	if p.Prober == nil {
		return nil, errors.New("bucket protection probing is not configured on this control plane")
	}
	var out []store.BackupTargetProtection
	for _, id := range targetIDs {
		out = append(out, RecordProtection(ctx, p.Store, p.Secrets, p.Prober, id, p.now()))
	}
	return out, nil
}

// VolumeExists reports whether a volume of that name is already on the
// runtime's node. known is false when the node cannot list volumes, in which
// case only a freshly generated name may be used.
func (p *Protection) VolumeExists(ctx context.Context, rtm docker.Runtime, name string) (exists, known bool, err error) {
	lister, ok := rtm.(interface {
		ListVolumesByPrefix(ctx context.Context, prefix string) ([]docker.NamedVolume, error)
	})
	if !ok {
		return false, false, nil
	}
	vols, err := lister.ListVolumesByPrefix(ctx, name)
	if err != nil {
		return false, false, fmt.Errorf("list volumes on node: %w", err)
	}
	for _, v := range vols {
		if v.Name == name {
			return true, true, nil
		}
	}
	return false, true, nil
}

// RestoreVolumeTo restores a volume backup into a brand-new volume on the
// node rtm drives. It never writes to an existing volume: the caller checks
// VolumeExists first.
func (p *Protection) RestoreVolumeTo(ctx context.Context, rtm docker.Runtime, historyID, sourceService, sourceVolume, newVolume, backupID string) error {
	if p.RestoreStore == nil || p.Downloader == nil {
		return errors.New("volume restore is not configured on this control plane")
	}
	r := &VolumeCloneRestoreRunner{
		Store:          p.RestoreStore,
		Secrets:        p.Secrets,
		Downloader:     p.Downloader,
		VolumeRestorer: &ContainerVolumeRestorer{Runtime: rtm},
		Volumes:        rtm,
	}
	if p.Sealer != nil {
		r.Identities = p.Sealer.Identities
	}
	return r.RunVolumeCloneRestore(ctx, historyID, sourceService, sourceVolume, newVolume, backupID)
}

// ValidateVolumeName reports whether name is a legal new Docker volume name.
func ValidateVolumeName(name string) error {
	if name == "" || len(name) > 128 {
		return errors.New("volume name must be 1 to 128 characters")
	}
	for i, c := range name {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || (i > 0 && strings.ContainsRune("_.-", c))
		if !ok {
			return fmt.Errorf("volume name %q has an invalid character at position %d", name, i+1)
		}
	}
	return nil
}
