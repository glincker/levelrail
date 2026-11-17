package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runClustersDeploy(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	name, rest := splitClusterName(args)
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "clusters deploy", "print the result as JSON to stdout and nothing else", stderr)
	var app, image, envFile, host, namespace, pullSecret string
	var port, replicas int
	var allowRoot bool
	fs.StringVar(&app, "app", "", "app name")
	fs.StringVar(&image, "image", "", "container image")
	fs.IntVar(&port, "port", 0, "container port")
	fs.IntVar(&replicas, "replicas", 0, "replica count (default: the server default)")
	fs.StringVar(&envFile, "env-file", "", "KEY=VALUE file, or - for stdin; values are never printed")
	fs.StringVar(&host, "host", "", "ingress host name")
	fs.StringVar(&namespace, "namespace", "", "namespace (default: the server default)")
	fs.StringVar(&pullSecret, "pull-secret", "", "stored registry credential for a private image (see clusters pull-secret add)")
	fs.BoolVar(&allowRoot, "allow-root", false, "let the image run as its own user, which may be root")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, rest, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if name == "" && fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" || app == "" || image == "" || port <= 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("a cluster name, --app, --image and --port are required"))
	}
	req := apiclient.K8sDeployRequest{App: app, Namespace: namespace, Image: image, Port: port, Host: host, AllowRoot: allowRoot, PullSecret: pullSecret}
	if replicas > 0 {
		req.Replicas = &replicas
	}
	if envFile != "" {
		raw, err := readSecretFileOrStdin(envFile)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
		}
		req.Env = map[string]string{}
		for _, e := range parseEnvFileBytes([]byte(raw)) {
			req.Env[e.Key] = e.Value
		}
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.DeployK8sApp(context.Background(), name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clusters deploy: %w", err))
	}
	return renderClusterOut(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "deployed %s to namespace %s\n", res.App, res.Namespace)
		for _, o := range res.Objects {
			_, _ = fmt.Fprintf(stdout, "  %s\n", o)
		}
	})
}

func renderClusterOut(stdout, stderr io.Writer, of outputFlags, data any, human func()) int {
	if err := renderResult(stdout, of.Format, of.Query, data, human); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runClustersDeployVerb handles "deployments <cluster>" and "status <cluster> <app>".
func runClustersDeployVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	name, rest := splitClusterName(args)
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "clusters "+verb, "print the result as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, rest, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos := fs.Args()
	if name == "" && len(pos) > 0 {
		name, pos = pos[0], pos[1:]
	}
	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("a cluster name is required"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if verb == "deployments" {
		rows, err := client.ListK8sDeployments(context.Background(), name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("clusters deployments: %w", err))
		}
		return renderClusterOut(stdout, stderr, of, rows, func() { printK8sDeployments(stdout, rows) })
	}
	if len(pos) == 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("an app name is required"))
	}
	st, err := client.GetK8sDeploymentStatus(context.Background(), name, pos[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clusters status: %w", err))
	}
	return renderClusterOut(stdout, stderr, of, st, func() { printK8sStatus(stdout, st) })
}

func printK8sDeployments(out io.Writer, rows []apiclient.K8sDeploymentRow) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "no deployments")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "APP\tNAMESPACE\tIMAGE\tREPLICAS\tHOST")
	for _, d := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", d.App, d.Namespace, d.Image, d.Replicas, d.Host)
	}
	_ = tw.Flush()
}

func printK8sStatus(out io.Writer, s apiclient.K8sDeploymentStatus) {
	if !s.Found {
		_, _ = fmt.Fprintf(out, "%s (%s): not found in the cluster\n", s.App, s.Namespace)
		if s.Reason != "" {
			_, _ = fmt.Fprintf(out, "reason: %s\n", s.Reason)
		}
		return
	}
	_, _ = fmt.Fprintf(out, "%s (%s): ready=%t, %d/%d replicas ready, %d updated\n", s.App, s.Namespace, s.Ready, s.ReadyReplicas, s.Replicas, s.Updated)
	if s.IngressAddress != "" {
		_, _ = fmt.Fprintf(out, "ingress: %s\n", s.IngressAddress)
	}
	if s.Reason != "" {
		_, _ = fmt.Fprintf(out, "reason: %s\n", s.Reason)
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	for _, c := range s.Conditions {
		_, _ = fmt.Fprintf(tw, "condition\t%s\t%s\t%s\n", c.Type, c.Status, strings.TrimSpace(c.Reason+" "+c.Message))
	}
	for _, p := range s.Pods {
		_, _ = fmt.Fprintf(tw, "pod\t%s\t%s\t%s\n", p.Name, p.Phase, strings.TrimSpace(p.Reason+" "+p.Message))
	}
	_ = tw.Flush()
}
