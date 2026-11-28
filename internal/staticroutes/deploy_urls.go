package staticroutes

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/glincker/levelrail/internal/store"
)

const (
	defaultKeepDeploys = 10
	minKeepDeploys     = 3
	deployIDChars      = 8
)

// KeepDeploys is how many deploys of one Pages site stay on disk and keep
// their own URL. APP_PAGES_KEEP_DEPLOYS wins, then the older
// APP_STATIC_KEEP_RELEASES; unset or invalid gives 10, never below 3.
func KeepDeploys() int {
	for _, key := range []string{"APP_PAGES_KEEP_DEPLOYS", "APP_STATIC_KEEP_RELEASES"} {
		if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil {
			return max(n, minKeepDeploys)
		}
	}
	return defaultKeepDeploys
}

// DeployHost is the immutable hostname of one deploy: the first 8 letters or
// digits of its id, two hyphens, then the site's fallback hostname. It
// returns "" when either part is unusable.
func DeployHost(fallbackHost, deployID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimPrefix(deployID, "dep_")) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			if b.Len() == deployIDChars {
				break
			}
		}
	}
	if b.Len() < deployIDChars || fallbackHost == "" {
		return ""
	}
	return b.String() + "--" + fallbackHost
}

// RetainedDeploy is a past deploy whose files are still on disk.
type RetainedDeploy struct {
	Attempt store.DeployAttempt
	Dir     string
}

// RetainedDeploys picks, newest first and capped at KeepDeploys, the
// succeeded static deploys of a site whose release directory under svcDir
// still exists. Rollback records are skipped since they reuse a past tree.
func RetainedDeploys(attempts []store.DeployAttempt, svcDir string) []RetainedDeploy {
	keep := KeepDeploys()
	seen := map[string]bool{}
	var out []RetainedDeploy
	for _, a := range attempts {
		if len(out) >= keep {
			break
		}
		if a.Status != store.DeployAttemptStatusSucceeded || a.DigestReason != store.DigestReasonStaticTree ||
			a.CommitSHA == "" || !filepath.IsLocal(a.CommitSHA) || strings.HasPrefix(a.Reason, "RollbackTo:") || seen[a.CommitSHA] {
			continue
		}
		seen[a.CommitSHA] = true
		dir := filepath.Join(svcDir, a.CommitSHA)
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}
		out = append(out, RetainedDeploy{Attempt: a, Dir: dir})
	}
	return out
}
