// Command plugin-notify is the smallest notification_channel plugin: it logs each
// event title and reports delivery. It never logs the per-call secret.
package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

const (
	pluginName      = "example-notify"
	pluginVersion   = "0.1.0"
	contractVersion = 1
)

type server struct {
	pluginpb.UnimplementedPluginServiceServer
	pluginpb.UnimplementedNotificationChannelServiceServer
}

func (server) Describe(_ context.Context, req *pluginpb.DescribeRequest) (*pluginpb.DescribeResponse, error) {
	for _, v := range req.GetHostContractVersions() {
		if v == contractVersion {
			return &pluginpb.DescribeResponse{
				ContractVersion: contractVersion, Name: pluginName, Version: pluginVersion, Kind: "notification_channel",
				RequestedAbilities: []string{"read"}, ConfigSchema: `{"type":"object","additionalProperties":false}`,
			}, nil
		}
	}
	return nil, errUnsupportedContract
}

func (server) Health(context.Context, *pluginpb.HealthRequest) (*pluginpb.HealthResponse, error) {
	return &pluginpb.HealthResponse{Ok: true}, nil
}

func (server) Send(_ context.Context, req *pluginpb.SendRequest) (*pluginpb.SendResponse, error) {
	log.Printf("event %s: %s (%s)", req.GetEvent().GetId(), req.GetEvent().GetTitle(), req.GetEvent().GetStatus())
	return &pluginpb.SendResponse{Delivered: true, Detail: "logged"}, nil
}

func newServer() *grpc.Server {
	s := grpc.NewServer()
	pluginpb.RegisterPluginServiceServer(s, server{})
	pluginpb.RegisterNotificationChannelServiceServer(s, server{})
	return s
}

func main() {
	path := os.Getenv("PLUGIN_SOCKET")
	if path == "" {
		log.Fatal("PLUGIN_SOCKET is not set")
	}
	_ = os.Remove(path) //nolint:gosec // path is the operator-set socket location
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", path)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	s := newServer()
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
		<-stop
		s.GracefulStop()
	}()
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
