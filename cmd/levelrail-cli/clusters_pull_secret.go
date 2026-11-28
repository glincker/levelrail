package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/glincker/levelrail/internal/apiclient"
)

// runClustersPullSecret handles pull-secret add|list|remove. The password comes from a file
// or stdin only, so it never lands in argv or shell history.
func runClustersPullSecret(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		_, _ = fmt.Fprint(stderr, clustersUsage(prog))
		return exitUsage
	}
	verb, rest := args[0], args[1:]
	if verb != "add" && verb != "list" && verb != "remove" {
		_, _ = fmt.Fprintf(stderr, "%s: unknown pull-secret subcommand %q\n\n", prog, verb)
		_, _ = fmt.Fprint(stderr, clustersUsage(prog))
		return exitUsage
	}
	cluster, rest := splitClusterName(rest)
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "clusters pull-secret "+verb, "print the result as JSON to stdout and nothing else", stderr)
	var registry, username, passwordFile string
	if verb == "add" {
		fs.StringVar(&registry, "registry", "", "registry host, for example ghcr.io")
		fs.StringVar(&username, "username", "", "registry username")
		fs.StringVar(&passwordFile, "password-file", "", "file with the password or token, or - for stdin")
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, rest, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos := fs.Args()
	if cluster == "" && len(pos) > 0 {
		cluster, pos = pos[0], pos[1:]
	}
	if cluster == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("a cluster name is required"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	if verb == "list" {
		rows, err := client.ListK8sPullSecrets(ctx, cluster)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("clusters pull-secret list: %w", err))
		}
		return renderClusterOut(stdout, stderr, of, rows, func() { printK8sPullSecrets(stdout, rows) })
	}
	if len(pos) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("a pull secret name is required"))
	}
	name := pos[0]
	if verb == "remove" {
		if err := client.RemoveK8sPullSecret(ctx, cluster, name); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("clusters pull-secret remove: %w", err))
		}
		return renderClusterOut(stdout, stderr, of, map[string]string{"removed": name}, func() {
			_, _ = fmt.Fprintf(stdout, "removed pull secret %s\n", name)
		})
	}
	if registry == "" || username == "" || passwordFile == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--registry, --username and --password-file are required"))
	}
	raw, err := readSecretFileOrStdin(passwordFile)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
	}
	password := strings.TrimRight(raw, "\r\n")
	res, err := client.AddK8sPullSecret(ctx, cluster, apiclient.AddK8sPullSecretRequest{Name: name, Registry: registry, Username: username, Password: password})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clusters pull-secret add: %w", err))
	}
	return renderClusterOut(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "stored pull secret %s for %s\n", res.Name, res.Registry)
	})
}

func printK8sPullSecrets(out io.Writer, rows []apiclient.K8sPullSecret) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "no pull secrets")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tREGISTRY")
	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", r.Name, r.Registry)
	}
	_ = tw.Flush()
}
