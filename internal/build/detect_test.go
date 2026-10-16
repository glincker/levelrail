package build

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestValidatePublicRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		wantErr bool
	}{
		{name: "https", repoURL: "https://github.com/acme/app.git"},
		{name: "http", repoURL: "http://internal.example/app.git"},
		{name: "file scheme rejected", repoURL: "file:///etc/passwd", wantErr: true},
		{name: "ssh scheme rejected", repoURL: "ssh://git@github.com/acme/app.git", wantErr: true},
		{name: "unparseable", repoURL: "://not a url", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePublicRepoURL(tt.repoURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePublicRepoURL(%q) error = %v, wantErr %v", tt.repoURL, err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !errors.Is(err, ErrRepoURLSchemeNotAllowed) {
				// The unparseable case fails at url.Parse, before the
				// scheme check, so it never wraps this sentinel; every
				// other rejection must.
				if tt.name != "unparseable" {
					t.Errorf("ValidatePublicRepoURL(%q) error = %v, want wrapping ErrRepoURLSchemeNotAllowed", tt.repoURL, err)
				}
			}
		})
	}
}

func TestDetect_RejectsNonHTTPScheme(t *testing.T) {
	_, err := Detect(context.Background(), DetectRequest{RepoURL: "file:///etc/passwd"})
	if !errors.Is(err, ErrRepoURLSchemeNotAllowed) {
		t.Fatalf("Detect() error = %v, want ErrRepoURLSchemeNotAllowed", err)
	}
}

// localRepoFrom builds a real, single-commit git repository in a fresh
// temp dir containing a copy of fixtureDir's files, so detectUnchecked
// can clone it exactly like a real remote (go-git resolves a plain
// filesystem path as a "file" transport endpoint, the same free-of-any-
// network-dependency tradeoff internal/api/git_branches_live_test.go's
// own TestListRemoteBranches_Live already documents), without a live
// GitHub dependency.
func localRepoFrom(t *testing.T, fixtureDir string) string {
	t.Helper()
	srcDir := t.TempDir()

	err := filepath.WalkDir(fixtureDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(fixtureDir, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		dest := filepath.Join(srcDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o750)
		}
		// path is always a WalkDir-yielded child of the hardcoded
		// testdata fixture this test passed in, never external input.
		data, readErr := os.ReadFile(path) //nolint:gosec // fixed testdata fixture path, not user input
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(dest, data, 0o600) //nolint:gosec // dest is derived from t.TempDir() plus a WalkDir-relative path under the fixed testdata fixture, not user input
	})
	if err != nil {
		t.Fatalf("copy fixture %q: %v", fixtureDir, err)
	}

	repo, err := git.PlainInit(srcDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sig := &object.Signature{Name: "Levelrail Test", Email: "test@example.invalid", When: time.Now()}
	if _, err := wt.Commit("fixture commit", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return srcDir
}

func TestDetectUnchecked(t *testing.T) {
	tests := []struct {
		name          string
		fixture       string
		wantProvider  string
		wantFramework string
	}{
		{name: "node", fixture: "testdata/railpack-node", wantProvider: "node", wantFramework: "Node.js"},
		{name: "next.js app router", fixture: "testdata/railpack-nextjs", wantProvider: "node", wantFramework: "Next.js"},
		{name: "go", fixture: "testdata/railpack-go", wantProvider: "golang", wantFramework: "Go"},
		{name: "python", fixture: "testdata/railpack-python", wantProvider: "python", wantFramework: "Python (Django)"},
		// Railpack itself detects a real provider here (ruby), just one
		// outside supportedRailpackProviders: Provider still reports what
		// Railpack found, FrameworkName stays empty so the wizard falls
		// back to manual build-type selection rather than claiming a name
		// for a stack it can't actually build yet.
		{name: "unsupported provider reports raw provider, no framework name", fixture: "testdata/railpack-unsupported", wantProvider: "ruby", wantFramework: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoDir := localRepoFrom(t, tt.fixture)
			result, err := detectUnchecked(context.Background(), DetectRequest{RepoURL: repoDir})
			if err != nil {
				t.Fatalf("detectUnchecked() error = %v", err)
			}
			if result.Provider != tt.wantProvider {
				t.Errorf("Provider = %q, want %q", result.Provider, tt.wantProvider)
			}
			if result.FrameworkName != tt.wantFramework {
				t.Errorf("FrameworkName = %q, want %q", result.FrameworkName, tt.wantFramework)
			}
		})
	}
}

