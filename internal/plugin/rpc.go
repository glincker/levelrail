package plugin

import (
	"context"
	"fmt"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

// ContractVersions are the plugin contract versions this host speaks.
var ContractVersions = []uint32{1}

// rpcClient is what the runtime needs from a connected plugin.
type rpcClient interface {
	Describe(ctx context.Context, req *pluginpb.DescribeRequest) (*pluginpb.DescribeResponse, error)
	Health(ctx context.Context, req *pluginpb.HealthRequest) (*pluginpb.HealthResponse, error)
	Send(ctx context.Context, req *pluginpb.SendRequest) (*pluginpb.SendResponse, error)
}

type dialFunc func(ctx context.Context, socketPath string, maxSend, maxRecv int) (rpcClient, io.Closer, error)

type grpcClient struct {
	svc    pluginpb.PluginServiceClient
	notify pluginpb.NotificationChannelServiceClient
}

func (g grpcClient) Describe(ctx context.Context, r *pluginpb.DescribeRequest) (*pluginpb.DescribeResponse, error) {
	return g.svc.Describe(ctx, r)
}

func (g grpcClient) Health(ctx context.Context, r *pluginpb.HealthRequest) (*pluginpb.HealthResponse, error) {
	return g.svc.Health(ctx, r)
}

func (g grpcClient) Send(ctx context.Context, r *pluginpb.SendRequest) (*pluginpb.SendResponse, error) {
	return g.notify.Send(ctx, r)
}

// dialUnix connects lazily over a unix socket. Message size caps apply in both directions.
func dialUnix(_ context.Context, socketPath string, maxSend, maxRecv int) (rpcClient, io.Closer, error) {
	conn, err := grpc.NewClient("unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(maxSend), grpc.MaxCallRecvMsgSize(maxRecv)),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("plugin: dial %s: %w", socketPath, err)
	}
	return grpcClient{svc: pluginpb.NewPluginServiceClient(conn), notify: pluginpb.NewNotificationChannelServiceClient(conn)}, conn, nil
}
