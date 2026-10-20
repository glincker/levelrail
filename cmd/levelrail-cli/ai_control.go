package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAIControl implements "ai-control status|set|revoke-agents": the server enforced switch for agent and AI access (internal/api/ai_control.go).
func runAIControl(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, aiControlUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, aiControlUsage(prog))
		return exitOK
	case "status":
		return runAIControlStatus(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAIControlSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "revoke-agents":
		return runAIControlRevoke(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown ai-control subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, aiControlUsage(prog))
		return exitUsage
	}
}

func aiControlUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s ai-control status [flags]
  %[1]s ai-control set --mode off|observe|operate|admin [--env-kinds dev,test,...] [flags]
  %[1]s ai-control revoke-agents --yes [flags]

Controls what agents and AI may do on this instance, enforced by the server:
  off      every agent request is refused
  observe  agents may only read
  operate  agents may read, write and deploy, never use root, only in allowed environment kinds
  admin    agents may use any ability their token holds, still bound by environment kinds
           (needs the experimental feature ai-control)
Environment kinds: dev, test, uat, production, preview, custom.
An agent is a token with an agent label, or the built-in assistant.
`, prog)
}

func runAIControlStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[apiclient.AIControlResource]{
		cmdLabel:  "ai-control status",
		jsonUsage: "print the AI control settings as JSON to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s ai-control status [flags]\n\nShows the current AI control mode and allowed environment kinds.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) (apiclient.AIControlResource, error) {
			return c.GetAIControl(ctx)
		},
		errVerb: "get ai-control settings",
		print:   func(out io.Writer, s apiclient.AIControlResource) { printAIControl(out, s) },
	})
}

func runAIControlSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "ai-control set", "print the updated AI control settings as JSON to stdout and nothing else", stderr)
	var mode, envKinds string
	fs.StringVar(&mode, "mode", "", "off, observe, operate or admin (required)")
	fs.StringVar(&envKinds, "env-kinds", "", "comma separated environment kinds agents may act in (default: keep the current list)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s ai-control set --mode MODE [--env-kinds a,b] [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if mode == "" {
		_, _ = fmt.Fprintf(stderr, "%s: ai-control set requires --mode\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	req := apiclient.UpdateAIControlRequest{Mode: mode}
	if envKinds != "" {
		for _, k := range strings.Split(envKinds, ",") {
			if k = strings.TrimSpace(k); k != "" {
				req.AllowedEnvKinds = append(req.AllowedEnvKinds, k)
			}
		}
	} else {
		cur, err := client.GetAIControl(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get ai-control settings: %w", err))
		}
		req.AllowedEnvKinds = cur.AllowedEnvKinds
	}
	res, err := client.UpdateAIControl(ctx, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set ai-control settings: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printAIControl(stdout, res) })
}

func runAIControlRevoke(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "ai-control revoke-agents", "print the result as JSON to stdout and nothing else", stderr)
	var yes bool
	fs.BoolVar(&yes, "yes", false, "confirm revoking every agent token")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s ai-control revoke-agents --yes [flags]\n\nRevokes every token that carries an agent label.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if !yes {
		_, _ = fmt.Fprintf(stderr, "%s: ai-control revoke-agents requires --yes\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.RevokeAgentTokens(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke agent tokens: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { _, _ = fmt.Fprintf(stdout, "revoked %d agent tokens\n", res.Revoked) })
}

func printAIControl(out io.Writer, s apiclient.AIControlResource) {
	_, _ = fmt.Fprintf(out, "mode:         %s\n", s.Mode)
	_, _ = fmt.Fprintf(out, "env kinds:    %s\n", strings.Join(s.AllowedEnvKinds, ", "))
	_, _ = fmt.Fprintf(out, "agent tokens: %d\n", s.AgentTokenCount)
	_, _ = fmt.Fprintf(out, "admin mode:   %v\n", s.AdminAvailable)
}
