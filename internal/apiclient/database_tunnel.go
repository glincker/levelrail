package apiclient

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/coder/websocket"
)

// DialDatabaseTunnel opens a byte stream to a managed database through the control plane, over GET /api/v1/databases/{name}/tunnel.
func (c *Client) DialDatabaseTunnel(ctx context.Context, name string) (net.Conn, error) {
	base, err := url.Parse(strings.TrimRight(c.baseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid API URL %q: %w", c.baseURL, err)
	}
	switch base.Scheme {
	case "http":
		base.Scheme = "ws"
	case "https":
		base.Scheme = "wss"
	default:
		return nil, fmt.Errorf("invalid API URL %q: expected an http or https scheme", c.baseURL)
	}
	prefix := strings.TrimRight(base.Path, "/")
	base.Path = prefix + "/api/v1/databases/" + name + "/tunnel"
	base.RawPath = prefix + "/api/v1/databases/" + url.PathEscape(name) + "/tunnel"

	header := http.Header{}
	if c.token != "" {
		header.Set("Authorization", "Bearer "+c.token)
	}
	ws, resp, err := websocket.Dial(ctx, base.String(), &websocket.DialOptions{HTTPHeader: header}) //nolint:bodyclose // coder/websocket closes the handshake body itself
	if err != nil {
		if resp != nil {
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return nil, fmt.Errorf("open tunnel to database %q: %s: %s", name, resp.Status, strings.TrimSpace(string(body)))
		}
		return nil, fmt.Errorf("open tunnel to database %q: %w", name, err)
	}
	ws.SetReadLimit(1 << 20)
	return websocket.NetConn(ctx, ws, websocket.MessageBinary), nil
}
