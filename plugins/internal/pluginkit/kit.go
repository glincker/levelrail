// Package pluginkit is the shared scaffolding for first-party notification
// channel plugins. It speaks only the public plugin contract (pluginpb).
package pluginkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
)

const contractVersion = 1

// Call is one delivery request: the event, the plugin-wide config and the
// per-call credential, which a plugin must never store or log.
type Call struct {
	Event  *pluginpb.Event
	Config json.RawMessage
	Secret string
}

// Result is the plugin's answer to a Call.
type Result struct {
	Delivered bool
	Retryable bool
	Detail    string
}

// Plugin describes one channel and how it delivers.
type Plugin struct {
	Name         string
	Version      string
	ConfigSchema string
	Send         func(ctx context.Context, c Call) Result
}

type server struct {
	pluginpb.UnimplementedPluginServiceServer
	pluginpb.UnimplementedNotificationChannelServiceServer
	p Plugin
}

func (s server) Describe(_ context.Context, req *pluginpb.DescribeRequest) (*pluginpb.DescribeResponse, error) {
	for _, v := range req.GetHostContractVersions() {
		if v == contractVersion {
			schema := s.p.ConfigSchema
			if schema == "" {
				schema = `{"type":"object","additionalProperties":false}`
			}
			return &pluginpb.DescribeResponse{
				ContractVersion: contractVersion, Name: s.p.Name, Version: s.p.Version, Kind: "notification_channel",
				RequestedAbilities: []string{"read"}, ConfigSchema: schema,
			}, nil
		}
	}
	return nil, status.Error(codes.FailedPrecondition, "no common contract version")
}

func (server) Health(context.Context, *pluginpb.HealthRequest) (*pluginpb.HealthResponse, error) {
	return &pluginpb.HealthResponse{Ok: true}, nil
}

func (s server) Send(ctx context.Context, req *pluginpb.SendRequest) (*pluginpb.SendResponse, error) {
	cfg := json.RawMessage(req.GetConfigJson())
	if strings.TrimSpace(string(cfg)) == "" {
		cfg = json.RawMessage("{}")
	}
	r := s.p.Send(ctx, Call{Event: req.GetEvent(), Config: cfg, Secret: req.GetSecret()})
	return &pluginpb.SendResponse{Delivered: r.Delivered, Detail: r.Detail, Retryable: r.Retryable}, nil
}

// NewServer returns the gRPC server for p, ready to Serve on a listener.
func NewServer(p Plugin) *grpc.Server {
	s := grpc.NewServer()
	srv := server{p: p}
	pluginpb.RegisterPluginServiceServer(s, srv)
	pluginpb.RegisterNotificationChannelServiceServer(s, srv)
	return s
}

// Main serves p on the unix socket named by PLUGIN_SOCKET until SIGTERM.
func Main(p Plugin) {
	path := os.Getenv("PLUGIN_SOCKET")
	if path == "" {
		log.Fatal("PLUGIN_SOCKET is not set")
	}
	_ = os.Remove(path) //nolint:gosec // path is the operator-set socket location
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", path)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	s := NewServer(p)
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

// ParseConfig decodes the plugin-wide config strictly.
func ParseConfig(raw json.RawMessage, into any) error {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	return nil
}

// Invalid is a permanent failure caused by operator configuration.
func Invalid(err error) Result {
	if err == nil {
		err = errors.New("invalid configuration")
	}
	return Result{Detail: err.Error()}
}
