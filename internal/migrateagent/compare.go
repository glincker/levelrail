package migrateagent

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/migrationplan"
	"github.com/GLINCKER/levelrail/internal/treestat"
)

// Actor is the checklist actor recorded when the agent advances a step.
const Actor = "migration-agent"

// Comparison statuses.
const (
	StatusMatch         = "match"
	StatusDiffers       = "differs"
	StatusChanged       = "changed_during_scan"
	StatusSourceMissing = "source_missing"
	StatusTargetMissing = "target_missing"
	StatusScanError     = "scan_error"
)

// Where a target measurement came from. Only a measurement this control plane
// took itself can advance the checklist.
const (
	TargetMeasured = "measured"
	TargetOperator = "operator-supplied"
)

// AppVolume pairs an app's volume or bind path on this host with the target it
// was copied from on the source.
type AppVolume struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

// TargetStat is the measured size of a volume on this host.
type TargetStat struct {
	Files int64
	Bytes int64
	Err   string
	// Operator marks numbers typed in by the caller instead of measured here.
	Operator bool
}

// Comparison is the result for one volume.
type Comparison struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	SourceFiles int64  `json:"source_files"`
	SourceBytes int64  `json:"source_bytes"`
	TargetFiles int64  `json:"target_files"`
	TargetBytes int64  `json:"target_bytes"`
	Status      string `json:"status"`
	TargetFrom  string `json:"target_from,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

// SourceTarget maps an imported app's volume back to the source volume name by
// dropping the app prefix the importer added. Bind paths pass through.
func SourceTarget(app, volume string) string {
	if strings.HasPrefix(volume, "/") {
		return volume
	}
	return strings.TrimPrefix(volume, "app-"+app+"-")
}

// Compare lines source scans up against target measurements. Anything other
// than an exact file count and byte match, a scan taken while files changed, or
// a missing side is a diff.
func Compare(vols []AppVolume, scan []StatDoc, target map[string]TargetStat) []Comparison {
	bySource := map[string]StatDoc{}
	for _, s := range scan {
		bySource[s.Target] = s
	}
	out := make([]Comparison, 0, len(vols))
	for _, v := range vols {
		c := Comparison{Name: v.Name, Source: v.Source}
		src, haveSrc := bySource[v.Source]
		tgt, haveTgt := target[v.Name]
		switch {
		case !haveSrc:
			c.Status, c.Detail = StatusSourceMissing, "the agent did not report this volume"
		case src.Error != "":
			c.Status, c.Detail = StatusScanError, src.Error
		case !haveTgt || tgt.Err != "":
			c.Status, c.Detail = StatusTargetMissing, firstNonEmpty(tgt.Err, "no measurement of the volume on this host")
		default:
			c.SourceFiles, c.SourceBytes, c.TargetFiles, c.TargetBytes = src.Files, src.Bytes, tgt.Files, tgt.Bytes
			c.TargetFrom = TargetMeasured
			if tgt.Operator {
				c.TargetFrom = TargetOperator
			}
			switch {
			case src.ChangedDuringScan:
				c.Status, c.Detail = StatusChanged, "files changed on the source while they were counted"
			case src.Files != tgt.Files || src.Bytes != tgt.Bytes:
				c.Status = StatusDiffers
			default:
				c.Status = StatusMatch
			}
		}
		out = append(out, c)
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ZeroDiffs is true only when there is something to compare and all of it matches.
func ZeroDiffs(cs []Comparison) bool {
	if len(cs) == 0 {
		return false
	}
	for _, c := range cs {
		if c.Status != StatusMatch {
			return false
		}
	}
	return true
}

// AdvanceResult says what AutoAdvance did.
type AdvanceResult struct {
	Advanced bool     `json:"advanced"`
	Reason   string   `json:"reason,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

// ErrNotAgentStep means Reopen found a data_verified step the agent did not complete.
var ErrNotAgentStep = errors.New("migrateagent: data_verified was not completed by the agent")

// AdvanceOptions carries the operator's explicit acknowledgements.
type AdvanceOptions struct {
	// AckEmpty accepts volumes that hold no files on either side.
	AckEmpty bool
}

