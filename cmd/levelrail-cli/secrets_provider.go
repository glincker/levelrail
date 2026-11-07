package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const secretsProviderJSONUsage = "print the result as JSON to stdout and nothing else"

// runSecretsProvider dispatches add/list/test/remove for external secret providers.
func runSecretsProvider(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, secretsProviderUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, secretsProviderUsage(prog))
		return exitOK
	case "add":
		return runSecretsProviderAdd(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runSecretsProviderList(prog, args[1:], stdout, stderr, lookupEnv)
	case "test":
		return runSecretsProviderTest(prog, args[1:], stdout, stderr, lookupEnv)
	case "remove":
		return runSecretsProviderRemove(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown secrets provider subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, secretsProviderUsage(prog))
		return exitUsage
	}
}

func secretsProviderUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s secrets provider add NAME --type doppler --project P --environment CONFIG --provider-token-file PATH|-
  %[1]s secrets provider add NAME --type infisical --project WORKSPACE_ID --environment ENV [--path /dir] [--base-url URL] --provider-token-file PATH|-
  %[1]s secrets provider add vault --type vault --address URL --provider-token-file PATH|-
  %[1]s secrets provider list [flags]
  %[1]s secrets provider test NAME [flags]
  %[1]s secrets provider remove NAME [flags]

Connects read-only external secret providers. An app secret whose value is
doppler://NAME/SECRET or infisical://NAME/[dir/]SECRET is read live from the
provider when its container is created. Tokens are stored encrypted and never
shown. For Vault, NAME is ignored and the existing vault settings are used.
`, prog)
}

func secretsProviderName(prog string, fs interface{ Arg(int) string }, stderr io.Writer) (string, bool) {
	name := fs.Arg(0)
	if name == "" {
		_, _ = fmt.Fprintf(stderr, "%s: a provider NAME is required\n", prog)
		return "", false
	}
	return name, true
}

func runSecretsProviderAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets provider add", secretsProviderJSONUsage, stderr)
	var typeFlag, projectFlag, envFlag, pathFlag, baseURLFlag, tokenFileFlag, addressFlag string
	fs.StringVar(&typeFlag, "type", "", "provider type: doppler, infisical or vault")
	fs.StringVar(&projectFlag, "project", "", "Doppler project, or Infisical workspace ID")
	fs.StringVar(&envFlag, "environment", "", "Doppler config, or Infisical environment slug")
	fs.StringVar(&pathFlag, "path", "", "Infisical folder path (default /)")
	fs.StringVar(&baseURLFlag, "base-url", "", "self-hosted Infisical URL; private addresses are allowed only here")
	fs.StringVar(&addressFlag, "address", "", "Vault address (--type vault)")
	fs.StringVar(&tokenFileFlag, "provider-token-file", "", "file holding the provider token, or - for stdin; omit to keep the stored token")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, secretsProviderUsage(prog)+"\nFlags:\n")
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
	var providerToken string
	if tokenFileFlag != "" {
		v, err := readSecretFileOrStdin(tokenFileFlag)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: read provider token: %v\n", prog, err)
			return exitUsage
		}
		providerToken = strings.TrimSpace(v)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if typeFlag == "vault" {
		settings, err := client.SetVault(context.Background(), updateVaultSettingsRequest{Enabled: true, Address: addressFlag, AuthMethod: "token", Credential: providerToken})
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("set vault settings: %w", err))
		}
		return renderOrFail(stdout, stderr, of, settings, func() { printVaultSettings(stdout, settings) })
	}
	if name == "" {
		_, _ = fmt.Fprintf(stderr, "%s: a provider NAME is required\n", prog)
		return exitUsage
	}
	res, err := client.PutSecretProvider(context.Background(), name, apiclient.PutSecretProviderRequest{
		Type: typeFlag, Project: projectFlag, Environment: envFlag, Path: pathFlag, BaseURL: baseURLFlag, Token: providerToken,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("save secret provider: %w", err))
	}
	return renderOrFail(stdout, stderr, of, res, func() { printSecretProviders(stdout, []apiclient.SecretProviderResource{res}) })
}

func runSecretsProviderList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets provider list", secretsProviderJSONUsage, stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListSecretProviders(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list secret providers: %w", err))
	}
	vaultSettings, vaultErr := client.GetVault(context.Background())
	return renderOrFail(stdout, stderr, of, list, func() {
		printSecretProviders(stdout, list)
		if vaultErr == nil && vaultSettings.Address != "" {
			_, _ = fmt.Fprintf(stdout, "vault\tvault\t%s\tenabled=%t\thas_credential=%t\n", vaultSettings.Address, vaultSettings.Enabled, vaultSettings.HasCredential)
		}
	})
}

func runSecretsProviderTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets provider test", secretsProviderJSONUsage, stderr)
	args, name := splitLeadingName(args)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if name == "" {
		if name, ok = secretsProviderName(prog, fs, stderr); !ok {
			return exitUsage
		}
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.TestSecretProvider(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test secret provider: %w", err))
	}
	code := renderOrFail(stdout, stderr, of, res, func() {
		if res.OK {
			_, _ = fmt.Fprintf(stdout, "%s: connection ok\n", name)
			return
		}
		_, _ = fmt.Fprintf(stdout, "%s: connection failed: %s\n", name, res.Reason)
	})
	if code == exitOK && !res.OK {
		return exitCheckFailed
	}
	return code
}

func runSecretsProviderRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "secrets provider remove", secretsProviderJSONUsage, stderr)
	var typeFlag string
	fs.StringVar(&typeFlag, "type", "", "pass vault to disconnect the Vault integration")
	args, name := splitLeadingName(args)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if typeFlag == "vault" {
		if _, err := client.DisconnectVault(context.Background()); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("disconnect vault: %w", err))
		}
		_, _ = fmt.Fprintln(stdout, "vault: disconnected")
		return exitOK
	}
	if name == "" {
		if name, ok = secretsProviderName(prog, fs, stderr); !ok {
			return exitUsage
		}
	}
	if err := client.DeleteSecretProvider(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("remove secret provider: %w", err))
	}
	_, _ = fmt.Fprintf(stdout, "%s: removed\n", name)
	return exitOK
}

// splitLeadingName lets NAME come before the flags, as in "add prod --type doppler".
func splitLeadingName(args []string) ([]string, string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[1:], args[0]
	}
	return args, ""
}

func printSecretProviders(out io.Writer, list []apiclient.SecretProviderResource) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no Doppler or Infisical providers configured")
		return
	}
	for _, p := range list {
		scope := p.Project + "/" + p.Environment
		if p.Path != "" {
			scope += p.Path
		}
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\thas_token=%t\n", p.Name, p.Type, scope, p.HasToken)
	}
}
