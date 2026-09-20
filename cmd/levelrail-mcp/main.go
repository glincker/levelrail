// Command levelrail-mcp is an MCP (Model Context Protocol) server that
// lets an AI agent manage a control plane deployment through the same
// versioned REST API (internal/api, mounted at /api/v1) cmd/levelrail-cli
// already drives: see internal/apiclient's own doc comment, and
// internal/store/tokens.go's APIToken doc comment on why a bearer token
// was always meant to authenticate "a CLI, an MCP server, or a
// third-party integration" alike. Every tool below is a thin wrapper
// around one REST call; there is no special internal access, and a
// token scoped to fewer abilities than a tool needs gets back the same
// 403 the REST API itself would return (internal/api/router.go's own
// comment on why the scoped-ability token system exists).
//
// # Configuring an MCP client
//
// Add this to Claude Desktop's/Claude Code's MCP server config, using an
// API token minted via "levelrail-cli auth login" or "levelrail-cli
// tokens create":
//
//	{
//	  "mcpServers": {
//	    "levelrail": {
//	      "command": "levelrail-mcp",
//	      "args": [],
//	      "env": {
//	        "APP_API_TOKEN": "<your API token>",
//	        "APP_API_URL": "http://localhost:8080"
//	      }
//	    }
//	  }
//	}
//
// If levelrail-cli has already been configured on the same machine (its
// own "auth login" writes ~/.config/levelrail-cli/credentials), this
// server picks up the same token and URL automatically and the env
// block above can be omitted entirely; --token/--api-url flags and the
// APP_API_TOKEN/APP_API_URL env vars still take precedence when set,
// exactly like the CLI's own resolution order.
//
// # Running over the network
//
// --transport=http runs the MCP Streamable HTTP transport instead of
// stdio, for a client that cannot spawn this as a local subprocess (a
// remotely hosted AI assistant, for instance). It binds to loopback only
// by default (--listen overrides this) and requires the same API token
// this process already uses outbound, presented as a bearer token on
// every incoming request; see docs/ai-assistant.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	prog := filepath.Base(os.Args[0])
	// stdio is the MCP transport: stdout carries the JSON-RPC stream to
	// the client, so every diagnostic goes to stderr instead, never
	// stdout, or it would corrupt that stream.
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if err := run(prog, os.Args[1:], os.LookupEnv, logger); err != nil {
		logger.Error("exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(prog string, args []string, lookupEnv func(string) (string, bool), logger *slog.Logger) error {
	tokenFlag, apiURLFlag, profileFlag, transportFlag, listenFlag, err := parseFlags(prog, args)
	if err != nil {
		return err
	}

	profile := resolveProfile(profileFlag, lookupEnv)
	token := resolveToken(tokenFlag, lookupEnv, prog, profile)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	client := apiclient.NewClient(apiURL, token, apiclient.WithUserAgent("levelrail-mcp/"+version.Version))

	server := newServer(client)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch transportFlag {
	case "", transportStdio:
		logger.Info("levelrail-mcp starting", slog.String("api_url", apiURL), slog.String("transport", transportStdio))
		return server.Run(ctx, &mcp.StdioTransport{})
	case transportHTTP:
		// Unlike stdio, where only a local process on the same trusted
		// machine can connect, a network listener is reachable by
		// anything that can route to it: refuse to start unauthenticated.
		if token == "" {
			return fmt.Errorf("--transport=http requires an API token (set --token, %s, or run \"levelrail-cli auth login\"); refusing to start an unauthenticated network listener", apiclient.EnvAPIToken)
		}
		listenAddr := listenFlag
		if listenAddr == "" {
			listenAddr = defaultListenAddr
		}
		logger.Info("levelrail-mcp starting", slog.String("api_url", apiURL), slog.String("transport", transportHTTP), slog.String("listen", listenAddr))
		return serveHTTP(ctx, server, listenAddr, token, logger)
	default:
		return fmt.Errorf("unknown --transport %q, want %q or %q", transportFlag, transportStdio, transportHTTP)
	}
}

const (
	transportStdio = "stdio"
	transportHTTP  = "http"
)

// defaultListenAddr binds to loopback only, so running in network mode
// doesn't accidentally expose the server beyond the local machine unless
// an operator explicitly passes a different --listen address.
const defaultListenAddr = "127.0.0.1:8090"

func parseFlags(prog string, args []string) (token, apiURL, profile, transport, listen string, err error) {
	fs := flag.NewFlagSet(prog, flag.ContinueOnError)
	fs.StringVar(&token, "token", "", "API token (overrides "+apiclient.EnvAPIToken+" and the credentials file)")
	fs.StringVar(&apiURL, "api-url", "", "control plane API base URL (overrides "+apiclient.EnvAPIURL+" and the credentials file, default "+apiclient.DefaultAPIURL+")")
	fs.StringVar(&profile, "profile", "", "named credentials profile to read (overrides "+apiclient.EnvProfile+", default \""+apiclient.DefaultProfile+"\")")
	fs.StringVar(&transport, "transport", "", "MCP transport: \""+transportStdio+"\" (default, spawn as a local subprocess) or \""+transportHTTP+"\" (network-reachable, requires an API token)")
	fs.StringVar(&listen, "listen", "", "address to bind in --transport=http mode (default "+defaultListenAddr+", loopback only)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "%s: an MCP server exposing the control plane's REST API as tools over stdio or streamable HTTP.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	if parseErr := fs.Parse(args); parseErr != nil {
		return "", "", "", "", "", parseErr
	}
	return token, apiURL, profile, transport, listen, nil
}

// newServer builds the MCP server and registers every tool against
// client. Split out from run so tests can connect to it directly over
// an in-memory transport (mcp.NewInMemoryTransports), without spawning a
// real process or touching stdio.
func newServer(client *apiclient.Client) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "levelrail-mcp", Version: version.Version}, nil)

	registerAppTools(server, client)
	registerDatabaseTools(server, client)
	registerServiceTemplateTools(server, client)
	registerNodeTools(server, client)
	registerPreviewTools(server, client)
	registerAlertTools(server, client)
	registerAppMetricsTools(server, client)
	registerDiagnosticTools(server, client)
	registerResourceRecommendationTools(server, client)
	registerFeatureFlagTools(server, client)
	registerSystemTools(server, client)
	registerWebhookTools(server, client)
	registerBackupVerificationTools(server, client)
	registerDeployCompareTools(server, client)
	registerPromoteTools(server, client)
	registerNotificationTools(server, client)
	registerAuditTools(server, client)
	registerIAMTools(server, client)
	registerOrganizationTools(server, client)
	registerRegistryCredentialTools(server, client)
	registerAppConfigTools(server, client)
	registerBackupTargetTools(server, client)
	registerVolumeBackupTools(server, client)
	registerScheduledTaskTools(server, client)
	registerEnvironmentTools(server, client)
	registerDomainTools(server, client)
	registerCloudflareTools(server, client)
	registerCertificateTools(server, client)
	registerLogDrainTools(server, client)
	registerSettingsTools(server, client)

	return server
}
