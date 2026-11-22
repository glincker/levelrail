package agentupdateguard

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/agentupdate"
	"github.com/GLINCKER/levelrail/internal/cliupdate"
	"github.com/GLINCKER/levelrail/internal/nodeinsight"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

// release is the same Fetcher the agent uses, pointed at an in-process server,
// with signature verification stubbed because no signing service exists here.
type release struct{ f selfupgrade.Fetcher }

func (r release) Fetch(ctx context.Context, tag, asset, dir string) (selfupgrade.Fetched, error) {
	return r.f.Fetch(ctx, tag, asset, dir)
}
func (release) Verify(context.Context, string, string) (string, error) { return "stub-verified", nil }

type svc struct{ restarts int }

func (s *svc) Restart(context.Context) error { s.restarts++; return nil }

func fakeServer(t *testing.T, asset, tag, reported string) (*httptest.Server, *[]string) {
	t.Helper()
	body := "#!/bin/sh\necho " + reported + "\n"
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/" + tag + "/" + asset:
			_, _ = fmt.Fprint(w, body)
		case "/" + tag + "/checksums.txt":
			_, _ = fmt.Fprintf(w, "%x  %s\n", sha256.Sum256([]byte(body)), asset)
		case "/" + tag + "/checksums.txt.sigstore.json":
			_, _ = fmt.Fprint(w, "{}")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &paths
}

func options(t *testing.T, srv *httptest.Server, asset, tag string, auto bool) (agentupdate.Options, *svc) {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "levelrail-agent")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho v1.0.0\n"), 0o700); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	s := &svc{}
	return agentupdate.Options{
		Decide: func(context.Context) (nodeinsight.UpdateDecision, error) {
			return nodeinsight.UpdateDecision{Action: nodeinsight.UpdateInstall, Target: tag, AutoApply: auto, Reason: "behind"}, nil
		},
		Release: release{selfupgrade.Fetcher{BaseURL: srv.URL, AllowHTTP: true}},
		Service: s, Asset: asset, Current: "v1.0.0", ExePath: exe, DataDir: dir,
		Smoke: func(ctx context.Context, p string) (string, error) {
			out, err := exec.CommandContext(ctx, p).Output() //nolint:gosec // staged fixture script in a temp dir
			return strings.TrimSpace(string(out)), err
		},
	}, s
}

func TestEndToEndUpdateSwapsAndRollsBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture binaries are shell scripts")
	}
	asset := cliupdate.Asset("levelrail-agent", "linux", "amd64")
	srv, paths := fakeServer(t, asset, "v1.1.0", "v1.1.0")
	o, s := options(t, srv, asset, "v1.1.0", true)
	res, err := agentupdate.Run(context.Background(), o)
	if err != nil || res.Action != agentupdate.ActionRestarted || s.restarts != 1 {
		t.Fatalf("res=%+v err=%v restarts=%d", res, err, s.restarts)
	}
	got, _ := os.ReadFile(o.ExePath) //nolint:gosec // test temp path
	if !strings.Contains(string(got), "v1.1.0") {
		t.Fatalf("binary was not swapped: %q", got)
	}
	joined := strings.Join(*paths, " ")
	for _, want := range []string{"/v1.1.0/" + asset, "/v1.1.0/checksums.txt", "/v1.1.0/checksums.txt.sigstore.json"} {
		if !strings.Contains(joined, want) {
			t.Errorf("agent never requested %s (got %s)", want, joined)
		}
	}
	if _, err := agentupdate.Rollback(context.Background(), o.ExePath, o.DataDir, s); err != nil {
		t.Fatal(err)
	}
	back, _ := os.ReadFile(o.ExePath) //nolint:gosec // test temp path
	if !strings.Contains(string(back), "v1.0.0") {
		t.Fatalf("rollback did not restore the previous binary: %q", back)
	}
}

func TestEndToEndBadSmokeLeavesAgentUntouched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture binaries are shell scripts")
	}
	asset := cliupdate.Asset("levelrail-agent", "linux", "amd64")
	srv, _ := fakeServer(t, asset, "v1.1.0", "v9.9.9")
	o, s := options(t, srv, asset, "v1.1.0", true)
	before, _ := os.ReadFile(o.ExePath) //nolint:gosec // test temp path
	if _, err := agentupdate.Run(context.Background(), o); err == nil {
		t.Fatal("a staged binary reporting the wrong version must be refused")
	}
	after, _ := os.ReadFile(o.ExePath) //nolint:gosec // test temp path
	if string(before) != string(after) || s.restarts != 0 {
		t.Fatal("a refused update must leave the running binary alone and not restart")
	}
}
