package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// NextProfile describes a Next.js project found in a source directory.
type NextProfile struct {
	Detected      bool
	Major         int
	Standalone    bool
	UsesAppRouter bool
}

var (
	nextMajorRe  = regexp.MustCompile(`\d+`)
	standaloneRe = regexp.MustCompile(`output\s*:\s*['"` + "`" + `]standalone['"` + "`" + `]`)
	lockNextRes  = []*regexp.Regexp{
		regexp.MustCompile(`"node_modules/next":\s*\{\s*"version":\s*"(\d+)\.`),
		regexp.MustCompile(`(?m)^"?next@[^:\n]*:\n\s+version "?(\d+)\.`),
	}
)

// DetectNext inspects dir without network access; any unreadable file counts as absent.
func DetectNext(dir string) NextProfile {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json")) //nolint:gosec // dir is the build source root
	if err != nil {
		return NextProfile{}
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return NextProfile{}
	}
	rng, ok := pkg.Dependencies["next"]
	if !ok {
		rng, ok = pkg.DevDependencies["next"]
	}
	if !ok {
		return NextProfile{}
	}
	p := NextProfile{Detected: true, Major: nextMajorFromLock(dir)}
	if p.Major == 0 {
		if m := nextMajorRe.FindString(rng); m != "" {
			p.Major, _ = strconv.Atoi(m)
		}
	}
	for _, name := range []string{"next.config.js", "next.config.mjs", "next.config.cjs", "next.config.ts", "next.config.mts"} {
		if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil && standaloneRe.Match(b) { //nolint:gosec // fixed names under the source root
			p.Standalone = true
			break
		}
	}
	p.UsesAppRouter = hasDir(dir, "app") || hasDir(dir, filepath.Join("src", "app"))
	return p
}

func hasDir(root, rel string) bool {
	st, err := os.Stat(filepath.Join(root, rel))
	return err == nil && st.IsDir()
}

// nextMajorFromLock reads the resolved version from an npm or yarn lockfile, 0 when unknown.
func nextMajorFromLock(dir string) int {
	for _, f := range []string{"package-lock.json", "yarn.lock"} {
		b, err := os.ReadFile(filepath.Join(dir, f)) //nolint:gosec // fixed names under the source root
		if err != nil {
			continue
		}
		for _, re := range lockNextRes {
			if m := re.FindSubmatch(b); m != nil {
				n, _ := strconv.Atoi(string(m[1]))
				return n
			}
		}
	}
	return 0
}
