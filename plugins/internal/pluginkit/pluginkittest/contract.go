// Package pluginkittest holds the checks every first-party plugin test shares.
package pluginkittest

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/GLINCKER/levelrail/internal/plugin"
	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit"
)

// NoSleep makes retries instant in tests.
func NoSleep(context.Context, time.Duration) error { return nil }

// Event is a representative firing alert.
func Event() *pluginpb.Event {
	return &pluginpb.Event{Id: "evt-1", Title: "Disk almost full", Body: "node-1 at 93%", Status: "firing", Severity: "critical", Link: "https://example.com/alerts/1"}
}

// CheckContract validates the manifest on disk and drives Describe, Health and
// Send through a real gRPC server on a unix socket.
func CheckContract(t *testing.T, p pluginkit.Plugin, manifestPath string) {
	t.Helper()
	raw, err := os.ReadFile(manifestPath) //nolint:gosec // test reads its own manifest
	if err != nil {
		t.Fatal(err)
	}
	man, err := plugin.ParseManifest(raw)
	if err != nil {
		t.Fatalf("plugin.yaml: %v", err)
	}
	if man.Name != p.Name || man.Version != p.Version {
		t.Fatalf("manifest %s@%s does not match plugin %s@%s", man.Name, man.Version, p.Name, p.Version)
	}
	dir, err := os.MkdirTemp("/tmp", "pk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "p.sock")
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	s := pluginkit.NewServer(p)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	conn, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d, err := pluginpb.NewPluginServiceClient(conn).Describe(ctx, &pluginpb.DescribeRequest{HostContractVersions: plugin.ContractVersions})
	if err != nil || d.GetName() != p.Name || d.GetKind() != plugin.KindNotificationChannel {
		t.Fatalf("Describe = %v, %v", d, err)
	}
	if len(d.GetRequestedAbilities()) != len(man.Abilities) || d.GetRequestedAbilities()[0] != man.Abilities[0] {
		t.Fatalf("abilities differ: plugin %v manifest %v", d.GetRequestedAbilities(), man.Abilities)
	}
	if _, err := pluginpb.NewPluginServiceClient(conn).Describe(ctx, &pluginpb.DescribeRequest{HostContractVersions: []uint32{99}}); err == nil {
		t.Fatal("a host with no common contract version was accepted")
	}
	if h, err := pluginpb.NewPluginServiceClient(conn).Health(ctx, &pluginpb.HealthRequest{}); err != nil || !h.GetOk() {
		t.Fatalf("Health = %v, %v", h, err)
	}
	r, err := pluginpb.NewNotificationChannelServiceClient(conn).Send(ctx, &pluginpb.SendRequest{Event: Event(), ConfigJson: "{}", Secret: "s"})
	if err != nil || r.GetDelivered() {
		t.Fatalf("Send with no usable config must fail cleanly: %v, %v", r, err)
	}
}

// Status returns a handler that always answers with code and body.
func Status(code int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}
}

// ErrorCase is one shared HTTP failure every plugin must map the same way.
type ErrorCase struct {
	Name      string
	Handler   http.HandlerFunc
	Retryable bool
	Contains  string
}

// ErrorCases lists 401, 429 with a long Retry-After and 5xx.
func ErrorCases() []ErrorCase {
	return []ErrorCase{
		{"401", Status(401, `{"error":"bad"}`), false, "authentication failed"},
		{"429", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(429)
		}, true, "rate limited"},
		{"503", Status(503, "down"), true, "service error"},
	}
}

// RunErrorCases runs every ErrorCase against send, which must target the
// server URL it is given.
func RunErrorCases(t *testing.T, send func(serverURL string) pluginkit.Result) {
	t.Helper()
	for _, tc := range ErrorCases() {
		t.Run(tc.Name, func(t *testing.T) {
			srv := httptest.NewServer(tc.Handler)
			defer srv.Close()
			r := send(srv.URL)
			if r.Delivered || r.Retryable != tc.Retryable || !strings.Contains(r.Detail, tc.Contains) {
				t.Fatalf("got %+v", r)
			}
		})
	}
}
