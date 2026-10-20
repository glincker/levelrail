package deploy

import (
	"fmt"
	"path/filepath"
	"strings"
)

// resolveBuildRoot joins sourceDir with baseDirectory, or returns sourceDir
// unchanged if baseDirectory is empty. This is the actual security boundary
// against path traversal (spec.Validate only checks the raw string).
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
