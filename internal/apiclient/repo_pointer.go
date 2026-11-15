package apiclient

import (
	"bufio"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// RepoPointerPath is the repo-relative file that names a profile and API URL, never a secret.
const RepoPointerPath = ".levelrail/profile"

// RepoPointer is the parsed pointer file; Path is where it was found.
type RepoPointer struct {
	Profile string
	APIURL  string
	Path    string
}

// FindRepoPointer walks up from dir to the first pointer file. Only the
// profile and api_url keys are read, so a pasted secret is never picked up.
func FindRepoPointer(dir string) (RepoPointer, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return RepoPointer{}, false
	}
	for {
		path := filepath.Join(abs, filepath.FromSlash(RepoPointerPath))
		if p, ok := readRepoPointer(path); ok {
			return p, true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return RepoPointer{}, false
		}
		abs = parent
	}
}

// FindRepoPointerFromCwd is FindRepoPointer from the process working directory.
func FindRepoPointerFromCwd() (RepoPointer, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return RepoPointer{}, false
	}
	return FindRepoPointer(wd)
}

func readRepoPointer(path string) (RepoPointer, bool) {
	f, err := os.Open(path) //nolint:gosec // fixed pointer file name found by walking up from the working directory
	if err != nil {
		return RepoPointer{}, false
	}
	defer func() { _ = f.Close() }()
	p := RepoPointer{Path: path}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "profile":
			p.Profile = value
		case "api_url":
			if u, err := url.Parse(value); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
				p.APIURL = value
			}
		}
	}
	if p.Profile == "" && p.APIURL == "" {
		return RepoPointer{}, false
	}
	return p, true
}

// RepoPointerContent renders the pointer file body.
func RepoPointerContent(profile, apiURL string) string {
	var sb strings.Builder
	sb.WriteString("# Which saved login to use here. No secrets belong in this file.\n")
	if profile != "" {
		sb.WriteString("profile=" + profile + "\n")
	}
	if apiURL != "" {
		sb.WriteString("api_url=" + apiURL + "\n")
	}
	return sb.String()
}
