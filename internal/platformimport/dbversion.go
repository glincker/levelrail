package platformimport

import (
	"fmt"
	"strings"
)

// ParseDBVersions reads "postgres=16,redis=7" into an engine to version map.
func ParseDBVersions(raw string) (map[string]string, error) {
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		engine, version, ok := strings.Cut(part, "=")
		engine, version = strings.ToLower(strings.TrimSpace(engine)), strings.TrimSpace(version)
		if !ok || engine == "" || version == "" {
			return nil, fmt.Errorf("invalid database version %q, want engine=version", part)
		}
		out[engine] = version
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// chooseDBVersion picks a version for a database whose source reports none:
// the user's choice, else the newest catalog version, with a warning either way.
func chooseDBVersion(db Database, o PlanOptions) (string, []string) {
	if v := o.DBVersions[db.Engine]; v != "" {
		return v, []string{fmt.Sprintf("the source does not report the %s version, using the chosen %s: check compatibility with the source data before restoring", db.Engine, v)}
	}
	if v := o.LatestDBVersions[db.Engine]; v != "" {
		return v, []string{fmt.Sprintf("the source does not report the %s version, using the newest supported (%s): check compatibility with the source data before restoring, or choose one with the db-version option", db.Engine, v)}
	}
	return "", nil
}