// OverrideCount is how many comparisons used caller-supplied target numbers.
func OverrideCount(cs []Comparison) int {
	n := 0
	for _, c := range cs {
		if c.TargetFrom == TargetOperator {
			n++
		}
	}
	return n
}

// EmptyCount is how many matched volumes are empty on the source.
func EmptyCount(cs []Comparison) int {
	n := 0
	for _, c := range cs {
		if c.Status == StatusMatch && c.SourceFiles == 0 && c.SourceBytes == 0 {
			n++
		}
	}
	return n
}

// AutoAdvance completes data_verified when every volume matches. It never
// touches a step after data_verified, and it never runs for any other step.
// Databases are not measured by the agent, so their presence is recorded in the
// evidence instead of being claimed as verified.
func AutoAdvance(c *migrationplan.Checklist, cs []Comparison, databases int, opts AdvanceOptions, now time.Time) AdvanceResult {
	if cur := c.Current(); cur != migrationplan.StepDataVerified {
		return AdvanceResult{Reason: "the checklist is not waiting on data_verified (current step: " + cur + ")"}
	}
	if len(cs) == 0 {
		return AdvanceResult{Reason: "the app has no volumes to compare"}
	}
	if !ZeroDiffs(cs) {
		return AdvanceResult{Reason: "volumes differ or could not be compared"}
	}
	if n := OverrideCount(cs); n > 0 {
		return AdvanceResult{Reason: fmt.Sprintf("%d volume(s) use operator-supplied target numbers, so the agent cannot vouch for them: complete the data_verified step manually", n)}
	}
	empty := EmptyCount(cs)
	if empty > 0 && !opts.AckEmpty {
		return AdvanceResult{Reason: fmt.Sprintf("%d volume(s) are empty on the source: confirm they are meant to be empty and verify again with the empty-volume acknowledgement", empty)}
	}
	stats := make([]migrationplan.VolumeStat, 0, len(cs))
	names := make([]string, 0, len(cs))
	for _, v := range cs {
		stats = append(stats, migrationplan.VolumeStat{Name: v.Name, SourceFiles: v.SourceFiles, SourceBytes: v.SourceBytes,
			TargetFiles: v.TargetFiles, TargetBytes: v.TargetBytes})
		names = append(names, v.Name)
	}
	evidence, problems := migrationplan.VerifyData(nil, names, stats)
	if len(problems) > 0 {
		return AdvanceResult{Reason: strings.Join(problems, "; ")}
	}
	if empty > 0 {
		evidence = append(evidence, fmt.Sprintf("%d empty volume(s) acknowledged by the operator", empty))
	}
	if databases > 0 {
		evidence = append(evidence, fmt.Sprintf("%d database(s) on the source are not checked by the agent: verify them separately", databases))
	}
	if err := c.Complete(migrationplan.StepDataVerified, Actor, "volumes verified by the migration agent", evidence, now); err != nil {
		return AdvanceResult{Reason: err.Error()}
	}
	return AdvanceResult{Advanced: true, Evidence: evidence}
}

// Reopen undoes an agent advance. Later steps are reset with it because they
// depended on the data being verified.
func Reopen(c *migrationplan.Checklist) error {
	if c.States[migrationplan.StepDataVerified].Actor != Actor {
		return ErrNotAgentStep
	}
	if err := c.Reset(migrationplan.StepDataVerified); err != nil {
		return fmt.Errorf("migrateagent: reopen: %w", err)
	}
	return nil
}

// MeasureTarget walks a volume's data directory under dir, or a bind path as given.
func MeasureTarget(ctx context.Context, dir, name string) TargetStat {
	root := name
	if strings.Contains(name, "..") {
		return TargetStat{Err: "volume name is not valid"}
	}
	if !strings.HasPrefix(name, "/") {
		root = filepath.Join(dir, name, "_data")
	}
	st, err := treestat.Walk(ctx, root)
	if err != nil {
		return TargetStat{Err: err.Error()}
	}
	return TargetStat{Files: st.Files, Bytes: st.Bytes}
}
