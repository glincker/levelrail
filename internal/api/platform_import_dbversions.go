package api

import (
	"fmt"
	"strings"
)

// importDBVersions validates the user's per-engine version choices against
// the version catalog and returns them with the newest recommended version
// per engine. Without a catalog only the user's choices pass through.
func (rt *Router) importDBVersions(chosen map[string]string) (map[string]string, map[string]string, error) {
	if rt.dbVersions == nil {
		return chosen, nil, nil
	}
	latest := map[string]string{}
	for _, engine := range rt.dbVersions.Engines() {
		view, err := rt.dbVersions.Versions(engine)
		if err != nil {
			continue
		}
		for _, v := range view.Versions {
			if v.Recommended {
				latest[engine] = v.Version
			}
		}
	}
	for engine, version := range chosen {
		view, err := rt.dbVersions.Versions(engine)
		if err != nil {
			return nil, nil, fmt.Errorf("db_versions: unknown engine %q", engine)
		}
		ok := false
		var known []string
		for _, v := range view.Versions {
			known = append(known, v.Version)
			ok = ok || v.Version == version
		}
		if !ok {
			return nil, nil, fmt.Errorf("db_versions: %s %q is not in the catalog (have %s)", engine, version, strings.Join(known, ", "))
		}
	}
	return chosen, latest, nil
}
