package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// shutdownTimeout bounds how long serveHTTP waits for in-flight MCP
// requests to finish once ctx is canceled, before forcing the listener
// closed.
const shutdownTimeout = 5 * time.Second

// newAuthedHandler builds the MCP Streamable HTTP handler for server,
// wrapped so every request must carry token as a bearer credential. Split
// out from serveHTTP so tests can drive it with httptest.NewServer
// instead of a real net.Listen.
func newAuthedHandler(server *mcp.Server, token string) http.Handler {
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, nil)

	authOpts := &auth.RequireBearerTokenOptions{
		// token is a static, non-expiring shared secret, not an
		// OAuth-issued credential, so it carries no exp claim to check.
		AllowMissingExpiration: true,
	}
	return auth.RequireBearerToken(bearerTokenVerifier(token), authOpts)(handler)
}

// serveHTTP runs server over the MCP Streamable HTTP transport, bound to
// listenAddr, rejecting any request that doesn't carry token as a bearer
// credential. It blocks until ctx is canceled, then shuts the listener
// down gracefully.
func serveHTTP(ctx context.Context, server *mcp.Server, listenAddr, token string, logger *slog.Logger) error {
	httpServer := &http.Server{
		Addr:              listenAddr,
		Handler:           newAuthedHandler(server, token),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()

	select {
	case <-ctx.Done():
		logger.Info("levelrail-mcp shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down http server: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// bearerTokenVerifier checks an incoming bearer token against want using a
// constant-time comparison: want is the same static API token this process
// already uses for its own outbound REST calls (see config.go's
// resolveToken), not a credential the verifier looks up or parses.
func bearerTokenVerifier(want string) auth.TokenVerifier {
	return func(_ context.Context, got string, _ *http.Request) (*auth.TokenInfo, error) {
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{}, nil
	}
}
