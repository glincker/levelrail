package main

import (
	"context"
	"fmt"
	"io"
)

// vaultJSONUsage is every vault subcommand's --json flag description:
// identical across get/set/disconnect since each returns the same
// settings shape.
const vaultJSONUsage = "print the settings as JSON to stdout and nothing else"

// runVault dispatches "vault <verb> [flags]" to one of get/set/
// disconnect, mirroring runCloudflareTunnel's own get/set/disconnect
// dispatch shape. Not nested under "settings": Cloudflare Tunnel, the
// other instance-level external-integration credential, is its own
// top-level command too.
func runVault(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, vaultUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, vaultUsage(prog))
		return exitOK
	case "get":
		return runVaultGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runVaultSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "disconnect":
		return runVaultDisconnect(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown vault subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, vaultUsage(prog))
		return exitUsage
	}
}

func vaultUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s vault get [flags]                                           show the current settings
  %[1]s vault set --address URL --auth-method token --vault-token TOKEN   configure and enable (token auth)
  %[1]s vault set --address URL --auth-method approle --role-id ID --secret-id SECRET_ID   configure and enable (AppRole auth)
  %[1]s vault disconnect [flags]                                    disable and forget the stored credential

Configures resolving an app.yaml env var's value live from an external
HashiCorp Vault instance ({ vault: { path, key } }), as an alternative to
"%[1]s secrets" (this platform's own envelope-encrypted storage). The
credential (a Vault token or an AppRole secret ID) is stored encrypted,
never shown back; role_id is not itself sensitive and is echoed on get.

Run "%[1]s vault <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runVaultGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "vault get", vaultJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s vault get [flags]\n\nShows the current external Vault integration settings.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.GetVault(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get vault settings: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printVaultSettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runVaultSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "vault set", vaultJSONUsage, stderr)
	var (
		addressFlag    string
		authMethodFlag string
		namespaceFlag  string
		roleIDFlag     string
		mountPathFlag  string
		vaultTokenFlag string
		secretIDFlag   string
		enabledFlag    bool
	)
	fs.StringVar(&addressFlag, "address", "", "Vault server address, e.g. https://vault.internal:8200 (required the first time vault is enabled)")
	fs.StringVar(&authMethodFlag, "auth-method", "token", "how to authenticate to Vault: \"token\" or \"approle\"")
	fs.StringVar(&namespaceFlag, "namespace", "", "Vault Enterprise namespace, if any")
	fs.StringVar(&roleIDFlag, "role-id", "", "AppRole role ID (required for --auth-method approle; not itself sensitive)")
	fs.StringVar(&mountPathFlag, "mount-path", "", "KV v2 secrets engine mount path (default \"secret\")")
	fs.StringVar(&vaultTokenFlag, "vault-token", "", "Vault token (--auth-method token; required the first time vault is enabled with this auth method; omit to keep the currently stored credential; distinct from --token, this CLI's own bearer auth flag)")
	fs.StringVar(&secretIDFlag, "secret-id", "", "AppRole secret ID (--auth-method approle; required the first time vault is enabled with this auth method; omit to keep the currently stored credential)")
	fs.BoolVar(&enabledFlag, "enabled", true, "resolve vault-backed app.yaml env vars (pass --enabled=false to disable without clearing the stored credential)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s vault set --address URL --auth-method token|approle [flags]\n\nConfigures the external Vault integration. A credential (--token for\ntoken auth, --secret-id for approle auth) is required the first time\nvault is enabled with a given auth method.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	credential := vaultTokenFlag
	if authMethodFlag == "approle" {
		credential = secretIDFlag
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.SetVault(context.Background(), updateVaultSettingsRequest{
		Enabled:    enabledFlag,
		Address:    addressFlag,
		AuthMethod: authMethodFlag,
		Namespace:  namespaceFlag,
		RoleID:     roleIDFlag,
		MountPath:  mountPathFlag,
		Credential: credential,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set vault settings: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printVaultSettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runVaultDisconnect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "vault disconnect", vaultJSONUsage, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s vault disconnect [flags]\n\nDisables vault and forgets the stored credential.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.DisconnectVault(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("disconnect vault: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, settings, func() { printVaultSettings(stdout, settings) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printVaultSettings(out io.Writer, s vaultSettingsResource) {
	_, _ = fmt.Fprintf(out, "enabled:        %v\n", s.Enabled)
	_, _ = fmt.Fprintf(out, "address:        %s\n", s.Address)
	_, _ = fmt.Fprintf(out, "auth_method:    %s\n", s.AuthMethod)
	if s.Namespace != "" {
		_, _ = fmt.Fprintf(out, "namespace:      %s\n", s.Namespace)
	}
	if s.RoleID != "" {
		_, _ = fmt.Fprintf(out, "role_id:        %s\n", s.RoleID)
	}
	_, _ = fmt.Fprintf(out, "mount_path:     %s\n", s.MountPath)
	_, _ = fmt.Fprintf(out, "has_credential: %v\n", s.HasCredential)
}