func TestDetectUnchecked_NonexistentRemote(t *testing.T) {
	_, err := detectUnchecked(context.Background(), DetectRequest{RepoURL: filepath.Join(t.TempDir(), "does-not-exist")})
	if err == nil {
		t.Fatal("detectUnchecked() error = nil, want a clone error for a nonexistent remote")
	}
}

func TestNextJSLabel(t *testing.T) {
	tests := []struct {
		name    string
		pkgJSON string // empty means no package.json is written at all
		want    string
	}{
		{name: "next dependency", pkgJSON: `{"dependencies":{"next":"15.5.0"}}`, want: "Next.js"},
		{name: "next devDependency", pkgJSON: `{"devDependencies":{"next":"15.5.0"}}`, want: "Next.js"},
		{name: "no next dependency", pkgJSON: `{"dependencies":{"express":"4.19.0"}}`, want: ""},
		{name: "missing package.json", pkgJSON: "", want: ""},
		{name: "malformed package.json", pkgJSON: `{not json`, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.pkgJSON != "" {
				if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(tt.pkgJSON), 0o600); err != nil {
					t.Fatalf("write package.json: %v", err)
				}
			}
			if got := nextJSLabel(dir); got != tt.want {
				t.Errorf("nextJSLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDirSize_StopsAtCap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), make([]byte, 100), 0o600); err != nil {
		t.Fatalf("write a.bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.bin"), make([]byte, 100), 0o600); err != nil {
		t.Fatalf("write b.bin: %v", err)
	}

	size, err := dirSize(dir, 50)
	if err != nil {
		t.Fatalf("dirSize() error = %v", err)
	}
	if size <= 50 {
		t.Errorf("dirSize() = %d, want > cap (50) once exceeded", size)
	}
}

func TestDirSize_UnderCap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), make([]byte, 10), 0o600); err != nil {
		t.Fatalf("write a.bin: %v", err)
	}

	size, err := dirSize(dir, 1000)
	if err != nil {
		t.Fatalf("dirSize() error = %v", err)
	}
	if size != 10 {
		t.Errorf("dirSize() = %d, want 10", size)
	}
}

func TestExtractPort(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   int
	}{
		{name: "--port=4173", script: "vite preview --port=4173", want: 4173},
		{name: "--port 4173 (spaced)", script: "vite preview --port 4173", want: 4173},
		{name: "-p flag", script: "node -p 3000 app.js", want: 3000},
		{name: "PORT= prefix", script: "PORT=5000 npm start", want: 5000},
		{name: "-l listen syntax", script: "node -l tcp://0.0.0.0:8080 server.js", want: 8080},
		{name: "multiple flags, first wins", script: "--port 3000 --port 4000", want: 3000},
		{name: "no port flag", script: "node index.js", want: 0},
		{name: "empty string", script: "", want: 0},
		{name: "invalid port number", script: "--port abc", want: 0},
		{name: "port out of range", script: "--port 70000", want: 0},
		{name: "port 0", script: "--port 0", want: 0},
		{name: "port 1", script: "--port 1", want: 1},
		{name: "port 65535", script: "--port 65535", want: 65535},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractPort(tt.script); got != tt.want {
				t.Errorf("extractPort(%q) = %d, want %d", tt.script, got, tt.want)
			}
		})
	}
}

func TestDetectFramework(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{name: "vite preview", script: "vite preview --host --port 4173", want: "vite preview"},
		{name: "vite dev", script: "vite dev", want: "vite dev"},
		{name: "next start", script: "next start", want: "next start"},
		{name: "next dev", script: "next dev", want: "next dev"},
		{name: "nuxt preview", script: "nuxt preview", want: "nuxt preview"},
		{name: "astro preview", script: "astro preview", want: "astro preview"},
		{name: "remix-serve", script: "remix-serve ./build/index.js", want: "remix-serve"},
		{name: "sveltekit fallback", script: "node build", want: ""},
		{name: "serve simple", script: "serve -s build", want: "serve"},
		{name: "http-server", script: "http-server", want: "http-server"},
		{name: "unknown", script: "node index.js", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectFramework(tt.script); got != tt.want {
				t.Errorf("detectFramework(%q) = %q, want %q", tt.script, got, tt.want)
			}
		})
	}
}

