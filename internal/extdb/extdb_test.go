package extdb

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		conn    Conn
		policy  Policy
		wantErr string
		check   func(t *testing.T, c Conn)
	}{
		{name: "defaults fill port and tls", conn: Conn{Engine: "postgres", Host: "db.internal"}, check: func(t *testing.T, c Conn) {
			if c.Port != 5432 || c.TLSMode != TLSPrefer {
				t.Fatalf("got port %d tls %q", c.Port, c.TLSMode)
			}
		}},
		{name: "redis defaults to no tls", conn: Conn{Engine: "redis", Host: "10.0.0.5"}, check: func(t *testing.T, c Conn) {
			if c.Port != 6379 || c.TLSMode != TLSDisable {
				t.Fatalf("got port %d tls %q", c.Port, c.TLSMode)
			}
		}},
		{name: "mongo gets admin auth db", conn: Conn{Engine: "mongodb", Host: "m"}, check: func(t *testing.T, c Conn) {
			if c.AuthDatabase != "admin" {
				t.Fatalf("auth db %q", c.AuthDatabase)
			}
		}},
		{name: "private ip allowed", conn: Conn{Engine: "mysql", Host: "192.168.1.20"}},
		{name: "unsupported engine", conn: Conn{Engine: "oracle", Host: "h"}, wantErr: "not supported"},
		{name: "empty host", conn: Conn{Engine: "postgres"}, wantErr: "host is required"},
		{name: "loopback ip", conn: Conn{Engine: "postgres", Host: "127.0.0.1"}, wantErr: "loopback"},
		{name: "localhost", conn: Conn{Engine: "postgres", Host: "localhost"}, wantErr: "localhost"},
		{name: "metadata ip", conn: Conn{Engine: "postgres", Host: "169.254.169.254"}, wantErr: "metadata"},
		{name: "metadata ip opt in", conn: Conn{Engine: "postgres", Host: "169.254.169.254"}, policy: Policy{AllowLinkLocal: true}},
		{name: "link local v6", conn: Conn{Engine: "postgres", Host: "fe80::1"}, wantErr: "link-local"},
		{name: "metadata name", conn: Conn{Engine: "postgres", Host: "metadata.google.internal"}, wantErr: "metadata"},
		{name: "unspecified", conn: Conn{Engine: "postgres", Host: "0.0.0.0"}, wantErr: "not a connectable"},
		{name: "bad host chars", conn: Conn{Engine: "postgres", Host: "a b;rm"}, wantErr: "hostname"},
		{name: "bad port", conn: Conn{Engine: "postgres", Host: "h", Port: 70000}, wantErr: "port"},
		{name: "bad tls", conn: Conn{Engine: "postgres", Host: "h", TLSMode: "verify"}, wantErr: "tls_mode"},
		{name: "bad user", conn: Conn{Engine: "postgres", Host: "h", User: "a;b"}, wantErr: "user"},
		{name: "bad network", conn: Conn{Engine: "postgres", Host: "h", Network: "a b"}, wantErr: "network"},
		{name: "newline in password", conn: Conn{Engine: "postgres", Host: "h", Password: "a\nb"}, wantErr: "password"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.conn
			err := c.Validate(tc.policy)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.check != nil {
				tc.check(t, c)
			}
		})
	}
}

func TestBlockedAddressIsTyped(t *testing.T) {
	c := Conn{Engine: "postgres", Host: "169.254.169.254"}
	if err := c.Validate(Policy{}); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("err = %v", err)
	}
}

type fakeResolver struct {
	addrs []string
	err   error
}

func (f fakeResolver) LookupHost(context.Context, string) ([]string, error) { return f.addrs, f.err }

