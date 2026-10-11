package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runNetwork implements "network mesh status|enable|disable".
func runNetwork(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, networkUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, networkUsage(prog))
		return exitOK
	case "mesh":
		return runNetworkMesh(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown network subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, networkUsage(prog))
		return exitUsage
	}
}

func networkUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s network mesh status [flags]     show whether the mesh is on and whether this server can run it
  %[1]s network mesh enable [flags]     turn the WireGuard mesh on (applies after a control plane restart)
  %[1]s network mesh disable [flags]    turn the mesh off; --force when other nodes are enrolled
`, prog)
}

func runNetworkMesh(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, networkUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "status":
		return runListCommand(prog, args[1:], stdout, stderr, lookupEnv, listCommandParams[apiclient.MeshSettings]{
			cmdLabel:  "network mesh status",
			jsonUsage: "print the mesh settings as JSON to stdout and nothing else",
			usageText: fmt.Sprintf("Usage:\n  %s network mesh status [flags]\n\nFlags:\n", prog),
			fetch: func(c *Client, ctx context.Context) (apiclient.MeshSettings, error) {
				return c.GetMeshSettings(ctx)
			},
			errVerb: "get mesh settings",
			print:   printMeshSettings,
		})
	case "enable":
		return runNetworkMeshSet(prog, args[1:], true, stdout, stderr, lookupEnv)
	case "disable":
		return runNetworkMeshSet(prog, args[1:], false, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown network mesh subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, networkUsage(prog))
		return exitUsage
	}
}

func runNetworkMeshSet(prog string, args []string, enable bool, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "network mesh disable"
	if enable {
		label = "network mesh enable"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the updated mesh settings as JSON to stdout and nothing else", stderr)
	var force bool
	if !enable {
		fs.BoolVar(&force, "force", false, "disable even though other nodes are enrolled and will lose their mesh path")
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s [flags]\n\nFlags:\n", prog, label)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.UpdateMeshSettings(context.Background(), apiclient.UpdateMeshSettingsRequest{Enabled: enable, Force: force})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", label, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printMeshSettings(stdout, res) })
}

func printMeshSettings(out io.Writer, s apiclient.MeshSettings) {
	state := "off"
	if s.Effective {
		state = "on"
	}
	_, _ = fmt.Fprintf(out, "mesh:        %s (configured %s, from %s)\n", state, meshOnOff(s.Configured), s.Source)
	if s.RestartRequired {
		_, _ = fmt.Fprintf(out, "restart:     required to apply: %s\n", s.RestartHint)
	}
	if s.StartError != "" {
		_, _ = fmt.Fprintf(out, "start error: %s\n", s.StartError)
	}
	_, _ = fmt.Fprintf(out, "other nodes: %d enrolled\n", s.RemoteNodes)
	for _, c := range s.Checks {
		_, _ = fmt.Fprintf(out, "  %-4s %-15s %s\n", c.Status, c.ID, c.Detail)
		if c.Hint != "" && c.Status != "pass" {
			_, _ = fmt.Fprintf(out, "       fix: %s\n", c.Hint)
		}
	}
}

func meshOnOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
