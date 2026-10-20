package migrationplan

import (
	"fmt"
	"strings"
)

// VolumeStat is the file count and total size of one volume, measured on
// both hosts by the operator (the control plane never reaches the source).
type VolumeStat struct {
	Name        string `json:"name"`
	SourceFiles int64  `json:"source_files"`
	SourceBytes int64  `json:"source_bytes"`
	TargetFiles int64  `json:"target_files"`
	TargetBytes int64  `json:"target_bytes"`
}

// VolumeCheck is the comparison of one volume.
type VolumeCheck struct {
	VolumeStat
	OK bool `json:"ok"`
}

// CompareVolumes requires identical file counts and sizes on both sides.
func CompareVolumes(stats []VolumeStat) []VolumeCheck {
	out := make([]VolumeCheck, 0, len(stats))
	for _, s := range stats {
		out = append(out, VolumeCheck{VolumeStat: s, OK: s.SourceFiles == s.TargetFiles && s.SourceBytes == s.TargetBytes})
	}
	return out
}

// DatabaseResult is a database copy's recorded verification.
type DatabaseResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Checked    int    `json:"checked"`
	Mismatched int    `json:"mismatched"`
	Found      bool   `json:"found"`
}

// VerifyData decides the data_verified step. wantVolumes lists the app's
// volume names that need a stat; it returns the evidence lines and the
// reasons it cannot pass.
func VerifyData(dbs []DatabaseResult, wantVolumes []string, stats []VolumeStat) (evidence, problems []string) {
	for _, d := range dbs {
		switch {
		case !d.Found:
			problems = append(problems, fmt.Sprintf("database %s has no data copy yet", d.Name))
		case d.Status != "verified":
			problems = append(problems, fmt.Sprintf("database %s copy is %s, not verified", d.Name, d.Status))
		case d.Checked == 0:
			problems = append(problems, fmt.Sprintf("database %s verified no tables", d.Name))
		case d.Mismatched > 0:
			problems = append(problems, fmt.Sprintf("database %s: %d of %d tables differ", d.Name, d.Mismatched, d.Checked))
		default:
			evidence = append(evidence, fmt.Sprintf("database %s: row counts match on %d tables", d.Name, d.Checked))
		}
	}
	byName := map[string]VolumeCheck{}
	for _, c := range CompareVolumes(stats) {
		byName[c.Name] = c
	}
	for _, v := range wantVolumes {
		c, ok := byName[v]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("volume %s has no file count and size comparison", v))
		case !c.OK:
			problems = append(problems, fmt.Sprintf("volume %s differs: source %d files %d bytes, target %d files %d bytes", v, c.SourceFiles, c.SourceBytes, c.TargetFiles, c.TargetBytes))
		default:
			evidence = append(evidence, fmt.Sprintf("volume %s: %d files, %d bytes on both sides", v, c.SourceFiles, c.SourceBytes))
		}
	}
	if len(dbs) == 0 && len(wantVolumes) == 0 {
		evidence = append(evidence, "no databases or volumes to compare")
	}
	return evidence, problems
}

// VolumeStatCommand is StatCommand for a named Docker volume on this host.
func VolumeStatCommand(volume string) string {
	q := "'" + strings.ReplaceAll(volume, "'", `'\''`) + "'"
	return `d=$(docker volume inspect -f '{{.Mountpoint}}' ` + q + `); echo $(find "$d" -type f | wc -l) $(du -sb "$d" | cut -f1)`
}

// StatCommand prints "files bytes" for a directory, for the operator to run
// on the source and on the target and paste back.
func StatCommand(dir string) string {
	q := "'" + strings.ReplaceAll(dir, "'", `'\''`) + "'"
	return "echo $(find " + q + " -type f | wc -l) $(du -sb " + q + " | cut -f1)"
}
