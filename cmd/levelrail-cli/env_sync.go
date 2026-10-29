package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const defaultEnvFile = ".env"

func runEnv(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, envUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, envUsage(prog))
		return exitOK
	case "set":
		return runEnvSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "unset":
		return runEnvUnset(prog, args[1:], stdout, stderr, lookupEnv)
	case "pull":
		return runEnvPull(prog, args[1:], stdout, stderr, lookupEnv)
	case "push":
		return runEnvPush(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown env subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, envUsage(prog))
		return exitUsage
	}
}

func envUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s env set <app> KEY=VALUE... [--apply] [flags]                    set plain env vars on an app
  %[1]s env unset <app> KEY... [--apply] [flags]                        remove plain env vars from an app
  %[1]s env pull <app> [--file .env] [--masked] [flags]                 write an app's env vars to a local .env file
  %[1]s env push <app> [--file .env] [--dry-run] [--keep-existing] [flags]   merge a local .env file into an app's env vars

Pull asks for your password (and authenticator code) once so secret values
are written too, and creates the file with owner-only permissions. Use
--masked to skip that and write secrets as empty keys. Database and storage
links inject their connection env at container start and are not part of the
pulled file. Push checks new values for credentials and warns by key name
only; keys already stored as secrets are never overwritten.
`, prog)
}

// splitEnvSyncFlags pulls out the flags env pull/push own and returns the
// rest to hand to the apps env subcommand they wrap.
func splitEnvSyncFlags(args []string) (file string, masked bool, rest []string) {
	file = defaultEnvFile
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--masked" || a == "-masked":
			masked = true
		case a == "--file" || a == "-file":
			if i+1 < len(args) {
				file = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--file="):
			file = strings.TrimPrefix(a, "--file=")
		default:
			rest = append(rest, a)
		}
	}
	return file, masked, rest
}

func runEnvPull(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	file, masked, rest := splitEnvSyncFlags(args)
	wrapped := append(rest, "--out", file)
	if !masked {
		wrapped = append(wrapped, "--reveal")
	}
	return runAppsEnvExport(prog, wrapped, stdout, stderr, lookupEnv)
}

func runEnvPush(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	file, _, rest := splitEnvSyncFlags(args)
	return runAppsEnvImport(prog, append(rest, "--file", file), stdout, stderr, lookupEnv)
}

// warnEnvSecrets reports keys whose new values look like credentials. A
// control plane without scanning (or any error) just skips the check.
func warnEnvSecrets(ctx context.Context, client *Client, stderr io.Writer, app string, plan envImportPlan) {
	vars := map[string]string{}
	for _, k := range append(append([]string{}, plan.New...), plan.Changed...) {
		vars[k] = plan.Merged[k]
	}
	if len(vars) == 0 {
		return
	}
	res, err := client.ScanAppEnv(ctx, app, vars)
	if err != nil {
		return
	}
	printEnvSecretWarnings(stderr, res.Findings)
}

func printEnvSecretWarnings(w io.Writer, findings []apiclient.EnvScanFinding) {
	for _, f := range findings {
		if f.Allowed {
			continue
		}
		_, _ = fmt.Fprintf(w, "warning: %s looks like a credential (%s); store it with \"apps secrets set\" so it stays encrypted\n", f.Var, f.Description)
	}
}
