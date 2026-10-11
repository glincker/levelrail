package spec

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// NormalizeRepoPath cleans a repository-relative path typed by an operator.
// An empty result means the repository root. It rejects absolute paths,
// backslashes, NUL bytes and any ".." that would leave the repository.
func NormalizeRepoPath(field, p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil
	}
	if strings.ContainsRune(p, 0) || strings.Contains(p, "\\") {
		return "", fmt.Errorf("%s %q contains an invalid character", field, p)
	}
	if strings.HasPrefix(p, "/") || (len(p) > 1 && p[1] == ':') {
		return "", fmt.Errorf("%s %q must be relative to the repository root, not absolute", field, p)
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%s %q must stay inside the repository", field, p)
	}
	if clean == "." {
		return "", nil
	}
	return clean, nil
}

// ResolveGitBuild turns a git source's persisted build settings into the
// Build a deploy runs. baseDirectory is the build context (empty is the
// repository root); buildPath is the Dockerfile (or static output
// directory) relative to the repository root, never to baseDirectory. The
// returned Build.Path is relative to the context, which is what the deploy
// pipeline joins onto it.
func ResolveGitBuild(buildType, baseDirectory, buildPath string) (Build, error) {
	base, err := NormalizeRepoPath("base directory", baseDirectory)
	if err != nil {
		return Build{}, err
	}
	p, err := NormalizeRepoPath("path", buildPath)
	if err != nil {
		return Build{}, err
	}
	out := Build{Type: buildType, BaseDirectory: base}
	if buildType == BuildRailpack && p != "" {
		return Build{}, errors.New("a path is not meaningful for the auto-detect build pack")
	}
	if p == "" || base == "" {
		out.Path = p
		return out, nil
	}
	rel, ok := strings.CutPrefix(p, base+"/")
	if !ok {
		return Build{}, fmt.Errorf("path %q is outside the base directory %q: paths are relative to the repository root, so it must start with %q", p, base, base+"/")
	}
	out.Path = rel
	return out, nil
}
