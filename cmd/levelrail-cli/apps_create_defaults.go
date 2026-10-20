package main

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/kit/stackdetect"
)

var nonNameChars = regexp.MustCompile(`[^a-z0-9]+`)

// applyGitBuildDefaults fills what the git-build path can work out itself: name from the repo URL, image repo from the name,
// and, when the repo is the local checkout, build type and port from the stack. Explicit flags always win.
func applyGitBuildDefaults(f *createFlags, detected detectedGit, dir string, remotePort func(repo, ref string) (int, bool)) {
	if f.image != "" || f.file != "" {
		return
	}
	repo := f.repo
	if repo == "" {
		repo = detected.RepoURL
	}
	if repo == "" {
		return
	}
	if f.name == "" {
		f.name = repoBaseName(repo)
	}
	if f.imageRepo == "" {
		f.imageRepo = f.name
	}
	if f.repo == "" || f.repo == detected.RepoURL {
		if st, err := stackdetect.Detect(filepath.Join(dir, f.baseDirectory)); err == nil && st.Detected() {
			if f.buildType == "" && st.Build == stackdetect.BuildRailpack {
				f.buildType = stackdetect.BuildRailpack
			}
			if f.port <= 0 && st.Port > 0 {
				f.port = st.Port
			}
		}
	}
	if f.port <= 0 && remotePort != nil {
		if p, ok := remotePort(repo, resolveRef(f.ref, detected.Ref)); ok {
			f.port = p
		}
	}
}

// repoBaseName turns https://host/org/My_App.git or git@host:org/my-app into a valid app name.
func repoBaseName(repo string) string {
	repo = strings.TrimSuffix(strings.TrimRight(repo, "/"), ".git")
	if i := strings.LastIndexAny(repo, "/:"); i >= 0 {
		repo = repo[i+1:]
	}
	name := strings.Trim(nonNameChars.ReplaceAllString(strings.ToLower(repo), "-"), "-")
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return ""
	}
	return name
}
