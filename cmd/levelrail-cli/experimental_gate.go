package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

var experimentalCommands = map[string]experimental.Feature{
	"ai":                experimental.AIChat,
	"models":            experimental.AIModels,
	"lb":                experimental.LoadBalancer,
	"apply":             experimental.IaC,
	"diff":              experimental.IaC,
	"export":            experimental.IaC,
	"stack":             experimental.IaC,
	"cloudflare-tunnel": experimental.CloudflareTunnel,
	"roles":             experimental.AccessRoles,
	"environments":      experimental.GlobalEnvironments,
	"plugins":           experimental.Plugins,
	"git":               experimental.GitPush,
}

// experimentalFeatureFor returns the gated feature a command line invokes.
func experimentalFeatureFor(args []string) (experimental.Feature, bool) {
	if len(args) == 0 {
		return "", false
	}
	if args[0] == "export" && len(args) > 1 && (args[1] == "destinations" || args[1] == "audit") {
		return "", false
	}
	if f, ok := experimentalCommands[args[0]]; ok {
		return f, true
	}
	if (args[0] == "apps" || args[0] == "databases") && len(args) > 1 && args[1] == "move-env" {
		return experimental.GlobalEnvironments, true
	}
	if args[0] == "settings" && len(args) > 1 && args[1] == "ai-assistant" {
		return experimental.AIChat, true
	}
	if args[0] == "users" && len(args) > 1 && (args[1] == "role" || args[1] == "grants") {
		return experimental.AccessRoles, true
	}
	return "", false
}

// rejectDisabledExperimental prints an error and returns true when args
// invoke a feature that is off.
func rejectDisabledExperimental(prog string, args []string, stderr io.Writer) bool {
	f, gated := experimentalFeatureFor(args)
	if !gated || experimental.Enabled(f) {
		return false
	}
	_, _ = fmt.Fprintf(stderr, "%s: %s: %s\n", prog, args[0], experimental.DisabledMessage(f))
	return true
}

// filterExperimentalUsage drops help lines for features that are off.
func filterExperimentalUsage(usage string) string {
	var out []string
	for _, line := range strings.Split(usage, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			if f, ok := experimentalFeatureFor(fields[1:]); ok && !experimental.Enabled(f) {
				continue
			}
		}
		out = append(out, line)
	}
	joined := strings.Join(out, "\n")
	if !experimental.Enabled(experimental.AIChat) {
		joined = strings.ReplaceAll(joined, "|ai-assistant", "")
		joined = strings.ReplaceAll(joined, ", and the BYOK AI assistant", "")
	}
	return joined
}

const betaSyncTimeout = 3 * time.Second

// syncServerBetaFeatures lets the control plane's Settings switches decide what
// a gated command may do. Failures keep the env-only answer.
func syncServerBetaFeatures(prog string, args []string, lookupEnv func(string) (string, bool)) {
	f, gated := experimentalFeatureFor(args)
	help := len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help")
	if !help && (!gated || experimental.Enabled(f)) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), betaSyncTimeout)
	defer cancel()
	_ = apiClientFromFlags(prog, "", "", "", lookupEnv).SyncBetaFeatures(ctx)
}
