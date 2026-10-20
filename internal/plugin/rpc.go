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
	PanelData(ctx context.Context, req *pluginpb.PanelDataRequest) (*pluginpb.PanelDataResponse, error)
	PanelAction(ctx context.Context, req *pluginpb.PanelActionRequest) (*pluginpb.PanelActionResponse, error)
	dnsClient
}

type dnsClient interface {
	ListZones(ctx context.Context, req *pluginpb.ListZonesRequest) (*pluginpb.ListZonesResponse, error)
	ListRecords(ctx context.Context, req *pluginpb.ListRecordsRequest) (*pluginpb.ListRecordsResponse, error)
	UpsertRecord(ctx context.Context, req *pluginpb.UpsertRecordRequest) (*pluginpb.UpsertRecordResponse, error)
	DeleteRecord(ctx context.Context, req *pluginpb.DeleteRecordRequest) (*pluginpb.DeleteRecordResponse, error)
	VerifyRecord(ctx context.Context, req *pluginpb.VerifyRecordRequest) (*pluginpb.VerifyRecordResponse, error)
}

type dialFunc func(ctx context.Context, socketPath string, maxSend, maxRecv int) (rpcClient, io.Closer, error)

type grpcClient struct {
	svc    pluginpb.PluginServiceClient
	notify pluginpb.NotificationChannelServiceClient
	panel  pluginpb.PanelServiceClient
	dns    pluginpb.DnsProviderServiceClient
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

func (g grpcClient) PanelData(ctx context.Context, r *pluginpb.PanelDataRequest) (*pluginpb.PanelDataResponse, error) {
	return g.panel.PanelData(ctx, r)
}

func (g grpcClient) PanelAction(ctx context.Context, r *pluginpb.PanelActionRequest) (*pluginpb.PanelActionResponse, error) {
	return g.panel.PanelAction(ctx, r)
}

func (g grpcClient) ListZones(ctx context.Context, r *pluginpb.ListZonesRequest) (*pluginpb.ListZonesResponse, error) {
	return g.dns.ListZones(ctx, r)
}

func (g grpcClient) ListRecords(ctx context.Context, r *pluginpb.ListRecordsRequest) (*pluginpb.ListRecordsResponse, error) {
	return g.dns.ListRecords(ctx, r)
}

func (g grpcClient) UpsertRecord(ctx context.Context, r *pluginpb.UpsertRecordRequest) (*pluginpb.UpsertRecordResponse, error) {
	return g.dns.UpsertRecord(ctx, r)
}

func (g grpcClient) DeleteRecord(ctx context.Context, r *pluginpb.DeleteRecordRequest) (*pluginpb.DeleteRecordResponse, error) {
	return g.dns.DeleteRecord(ctx, r)
}

func (g grpcClient) VerifyRecord(ctx context.Context, r *pluginpb.VerifyRecordRequest) (*pluginpb.VerifyRecordResponse, error) {
	return g.dns.VerifyRecord(ctx, r)
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
	return grpcClient{svc: pluginpb.NewPluginServiceClient(conn), notify: pluginpb.NewNotificationChannelServiceClient(conn), panel: pluginpb.NewPanelServiceClient(conn), dns: pluginpb.NewDnsProviderServiceClient(conn)}, conn, nil
}