func TestFrameworkPort(t *testing.T) {
	tests := []struct {
		name      string
		framework string
		want      int
	}{
		{name: "vite preview", framework: "vite preview", want: 4173},
		{name: "vite dev", framework: "vite dev", want: 5173},
		{name: "next dev", framework: "next dev", want: 3000},
		{name: "next start", framework: "next start", want: 3000},
		{name: "nuxt dev", framework: "nuxt dev", want: 3000},
		{name: "astro preview", framework: "astro preview", want: 4321},
		{name: "astro dev", framework: "astro dev", want: 4321},
		{name: "remix-serve", framework: "remix-serve", want: 3000},
		{name: "sveltekit", framework: "sveltekit", want: 3000},
		{name: "serve", framework: "serve", want: 3000},
		{name: "http-server", framework: "http-server", want: 8080},
		{name: "unknown", framework: "unknown", want: 0},
		{name: "empty", framework: "", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frameworkPort(tt.framework); got != tt.want {
				t.Errorf("frameworkPort(%q) = %d, want %d", tt.framework, got, tt.want)
			}
		})
	}
}

func TestDetectNodePort(t *testing.T) {
	tests := []struct {
		name         string
		pkgJSON      string
		wantPort     int
		wantSourceOK bool
	}{
		{
			name:         "scripts.start with --port flag",
			pkgJSON:      `{"scripts":{"start":"vite preview --host --port 4173"}}`,
			wantPort:     4173,
			wantSourceOK: true,
		},
		{
			name:         "scripts.preview fallback",
			pkgJSON:      `{"scripts":{"preview":"vite preview --port 3001"}}`,
			wantPort:     3001,
			wantSourceOK: true,
		},
		{
			name:         "scripts.start with framework default (vite preview)",
			pkgJSON:      `{"scripts":{"start":"vite preview"}}`,
			wantPort:     4173,
			wantSourceOK: true,
		},
		{
			name:         "scripts.start with framework default (next dev)",
			pkgJSON:      `{"scripts":{"start":"next dev"}}`,
			wantPort:     3000,
			wantSourceOK: true,
		},
		{
			name:         "no scripts field",
			pkgJSON:      `{}`,
			wantPort:     0,
			wantSourceOK: false,
		},
		{
			name:         "missing package.json",
			pkgJSON:      "",
			wantPort:     0,
			wantSourceOK: false,
		},
		{
			name:         "malformed package.json",
			pkgJSON:      `{not json}`,
			wantPort:     0,
			wantSourceOK: false,
		},
		{
			name:         "start with PORT= prefix",
			pkgJSON:      `{"scripts":{"start":"PORT=8080 node server.js"}}`,
			wantPort:     8080,
			wantSourceOK: true,
		},
		{
			name:         "start with -p flag",
			pkgJSON:      `{"scripts":{"start":"node -p 9000 app.js"}}`,
			wantPort:     9000,
			wantSourceOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.pkgJSON != "" {
				if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(tt.pkgJSON), 0o600); err != nil {
					t.Fatalf("write package.json: %v", err)
				}
			}
			port, source := detectNodePort(dir)
			if port != tt.wantPort {
				t.Errorf("detectNodePort() port = %d, want %d", port, tt.wantPort)
			}
			if (source != "") != tt.wantSourceOK {
				t.Errorf("detectNodePort() source = %q, want non-empty=%v", source, tt.wantSourceOK)
			}
		})
	}
}

func TestDetectUnchecked_ViteReact(t *testing.T) {
	repoDir := localRepoFrom(t, "testdata/railpack-vite-react")
	result, err := detectUnchecked(context.Background(), DetectRequest{RepoURL: repoDir})
	if err != nil {
		t.Fatalf("detectUnchecked() error = %v", err)
	}
	if result.Provider != "node" {
		t.Errorf("Provider = %q, want %q", result.Provider, "node")
	}
	if result.Port != 4173 {
		t.Errorf("Port = %d, want 4173", result.Port)
	}
	if result.PortSource == "" {
		t.Errorf("PortSource = %q, want non-empty", result.PortSource)
	}
}