func TestCheckResolved(t *testing.T) {
	c := Conn{Engine: "postgres", Host: "db.example.com"}
	if err := c.CheckResolved(t.Context(), Policy{}, fakeResolver{addrs: []string{"169.254.169.254"}}); err == nil {
		t.Fatal("want rebinding to metadata rejected")
	}
	if err := c.CheckResolved(t.Context(), Policy{AllowLinkLocal: true}, fakeResolver{addrs: []string{"169.254.169.254"}}); err != nil {
		t.Fatalf("opt in: %v", err)
	}
	if err := c.CheckResolved(t.Context(), Policy{}, fakeResolver{err: errors.New("no such host")}); err != nil {
		t.Fatalf("docker-only name must pass: %v", err)
	}
	if err := c.CheckResolved(t.Context(), Policy{}, fakeResolver{addrs: []string{"10.1.2.3"}}); err != nil {
		t.Fatalf("private ok: %v", err)
	}
}

func TestScrub(t *testing.T) {
	c := Conn{Password: "p@ss/w ord"}
	for _, in := range []string{"bad p@ss/w ord here", "url p%40ss%2Fw+ord", "path p@ss%2Fw%20ord"} {
		if out := c.Scrub(in); strings.Contains(out, "ss") && !strings.Contains(out, "[redacted]") {
			t.Fatalf("not scrubbed: %q", out)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		engine string
		out    string
		err    error
		want   string
	}{
		{"pg ok", "postgres", "app|appdb|PostgreSQL 16.4 on x86\n\nLR_EXIT:0\n", nil, StatusReachable},
		{"pg wrong password", "postgres", `psql: error: connection to server at "db" (10.0.0.2), port 5432 failed: FATAL:  password authentication failed for user "app"` + "\nLR_EXIT:2\n", nil, StatusAuthFailed},
		{"pg refused", "postgres", `psql: error: connection to server at "db" (10.0.0.2), port 5432 failed: Connection refused` + "\nLR_EXIT:2\n", nil, StatusUnreachable},
		{"pg dns", "postgres", `psql: error: could not translate host name "nope" to address: Name or service not known` + "\nLR_EXIT:2\n", nil, StatusUnreachable},
		{"pg ssl required", "postgres", `psql: error: connection to server failed: server does not support SSL, but SSL was required` + "\nLR_EXIT:2\n", nil, StatusTLSError},
		{"pg hba ssl", "postgres", `FATAL:  no pg_hba.conf entry for host "1.2.3.4", user "u", database "d", SSL off` + "\nLR_EXIT:2\n", nil, StatusTLSError},
		{"pg hba no encryption", "postgres", `FATAL:  no pg_hba.conf entry for host "1.2.3.4", user "u", database "d", no encryption` + "\nLR_EXIT:2\n", nil, StatusAuthFailed},
		{"mysql ok", "mysql", "root@%\tapp\t8.0.36\tapp,other\nLR_EXIT:0\n", nil, StatusReachable},
		{"mysql denied", "mysql", "ERROR 1045 (28000): Access denied for user 'root'@'10.0.0.1' (using password: YES)\nLR_EXIT:1\n", nil, StatusAuthFailed},
		{"mysql refused", "mysql", "ERROR 2003 (HY000): Can't connect to MySQL server on 'db:3306' (111)\nLR_EXIT:1\n", nil, StatusUnreachable},
		{"mysql ssl", "mysql", "ERROR 2026 (HY000): SSL connection error: wrong version number\nLR_EXIT:1\n", nil, StatusTLSError},
		{"redis ok", "redis", "PONG\nLR_EXIT:0\n", nil, StatusReachable},
		{"redis noauth", "redis", "NOAUTH Authentication required.\nLR_EXIT:0\n", nil, StatusAuthFailed},
		{"redis refused", "redis", "Could not connect to Redis at db:6379: Connection refused\nLR_EXIT:1\n", nil, StatusUnreachable},
		{"mongo ok", "mongodb", `{"ok":1,"version":"7.0.2"}` + "\nLR_EXIT:0\n", nil, StatusReachable},
		{"mongo auth", "mongodb", "MongoServerError: Authentication failed.\nLR_EXIT:1\n", nil, StatusAuthFailed},
		{"error carried by stream", "postgres", "", &docker.ExecExitError{ExitCode: 2, Stderr: `FATAL: password authentication failed for user "x"`}, StatusAuthFailed},
		{"stream error without text", "postgres", "", errors.New("daemon went away"), StatusUnknown},
		{"garbage", "postgres", "something odd\nLR_EXIT:0\n", nil, StatusUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.engine, tc.out, tc.err)
			if got.Status != tc.want {
				t.Fatalf("status = %q (%s), want %q", got.Status, got.Reason, tc.want)
			}
		})
	}
}

