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

func pluginsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s plugins list [flags]                              list installed plugins
  %[1]s plugins show <name> [flags]                       show one plugin, its requested and approved abilities
  %[1]s plugins inspect (--manifest FILE | --url URL) [--signature FILE] [flags]
                                                          verify a package and show its abilities and what would change, installs nothing
  %[1]s plugins install (--manifest FILE | --url URL) [--signature FILE] [--key-id ID] [--digest DIGEST] [--config FILE] [--approve ABILITIES] [flags]
                                                          verify and record a plugin; --approve must list exactly the requested abilities
  %[1]s plugins upgrade (--manifest FILE | --url URL) [same flags as install]
                                                          replace an installed plugin; wider abilities or hosts need a new approval
  %[1]s plugins approve <name> --abilities read[,read:sensitive] [flags]
                                                          grant the listed abilities, never implied by install
  %[1]s plugins enable|disable|restart <name> [flags]     start, stop or restart a plugin (restart clears failure backoff)
  %[1]s plugins uninstall|remove <name> [flags]           stop and delete a plugin
  %[1]s plugins test <name> [flags]                       check a dns_provider plugin can reach its provider with the stored credential
  %[1]s plugins dns-provider [<name> | --off] [flags]     show or choose the dns_provider plugin that writes DNS records (default: built-in client)
  %[1]s plugins run <plugin> <command> [--app NAME] [flags]
                                                          run a command the plugin declares in its manifest (shows data or presses a button)
  %[1]s plugins trust add --name NAME --public-key B64 [flags]   trust a publisher's ed25519 public key
  %[1]s plugins trust list|remove <key-id> [flags]        show or revoke trusted keys
  %[1]s plugins marketplace search|info|install|update|refresh|sources ...
                                                          browse and one-step install from signed indexes (run it with no arguments for details)

