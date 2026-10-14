package database

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/store"
)

// pinnedVersionPatterns are the tag shapes these images publish. A pinned
// tag is required: "latest" would change the on-disk format under a restart.
var pinnedVersionPatterns = map[string]*regexp.Regexp{
	store.EngineMeilisearch: regexp.MustCompile(`^v1\.(\d+)(\.\d+)?$`),
	store.EngineQdrant:      regexp.MustCompile(`^v1\.(\d+)(\.\d+)?$`),
	store.EngineTypesense:   regexp.MustCompile(`^(\d+)\.(\d+)(\.\d+)?$`),
	store.EngineMemcached:   regexp.MustCompile(`^1\.(\d+)(\.\d+)?$`),
}

func validatePinnedVersion(engine, version string) error {
	pat, ok := pinnedVersionPatterns[engine]
	if !ok {
		return nil
	}
	if !pat.MatchString(version) {
		return fmt.Errorf("version %q is not a pinned %s image tag: use a release such as %q", version, engine, exampleVersion[engine])
	}
	return nil
}

var exampleVersion = map[string]string{
	store.EngineMeilisearch: "v1.55",
	store.EngineQdrant:      "v1.19",
	store.EngineTypesense:   "30.2",
	store.EngineMemcached:   "1.6",
}

var majorMinorPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)`)

// majorMinor reads the first two numeric components, ignoring a "v" prefix.
func majorMinor(v string) (int, int, bool) {
	m := majorMinorPattern.FindStringSubmatch(v)
	if m == nil {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(m[1])
	minor, err2 := strconv.Atoi(m[2])
	return major, minor, err1 == nil && err2 == nil
}

// checkSearchVersionChange applies each engine's documented upgrade rule:
// Meilisearch data is tied to its minor line, Qdrant upgrades one minor at
// a time, and none of them supports a downgrade.
func checkSearchVersionChange(engine, from, to string) (handled bool, err error) {
	if _, ok := pinnedVersionPatterns[engine]; !ok {
		return false, nil
	}
	if engine == store.EngineMemcached {
		// Nothing on disk to be incompatible with: any pinned version works.
		return true, nil
	}
	fMaj, fMin, ok1 := majorMinor(from)
	tMaj, tMin, ok2 := majorMinor(to)
	if !ok1 || !ok2 {
		return true, fmt.Errorf("cannot compare %q with %q: use pinned versions, or restore a backup into a new database", from, to)
	}
	if tMaj < fMaj || (tMaj == fMaj && tMin < fMin) {
		return true, fmt.Errorf("downgrading %s from %s to %s is not supported: its data files are not backward compatible", engine, from, to)
	}
	switch engine {
	case store.EngineMeilisearch:
		if tMaj != fMaj || tMin != fMin {
			return true, fmt.Errorf("meilisearch data is tied to its %d.%d release line: changing to %s in place would refuse to start; take a backup (a dump) and restore it into a new database on %s", fMaj, fMin, to, to)
		}
	case store.EngineQdrant:
		if tMaj != fMaj || tMin > fMin+1 {
			return true, fmt.Errorf("qdrant upgrades one minor version at a time: move from %s to v%d.%d first", from, fMaj, fMin+1)
		}
	}
	return true, nil
}
