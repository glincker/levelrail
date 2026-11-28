package build

import (
	"context"
	"net"

	bkclient "github.com/moby/buildkit/client"
	dockerclient "github.com/moby/moby/client"
)

// HijackClientOpts reaches the BuildKit inside dockerd over the daemon's
// hijacked HTTP connection. The moby client module no longer ships this helper.
func HijackClientOpts(c *dockerclient.Client) []bkclient.ClientOpt {
	return []bkclient.ClientOpt{
		bkclient.WithSessionDialer(func(ctx context.Context, proto string, meta map[string][]string) (net.Conn, error) {
			return c.DialHijack(ctx, "/session", proto, meta)
		}),
		bkclient.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return c.DialHijack(ctx, "/grpc", "h2c", nil)
		}),
	}
}
