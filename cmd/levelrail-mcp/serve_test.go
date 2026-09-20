package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// bearerRoundTripper injects an Authorization header on every request, so
// tests can drive mcp.StreamableClientTransport (which has no header
// field of its own) with a bearer token.
type bearerRoundTripper struct {
	token string
}

func (rt bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if rt.token != "" {
		req.Header.Set("Authorization", "Bearer "+rt.token)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func newAuthedTestServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	client := apiclient.NewClient("http://unused.invalid", "test-token")
	server := newServer(client)
	srv := httptest.NewServer(newAuthedHandler(server, token))
	t.Cleanup(srv.Close)
	return srv
}

func TestServeHTTP_AuthenticatedHandshakeSucceeds(t *testing.T) {
	srv := newAuthedTestServer(t, "secret-token")

	transport := &mcp.StreamableClientTransport{
		Endpoint:   srv.URL,
		HTTPClient: &http.Client{Transport: bearerRoundTripper{token: "secret-token"}},
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := mcpClient.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("Connect() with a correct bearer token: %v", err)
	}
	defer func() { _ = session.Close() }()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() after a successful handshake: %v", err)
	}
	if len(tools.Tools) == 0 {
		t.Error("ListTools() returned no tools, want at least one registered tool")
	}
}

func TestServeHTTP_MissingTokenRejected(t *testing.T) {
	srv := newAuthedTestServer(t, "secret-token")

	transport := &mcp.StreamableClientTransport{Endpoint: srv.URL}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	if _, err := mcpClient.Connect(context.Background(), transport, nil); err == nil {
		t.Fatal("Connect() with no bearer token = nil error, want an error")
	}
}

func TestServeHTTP_WrongTokenRejected(t *testing.T) {
	srv := newAuthedTestServer(t, "secret-token")

	transport := &mcp.StreamableClientTransport{
		Endpoint:   srv.URL,
		HTTPClient: &http.Client{Transport: bearerRoundTripper{token: "wrong-token"}},
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	if _, err := mcpClient.Connect(context.Background(), transport, nil); err == nil {
		t.Fatal("Connect() with an incorrect bearer token = nil error, want an error")
	}
}

func TestRun_HTTPTransportRefusesToStartWithoutToken(t *testing.T) {
	lookupEnv := func(string) (string, bool) { return "", false }
	err := run("levelrail-mcp", []string{"--transport", "http"}, lookupEnv, discardLogger())
	if err == nil {
		t.Fatal("run() with --transport=http and no token = nil error, want an error")
	}
}

func TestParseFlags_DefaultListenIsLoopback(t *testing.T) {
	_, _, _, _, listen, err := parseFlags("levelrail-mcp", []string{"--transport", "http"})
	if err != nil {
		t.Fatalf("parseFlags() error = %v", err)
	}
	if listen != "" {
		t.Fatalf("listen flag = %q, want empty (caller falls back to defaultListenAddr)", listen)
	}
	if defaultListenAddr != "127.0.0.1:8090" {
		t.Errorf("defaultListenAddr = %q, want a 127.0.0.1 loopback address", defaultListenAddr)
	}
}
