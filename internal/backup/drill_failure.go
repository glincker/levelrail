package backup

import (
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Drill failure classes, one per typed remedy the dashboard offers.
const (
	DrillFailureDestination = "destination_unreachable"
	DrillFailureCredentials = "credentials"
	DrillFailureDisk        = "disk_space"
	DrillFailureObject      = "object_damaged"
	DrillFailureOther       = "other"
	// DrillFailureTargetMissing is a drill whose backup target was deleted or
	// replaced: a stale reference, not a failed restore.
	DrillFailureTargetMissing = "target_missing"
)

// Row severities: advisory needs action but is not an alarm.
const (
	DrillSeverityAdvisory = "advisory"
	DrillSeverityWarning  = "warning"
)

// DrillFailureClass names why a drill failed and what the operator should do.
type DrillFailureClass struct {
	Kind     string
	Guidance string
	Severity string
}

// Order matters: a 403 on a missing bucket is credentials before destination.
var drillFailureRules = []struct {
	kind     string
	guidance string
	needles  []string
}{
	{DrillFailureTargetMissing, "The drill points at a backup target that no longer exists. Point the resource at a current backup target, or remove the stale drill reference. No backup or target is deleted.",
		[]string{"backup target not found"}},
	{DrillFailureCredentials, "The storage credentials were rejected. Update the access key on the backup target, then press Re-check.",
		[]string{"accessdenied", "access denied", "403", "invalidaccesskey", "signaturedoesnotmatch", "forbidden", "unauthorized", "401"}},
	{DrillFailureDisk, "The control plane host ran out of disk space during the drill. Free space with Clean up unused Docker data, then press Re-check.",
		[]string{"no space left", "insufficient disk space", "disk quota"}},
	{DrillFailureObject, "The stored object is missing, truncated or does not match its checksum. Take a new backup and drill it.",
		[]string{"is missing from the bucket", "truncated", "checksum of the stored object", "nosuchkey"}},
	{DrillFailureDestination, "The storage destination could not be reached. Check the endpoint, region and network, use Test connection on the backup target, then press Re-check.",
		[]string{"no such host", "connection refused", "timeout", "timed out", "dial tcp", "tls:", "eof", "nosuchbucket", "unreachable"}},
}

// ClassifyDrillFailure maps a drill's error text to a remedy class.
func ClassifyDrillFailure(errText string) DrillFailureClass {
	low := strings.ToLower(errText)
	for _, rule := range drillFailureRules {
		for _, n := range rule.needles {
			if strings.Contains(low, n) {
				return DrillFailureClass{Kind: rule.kind, Guidance: rule.guidance, Severity: drillSeverity(rule.kind)}
			}
		}
	}
	return DrillFailureClass{Kind: DrillFailureOther, Guidance: "Open the failed drill for its stage and error, fix the cause, then press Re-check. A new drill runs from Backups.", Severity: DrillSeverityWarning}
}

// drillSeverity downgrades only a dangling target; every real restore failure stays a warning.
func drillSeverity(kind string) string {
	if kind == DrillFailureTargetMissing {
		return DrillSeverityAdvisory
	}
	return DrillSeverityWarning
}

// DrillFailureSummary splits the latest failed drills into stale target references and real failures.
type DrillFailureSummary struct {
	Dangling []store.BackupDrill
	Real     []store.BackupDrill
}

// SummarizeDrillFailures classifies each failed drill by its error text.
func SummarizeDrillFailures(failed []store.BackupDrill) DrillFailureSummary {
	var s DrillFailureSummary
	for _, d := range failed {
		if ClassifyDrillFailure(d.Error).Kind == DrillFailureTargetMissing {
			s.Dangling = append(s.Dangling, d)
		} else {
			s.Real = append(s.Real, d)
		}
	}
	return s
}

// Severity is advisory only when every failure is a dangling target.
func (s DrillFailureSummary) Severity() string {
	switch {
	case len(s.Real) > 0:
		return DrillSeverityWarning
	case len(s.Dangling) > 0:
		return DrillSeverityAdvisory
	}
	return ""
}

// DrillResourceLabel names the resource a drill belongs to.
func DrillResourceLabel(d store.BackupDrill) string {
	if d.ResourceKind == store.BackupResourceKindVolume {
		return "volume " + d.ServiceName + "/" + d.VolumeName
	}
	return "database " + d.DatabaseName
}

func drillResourceList(ds []store.BackupDrill) string {
	seen := map[string]bool{}
	var names []string
	for _, d := range ds {
		if l := DrillResourceLabel(d); !seen[l] {
			seen[l] = true
			names = append(names, l)
		}
	}
	return strings.Join(names, ", ")
}

// Detail is the plain-language row text for the failures.
func (s DrillFailureSummary) Detail() string {
	stale := ""
	if len(s.Dangling) > 0 {
		stale = fmt.Sprintf("the drill points at a backup target that no longer exists (%s), a stale setting rather than a failed restore", drillResourceList(s.Dangling))
	}
	if len(s.Real) == 0 {
		return stale
	}
	first := s.Real[0]
	failure := fmt.Sprintf("%d resource(s) failed their latest drill; first error at %s: %s", len(s.Real), first.Stage, first.Error)
	if stale == "" {
		return failure
	}
	return failure + "; also " + stale
}
