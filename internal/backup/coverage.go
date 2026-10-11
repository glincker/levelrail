package backup

import "sort"

// Coverage states, one per database or app volume. Only verified and
// unverified count as protected: a schedule that never ran protects nothing.
const (
	CoverageVerified    = "verified"
	CoverageUnverified  = "unverified"
	CoverageNeverRan    = "never_ran"
	CoverageFailing     = "failing"
	CoverageUnprotected = "unprotected"
)

// Primary actions a coverage row offers.
const (
	ActionProtect   = "protect"
	ActionBackupNow = "backup_now"
	ActionRestore   = "restore"
)

// CoverageResource is one database or app volume that exists in desired
// state, whether or not anything backs it up.
type CoverageResource struct {
	Kind     string
	AppName  string
	Name     string
	Schedule string
	TargetID string
	// Scratch marks preview and other disposable data, left out of the
	// protected totals.
	Scratch bool
}

// CoverageRow is the derived protection state of one CoverageResource.
type CoverageRow struct {
	Kind            string
	AppName         string
	Name            string
	Scratch         bool
	State           string
	Reason          string
	PrimaryAction   string
	Schedule        string
	TargetID        string
	NextRun         string
	LastBackupAt    string
	LastBackupID    string
	LastVerifiedAt  string
	TotalBytes      int64
	BackupCount     int
	Encrypted       bool
	ProtectionLevel string
}

// CoverageSummary aggregates the non-scratch rows.
type CoverageSummary struct {
	Total      int
	Protected  int
	Verified   int
	Unverified int
	NeverRan   int
	Failing    int
	// Unprotected counts resources with no schedule or no target.
	Unprotected         int
	Scratch             int
	StorageBytes        int64
	LastVerifiedRestore string
	NextRun             string
}

type coverageKey struct{ kind, app, name string }

// DeriveCoverage joins desired resources with their health rows. A health
// row whose resource no longer exists is ignored: its backups are a storage
// concern, not coverage.
func DeriveCoverage(resources []CoverageResource, health []ResourceHealth) ([]CoverageRow, CoverageSummary) {
	byKey := make(map[coverageKey]*ResourceHealth, len(health))
	for i := range health {
		h := &health[i]
		byKey[coverageKey{h.Kind, h.AppName, h.ResourceName}] = h
	}

	rows := make([]CoverageRow, 0, len(resources))
	var sum CoverageSummary
	for _, res := range resources {
		row := CoverageRow{Kind: res.Kind, AppName: res.AppName, Name: res.Name, Scratch: res.Scratch, Schedule: res.Schedule, TargetID: res.TargetID}
		h := byKey[coverageKey{res.Kind, res.AppName, res.Name}]
		if h != nil {
			fillFromHealth(&row, h)
		}
		row.State, row.Reason = coverageState(res, h)
		row.PrimaryAction = primaryAction(row.State)
		rows = append(rows, row)
		if res.Scratch {
			sum.Scratch++
			continue
		}
		sum.Total++
		sum.StorageBytes += row.TotalBytes
		switch row.State {
		case CoverageVerified:
			sum.Verified++
			sum.Protected++
		case CoverageUnverified:
			sum.Unverified++
			sum.Protected++
		case CoverageNeverRan:
			sum.NeverRan++
		case CoverageFailing:
			sum.Failing++
		default:
			sum.Unprotected++
		}
		if row.LastVerifiedAt > sum.LastVerifiedRestore {
			sum.LastVerifiedRestore = row.LastVerifiedAt
		}
		if row.NextRun != "" && (sum.NextRun == "" || row.NextRun < sum.NextRun) {
			sum.NextRun = row.NextRun
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Scratch != b.Scratch {
			return !a.Scratch
		}
		if ra, rb := stateRank(a.State), stateRank(b.State); ra != rb {
			return ra < rb
		}
		if a.Kind != b.Kind {
			return a.Kind > b.Kind
		}
		if a.AppName != b.AppName {
			return a.AppName < b.AppName
		}
		return a.Name < b.Name
	})
	return rows, sum
}

func fillFromHealth(row *CoverageRow, h *ResourceHealth) {
	row.NextRun = h.NextRun
	row.TotalBytes, row.BackupCount = h.TotalBytes, h.BackupCount
	row.Encrypted, row.ProtectionLevel = h.Encrypted, h.ProtectionLevel
	if h.LastBackup != nil {
		row.LastBackupAt, row.LastBackupID = h.LastBackup.At, h.LastBackup.ID
	}
	if h.LastVerifiedRestore != nil {
		row.LastVerifiedAt = h.LastVerifiedRestore.At
	}
}

func coverageState(res CoverageResource, h *ResourceHealth) (state, reason string) {
	hasBackup := h != nil && h.LastBackup != nil
	switch {
	case res.Schedule == "" || res.TargetID == "":
		if hasBackup {
			return CoverageUnprotected, "backed up by hand, but nothing keeps it current"
		}
		return CoverageUnprotected, "no backup schedule"
	case !hasBackup && h != nil && h.State == HealthFailing:
		return CoverageFailing, h.StateReason
	case !hasBackup:
		return CoverageNeverRan, "scheduled, but no backup has completed yet"
	case h.State == HealthFailing:
		return CoverageFailing, h.StateReason
	case h.LastVerifiedRestore != nil:
		if h.State == HealthWarning {
			return CoverageVerified, h.StateReason
		}
		return CoverageVerified, ""
	}
	return CoverageUnverified, "no restore has been proven to work yet"
}

func primaryAction(state string) string {
	switch state {
	case CoverageUnprotected:
		return ActionProtect
	case CoverageNeverRan, CoverageFailing:
		return ActionBackupNow
	}
	return ActionRestore
}

func stateRank(state string) int {
	switch state {
	case CoverageFailing:
		return 0
	case CoverageUnprotected:
		return 1
	case CoverageNeverRan:
		return 2
	case CoverageUnverified:
		return 3
	}
	return 4
}
