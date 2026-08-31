package main

import (
	"fmt"
	"io"
)

// runRegistryCredentials dispatches "registry-credentials <verb> [flags]"
// to one of list/get/create/update/delete: managing the pull credentials
// internal/api/registry_credentials.go exposes (Settings -> Registry
// credentials in the web UI), the same credentials app.yaml's
// build.registryCredential field references by name.
func runRegistryCredentials(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, registryCredentialsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, registryCredentialsUsage(prog))
		return exitOK
	case "list":
		return runRegistryCredentialsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runRegistryCredentialsGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runRegistryCredentialsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "update":
		return runRegistryCredentialsUpdate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runRegistryCredentialsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "test":
		return runRegistryCredentialsTest(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown registry-credentials subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, registryCredentialsUsage(prog))
		return exitUsage
	}
}

func registryCredentialsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s registry-credentials list [flags]                                                                    list connected registry credentials
  %[1]s registry-credentials get <id> [flags]                                                                 show one registry credential
  %[1]s registry-credentials create --name NAME --registry-host HOST --username USER --password PASS [flags]   connect a new registry credential
  %[1]s registry-credentials update <id> --name NAME --registry-host HOST --username USER [flags]              update a registry credential, optionally rotating its password
  %[1]s registry-credentials delete <id> [flags]                                                               disconnect a registry credential
  %[1]s registry-credentials test <id> [flags]                                                                 authenticate a credential against its registry, without pulling anything

Run "%[1]s registry-credentials <subcommand> -h" for a subcommand's own flags.
`, prog)
}
