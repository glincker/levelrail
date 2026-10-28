package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/k8simport"
)

func importK8sUsage(prog string) string {
	return fmt.Sprintf(`Usage: %[1]s import k8s -f <file|-> [flags]

Convert Kubernetes manifests (Deployment, StatefulSet, DaemonSet, Job, CronJob,
Service, Ingress, ConfigMap, Secret, PersistentVolumeClaim, List) into Levelrail
apps and apply them. Nothing is fetched from a URL and nothing is executed.

Flags:
  -f, --file FILE          manifest file, or - for stdin (required)
  --dry-run                print the documents and the report, change nothing
  --app-prefix P           prefix every app name
  --namespace-as MODE      environment (default), prefix or ignore
  --project NAME           project for the environment documents (default kubernetes)
  --force                  apply even when the report holds blockers
  -o, --output FORMAT      table (default) or json
  --api-url, --profile     control plane to use
`, prog)
}

// runImportK8s implements "import k8s".
func runImportK8s(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var file, prefix, nsAs, project, output, apiURL, profile string
	var dry, force, asJSON bool
	fs := flag.NewFlagSet(prog+" import k8s", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, importK8sUsage(prog)) }
	fs.StringVar(&file, "f", "", "manifest file or - for stdin")
	fs.StringVar(&file, "file", "", "same as -f")
	fs.BoolVar(&dry, "dry-run", false, "print the plan and report, change nothing")
	fs.StringVar(&prefix, "app-prefix", "", "prefix for every app name")
	fs.StringVar(&nsAs, "namespace-as", "environment", "environment, prefix or ignore")
	fs.StringVar(&project, "project", "", "project for environment documents")
	fs.BoolVar(&force, "force", false, "apply even with blockers")
	fs.StringVar(&output, "o", "table", "table or json")
	fs.StringVar(&output, "output", "table", "same as -o")
	fs.BoolVar(&asJSON, "json", false, "same as -o json")
	fs.StringVar(&apiURL, "api-url", "", "control plane API base URL")
	fs.StringVar(&profile, "profile", "", "named credentials profile")
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		_, _ = fmt.Fprint(stdout, importK8sUsage(prog))
		return exitOK
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	jsonOut := asJSON || output == "json"
	if output != "table" && output != "json" {
		return reportError(stdout, stderr, jsonOut, newValidationError("-o must be table or json, got %q", output))
	}
	if file == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("import k8s needs -f <file|->"))
	}
	manifest, err := readK8sInput(file, stdin)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	client := apiClientFromFlags(prog, apiURL, "", profile, lookupEnv)
	res, err := client.ImportK8s(context.Background(), manifest, apiclient.K8sImportOptions{DryRun: dry, Force: force, AppPrefix: prefix, NamespaceAs: nsAs, Project: project})
	if err != nil && len(res.Findings) == 0 && len(res.Issues) == 0 {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("import k8s: %w", err))
	}
	if jsonOut {
		_ = writeJSONValue(stdout, res)
	} else {
		printK8sImport(stdout, res)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: import refused: %d blockers, pass --force to apply anyway after reviewing the report\n", prog, res.Blockers)
		return exitCheckFailed
	}
	return exitOK
}

// readK8sInput reads a local file or stdin up to the size cap. URLs are refused.
func readK8sInput(file string, stdin io.Reader) (string, error) {
	if strings.Contains(file, "://") {
		return "", newValidationError("remote URLs are never fetched: download the file and pass its path")
	}
	limit := int64(k8simport.LimitsFromEnv().MaxBytes) + 1
	r := stdin
	if file != "-" {
		f, err := os.Open(file) //nolint:gosec // operator-chosen manifest path
		if err != nil {
			return "", fmt.Errorf("read %s: %w", file, err)
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	raw, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", file, err)
	}
	if int64(len(raw)) >= limit {
		return "", errors.New("manifest is larger than the limit (" + k8simport.EnvMaxBytes + ")")
	}
	return string(raw), nil
}

func printK8sImport(w io.Writer, res apiclient.K8sImportResult) {
	verb := "Would create"
	if res.Applied {
		verb = "Created"
	}
	_, _ = fmt.Fprintf(w, "%s %d apps\n", verb, len(res.Apps))
	for _, a := range res.Apps {
		_, _ = fmt.Fprintf(w, "  %-24s %-32s port %d x%d  from %s\n", a.Name, a.Image, a.Port, a.Replicas, a.Source)
		if len(a.Domains) > 0 {
			_, _ = fmt.Fprintf(w, "    domains: %s\n", strings.Join(a.Domains, ", "))
		}
		if len(a.SecretKeys) > 0 {
			_, _ = fmt.Fprintf(w, "    secret keys (values hidden): %s\n", strings.Join(a.SecretKeys, ", "))
		}
	}
	if len(res.Findings) > 0 {
		_, _ = fmt.Fprintf(w, "\nReport: %d findings, %d blockers\n", len(res.Findings), res.Blockers)
		for _, f := range res.Findings {
			_, _ = fmt.Fprintf(w, "  [%s] %s/%s %s: %s\n", f.Severity, f.Kind, f.Name, f.Field, f.Reason)
		}
	}
	for _, is := range res.Issues {
		_, _ = fmt.Fprintf(w, "  invalid: %s\n", is.String())
	}
}
