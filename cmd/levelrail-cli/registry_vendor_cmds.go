package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/pkgregistry"
	"github.com/GLINCKER/levelrail/internal/registryvendor"
)

// runRegistryCredentialsNamespace dispatches "registry credentials <verb>":
// the vendor-aware front of the registry-credentials commands.
func runRegistryCredentialsNamespace(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		out := stderr
		code := exitUsage
		if len(args) > 0 {
			out, code = stdout, exitOK
		}
		_, _ = fmt.Fprintf(out, `Usage:
  %[1]s registry credentials list [flags]
  %[1]s registry credentials vendors [flags]          list the vendor presets for --vendor
  %[1]s registry credentials add --vendor ID [flags]  connect a credential with a vendor preset
  %[1]s registry credentials test <id> [flags]
  %[1]s registry credentials delete <id> [flags]
`, prog)
		return code
	}
	rest := args[1:]
	switch args[0] {
	case "list":
		return runRegistryCredentialsList(prog, rest, stdout, stderr, lookupEnv)
	case "test":
		return runRegistryCredentialsTest(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		return runRegistryCredentialsDelete(prog, rest, stdout, stderr, lookupEnv)
	case "vendors":
		return runRegistryVendors(prog, rest, stdout, stderr)
	case "add":
		return runRegistryCredentialsAdd(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown registry credentials subcommand %q\n", prog, args[0])
		return exitUsage
	}
}

func runRegistryVendors(prog string, args []string, stdout, stderr io.Writer) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry credentials vendors", "print the vendor presets as a JSON array to stdout and nothing else", stderr)
	_, _, _, _, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	vendors := registryvendor.Vendors()
	if err := renderResult(stdout, of.Format, of.Query, vendors, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "VENDOR\tNAME\tHOST\tUSERNAME")
		for _, v := range vendors {
			host := v.DefaultHost
			if host == "" {
				host = v.HostHint
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", v.ID, v.Name, host, v.FixedUsername)
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runRegistryCredentialsAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry credentials add", "print the created credential as JSON to stdout and nothing else", stderr)
	var vendorID, name, host, username, password, expiresAt string
	fs.StringVar(&vendorID, "vendor", registryvendor.Generic, "vendor preset: dockerhub, ghcr, gitlab, ecr, gar, acr, harbor, quay, jfrog, nexus or generic")
	fs.StringVar(&name, "name", "", "display name (default: the vendor id)")
	fs.StringVar(&host, "registry-host", "", "registry host (default: the vendor's)")
	fs.StringVar(&username, "username", "", "registry username (default: the vendor's fixed one)")
	fs.StringVar(&password, "password", "", "password or token; prefer stdin or the prompt, a flag value is visible in the process list")
	fs.StringVar(&expiresAt, "expires-at", "", "optional RFC3339 expiry (default: derived from the vendor's token lifetime)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s registry credentials add --vendor ID [--registry-host HOST] [--username USER] [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	vendor, found := registryvendor.Find(vendorID)
	if !found {
		return reportError(stdout, stderr, jsonOut, newValidationError("unknown --vendor %q, run \"%s registry credentials vendors\"", vendorID, prog))
	}
	if host == "" {
		host = vendor.DefaultHost
	}
	if host == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--registry-host is required for %s (%s)", vendor.Name, vendor.HostHint))
	}
	if username == "" {
		username = vendor.FixedUsername
	}
	if username == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--username is required for %s", vendor.Name))
	}
	if name == "" {
		name = vendor.ID
	}
	if password == "" {
		read, err := readNodeProviderProtectedValue(stderr, "Registry password or token: ", "registry password")
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		password = read
	}
	if password == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("a password or token is required: pass --password, pipe it on stdin, or run interactively"))
	}
	var expires *time.Time
	switch {
	case expiresAt != "":
		parsed, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("--expires-at must be RFC3339: %v", err))
		}
		expires = &parsed
	case vendor.TokenLifetimeHours > 0:
		t := time.Now().Add(time.Duration(vendor.TokenLifetimeHours) * time.Hour).UTC()
		expires = &t
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateRegistryCredential(context.Background(), createRegistryCredentialRequest{
		Name: name, RegistryHost: host, Username: username, Password: password, ExpiresAt: expires,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("add registry credential %q: %w", name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "registry credential %q (id %s, host %s) connected\n", created.Name, created.ID, created.RegistryHost)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// runRegistryPackageRegistries dispatches "registry package-registries".
func runRegistryPackageRegistries(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		out, code := stderr, exitUsage
		if len(args) > 0 {
			out, code = stdout, exitOK
		}
		_, _ = fmt.Fprintf(out, `Usage:
  %[1]s registry package-registries list [flags]
  %[1]s registry package-registries add --ecosystem E --name NAME --url URL [flags]
  %[1]s registry package-registries test <id> [flags]
  %[1]s registry package-registries delete <id> [flags]

ECOSYSTEM is one of %[2]s. The credential is mounted into Dockerfile
builds as a BuildKit secret (RUN --mount=type=secret,id=<build_secret_id>),
never as a build arg or an image layer.
`, prog, strings.Join(pkgregistry.Ecosystems, ", "))
		return code
	}
	rest := args[1:]
	switch args[0] {
	case "list":
		return runPackageRegistriesList(prog, rest, stdout, stderr, lookupEnv)
	case "add":
		return runPackageRegistriesAdd(prog, rest, stdout, stderr, lookupEnv)
	case "test":
		return runPackageRegistriesIDVerb(prog, "test", rest, stdout, stderr, lookupEnv)
	case "delete":
		return runPackageRegistriesIDVerb(prog, "delete", rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown registry package-registries subcommand %q\n", prog, args[0])
		return exitUsage
	}
}

func runPackageRegistriesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry package-registries list", "print package registries as a JSON array to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListPackageRegistries(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list package registries: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, list, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tNAME\tECOSYSTEM\tSCOPE\tURL\tSECRET ID")
		for _, p := range list {
			scope := p.ScopeKind
			if p.ScopeRef != "" {
				scope += ":" + p.ScopeRef
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", p.ID, p.Name, p.Ecosystem, scope, p.URL, p.BuildSecretID)
		}
		_ = tw.Flush()
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runPackageRegistriesAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "registry package-registries add", "print the created package registry as JSON to stdout and nothing else", stderr)
	var req apiclient.CreatePackageRegistryRequest
	fs.StringVar(&req.Ecosystem, "ecosystem", "", "npm, pypi, maven, nuget, cargo, go or rubygems (required)")
	fs.StringVar(&req.Name, "name", "", "unique display name (required)")
	fs.StringVar(&req.URL, "url", "", "registry URL, without embedded credentials (required)")
	fs.StringVar(&req.Username, "username", "", "username; omit for a bearer token registry")
	fs.StringVar(&req.Secret, "secret", "", "token or password; prefer stdin or the prompt, a flag value is visible in the process list")
	fs.StringVar(&req.ModulePatterns, "module-patterns", "", "go only: comma separated GOPRIVATE patterns")
	fs.StringVar(&req.ScopeKind, "scope", "instance", "instance, project or app")
	fs.StringVar(&req.ScopeRef, "scope-ref", "", "project id or app name, required for project and app scope")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s registry package-registries add --ecosystem E --name NAME --url URL [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	switch {
	case req.Ecosystem == "" || req.Name == "" || req.URL == "":
		return reportError(stdout, stderr, jsonOut, newValidationError("--ecosystem, --name and --url are required"))
	case !pkgregistry.IsEcosystem(req.Ecosystem):
		return reportError(stdout, stderr, jsonOut, newValidationError("--ecosystem must be one of %s", strings.Join(pkgregistry.Ecosystems, ", ")))
	}
	if req.Secret == "" {
		read, err := readNodeProviderProtectedValue(stderr, "Registry token or password: ", "package registry secret")
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		req.Secret = read
	}
	if req.Secret == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("a secret is required: pass --secret, pipe it on stdin, or run interactively"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreatePackageRegistry(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("add package registry %q: %w", req.Name, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stdout, "package registry %q (id %s) added; mount it with RUN --mount=type=secret,id=%s\n", created.Name, created.ID, created.BuildSecretID)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runPackageRegistriesIDVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	cmdName := "registry package-registries " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cmdName, "print the result as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, cmdName, "package registry id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if verb == "delete" {
		if err := client.DeletePackageRegistry(context.Background(), id); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete package registry %q: %w", id, err))
		}
		return finishPackageRegistryResult(stdout, stderr, of, map[string]bool{"deleted": true}, fmt.Sprintf("package registry %q removed", id))
	}
	res, err := client.TestPackageRegistry(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test package registry %q: %w", id, err))
	}
	if code := finishPackageRegistryResult(stdout, stderr, of, res, res.Message); code != exitOK {
		return code
	}
	if !res.OK {
		return exitAPIError
	}
	return exitOK
}

func finishPackageRegistryResult(stdout, stderr io.Writer, of outputFlags, v any, text string) int {
	if err := renderResult(stdout, of.Format, of.Query, v, func() { _, _ = fmt.Fprintln(stdout, text) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
