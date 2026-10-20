package backup

import "strings"

// Drill failure classes, one per typed remedy the dashboard offers.
const (
	DrillFailureDestination = "destination_unreachable"
	DrillFailureCredentials = "credentials"
	DrillFailureDisk        = "disk_space"
	DrillFailureObject      = "object_damaged"
	DrillFailureOther       = "other"
)

// DrillFailureClass names why a drill failed and what the operator should do.
type DrillFailureClass struct {
	Kind     string
	Guidance string
}

// Order matters: a 403 on a missing bucket is credentials before destination.
var drillFailureRules = []struct {
	kind     string
	guidance string
	needles  []string
}{
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
				return DrillFailureClass{Kind: rule.kind, Guidance: rule.guidance}
			}
		}
	}
	return DrillFailureClass{Kind: DrillFailureOther, Guidance: "Open the failed drill for its stage and error, fix the cause, then press Re-check. A new drill runs from Backups."}
}
