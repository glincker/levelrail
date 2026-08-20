package deploy

import (
	"fmt"
	"path/filepath"
	"strings"
)

// resolveBuildRoot returns the effective build root for a service:
// sourceDir itself when baseDirectory is empty, or sourceDir joined with
// baseDirectory otherwise. This is the real security boundary for
// build.baseDirectory: it rejects anything that would resolve outside
// sourceDir (a "../" traversal or an absolute path), so a malicious or
// mistaken app.yaml can never point BuildKit's context, a Dockerfile
// lookup, or the static-site copy at a directory outside the git
// checkout it came from. spec.Validate's own check on the raw string is
// a friendlier, earlier version of this same rule; this is the one that
// actually gates filesystem access.
func resolveBuildRoot(sourceDir, baseDirectory string) (string, error) {
	if baseDirectory == "" {
		return sourceDir, nil
	}

	cleanBase := filepath.Clean(baseDirectory)
	if filepath.IsAbs(cleanBase) {
		return "", fmt.Errorf("build.baseDirectory %q must be a relative path", baseDirectory)
	}

	root := filepath.Clean(sourceDir)
	joined := filepath.Join(root, cleanBase)
	if joined != root && !strings.HasPrefix(joined, root+string(filepath.Separator)) {
		return "", fmt.Errorf("build.baseDirectory %q escapes the repository root", baseDirectory)
	}

	return joined, nil
}
