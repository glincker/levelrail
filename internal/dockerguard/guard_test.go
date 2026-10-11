package dockerguard

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func dockerHardeningForTest() docker.HardeningConfig {
	return docker.HardeningConfig{Mode: docker.HardeningEnforce}
}

func TestGuardSocketIsPrivate(t *testing.T) {
	h := startGuard(t, ModeEnforce)
	info, err := os.Stat(h.server.Socket)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("socket mode = %o, want 600", perm)
	}
}

func TestGuardStreamsEvents(t *testing.T) {
	h := startGuard(t, ModeEnforce)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	msgs, errs := h.sdk(t).Events(ctx, events.ListOptions{})
	for i := 0; i < 3; i++ {
		select {
		case m := <-msgs:
			if m.Action != "start" {
				t.Fatalf("event %d action = %q", i, m.Action)
			}
		case err := <-errs:
			t.Fatalf("events stream failed: %v", err)
		case <-ctx.Done():
			t.Fatalf("only %d of 3 events arrived through the guard", i)
		}
	}
}

func TestGuardLogsFollowFlushesBeforeStreamEnds(t *testing.T) {
	h := startGuard(t, ModeEnforce)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rc, err := h.sdk(t).ContainerLogs(ctx, "c1", container.LogsOptions{ShowStdout: true, Follow: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	br := bufio.NewReader(rc)
	first, err := br.ReadString('\n')
	if err != nil || first != "line one\n" {
		t.Fatalf("first line = %q, %v (not flushed through the guard?)", first, err)
	}
	close(h.daemon.logRelease)
	second, err := br.ReadString('\n')
	if err != nil || second != "line two\n" {
		t.Fatalf("second line = %q, %v", second, err)
	}
}

func TestGuardExecHijack(t *testing.T) {
	h := startGuard(t, ModeEnforce)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cli := h.sdk(t)
	created, err := cli.ContainerExecCreate(ctx, "c1", container.ExecOptions{Cmd: []string{"cat"}, AttachStdin: true, AttachStdout: true})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := cli.ContainerExecAttach(ctx, created.ID, container.ExecAttachOptions{})
	if err != nil {
		t.Fatalf("exec attach through guard: %v", err)
	}
	defer resp.Close()
	if _, err := resp.Conn.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	got, err := resp.Reader.ReadString('\n')
	if err != nil || got != "echo: ping\n" {
		t.Fatalf("hijacked echo = %q, %v", got, err)
	}
}

func TestGuardEnforceDeniesPrivilegedCreate(t *testing.T) {
	h := startGuard(t, ModeEnforce)
	_, err := h.sdk(t).ContainerCreate(context.Background(), &container.Config{Image: "alpine", Env: []string{"SECRET=hunter2"}},
		&container.HostConfig{Privileged: true}, nil, nil, "evil")
	if err == nil || !strings.Contains(err.Error(), RulePrivileged) {
		t.Fatalf("want denial naming %s, got %v", RulePrivileged, err)
	}
	for _, r := range h.daemon.requests() {
		if strings.HasSuffix(r.Path, "/containers/create") {
			t.Fatal("denied create reached the daemon")
		}
	}
	d := h.waitDecisions(t, 1)[0]
	if !d.Denied || d.Rule() != RulePrivileged || d.Container != "evil" || d.Action() != ActionDenied {
		t.Fatalf("decision = %+v", d)
	}
	for _, v := range d.Violations {
		if strings.Contains(v.Reason, "hunter2") {
			t.Fatal("decision leaked an env value")
		}
	}
}

func TestGuardAuditForwardsAndDedups(t *testing.T) {
	h := startGuard(t, ModeAudit)
	cli := h.sdk(t)
	for i := 0; i < 2; i++ {
		if _, err := cli.ContainerCreate(context.Background(), &container.Config{Image: "alpine"}, &container.HostConfig{Privileged: true}, nil, nil, ""); err != nil {
			t.Fatalf("audit mode must forward, got %v", err)
		}
	}
	creates := 0
	for _, r := range h.daemon.requests() {
		if strings.HasSuffix(r.Path, "/containers/create") {
			creates++
		}
	}
	if creates != 2 {
		t.Fatalf("daemon saw %d creates, want 2", creates)
	}
	d := h.waitDecisions(t, 1)
	time.Sleep(50 * time.Millisecond)
	if got := len(h.sink.all()); got != 1 || d[0].Denied || d[0].Action() != ActionWouldDeny {
		t.Fatalf("want one deduplicated would_deny row, got %d: %+v", got, d)
	}
	snap := h.server.Guard.Stats().Snapshot()
	if len(snap) != 1 || snap[0].WouldDeny != 2 {
		t.Fatalf("stats = %+v, want 2 would-deny for privileged", snap)
	}
}

func TestGuardEnforceForwardsReencodedBody(t *testing.T) {
	h := startGuard(t, ModeEnforce)
	body := `{"Image":"alpine","NotARealField":"x","HostConfig":{"Privileged":false,"CapDrop":["ALL"]}}`
	resp, err := h.raw().Post("http://docker/v1.47/containers/create?name=ok", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var forwarded []byte
	for _, r := range h.daemon.requests() {
		if r.Path == "/v1.47/containers/create" {
			forwarded = r.Body
		}
	}
	if forwarded == nil || strings.Contains(string(forwarded), "NotARealField") || !strings.Contains(string(forwarded), `"CapDrop":["ALL"]`) {
		t.Fatalf("forwarded body = %s", forwarded)
	}
}

func TestGuardRawRequests(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantRule   string
	}{
		{name: "case variant duplicate key decodes like the daemon", method: http.MethodPost, path: "/v1.47/containers/create", body: `{"HostConfig":{"Privileged":false,"privileged":true}}`, wantStatus: http.StatusForbidden, wantRule: RulePrivileged},
		{name: "malformed create body", method: http.MethodPost, path: "/containers/create", body: `{"HostConfig":`, wantStatus: http.StatusForbidden, wantRule: RuleBodyInvalid},
		{name: "plugin install", method: http.MethodPost, path: "/v1.47/plugins/pull", wantStatus: http.StatusForbidden, wantRule: RuleEndpointNotAllowed},
		{name: "dot dot escape", method: http.MethodGet, path: "/v1.47/containers/../plugins", wantStatus: http.StatusForbidden, wantRule: RulePathNoncanonical},
		{name: "encoded slash", method: http.MethodDelete, path: "/images/a%2F..%2F..%2Fplugins", wantStatus: http.StatusForbidden, wantRule: RulePathNoncanonical},
		{name: "image import", method: http.MethodPost, path: "/images/create?fromSrc=http://x/rootfs.tar", wantStatus: http.StatusForbidden, wantRule: RuleImageImport},
		{name: "exec privileged", method: http.MethodPost, path: "/containers/c1/exec", body: `{"Cmd":["sh"],"Privileged":true}`, wantStatus: http.StatusForbidden, wantRule: RuleExecPrivileged},
		{name: "volume bind trick", method: http.MethodPost, path: "/volumes/create", body: `{"Name":"x","DriverOpts":{"type":"none","o":"bind","device":"/etc"}}`, wantStatus: http.StatusForbidden, wantRule: RuleVolumeBindDriverOpt},
		{name: "double slashes still matched", method: http.MethodGet, path: "//v1.47//containers//json", wantStatus: http.StatusOK},
		{name: "image pull allowed", method: http.MethodPost, path: "/images/create?fromImage=alpine&tag=3", wantStatus: http.StatusOK},
	}
	h := startGuard(t, ModeEnforce)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := rawRequest(t, tt.method, tt.path, tt.body)
			resp, err := h.raw().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if tt.wantRule != "" {
				if msg := decodeMessage(t, resp.Body); !strings.Contains(msg, "(rule "+tt.wantRule+")") {
					t.Fatalf("message %q does not name rule %s", msg, tt.wantRule)
				}
			}
		})
	}
}

