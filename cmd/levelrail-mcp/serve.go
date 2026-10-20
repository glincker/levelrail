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

// newAuthedHandler wraps the MCP handler so every request needs token as a bearer credential. Split out so tests can use httptest.
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

// serveHTTP serves server over MCP Streamable HTTP on listenAddr, requiring the bearer token. Blocks until ctx is canceled.
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

// bearerTokenVerifier compares the bearer token in constant time against the process's own static API token.
func bearerTokenVerifier(want string) auth.TokenVerifier {
	return func(_ context.Context, got string, _ *http.Request) (*auth.TokenInfo, error) {
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{}, nil
	}
}
