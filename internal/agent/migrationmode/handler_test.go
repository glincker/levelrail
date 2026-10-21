package migrationmode

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/migrationpb"
)

type fakeEngine map[string]string

func (f fakeEngine) RoundTrip(req *http.Request) (*http.Response, error) {
	body, ok := f[req.URL.Path]
	if !ok {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func newHandler(t *testing.T, volRoot string) *Handler {
	t.Helper()
	eng := fakeEngine{
		"/version":  `{"Version":"27.0.1"}`,
		"/networks": `[{"Id":"n1"},{"Id":"n2"}]`,
		"/containers/json": `[{"Id":"aaaaaaaaaaaaaaaaaaaa","Names":["/web"],"Image":"ghcr.io/x/web:1","State":"running","Status":"Up",` +
			`"Mounts":[{"Type":"volume","Name":"data","Source":"` + volRoot + `","Destination":"/data","RW":true}]},` +
			`{"Id":"bbbbbbbbbbbbbbbbbbbb","Names":["/db"],"Image":"docker.io/library/postgres:16.2","State":"running","Status":"Up","Mounts":[]}]`,
		"/containers/aaaaaaaaaaaaaaaaaaaa/json": `{"Config":{"Env":["SECRET_KEY=hunter2","PORT=80"]}}`,
		"/containers/bbbbbbbbbbbbbbbbbbbb/json": `{"Config":{"Env":["POSTGRES_PASSWORD=topsecret"]}}`,
		"/volumes":                              `{"Volumes":[{"Name":"data","Driver":"local","Mountpoint":"` + volRoot + `"}]}`,
	}
	return &Handler{Docker: NewDockerWithTransport(eng, 1<<20, nil), Scanner: NewScanner(), ScanTimeout: time.Second,
		Now: time.Now, Log: nil}
}

func TestDiscoverReportsEnvNamesOnlyAndDatabases(t *testing.T) {
	h := newHandler(t, t.TempDir())
	reply, err := h.Handle(context.Background(), &migrationpb.ControlMessage{Payload: &migrationpb.ControlMessage_Discover{
		Discover: &migrationpb.DiscoverRequest{RequestId: "r1"}}})
	if err != nil {
		t.Fatal(err)
	}
	inv := reply.GetInventory()
	if inv == nil || len(inv.GetContainers()) != 2 || inv.GetNetworkCount() != 2 || inv.GetDockerVersion() != "27.0.1" {
		t.Fatalf("unexpected inventory: %v", reply)
	}
	for _, c := range inv.GetContainers() {
		for _, k := range c.GetEnvKeys() {
			if strings.Contains(k, "hunter2") || strings.Contains(k, "topsecret") || strings.Contains(k, "=") {
				t.Fatalf("env value leaked: %q", k)
			}
		}
	}
	db := inv.GetContainers()[1]
	if db.GetDatabaseEngine() != "postgres" || db.GetDatabaseVersion() != "16.2" {
		t.Fatalf("db not detected: %v", db)
	}
}

func TestEngineOf(t *testing.T) {
	cases := []struct{ image, engine, version string }{
		{"postgres:16", "postgres", "16"},
		{"docker.io/library/mariadb:11", "mariadb", "11"},
		{"registry.local:5000/team/redis", "redis", ""},
		{"mongo@sha256:abc", "mongodb", ""},
		{"ghcr.io/x/web:1", "", ""},
		{"nginx", "", ""},
	}
	for _, tc := range cases {
		e, v := EngineOf(tc.image)
		if e != tc.engine || v != tc.version {
			t.Errorf("EngineOf(%q) = %q,%q want %q,%q", tc.image, e, v, tc.engine, tc.version)
		}
	}
}

func TestScanRefusesTargetsNotInInventory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHandler(t, root)
	if _, err := h.Handle(context.Background(), &migrationpb.ControlMessage{Payload: &migrationpb.ControlMessage_Discover{
		Discover: &migrationpb.DiscoverRequest{RequestId: "r1"}}}); err != nil {
		t.Fatal(err)
	}
	reply, err := h.Handle(context.Background(), &migrationpb.ControlMessage{Payload: &migrationpb.ControlMessage_Scan{
		Scan: &migrationpb.ScanRequest{RequestId: "r2", Targets: []string{"data", "/etc", "../../etc", "other"}}}})
	if err != nil {
		t.Fatal(err)
	}
	stats := reply.GetScanResult().GetStats()
	if len(stats) != 4 {
		t.Fatalf("want 4 stats, got %d", len(stats))
	}
	if stats[0].GetFiles() != 1 || stats[0].GetBytes() != 3 || stats[0].GetError() != "" || stats[0].GetChangedDuringScan() {
		t.Fatalf("known volume not measured: %v", stats[0])
	}
	for _, s := range stats[1:] {
		if s.GetError() == "" || s.GetFiles() != 0 {
			t.Fatalf("target %q outside the inventory was scanned: %v", s.GetTarget(), s)
		}
	}
}

func TestHandleRejectsMessagesOutsideTheMigrationSet(t *testing.T) {
	h := newHandler(t, t.TempDir())
	cases := []struct {
		name string
		msg  *migrationpb.ControlMessage
		want error
	}{
		{"empty payload", &migrationpb.ControlMessage{}, ErrUnknownControl},
		{"nil message", nil, ErrUnknownControl},
		{"go away", &migrationpb.ControlMessage{Payload: &migrationpb.ControlMessage_GoAway{GoAway: &migrationpb.GoAway{Reason: "revoked"}}}, ErrGoAway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := h.Handle(context.Background(), tc.msg); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestDiscoverDeniedWhenEngineFails(t *testing.T) {
	h := &Handler{Docker: NewDockerWithTransport(fakeEngine{}, 1<<20, nil), Scanner: NewScanner(), ScanTimeout: time.Second, Now: time.Now}
	reply, err := h.Handle(context.Background(), &migrationpb.ControlMessage{Payload: &migrationpb.ControlMessage_Discover{
		Discover: &migrationpb.DiscoverRequest{RequestId: "r9"}}})
	if err != nil {
		t.Fatal(err)
	}
	if reply.GetDenied() == nil || reply.GetDenied().GetRequestId() != "r9" {
		t.Fatalf("want a denied reply, got %v", reply)
	}
}

func TestEnrollRequiresPin(t *testing.T) {
	if _, err := Enroll(context.Background(), "127.0.0.1:1", "tok", " ", "v"); !errors.Is(err, ErrNoPin) {
		t.Fatalf("got %v, want ErrNoPin", err)
	}
}

func TestIdentityFileIsOwnerOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "identity.json")
	if err := SaveIdentity(p, &Identity{SessionID: "s1", ClientKeyPEM: []byte("k")}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("identity mode %v, want 0600", st.Mode().Perm())
	}
	got, err := LoadIdentity(p)
	if err != nil || got.SessionID != "s1" {
		t.Fatalf("round trip failed: %v %v", got, err)
	}
}
