package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const secretsConnectionJSONUsage = "print the result as JSON to stdout and nothing else"

func secretsConnectionUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s secrets connection list [flags]
  %[1]s secrets connection create NAME --type vault|doppler|infisical|onepassword [config flags] (--credential-file PATH|- | --credential-ref NS/KEY)
  %[1]s secrets connection update NAME [config flags] [--credential-file PATH|- | --credential-ref NS/KEY]
  %[1]s secrets connection delete NAME [flags]
  %[1]s secrets connection test NAME [--path PATH[#key]] [flags]

Named connections are read with secret://NAME/path#key references. The existing
Vault settings appear as the connection "vault" and existing providers keep
their names; change those with "secrets provider". Credentials are stored
encrypted and never shown. Config flags: --address, --namespace, --mount,
--auth-method, --role-id (Vault); --project, --environment, --path, --base-url.
"test" proves login only; with --path on a Vault connection it also checks the
credential's policy allows reading that path (no value is read or returned).
Addresses in the link-local, metadata, unspecified or multicast ranges are refused.
`, prog)
}

func runSecretsConnection(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, secretsConnectionUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, secretsConnectionUsage(prog))
		return exitOK
	case "list":
		return runSecretsConnectionList(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runSecretsConnectionWrite(prog, "create", args[1:], stdout, stderr, lookupEnv)
	case "update":
		return runSecretsConnectionWrite(prog, "update", args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runSecretsConnectionDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "test":
		return runSecretsConnectionTest(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown secrets connection subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, secretsConnectionUsage(prog))
	return exitUsage
}

func runSecretsConnectionList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets connection list", secretsConnectionJSONUsage, stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListSecretConnections(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list secret connections: %w", err))
	}
	return renderOrFail(stdout, stderr, of, list, func() { printSecretConnections(stdout, list) })
}

func runSecretsConnectionWrite(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets connection "+verb, secretsConnectionJSONUsage, stderr)
	var req apiclient.SecretConnectionRequest
	var credFile string
	fs.StringVar(&req.Type, "type", "", "connection type: vault, doppler, infisical or onepassword")
	fs.StringVar(&req.Config.Address, "address", "", "Vault or OpenBao address")
	fs.StringVar(&req.Config.Namespace, "namespace", "", "Vault namespace")
	fs.StringVar(&req.Config.Mount, "mount", "", "Vault KV v2 mount (default secret)")
	fs.StringVar(&req.Config.AuthMethod, "auth-method", "", "Vault auth method: token or approle")
	fs.StringVar(&req.Config.RoleID, "role-id", "", "Vault AppRole role ID")
	fs.StringVar(&req.Config.Project, "project", "", "Doppler project, Infisical workspace ID or default 1Password vault")
	fs.StringVar(&req.Config.Environment, "environment", "", "Doppler config or Infisical environment slug")
	fs.StringVar(&req.Config.Path, "path", "", "Infisical folder limit")
	fs.StringVar(&req.Config.BaseURL, "base-url", "", "self-hosted Infisical URL or 1Password Connect URL")
	fs.StringVar(&req.CredentialRef, "credential-ref", "", "use an existing stored secret as the credential, as NAMESPACE/KEY")
	fs.StringVar(&credFile, "credential-file", "", "file holding the credential, or - for stdin; omit to keep the stored one on update")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, secretsConnectionUsage(prog)+"\nFlags:\n")
		fs.PrintDefaults()
	}
	args, name := splitLeadingName(args)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if name == "" {
		name = fs.Arg(0)
	}
	if name == "" {
		_, _ = fmt.Fprintf(stderr, "%s: a connection NAME is required\n", prog)
		return exitUsage
	}
	if credFile != "" {
		v, err := readSecretFileOrStdin(credFile)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: read credential: %v\n", prog, err)
			return exitUsage
		}
		req.Credential = strings.TrimSpace(v)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var res apiclient.SecretConnection
	var err error
	if verb == "create" {
		req.Name = name
		res, err = client.CreateSecretConnection(context.Background(), req)
	} else {
		res, err = client.UpdateSecretConnection(context.Background(), name, req)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s secret connection: %w", verb, err))
	}
	return renderOrFail(stdout, stderr, of, res, func() { printSecretConnections(stdout, []apiclient.SecretConnection{res}) })
}

func runSecretsConnectionDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets connection delete", secretsConnectionJSONUsage, stderr)
	args, name := splitLeadingName(args)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if name == "" {
		name = fs.Arg(0)
	}
	if name == "" {
		_, _ = fmt.Fprintf(stderr, "%s: a connection NAME is required\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.DeleteSecretConnection(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete secret connection: %w", err))
	}
	_, _ = fmt.Fprintf(stdout, "%s: deleted\n", name)
	return exitOK
}

func runSecretsConnectionTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets connection test", secretsConnectionJSONUsage, stderr)
	var path string
	fs.StringVar(&path, "path", "", "also check read access to this secret path (PATH[#key], relative to the connection mount) without reading its value")
	args, name := splitLeadingName(args)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if name == "" {
		name = fs.Arg(0)
	}
	if name == "" {
		_, _ = fmt.Fprintf(stderr, "%s: a connection NAME is required\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.TestSecretConnection(context.Background(), name, path)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test secret connection: %w", err))
	}
	code := renderOrFail(stdout, stderr, of, res, func() {
		if res.OK {
			if path == "" {
				_, _ = fmt.Fprintf(stdout, "%s: connection ok (proves login only, add --path to check a secret path)\n", name)
				return
			}
			_, _ = fmt.Fprintf(stdout, "%s: connection ok and %s is readable by the credential\n", name, path)
			return
		}
		_, _ = fmt.Fprintf(stdout, "%s: connection failed (%s): %s\n", name, res.Class, res.Reason)
	})
	if code == exitOK && !res.OK {
		return exitCheckFailed
	}
	return code
}

func printSecretConnections(out io.Writer, list []apiclient.SecretConnection) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no secret connections configured")
		return
	}
	for _, c := range list {
		src := "stored"
		if c.Legacy {
			src = "legacy"
		}
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\thas_credential=%t\n", c.Name, c.Type, src, c.HasCredential)
	}
}
