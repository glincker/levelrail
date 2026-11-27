package dockerguard

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type profileUpstream struct {
	mu   sync.Mutex
	seen []string
	body string
}

func newProfileUpstream(t *testing.T) (*profileUpstream, string) {
	t.Helper()
	u := &profileUpstream{}
	sock := filepath.Join(shortTempDir(t), "u.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		u.mu.Lock()
		u.seen = append(u.seen, r.Method+" "+r.URL.RequestURI())
		u.body = string(b)
		u.mu.Unlock()
		switch r.URL.Path {
		case "/containers/mine/json":
			_, _ = w.Write([]byte(`{"Config":{"Labels":{"levelrail.app":"blog"}}}`))
		case "/containers/theirs/json":
			_, _ = w.Write([]byte(`{"Config":{"Labels":{"levelrail.app":"other"}}}`))
		case "/exec/e1/json":
			_, _ = w.Write([]byte(`{"ContainerID":"theirs"}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return u, sock
}

func newTestProfile(t *testing.T, p Profile, on bool) (*ProfileGuard, *profileUpstream) {
	t.Helper()
	u, sock := newProfileUpstream(t)
	pg, err := NewProfileGuard(sock, ProfileSpec{Profile: p, Project: "blog", LabelKey: "levelrail.app"}, Policy{}, Tunables{MaxBodyBytes: 1 << 20}, func() bool { return on }, nil)
	if err != nil {
		t.Fatal(err)
	}
	return pg, u
}

func doProfile(pg *ProfileGuard, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	pg.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestProfileGuardReadOnly(t *testing.T) {
	pg, up := newTestProfile(t, ProfileReadOnly, true)
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/containers/mine/json", 200},
		{"GET", "/containers/mine/logs", 200},
		{"GET", "/containers/theirs/json", 403},
		{"GET", "/containers/theirs/logs", 403},
		{"GET", "/containers/ghost/logs", 403},
		{"POST", "/containers/mine/start", 403},
		{"DELETE", "/containers/mine", 403},
		{"POST", "/containers/create", 403},
		{"POST", "/containers/mine/exec", 403},
		{"GET", "/images/json", 403},
		{"GET", "/volumes", 403},
	} {
		if got := doProfile(pg, tc.method, tc.path, "{}").Code; got != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, got, tc.want)
		}
	}
	doProfile(pg, "GET", "/containers/json?filters=%7B%22status%22%3A%5B%22running%22%5D%7D", "")
	last := up.seen[len(up.seen)-1]
	if !strings.Contains(last, "levelrail.app%3Dblog") || !strings.Contains(last, "running") {
		t.Fatalf("list not scoped to the project: %s", last)
	}
}

func TestProfileGuardProject(t *testing.T) {
	pg, up := newTestProfile(t, ProfileProject, true)
	if got := doProfile(pg, "POST", "/containers/mine/start", "").Code; got != 200 {
		t.Errorf("start own = %d", got)
	}
	if got := doProfile(pg, "POST", "/containers/theirs/stop", "").Code; got != 403 {
		t.Errorf("stop foreign = %d", got)
	}
	if got := doProfile(pg, "POST", "/exec/e1/start", "{}").Code; got != 403 {
		t.Errorf("exec of foreign container = %d", got)
	}
	if got := doProfile(pg, "POST", "/containers/mine/exec", `{"Privileged":true}`).Code; got != 403 {
		t.Errorf("privileged exec = %d", got)
	}
	if got := doProfile(pg, "POST", "/containers/create", `{"Image":"x","HostConfig":{"Privileged":true}}`).Code; got != 403 {
		t.Errorf("privileged create = %d", got)
	}
	if got := doProfile(pg, "POST", "/containers/create", `{"Image":"x","HostConfig":{"Binds":["/var/run/docker.sock:/s"]}}`).Code; got != 403 {
		t.Errorf("bind create = %d", got)
	}
	if got := doProfile(pg, "POST", "/containers/create?name=a", `{"Image":"x","Labels":{"levelrail.app":"other"}}`).Code; got != 200 {
		t.Errorf("plain create = %d", got)
	}
	if !strings.Contains(up.body, `"levelrail.app":"blog"`) {
		t.Fatalf("create label not forced to the project: %s", up.body)
	}
	if got := doProfile(pg, "DELETE", "/containers/mine", "").Code; got != 403 {
		t.Errorf("delete = %d", got)
	}
}

func TestProfileGuardOffDeniesEverything(t *testing.T) {
	pg, _ := newTestProfile(t, ProfileReadOnly, false)
	if got := doProfile(pg, "GET", "/_ping", "").Code; got != 403 {
		t.Fatalf("ping while off = %d", got)
	}
}

func TestProfileSpecNeedsProject(t *testing.T) {
	if _, err := NewProfileGuard("/x", ProfileSpec{Profile: ProfileReadOnly}, Policy{}, Tunables{}, nil, nil); err == nil {
		t.Fatal("unscoped spec accepted")
	}
}

func TestPolicyGrantSocketDirExemption(t *testing.T) {
	p := Policy{SensitivePaths: []string{"/data/lr"}, GrantSocketDir: "/data/lr/dockerapi"}
	if p.sensitiveHostPath("/data/lr/dockerapi/ab.sock") {
		t.Fatal("grant socket treated as sensitive")
	}
	if !p.sensitiveHostPath("/data/lr/secret") {
		t.Fatal("data dir no longer protected")
	}
}
