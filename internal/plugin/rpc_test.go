package plugin

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

type echoPlugin struct {
	pluginpb.UnimplementedPluginServiceServer
	pluginpb.UnimplementedNotificationChannelServiceServer
}

func (echoPlugin) Describe(context.Context, *pluginpb.DescribeRequest) (*pluginpb.DescribeResponse, error) {
	return &pluginpb.DescribeResponse{ContractVersion: 1, Name: "example", Kind: KindNotificationChannel, RequestedAbilities: []string{AbilityRead}}, nil
}

func (echoPlugin) Health(context.Context, *pluginpb.HealthRequest) (*pluginpb.HealthResponse, error) {
	return &pluginpb.HealthResponse{Ok: true}, nil
}

func (echoPlugin) Send(_ context.Context, r *pluginpb.SendRequest) (*pluginpb.SendResponse, error) {
	return &pluginpb.SendResponse{Delivered: true, Detail: r.GetEvent().GetTitle()}, nil
}

func TestDialUnixSocketRoundTripAndMessageCap(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "pl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "p.sock")
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	srv := grpc.NewServer()
	pluginpb.RegisterPluginServiceServer(srv, echoPlugin{})
	pluginpb.RegisterNotificationChannelServiceServer(srv, echoPlugin{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	client, closer, err := dialUnix(context.Background(), sock, 4<<10, 4<<10)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closer.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	d, err := client.Describe(ctx, &pluginpb.DescribeRequest{HostContractVersions: ContractVersions})
	if err != nil || d.GetName() != "example" {
		t.Fatalf("Describe = %v, %v", d, err)
	}
	r, err := client.Send(ctx, &pluginpb.SendRequest{Event: &pluginpb.Event{Title: "hello"}})
	if err != nil || r.GetDetail() != "hello" {
		t.Fatalf("Send = %v, %v", r, err)
	}
	if _, err := client.Send(ctx, &pluginpb.SendRequest{Event: &pluginpb.Event{Body: strings.Repeat("x", 64<<10)}}); err == nil {
		t.Fatal("a message over the cap was sent")
	}
}
