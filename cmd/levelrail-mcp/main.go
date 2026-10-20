// Command levelrail-mcp is an MCP server that lets an AI agent manage a control plane through its /api/v1 REST API.
// Each tool wraps one REST call, so a token with fewer abilities gets the same 403 the API returns.
// Write tools stay hidden unless --allow-write is set, and --transport=http binds loopback only by default.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/mcptools"
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
	f, err := parseAllFlags(prog, args)
	if err != nil {
		return err
	}
	tokenFlag, apiURLFlag, profileFlag, transportFlag, listenFlag := f.token, f.apiURL, f.profile, f.transport, f.listen
	if f.printConfig != "" {
		return runPrintConfig(prog, f, lookupEnv)
	}
	opts, err := resolveOptions(f.mode, f.toolsets, f.toolProfile, lookupEnv)
	if err != nil {
		return err
	}
	opts.Dynamic = !f.all && !envTrue(lookupEnv, mcptools.EnvAllTools) && len(opts.Toolsets) == 0 && opts.Profile == ""
	opts.AllowWrite = f.allowWrite || envTrue(lookupEnv, mcptools.EnvAllowWrite)
	opts.OmitOutputSchemas = !f.outputSchemas && !envTrue(lookupEnv, mcptools.EnvOutputSchemas)

	profile := resolveProfile(profileFlag, lookupEnv)
	token := resolveToken(tokenFlag, lookupEnv, prog, profile)
	apiURL := resolveAPIURL(apiURLFlag, lookupEnv, prog, profile)
	apiclient.SetScope(apiclient.ResolveScope(apiclient.Scope{Team: f.team, Project: f.project}, lookupEnv, prog, profile))
	client := apiclient.NewClient(apiURL, token, apiclient.WithUserAgent("levelrail-mcp/"+version.Version))
	if f.readonly || envTrue(lookupEnv, envReadonly) {
		if err := enforceReadonly(client); err != nil {
			return err
		}
		opts.Mode, opts.AllowWrite = mcptools.ModeReadOnly, false
	}

	syncCtx, cancelSync := context.WithTimeout(context.Background(), 5*time.Second)
	if err := client.SyncBetaFeatures(syncCtx); err != nil {
		logger.Warn("beta feature sync failed, gating follows APP_EXPERIMENTAL only", slog.String("error", err.Error()))
	}
	cancelSync()

	server, summary := mcptools.NewServerWithOptions(client, opts)
	logAttrs := []any{
		slog.String("mode", string(summary.Mode)),
		slog.String("tool_profile", summary.Profile),
		slog.Int("tools", summary.Total),
		slog.Int("read", summary.Read),
		slog.Int("mutating", summary.Mutate),
		slog.Int("destructive", summary.Destruct),
		slog.Bool("progressive", summary.Dynamic),
		slog.Bool("allow_write", opts.AllowWrite),
		slog.Bool("output_schemas", !opts.OmitOutputSchemas),
	}
	if len(summary.Toolsets) > 0 {
		logAttrs = append(logAttrs, slog.String("toolsets", strings.Join(summary.Toolsets, ",")))
	}
	logger.Info("levelrail-mcp tools", logAttrs...)

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

func resolveOptions(modeFlag, toolsetsFlag, toolProfileFlag string, lookupEnv func(string) (string, bool)) (mcptools.Options, error) {
	if modeFlag == "" {
		modeFlag, _ = lookupEnv(mcptools.EnvMode)
	}
	if toolsetsFlag == "" {
		toolsetsFlag, _ = lookupEnv(mcptools.EnvToolsets)
	}
	if toolProfileFlag == "" {
		toolProfileFlag, _ = lookupEnv(mcptools.EnvToolProfile)
	}
	toolProfile, err := mcptools.ParseToolProfile(toolProfileFlag)
	if err != nil {
		return mcptools.Options{}, fmt.Errorf("resolve mcp tool profile: %w", err)
	}
	mode, err := mcptools.ParseMode(modeFlag)
	if err != nil {
		return mcptools.Options{}, fmt.Errorf("resolve mcp mode: %w", err)
	}
	toolsets, err := mcptools.ParseToolsets(toolsetsFlag)
	if err != nil {
		return mcptools.Options{}, fmt.Errorf("resolve mcp toolsets: %w", err)
	}
	return mcptools.Options{Mode: mode, Toolsets: toolsets, Profile: toolProfile}, nil
}

