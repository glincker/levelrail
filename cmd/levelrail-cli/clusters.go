package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Dispatches the cluster verbs; the name is the first positional argument.
func runClusters(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, clustersUsage(prog))
		return exitUsage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, clustersUsage(prog))
		return exitOK
	case "rbac":
		return runClustersRBAC(prog, rest, stdout, stderr)
	case "list":
		return runClustersVerb(prog, sub, false, rest, stdout, stderr, lookupEnv, clustersList)
	case "show", "test", "nodes", "namespaces", "disconnect":
		return runClustersVerb(prog, sub, true, rest, stdout, stderr, lookupEnv, nil)
	case "deploy":
		return runClustersDeploy(prog, rest, stdout, stderr, lookupEnv)
	case "deployments", "status":
		return runClustersDeployVerb(prog, sub, rest, stdout, stderr, lookupEnv)
	case "logs":
		return runClustersLogs(prog, rest, stdout, stderr, lookupEnv)
	case "remove":
		return runClustersRemove(prog, rest, stdout, stderr, lookupEnv)
	case "pull-secret":
		return runClustersPullSecret(prog, rest, stdout, stderr, lookupEnv)
	case "connect":
		return runClustersConnect(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown clusters subcommand %q\n\n", prog, sub)
		_, _ = fmt.Fprint(stderr, clustersUsage(prog))
		return exitUsage
	}
}

func clustersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s clusters connect <name> (--kubeconfig FILE | --server URL --token-file FILE [--ca-file FILE]) [--context C] [--allow-cluster-admin] [flags]
  %[1]s clusters list [flags]                 connected Kubernetes clusters
  %[1]s clusters show <name> [flags]          one cluster
  %[1]s clusters test <name> [flags]          read-only connection test
  %[1]s clusters nodes <name> [flags]         nodes with readiness and roles
  %[1]s clusters namespaces <name> [flags]    namespaces
  %[1]s clusters disconnect <name> [flags]    forget the stored credential
  %[1]s clusters deploy <name> --app A --image I --port N [--env-file F] [--replicas N] [--host H] [--namespace NS] [--allow-root] [--pull-secret NAME]
  %[1]s clusters deployments <name> [flags]   apps deployed to a cluster
  %[1]s clusters status <name> <app> [flags]  rollout state, conditions and ingress address
  %[1]s clusters logs <name> <app> [--tail N] [--pod P] [flags]  tail the logs of a deployed app
  %[1]s clusters remove <name> <app> [--yes]  delete the app's Deployment, Service and Ingress (the namespace stays)
  %[1]s clusters pull-secret add <cluster> <name> --registry HOST --username U (--password-file FILE | --password-file -)  store a private registry credential
  %[1]s clusters pull-secret list <cluster> [flags]  stored registry credentials (names and hosts only)
  %[1]s clusters pull-secret remove <cluster> <name> [flags]  delete a stored registry credential
  %[1]s clusters rbac --namespace NS [--cluster-read] [flags]  print a namespace-scoped ServiceAccount, Role and RoleBinding (--cluster-read adds a read-only ClusterRole for nodes, namespaces and ingresses, needed for --host)