Managing plugins needs the plugins experimental feature and a root token. Running a
plugin command needs read (data commands) or write:sensitive (buttons) and reaches only what the
operator approved. Nothing third party
loads until a key is trusted: the default trust list is empty.
`, prog)
}

func runPlugins(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, pluginsUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, pluginsUsage(prog))
		return exitOK
	case "list":
		return runPluginsList(prog, rest, stdout, stderr, lookupEnv)
	case "show":
		return runPluginsNamed(prog, "show", rest, stdout, stderr, lookupEnv, func(ctx context.Context, c *apiclient.Client, name string) (apiclient.Plugin, error) {
			return c.GetPlugin(ctx, name)
		})
	case "enable":
		return runPluginsNamed(prog, "enable", rest, stdout, stderr, lookupEnv, func(ctx context.Context, c *apiclient.Client, name string) (apiclient.Plugin, error) {
			return c.EnablePlugin(ctx, name)
		})
	case "disable":
		return runPluginsNamed(prog, "disable", rest, stdout, stderr, lookupEnv, func(ctx context.Context, c *apiclient.Client, name string) (apiclient.Plugin, error) {
			return c.DisablePlugin(ctx, name)
		})
	case "restart":
		return runPluginsNamed(prog, "restart", rest, stdout, stderr, lookupEnv, func(ctx context.Context, c *apiclient.Client, name string) (apiclient.Plugin, error) {
			return c.RestartPlugin(ctx, name)
		})
	case "remove", "uninstall":
		return runPluginsRemove(prog, rest, stdout, stderr, lookupEnv)
	case "install":
		return runPluginsInstall(prog, rest, stdout, stderr, lookupEnv)
	case "inspect":
		return runPluginsInspect(prog, rest, stdout, stderr, lookupEnv)
	case "upgrade":
		return runPluginsUpgrade(prog, rest, stdout, stderr, lookupEnv)
	case "approve":
		return runPluginsApprove(prog, rest, stdout, stderr, lookupEnv)
	case "test":
		return runPluginsTest(prog, rest, stdout, stderr, lookupEnv)
	case "dns-provider":
		return runPluginsDNSProvider(prog, rest, stdout, stderr, lookupEnv)
	case "run":
		return runPluginsRun(prog, rest, stdout, stderr, lookupEnv)
	case "trust":
		return runPluginsTrust(prog, rest, stdout, stderr, lookupEnv)
	case "marketplace":
		return runPluginsMarketplace(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown plugins subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, pluginsUsage(prog))
		return exitUsage
	}
}

func runPluginsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "plugins list", "print plugins as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pluginsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	plugins, err := client.ListPlugins(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list plugins: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, plugins, func() { printPlugins(stdout, plugins) })
}

func printPlugins(out io.Writer, plugins []apiclient.Plugin) {
	if len(plugins) == 0 {
		_, _ = fmt.Fprintln(out, "no plugins installed")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tVERSION\tKIND\tSTATE\tRUNNING\tAPPROVED\tREQUESTED")
	for _, p := range plugins {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\t%s\t%s\n", p.Name, p.Version, p.Kind, p.State, p.Running, joinOrNone(p.ApprovedAbilities), joinOrNone(p.RequestedAbilities))
	}
	_ = tw.Flush()
}

func joinOrNone(l []string) string {
	if len(l) == 0 {
		return "none"
	}
	return strings.Join(l, ",")
}

func printPlugin(out io.Writer, p apiclient.Plugin) {
	_, _ = fmt.Fprintf(out, "Name:       %s %s (%s)\nPublisher:  %s\nImage:      %s\nState:      %s (running: %t)\n", p.Name, p.Version, p.Kind, p.Publisher, p.Image, p.State, p.Running)
	_, _ = fmt.Fprintf(out, "Requested:  %s\nApproved:   %s\nNetwork:    %s\nSigned by:  %s\nHealth:     %s (verification: %s)\n", joinOrNone(p.RequestedAbilities), joinOrNone(p.ApprovedAbilities), outboundSummary(p.OutboundHosts), joinOrNone([]string{p.KeyID}), p.Health, p.Verification)
	printNetworkNote(out, p.OutboundHosts)
	for _, panel := range p.Panels {
		_, _ = fmt.Fprintf(out, "Panel:      %s\n", panel.Title)
	}
	if p.LastError != "" {
		_, _ = fmt.Fprintf(out, "Last error: %s\n", p.LastError)
	}
}

func outboundSummary(hosts []string) string {
	if len(hosts) == 0 {
		return "none (no network)"
	}
	return strings.Join(hosts, ", ") + " (declared by the plugin, not restricted)"
}

// networkNote is printed under any plan that lists hosts, so the list is not read as a limit.
const networkNote = "Note: a plugin with any declared host currently has general network access."

func printNetworkNote(out io.Writer, hosts []string) {
	if len(hosts) > 0 {
		_, _ = fmt.Fprintln(out, networkNote)
	}
}

func runPluginsNamed(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), call func(context.Context, *apiclient.Client, string) (apiclient.Plugin, error)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "plugins "+verb, "print the plugin as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pluginsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "plugins "+verb, "plugin name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	p, err := call(context.Background(), client, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("plugins %s %q: %w", verb, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, p, func() { printPlugin(stdout, p) })
}

func runPluginsRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "plugins remove", "print the result as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pluginsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "plugins remove", "plugin name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.RemovePlugin(context.Background(), name); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("remove plugin %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"name": name, "status": "removed"}, func() {
		_, _ = fmt.Fprintf(stdout, "plugin %q removed\n", name)
	})
}

func readTrimmed(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // operator-supplied path to their own manifest or signature
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func runPluginsApprove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "plugins approve", "print the plugin as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pluginsUsage(prog)) }
	abilities := fs.String("abilities", "", "comma separated abilities to grant (required)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "plugins approve", "plugin name")
	if !ok {
		return exitUsage
	}
	if strings.TrimSpace(*abilities) == "" {
		_, _ = fmt.Fprintf(stderr, "%s: plugins approve needs --abilities, nothing is approved by default\n\n%s", prog, pluginsUsage(prog))
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	p, err := client.ApprovePlugin(context.Background(), name, strings.Split(*abilities, ","))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("approve plugin %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, p, func() { printPlugin(stdout, p) })
}

func runPluginsTrust(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, pluginsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "add":
		return runPluginsTrustAdd(prog, args[1:], stdout, stderr, lookupEnv)
	case "list":
		return runPluginsTrustList(prog, args[1:], stdout, stderr, lookupEnv)
	case "remove":
		return runPluginsTrustRemove(prog, args[1:], stdout, stderr, lookupEnv)
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, pluginsUsage(prog))
		return exitOK
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown plugins trust subcommand %q\n\n%s", prog, args[0], pluginsUsage(prog))
		return exitUsage
	}
}

func runPluginsTrustAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "plugins trust add", "print the key as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pluginsUsage(prog)) }
	name := fs.String("name", "", "a label for the publisher (required)")
	pub := fs.String("public-key", "", "base64 raw ed25519 public key (required)")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if *name == "" || *pub == "" {
		_, _ = fmt.Fprintf(stderr, "%s: plugins trust add needs --name and --public-key\n\n%s", prog, pluginsUsage(prog))
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	k, err := client.AddPluginKey(context.Background(), *name, *pub)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("trust key: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, k, func() {
		_, _ = fmt.Fprintf(stdout, "trusted %s (%s)\n", k.KeyID, k.Name)
	})
}

func runPluginsTrustList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "plugins trust list", "print keys as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pluginsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	keys, err := client.ListPluginKeys(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list trusted keys: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, keys, func() {
		if len(keys) == 0 {
			_, _ = fmt.Fprintln(stdout, "no trusted keys, so no third-party plugin can be installed")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KEY ID\tNAME\tADDED")
		for _, k := range keys {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", k.KeyID, k.Name, k.AddedAt)
		}
		_ = tw.Flush()
	})
}

func runPluginsTrustRemove(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "plugins trust remove", "print the result as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pluginsUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "plugins trust remove", "key id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.RemovePluginKey(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("remove trusted key %q: %w", id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"key_id": id, "status": "removed"}, func() {
		_, _ = fmt.Fprintf(stdout, "key %s no longer trusted; plugins it signed stop at the next supervisor pass\n", id)
	})
}
