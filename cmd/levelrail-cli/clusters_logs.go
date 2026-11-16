package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// runClustersLogs handles "logs <cluster> <app> [--tail N] [--pod P]".
func runClustersLogs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	name, rest := splitClusterName(args)
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "clusters logs", "print the result as JSON to stdout and nothing else", stderr)
	var pod string
	var tail int
	fs.StringVar(&pod, "pod", "", "pod name (default: the first running pod of the app)")
	fs.IntVar(&tail, "tail", 0, "lines from the end (default and cap: the server k8s.log_tail_lines_max)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, rest, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos := fs.Args()
	if name == "" && len(pos) > 0 {
		name, pos = pos[0], pos[1:]
	}
	if name == "" || len(pos) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("a cluster name and an app name are required"))
	}
	if tail < 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("--tail must not be negative"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	text, err := client.GetK8sDeploymentLogs(context.Background(), name, pos[0], pod, tail)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clusters logs: %w", err))
	}
	res := map[string]string{"cluster": name, "app": pos[0], "logs": text}
	return renderClusterOut(stdout, stderr, of, res, func() {
		_, _ = io.WriteString(stdout, text)
		if text != "" && !strings.HasSuffix(text, "\n") {
			_, _ = fmt.Fprintln(stdout)
		}
	})
}

// runClustersRemove handles "remove <cluster> <app> [--yes]". Without --yes it only previews.
func runClustersRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	name, rest := splitClusterName(args)
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "clusters remove", "print the result as JSON to stdout and nothing else", stderr)
	var yes bool
	fs.BoolVar(&yes, "yes", false, "delete the app's Deployment, Service and Ingress; without it nothing changes")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, rest, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos := fs.Args()
	if name == "" && len(pos) > 0 {
		name, pos = pos[0], pos[1:]
	}
	if name == "" || len(pos) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("a cluster name and an app name are required"))
	}
	app := pos[0]
	if !yes {
		_, _ = fmt.Fprintf(stdout, "preview only, nothing changed. Run %s clusters remove %s %s --yes to delete its Deployment, Service and Ingress (the namespace stays).\n", prog, name, app)
		return exitOK
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.RemoveK8sDeployment(context.Background(), name, app)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clusters remove: %w", err))
	}
	return renderClusterOut(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "removed %s from namespace %s\n", res.App, res.Namespace)
		for _, o := range res.Deleted {
			_, _ = fmt.Fprintf(stdout, "  deleted %s\n", o)
		}
		for _, o := range res.Absent {
			_, _ = fmt.Fprintf(stdout, "  already gone %s\n", o)
		}
	})
}
