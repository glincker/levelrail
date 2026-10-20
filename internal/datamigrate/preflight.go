package datamigrate

import (
	"fmt"
	"regexp"
	"strings"
)

// Severity of one preflight check.
const (
	SeverityOK    = "ok"
	SeverityWarn  = "warn"
	SeverityBlock = "block"
)

// PreflightCheck is one preflight finding with what to do about it.
type PreflightCheck struct {
	ID         string `json:"id"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	NextAction string `json:"next_action,omitempty"`
}

// PreflightInput is everything a database's preflight needs, gathered by the caller.
type PreflightInput struct {
	Engine        string
	DB            DatabaseInfo
	SourceMajor   int
	TargetName    string
	TargetVersion string
	// TargetExists is true when a managed database already has TargetName,
	// OwnTarget when this migration created it earlier (so a retry is fine).
	TargetExists bool
	OwnTarget    bool
	// FreeBytes is the target node's free disk, negative when unknown.
	FreeBytes int64
	// DiskMargin multiplies the database size to get the space required.
	DiskMargin float64
	// MBPerSecond is the assumed copy throughput for the time estimate.
	MBPerSecond float64
}

// PreflightResult is the checks plus the target version the plan resolved to.
type PreflightResult struct {
	Checks          []PreflightCheck `json:"checks"`
	TargetVersion   string           `json:"target_version"`
	RequiredBytes   int64            `json:"required_bytes"`
	EstimateSeconds int              `json:"estimate_seconds"`
	Blocked         bool             `json:"blocked"`
}

var targetNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// ValidTargetName reports whether name can be a managed database name.
func ValidTargetName(name string) bool { return targetNameRe.MatchString(name) }

// Preflight evaluates one database before anything is written. It never
// drops an extension: a missing one blocks the copy with a next action.
func Preflight(in PreflightInput) PreflightResult {
	res := PreflightResult{TargetVersion: in.TargetVersion}
	add := func(id, sev, msg, next string) {
		res.Checks = append(res.Checks, PreflightCheck{ID: id, Severity: sev, Message: msg, NextAction: next})
		if sev == SeverityBlock {
			res.Blocked = true
		}
	}

	if !ValidTargetName(in.TargetName) {
		add("name", SeverityBlock, fmt.Sprintf("%q is not a valid managed database name.", in.TargetName),
			"Use lowercase letters, digits and dashes, starting with a letter, 40 characters at most.")
	} else if in.TargetExists && !in.OwnTarget {
		add("collision", SeverityBlock, fmt.Sprintf("A managed database named %q already exists and was not created by this migration.", in.TargetName),
			"Pick a different name. Copying would replace that database's contents.")
	} else if in.TargetExists {
		add("collision", SeverityWarn, fmt.Sprintf("%q was created by an earlier run of this migration.", in.TargetName),
			"Copying again replaces its contents with a fresh copy of the source.")
	} else {
		add("collision", SeverityOK, fmt.Sprintf("The name %q is free.", in.TargetName), "")
	}

	if in.Engine == EnginePostgres {
		res.TargetVersion = pgExtensionAndVersionChecks(in, add)
	}

	res.RequiredBytes = requiredBytes(in.DB.SizeBytes, in.DiskMargin)
	switch {
	case in.FreeBytes < 0:
		add("disk", SeverityWarn, "Free disk on the target node could not be measured.",
			fmt.Sprintf("Make sure at least %s is free before copying.", HumanBytes(res.RequiredBytes)))
	case in.FreeBytes < res.RequiredBytes:
		add("disk", SeverityBlock, fmt.Sprintf("Needs about %s with the safety margin, only %s is free on the target node.", HumanBytes(res.RequiredBytes), HumanBytes(in.FreeBytes)),
			"Free disk space on the target node or choose a node with more room.")
	default:
		add("disk", SeverityOK, fmt.Sprintf("%s free for about %s needed.", HumanBytes(in.FreeBytes), HumanBytes(res.RequiredBytes)), "")
	}

	if in.MBPerSecond > 0 {
		res.EstimateSeconds = int(float64(in.DB.SizeBytes) / (in.MBPerSecond * 1024 * 1024))
		if res.EstimateSeconds < 1 {
			res.EstimateSeconds = 1
		}
		add("estimate", SeverityOK, fmt.Sprintf("Estimated copy time: about %s at %.0f MB/s.", HumanDuration(res.EstimateSeconds), in.MBPerSecond), "")
	}
	return res
}

func pgExtensionAndVersionChecks(in PreflightInput, add func(id, sev, msg, next string)) string {
	version := in.TargetVersion
	suffix, unsupported := requiredVariant(in.DB.Extensions)
	major := in.SourceMajor
	if tm := MajorOf(in.TargetVersion); tm > 0 {
		major = tm
	}
	switch {
	case len(unsupported) > 0:
		add("extensions", SeverityBlock, fmt.Sprintf("Uses extension(s) no managed image provides: %s.", strings.Join(unsupported, ", ")),
			"Remove the extension from the source database first, or leave this database out of the migration.")
	case suffix != "" && !PgvectorVariantAvailable:
		add("extensions", SeverityBlock, "Uses the vector extension (pgvector), which the managed Postgres image on this control plane does not include.",
			"Wait for the pgvector variant, or leave this database out. Nothing was changed.")
	case suffix != "":
		version = variantVersion(major, suffix)
		add("extensions", SeverityOK, fmt.Sprintf("Uses pgvector, the target will run version %s.", version), "")
	case len(in.DB.Extensions) > 0:
		add("extensions", SeverityOK, fmt.Sprintf("Extensions %s are in the standard image.", strings.Join(in.DB.Extensions, ", ")), "")
	default:
		add("extensions", SeverityOK, "No extensions to carry over.", "")
	}

	switch {
	case in.SourceMajor == 0 || MajorOf(version) == 0:
		add("version", SeverityWarn, "Could not compare the source and target Postgres versions.", "PreflightCheck them by hand before copying.")
	case MajorOf(version) < in.SourceMajor:
		add("version", SeverityBlock, fmt.Sprintf("The target runs Postgres %d, older than the source's %d.", MajorOf(version), in.SourceMajor),
			"Choose a target version of "+fmt.Sprint(in.SourceMajor)+" or newer.")
	case MajorOf(version) > in.SourceMajor:
		add("version", SeverityWarn, fmt.Sprintf("The target runs Postgres %d, newer than the source's %d.", MajorOf(version), in.SourceMajor),
			"A dump restores across majors, but test the app against it before switching.")
	default:
		add("version", SeverityOK, fmt.Sprintf("Source and target are both Postgres %d.", in.SourceMajor), "")
	}
	return version
}

func requiredBytes(size int64, margin float64) int64 {
	if margin < 1 {
		margin = 1
	}
	return int64(float64(size)*margin) + 64<<20
}

var nonNameRe = regexp.MustCompile(`[^a-z0-9]+`)

// DeriveTargetNames maps source database names to unique, valid managed names.
// Existing names (already taken) are avoided with a numeric suffix.
func DeriveTargetNames(sources []string, taken map[string]bool) map[string]string {
	used := map[string]bool{}
	for n := range taken {
		used[n] = true
	}
	out := make(map[string]string, len(sources))
	for _, src := range sources {
		base := strings.Trim(nonNameRe.ReplaceAllString(strings.ToLower(src), "-"), "-")
		if base == "" || base[0] < 'a' || base[0] > 'z' {
			base = "db-" + base
			base = strings.TrimRight(base, "-")
		}
		if len(base) > 36 {
			base = strings.TrimRight(base[:36], "-")
		}
		name := base
		for n := 2; used[name]; n++ {
			name = fmt.Sprintf("%s-%d", base, n)
		}
		used[name] = true
		out[src] = name
	}
	return out
}

// HumanBytes formats a byte count for messages.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// HumanDuration formats seconds for messages.
func HumanDuration(sec int) string {
	switch {
	case sec < 90:
		return fmt.Sprintf("%d s", sec)
	case sec < 5400:
		return fmt.Sprintf("%d min", (sec+30)/60)
	}
	return fmt.Sprintf("%.1f h", float64(sec)/3600)
}
