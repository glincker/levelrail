package migrateagent

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/agent/migrationmode"
	"github.com/GLINCKER/levelrail/internal/agent/migrationpb"
	"github.com/GLINCKER/levelrail/internal/agent/pinned"
)

type fakeEngine map[string]string

func (f fakeEngine) RoundTrip(req *http.Request) (*http.Response, error) {
	body, ok := f[req.URL.Path]
	code := 200
	if !ok {
		code, body = 404, "{}"
	}
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestEndToEndEnrollDiscoverScanRevoke(t *testing.T) {
	f := newFixture(t)
	vol := t.TempDir()
	if err := os.WriteFile(filepath.Join(vol, "a"), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	eng := fakeEngine{
		"/version": `{"Version":"27"}`, "/networks": `[]`,
		"/containers/json":                      `[{"Id":"cccccccccccccccccccc","Names":["/pg"],"Image":"postgres:16","State":"running","Status":"Up","Mounts":[]}]`,
		"/containers/cccccccccccccccccccc/json": `{"Config":{"Env":["POSTGRES_PASSWORD=s3cret"]}}`,
		"/volumes":                              `{"Volumes":[{"Name":"data","Driver":"local","Mountpoint":"` + vol + `"}]}`,
	}
	h := &migrationmode.Handler{Docker: migrationmode.NewDockerWithTransport(eng, 1<<20, nil), Scanner: migrationmode.NewScanner(),
		ScanTimeout: 5 * time.Second, Now: time.Now}

	creds, err := agent.NewServerCredentials(f.ca, []string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer(grpc.Creds(creds))
	migrationpb.RegisterMigrationServiceServer(srv, f.svc)
	agentpb.RegisterAgentServiceServer(srv, agent.NewServer(f.ca, f.db, agent.NewRegistry(), nil))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	created := mustCreate(t, f)
	if _, err := migrationmode.Enroll(ctx, lis.Addr().String(), created.Token, "", "v1"); !errors.Is(err, migrationmode.ErrNoPin) {
		t.Fatalf("enroll without a pin got %v", err)
	}
	if _, err := migrationmode.Enroll(ctx, lis.Addr().String(), created.Token, strings.Repeat("0", 64), "v1"); err == nil {
		t.Fatal("enroll against a wrong pin must fail")
	}
	id, err := migrationmode.Enroll(ctx, lis.Addr().String(), created.Token, f.ca.Fingerprint(), "v1")
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}

	assertNotAWorker(ctx, t, lis.Addr().String(), id)

	connErr := make(chan error, 1)
	go func() { connErr <- migrationmode.Connect(ctx, lis.Addr().String(), id, h, "v1", 200*time.Millisecond) }()

	var inv InventoryDoc
	deadline := time.Now().Add(10 * time.Second)
	for {
		inv, err = f.svc.Discover(ctx, created.ID)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(inv.Containers) != 1 || inv.Containers[0].DatabaseEngine != "postgres" {
		t.Fatalf("unexpected inventory: %+v", inv)
	}
	for _, k := range inv.Containers[0].EnvKeys {
		if strings.Contains(k, "s3cret") {
			t.Fatal("env value reached the control plane")
		}
	}
	scan, err := f.svc.Scan(ctx, created.ID, []string{"data", "/etc"})
	if err != nil {
		t.Fatal(err)
	}
	if scan.Stats[0].Files != 1 || scan.Stats[0].Bytes != 5 || scan.Stats[1].Error == "" {
		t.Fatalf("unexpected scan: %+v", scan.Stats)
	}
	if _, err := migrationmode.Enroll(ctx, lis.Addr().String(), created.Token, f.ca.Fingerprint(), "v1"); err == nil {
		t.Fatal("token must not enroll a second time")
	}

	if err := f.svc.Revoke(ctx, created.ID, "alice"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-connErr:
		if !errors.Is(err, migrationmode.ErrGoAway) {
			t.Fatalf("agent got %v, want a go-away", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("revoke did not close the live stream")
	}
	if err := migrationmode.Connect(ctx, lis.Addr().String(), id, h, "v1", 200*time.Millisecond); err == nil {
		t.Fatal("a revoked session must not reconnect")
	}
	actions := auditActions(t, f.db)
	for _, want := range []string{ActionSessionStarted, ActionDiscovery, ActionScan, ActionRevoked, ActionSessionRejected, ActionSessionEnded} {
		if !hasAction(actions, want) {
			t.Errorf("audit log is missing %s: %v", want, actions)
		}
	}
}

func assertNotAWorker(ctx context.Context, t *testing.T, addr string, id *migrationmode.Identity) {
	t.Helper()
	cfg, err := pinned.SessionTLSConfig(id.CACertPEM, id.ClientCertPEM, id.ClientKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	stream, err := agentpb.NewAgentServiceClient(conn).Session(ctx)
	if err == nil {
		_, err = stream.Recv()
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("a migration certificate opened a node session: %v", err)
	}
}
