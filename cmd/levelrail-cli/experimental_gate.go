package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

var experimentalCommands = map[string]experimental.Feature{
	"ai":                experimental.AIChat,
	"models":            experimental.AIModels,
	"lb":                experimental.LoadBalancer,
	"apply":             experimental.IaC,
	"diff":              experimental.IaC,
	"export":            experimental.IaC,
	"cloudflare-tunnel": experimental.CloudflareTunnel,
	"roles":             experimental.AccessRoles,
	"environments":      experimental.GlobalEnvironments,
}

// experimentalFeatureFor returns the gated feature a command line invokes.
func experimentalFeatureFor(args []string) (experimental.Feature, bool) {
	if len(args) == 0 {
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
