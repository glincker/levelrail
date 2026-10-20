package pluginkittest

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/GLINCKER/levelrail/internal/plugin"
	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit"
)

// ServeDNS runs p on a unix socket and returns a client for its DNS service
// and the connection for the plugin service.
func ServeDNS(t *testing.T, p pluginkit.Plugin) (pluginpb.DnsProviderServiceClient, pluginpb.PluginServiceClient) {
	t.Helper()
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
	return pluginpb.NewDnsProviderServiceClient(conn), pluginpb.NewPluginServiceClient(conn)
}

// CheckDNSContract validates the manifest on disk and drives Describe, Health
// and a credential-less ListZones through a real gRPC server.
func CheckDNSContract(t *testing.T, p pluginkit.Plugin, manifestPath string) {
	t.Helper()
	raw, err := os.ReadFile(manifestPath) //nolint:gosec // test reads its own manifest
	if err != nil {
		t.Fatal(err)
	}
	man, err := plugin.ParseManifest(raw)
	if err != nil {
		t.Fatalf("plugin.yaml: %v", err)
	}
	if man.Name != p.Name || man.Version != p.Version || man.Kind != plugin.KindDNSProvider {
		t.Fatalf("manifest %s@%s (%s) does not match plugin %s@%s", man.Name, man.Version, man.Kind, p.Name, p.Version)
	}
	dns, svc := ServeDNS(t, p)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d, err := svc.Describe(ctx, &pluginpb.DescribeRequest{HostContractVersions: plugin.ContractVersions})
	if err != nil || d.GetName() != p.Name || d.GetKind() != plugin.KindDNSProvider {
		t.Fatalf("Describe = %v, %v", d, err)
	}
	if len(d.GetRequestedAbilities()) != len(man.Abilities) || d.GetRequestedAbilities()[0] != man.Abilities[0] {
		t.Fatalf("abilities differ: plugin %v manifest %v", d.GetRequestedAbilities(), man.Abilities)
	}
	if _, err := svc.Describe(ctx, &pluginpb.DescribeRequest{HostContractVersions: []uint32{99}}); err == nil {
		t.Fatal("a host with no common contract version was accepted")
	}
	if h, err := svc.Health(ctx, &pluginpb.HealthRequest{}); err != nil || !h.GetOk() {
		t.Fatalf("Health = %v, %v", h, err)
	}
	z, err := dns.ListZones(ctx, &pluginpb.ListZonesRequest{ConfigJson: "{}"})
	if err != nil || z.GetOutcome().GetOk() {
		t.Fatalf("ListZones with no credential must fail cleanly: %v, %v", z, err)
	}
}