Credentials come from a file, or "-" for stdin, never from an argument.
`, prog)
}

// clustersList is the only verb without a name, kept in the same shape as the named ones.
func clustersList(ctx context.Context, c *Client, _ string) (any, func(io.Writer), error) {
	rows, err := c.ListK8sClusters(ctx)
	if err != nil {
		return nil, nil, err
	}
	return rows, func(w io.Writer) { printClustersTable(w, rows) }, nil
}

type clusterVerbFn func(ctx context.Context, c *Client, name string) (any, func(io.Writer), error)

func clusterVerbFor(verb string) clusterVerbFn {
	switch verb {
	case "show":
		return func(ctx context.Context, c *Client, name string) (any, func(io.Writer), error) {
			r, err := c.GetK8sCluster(ctx, name)
			return r, func(w io.Writer) { printClustersTable(w, []apiclient.K8sClusterResource{r}) }, err
		}
	case "test":
		return func(ctx context.Context, c *Client, name string) (any, func(io.Writer), error) {
			r, err := c.TestK8sCluster(ctx, name)
			return r, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "ok=%t class=%s cluster_admin=%t\n", r.OK, r.Class, r.ClusterAdmin)
			}, err
		}
	case "nodes":
		return func(ctx context.Context, c *Client, name string) (any, func(io.Writer), error) {
			rows, err := c.ListK8sNodes(ctx, name)
			return rows, func(w io.Writer) { printClusterNodes(w, rows) }, err
		}
	case "namespaces":
		return func(ctx context.Context, c *Client, name string) (any, func(io.Writer), error) {
			rows, err := c.ListK8sNamespaces(ctx, name)
			return rows, func(w io.Writer) { printClusterNamespaces(w, rows) }, err
		}
	default:
		return func(ctx context.Context, c *Client, name string) (any, func(io.Writer), error) {
			err := c.DisconnectK8sCluster(ctx, name)
			res := map[string]string{"cluster": name, "status": "disconnected"}
			return res, func(w io.Writer) { _, _ = fmt.Fprintf(w, "cluster %q disconnected\n", name) }, err
		}
	}
}

// splitClusterName lets the name come before or after the flags.
func splitClusterName(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

func runClustersVerb(prog, verb string, needName bool, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), fn clusterVerbFn) int {
	name, rest := splitClusterName(args)
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "clusters "+verb, "print the result as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, rest, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if needName {
		if name == "" && fs.NArg() > 0 {
			name = fs.Arg(0)
		}
		if name == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("a cluster name is required"))
		}
	}
	if fn == nil {
		fn = clusterVerbFor(verb)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	data, human, err := fn(context.Background(), client, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clusters %s: %w", verb, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, data, func() { human(stdout) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runClustersConnect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	name, rest := splitClusterName(args)
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "clusters connect", "print the cluster as JSON to stdout and nothing else", stderr)
	var kubeconfig, ctxName, server, tokenFile, caFile string
	var allowAdmin bool
	fs.StringVar(&kubeconfig, "kubeconfig", "", "kubeconfig file to read, or - for stdin; sent once, never stored locally")
	fs.StringVar(&ctxName, "context", "", "kubeconfig context to use (default: the current one)")
	fs.StringVar(&server, "server", "", "API server URL, for a service-account token instead of a kubeconfig")
	fs.StringVar(&tokenFile, "token-file", "", "file holding the service-account token, or - for stdin")
	fs.StringVar(&caFile, "ca-file", "", "file holding the cluster CA certificate (PEM)")
	fs.BoolVar(&allowAdmin, "allow-cluster-admin", false, "accept a cluster-admin credential; also needs the server setting k8s.allow_cluster_admin")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, rest, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if name == "" && fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("a cluster name is required"))
	}
	if (kubeconfig == "") == (server == "") {
		return reportError(stdout, stderr, jsonOut, newValidationError("pass exactly one of --kubeconfig or --server"))
	}
	if kubeconfig == "-" && tokenFile == "-" {
		return reportError(stdout, stderr, jsonOut, newValidationError("only one input can read stdin"))
	}
	req := apiclient.ConnectK8sClusterRequest{Name: name, Context: ctxName, Server: server, AllowClusterAdmin: allowAdmin}
	var err error
	if kubeconfig != "" {
		req.Kubeconfig, err = readSecretFileOrStdin(kubeconfig)
	} else {
		if tokenFile == "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--server needs --token-file"))
		}
		if req.Token, err = readSecretFileOrStdin(tokenFile); err == nil {
			req.Token = strings.TrimSpace(req.Token)
		}
		if err == nil && caFile != "" {
			var b []byte
			b, err = os.ReadFile(caFile) //nolint:gosec // operator-supplied path, reading it is the command's job
			req.CA = string(b)
		}
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ConnectK8sCluster(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("connect cluster %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		printClustersTable(stdout, []apiclient.K8sClusterResource{res})
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printClustersTable(out io.Writer, rows []apiclient.K8sClusterResource) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "no clusters")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tSERVER\tAUTH\tLAST CHECK")
	for _, c := range rows {
		check := c.LastCheckClass
		if check == "" {
			check = "never"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.Name, c.Server, c.AuthMode, check)
	}
	_ = tw.Flush()
}

func printClusterNodes(out io.Writer, rows []apiclient.K8sNodeRow) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tREADY\tROLES\tKUBELET")
	for _, n := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%t\t%s\t%s\n", n.Name, n.Ready, strings.Join(n.Roles, ","), n.Kubelet)
	}
	_ = tw.Flush()
}

func printClusterNamespaces(out io.Writer, rows []apiclient.K8sNamespaceRow) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tPHASE")
	for _, n := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", n.Name, n.Phase)
	}
	_ = tw.Flush()
}