func TestClassifyDiscoversDatabases(t *testing.T) {
	r := Classify("postgres", "postgres|appdb|PostgreSQL 16 x|appdb,other\nLR_EXIT:0\n", nil)
	if r.User != "postgres" || r.Database != "appdb" || len(r.Databases) != 2 {
		t.Fatalf("result = %+v", r)
	}
}

func TestEngineForImage(t *testing.T) {
	cases := map[string]string{
		"postgres:16":                            EnginePostgres,
		"pgvector/pgvector:pg16":                 EnginePostgres,
		"postgis/postgis:15-3.4":                 EnginePostgres,
		"docker.io/library/mysql:8":              EngineMySQL,
		"registry.local:5000/bitnami/postgresql": EnginePostgres,
		"mariadb:11":                             EngineMariaDB,
		"mongo:7":                                EngineMongoDB,
		"redis:7-alpine":                         EngineRedis,
		"postgres@sha256:abcdef":                 EnginePostgres,
		"nginx:latest":                           "",
		"prom/postgres-exporter":                 "",
	}
	for image, want := range cases {
		if got := EngineForImage(image); got != want {
			t.Errorf("EngineForImage(%q) = %q, want %q", image, got, want)
		}
	}
}

type fakeRuntime struct {
	docker.Runtime
	mu      sync.Mutex
	states  []docker.ContainerState
	created []docker.ContainerSpec
	removed []string
	execOut string
	execErr error
	cmds    [][]string
}

func (f *fakeRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return f.states, nil
}

func (f *fakeRuntime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	for i := range f.states {
		if f.states[i].Name == name {
			return &f.states[i], nil
		}
	}
	return nil, nil
}

func (f *fakeRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, spec)
	return "helper-1", nil
}

func (f *fakeRuntime) Start(context.Context, string) error { return nil }

func (f *fakeRuntime) Remove(_ context.Context, id string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, id)
	return nil
}

func (f *fakeRuntime) Exec(_ context.Context, _ string, cmd []string) (io.ReadCloser, error) {
	f.mu.Lock()
	f.cmds = append(f.cmds, cmd)
	f.mu.Unlock()
	return io.NopCloser(strings.NewReader(f.execOut)), f.execErr
}

func TestListCandidates(t *testing.T) {
	rt := &fakeRuntime{states: []docker.ContainerState{
		{ID: "1", Name: "coolify-pg", Image: "postgres:16", Running: true, Networks: []docker.ContainerNetwork{{Name: "bridge", IPAddress: "172.17.0.3"}, {Name: "coolify", IPAddress: "10.0.1.2"}}},
		{ID: "2", Name: "stopped-redis", Image: "redis:7", Running: false},
		{ID: "3", Name: "managed", Image: "postgres:16", Running: true, Labels: map[string]string{appspec.InstanceLabelKey: "x"}},
		{ID: "4", Name: "web", Image: "nginx", Running: true},
		{ID: "5", Name: "legacy-mysql", Image: "mysql:8", Running: true, Networks: []docker.ContainerNetwork{{Name: "bridge", IPAddress: "172.17.0.9"}}},
	}}
	got, err := ListCandidates(t.Context(), rt)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("candidates = %+v", got)
	}
	if got[0].Container != "coolify-pg" || got[0].SuggestedHost != "coolify-pg" || got[0].Network != "coolify" || got[0].SuggestedUser != "postgres" {
		t.Fatalf("first = %+v", got[0])
	}
	if got[1].SuggestedHost != "172.17.0.9" || got[1].Note == "" {
		t.Fatalf("second = %+v", got[1])
	}
}