const (
	transportStdio = "stdio"
	transportHTTP  = "http"
)

// defaultListenAddr binds to loopback only, so running in network mode
// doesn't accidentally expose the server beyond the local machine unless
// an operator explicitly passes a different --listen address.
const defaultListenAddr = "127.0.0.1:8090"

// cliFlags holds every parsed command line flag.
type cliFlags struct {
	token, apiURL, profile, transport, listen, mode, toolsets, toolProfile string
	team, project, printConfig                                             string
	allowWrite, outputSchemas, readonly, all                               bool
}

func parseAllFlags(prog string, args []string) (f cliFlags, err error) {
	fs := flag.NewFlagSet(prog, flag.ContinueOnError)
	fs.StringVar(&f.token, "token", "", "API token (overrides "+apiclient.EnvAPIToken+" and the credentials file)")
	fs.StringVar(&f.apiURL, "api-url", "", "control plane API base URL (overrides "+apiclient.EnvAPIURL+" and the credentials file, default "+apiclient.DefaultAPIURL+")")
	fs.StringVar(&f.profile, "profile", "", "named credentials profile to read (overrides "+apiclient.EnvProfile+", default \""+apiclient.DefaultProfile+"\")")
	fs.StringVar(&f.transport, "transport", "", "MCP transport: \""+transportStdio+"\" (default, spawn as a local subprocess) or \""+transportHTTP+"\" (network-reachable, requires an API token)")
	fs.StringVar(&f.listen, "listen", "", "address to bind in --transport=http mode (default "+defaultListenAddr+", loopback only)")
	fs.StringVar(&f.mode, "mode", "", "tool exposure: \"read-only\", \"standard\" (default, no destructive tools) or \"full\" (overrides "+mcptools.EnvMode+")")
	fs.StringVar(&f.toolsets, "toolsets", "", "comma separated tool groups to expose, default all (overrides "+mcptools.EnvToolsets+")")
	fs.StringVar(&f.toolProfile, "tool-profile", "", "tool profile: \""+mcptools.ProfileAgentCore+"\" exposes a small allowlist of core agent tools (overrides "+mcptools.EnvToolProfile+")")
	fs.StringVar(&f.team, "team", "", "narrow every request to one team (overrides "+apiclient.EnvTeam+")")
	fs.StringVar(&f.project, "project", "", "narrow every request to one project (overrides "+apiclient.EnvProject+")")
	fs.StringVar(&f.printConfig, "print-config", "", "print a ready MCP client config block for one of: "+strings.Join(configClients, ", ")+", then exit (read-only unless --allow-write or --mode is set)")
	fs.BoolVar(&f.all, "all", false, "list every tool at once instead of the small core plus enable_toolset, for clients without tools/list_changed support (or set "+mcptools.EnvAllTools+"=1)")
	fs.BoolVar(&f.allowWrite, "allow-write", false, "expose confirm-gated write tools such as extend_preview and run_restore_drill (or set "+mcptools.EnvAllowWrite+"=1); never exposed in read-only mode")
	fs.BoolVar(&f.readonly, "readonly", false, "refuse to start unless the API token holds no write, write:sensitive, deploy or root ability, and expose read tools only (or set "+envReadonly+"=1)")
	fs.BoolVar(&f.outputSchemas, "output-schemas", false, "advertise every tool's outputSchema in tools/list, about 2.4x the listing size (or set "+mcptools.EnvOutputSchemas+"=1)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), "%s: an MCP server exposing the control plane's REST API as tools over stdio or streamable HTTP.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	if parseErr := fs.Parse(args); parseErr != nil {
		return cliFlags{}, parseErr
	}
	return f, nil
}

func envTrue(lookupEnv func(string) (string, bool), key string) bool {
	v, _ := lookupEnv(key)
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
