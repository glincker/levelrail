package sourceupload

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// DefaultIgnores are never uploaded unless named in Options.Include. Secrets
// and dependency or cache directories stay on the laptop by default.
var DefaultIgnores = []string{
	".git", ".svn", ".hg", "node_modules", ".next", ".nuxt", ".turbo", ".venv", "venv",
	"__pycache__", "target", "coverage", ".terraform", ".DS_Store",
	".env", ".env.*", "*.pem", "*.key", "id_rsa*", "id_ed25519*",
}

// EditorConfigDirs are directory names holding personal editor or agent
// settings. Files below them are skipped unless named in Options.Include.
var EditorConfigDirs = []string{".claude", ".cursor", ".windsurf", ".codex", ".idea", ".fleet", ".history"}

// EditorConfigFiles are file names, and EditorConfigGlobs base name patterns,
// that are machine-local and never needed by a build.
var (
	EditorConfigFiles = []string{".DS_Store", "Thumbs.db"}
	EditorConfigGlobs = []string{"*.local.json", "*.local.yaml", "*.local.yml", "settings.local.*"}
)

// editorConfig reports whether a file path is personal editor or agent config.
// Dot-aider entries are matched by prefix, .vscode only for settings and workspaces.
func editorConfig(rel string) bool {
	parts := strings.Split(rel, "/")
	base := parts[len(parts)-1]
	for i, c := range parts {
		for _, d := range EditorConfigDirs {
			if c == d {
				return true
			}
		}
		if strings.HasPrefix(c, ".aider") {
			return true
		}
		if c == ".vscode" && i == len(parts)-2 && (base == "settings.json" || strings.HasSuffix(base, ".code-workspace")) {
			return true
		}
	}
	for _, f := range EditorConfigFiles {
		if base == f {
			return true
		}
	}
	for _, g := range EditorConfigGlobs {
		if ok, _ := path.Match(g, base); ok {
			return true
		}
	}
	return false
}

// envTemplates are .env.* files that carry no secrets by convention.
var envTemplates = []string{".env.example", ".env.sample", ".env.template"}

// ignoreSet decides which paths are excluded from an archive.
type ignoreSet struct {
	defaults gitignore.Matcher
	matcher  gitignore.Matcher
	patterns []gitignore.Pattern
	include  []string
}

func newIgnoreSet(include []string) *ignoreSet {
	s := &ignoreSet{include: include}
	var def []gitignore.Pattern
	for _, p := range DefaultIgnores {
		def = append(def, gitignore.ParsePattern(p, nil))
	}
	s.defaults = gitignore.NewMatcher(def)
	s.matcher = gitignore.NewMatcher(nil)
	return s
}

func (s *ignoreSet) addFile(absFile string, domain []string) {
	f, err := os.Open(absFile) //nolint:gosec // reads a .gitignore inside the directory being packed
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	added := false
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		s.patterns = append(s.patterns, gitignore.ParsePattern(line, domain))
		added = true
	}
	if added {
		s.matcher = gitignore.NewMatcher(s.patterns)
	}
}

func (s *ignoreSet) included(rel string) bool {
	for _, inc := range s.include {
		inc = strings.Trim(filepath.ToSlash(inc), "/")
		if inc == "" {
			continue
		}
		if ok, _ := path.Match(inc, rel); ok || rel == inc || strings.HasPrefix(rel, inc+"/") || strings.HasPrefix(inc, rel+"/") {
			return true
		}
	}
	return false
}

// excluded reports whether rel (slash separated, relative to the root) is left out.
// The defaults are matched separately so a .gitignore negation cannot re-admit a secret.
func (s *ignoreSet) excluded(rel string, isDir bool) bool {
	if s.included(rel) {
		return false
	}
	parts := strings.Split(rel, "/")
	if s.matcher.Match(parts, isDir) {
		return true
	}
	base := path.Base(rel)
	for _, t := range envTemplates {
		if base == t {
			return false
		}
	}
	return s.defaults.Match(parts, isDir)
}

// editorSkipped reports a file left out as editor or agent config; an include wins.
func (s *ignoreSet) editorSkipped(rel string) bool {
	return !s.included(rel) && editorConfig(rel)
}