func rawRequest(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, "http://docker/", rdr)
	if err != nil {
		t.Fatal(err)
	}
	p, q, _ := strings.Cut(path, "?")
	req.URL.Opaque = p
	req.URL.RawQuery = q
	return req
}

func TestGuardForwardsVersionPrefix(t *testing.T) {
	h := startGuard(t, ModeEnforce)
	resp, err := h.raw().Get("http://docker/v1.43/containers/json")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	reqs := h.daemon.requests()
	if len(reqs) == 0 || reqs[len(reqs)-1].Path != "/v1.43/containers/json" {
		t.Fatalf("daemon saw %+v", reqs)
	}
}

func TestGuardModeSwitchIsLive(t *testing.T) {
	h := startGuard(t, ModeAudit)
	send := func() int {
		resp, err := h.raw().Post("http://docker/containers/create", "application/json", strings.NewReader(`{"HostConfig":{"Privileged":true}}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if got := send(); got != http.StatusCreated {
		t.Fatalf("audit: status %d", got)
	}
	h.server.Guard.SetMode(ModeEnforce)
	if got := send(); got != http.StatusForbidden {
		t.Fatalf("enforce: status %d", got)
	}
}

func TestGuardUpstreamDown(t *testing.T) {
	g := New(Config{Mode: ModeEnforce, Upstream: "/nonexistent/docker.sock", Tunables: Tunables{MaxBodyBytes: 1024, HeaderTimeout: time.Second, RecordQueue: 1}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := Listen(ctx, g, shortTempDir(t)+"/g/"+SocketName)
	if err != nil {
		t.Fatal(err)
	}
	h := &guardHarness{server: srv}
	resp, err := h.raw().Get("http://docker/_ping")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
}

func TestGuardBodyTooLarge(t *testing.T) {
	h := startGuard(t, ModeEnforce)
	h.server.Guard.tunables.MaxBodyBytes = 16
	resp, err := h.raw().Post("http://docker/containers/create", "application/json", strings.NewReader(`{"Image":"alpine","Cmd":["a","b","c"]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(decodeMessage(t, resp.Body), RuleBodyTooLarge) {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestGuardConcurrentMixedTraffic(t *testing.T) {
	h := startGuard(t, ModeEnforce)
	client := h.raw()
	var wg sync.WaitGroup
	errs := make(chan string, 200)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			release := h.grants.DeclareCreate(docker.CreateDeclaration{Name: "gpu", GPU: true})
			defer release()
			body, want := `{"HostConfig":{"CapDrop":["ALL"]}}`, http.StatusCreated
			if i%2 == 0 {
				body, want = `{"HostConfig":{"Privileged":true}}`, http.StatusForbidden
			}
			resp, err := client.Post("http://docker/v1.47/containers/create", "application/json", strings.NewReader(body))
			if err != nil {
				errs <- err.Error()
				return
			}
			_ = resp.Body.Close()
			if resp.StatusCode != want {
				errs <- resp.Status
			}
			_ = h.server.Guard.Stats().Snapshot()
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	h.server.Guard.SetMode(ModeAudit)
	if snap := h.server.Guard.Stats().Snapshot(); len(snap) != 1 || snap[0].Denied != 20 {
		t.Fatalf("stats = %+v, want 20 privileged denials", snap)
	}
}