func TestProbeCleansUpAndNeverLeaksPassword(t *testing.T) {
	rt := &fakeRuntime{execOut: `FATAL: password authentication failed for user "app" (pw s3cr3t!)` + "\nLR_EXIT:2\n"}
	p := &Prober{Runtime: rt}
	c := Conn{Engine: "postgres", Host: "db", Port: 5432, User: "app", Password: "s3cr3t!", TLSMode: TLSPrefer, Network: "coolify"}
	res := p.Probe(t.Context(), "main", c)
	if res.Status != StatusAuthFailed {
		t.Fatalf("status = %q", res.Status)
	}
	if strings.Contains(res.Reason, "s3cr3t!") {
		t.Fatalf("reason leaks password: %q", res.Reason)
	}
	if len(rt.removed) != 1 {
		t.Fatalf("helper not removed: %v", rt.removed)
	}
	if rt.created[0].Network == nil || rt.created[0].Network.Name != "coolify" {
		t.Fatalf("helper network = %+v", rt.created[0].Network)
	}
	for _, cmd := range rt.cmds {
		if strings.Contains(strings.Join(cmd, " "), "s3cr3t!") {
			t.Fatalf("password in argv: %v", cmd)
		}
	}
}

func TestProbeScriptsAreReadOnly(t *testing.T) {
	for _, e := range []string{EnginePostgres, EngineMySQL, EngineMariaDB, EngineMongoDB, EngineRedis} {
		s, err := ProbeScript(e)
		if err != nil {
			t.Fatal(err)
		}
		up := strings.ToUpper(s)
		for _, bad := range []string{"INSERT ", "UPDATE ", "DELETE ", "DROP ", "CREATE ", "ALTER ", "TRUNCATE ", "GRANT ", "FLUSH", "CONFIG SET"} {
			if strings.Contains(up, bad) {
				t.Errorf("%s probe script contains %q", e, bad)
			}
		}
	}
}

func TestProbeSlow(t *testing.T) {
	rt := &slowRuntime{fakeRuntime: fakeRuntime{execOut: "u|d|PostgreSQL 16|d\nLR_EXIT:0\n"}, delay: 30 * time.Millisecond}
	p := &Prober{Runtime: rt, SlowAfter: 5 * time.Millisecond}
	res := p.Probe(t.Context(), "main", Conn{Engine: "postgres", Host: "db", Port: 5432})
	if res.Status != StatusSlow {
		t.Fatalf("status = %q", res.Status)
	}
}

type slowRuntime struct {
	fakeRuntime
	delay time.Duration
}

func (s *slowRuntime) Exec(ctx context.Context, id string, cmd []string) (io.ReadCloser, error) {
	time.Sleep(s.delay)
	return s.fakeRuntime.Exec(ctx, id, cmd)
}

func TestExecerRemovesHelperOnClose(t *testing.T) {
	rt := &fakeRuntime{}
	e := &Execer{Helper: &Helper{Runtime: rt}, Name: "main", Conn: Conn{Engine: "postgres", Host: "db", Port: 5432}}
	rt2 := &execInputRuntime{fakeRuntime: rt}
	e.Helper.Runtime = rt2
	rc, err := e.ExecWithInput(t.Context(), "ignored", []string{"true"}, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()
	_ = rc.Close()
	if len(rt.removed) != 1 {
		t.Fatalf("removed = %v", rt.removed)
	}
}

type execInputRuntime struct{ *fakeRuntime }

func (e *execInputRuntime) ExecWithInput(context.Context, string, []string, io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("x")), nil
}
