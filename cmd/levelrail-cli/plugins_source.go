package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type pluginSourceFlags struct {
	manifest, url, signature, keyID, digest, config, approve *string
}

func addPluginSourceFlags(fs *flag.FlagSet, withApply bool) pluginSourceFlags {
	f := pluginSourceFlags{
		manifest:  fs.String("manifest", "", "path to a local plugin.yaml"),
		url:       fs.String("url", "", "https URL of a plugin.yaml (a <url>.sig file next to it is used as the signature)"),
		signature: fs.String("signature", "", "path to the detached base64 signature (optional when the publisher key is trusted and signed it, required unless unsigned plugins are allowed)"),
		keyID:     fs.String("key-id", "", "fingerprint of the trusted key that signed it (found automatically when omitted)"),
		digest:    fs.String("digest", "", "refuse unless the manifest image is pinned to this sha256 digest"),
	}
	if withApply {
		f.config = fs.String("config", "", "path to a JSON object with the plugin's own non-secret config")
		f.approve = fs.String("approve", "", "comma separated abilities to approve now, must equal the requested set exactly")
	}
	return f
}

func (f pluginSourceFlags) input() (apiclient.InstallPluginInput, error) {
	in := apiclient.InstallPluginInput{KeyID: *f.keyID, ExpectedDigest: *f.digest}
	switch {
	case *f.manifest != "" && *f.url != "":
		return in, fmt.Errorf("give --manifest or --url, not both")
	case *f.url != "":
		in.ManifestURL = *f.url
	case *f.manifest != "":
		m, err := readManifest(*f.manifest)
		if err != nil {
			return in, err
		}
		in.Manifest = m
	default:
		return in, fmt.Errorf("a source is required: --manifest FILE or --url URL")
	}
	if *f.signature != "" {
		sig, err := readTrimmed(*f.signature)
		if err != nil {
			return in, fmt.Errorf("read signature: %w", err)
		}
		in.Signature = sig
	}
	if f.config != nil && *f.config != "" {
		cfg, err := readTrimmed(*f.config)
		if err != nil {
			return in, fmt.Errorf("read config: %w", err)
		}
		in.ConfigJSON = cfg
	}
	if f.approve != nil && strings.TrimSpace(*f.approve) != "" {
		in.ApproveAbilities = strings.Split(*f.approve, ",")
	}
	return in, nil
}

func readManifest(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // operator-supplied path to their own manifest; read verbatim because the signature covers the exact bytes
	if err != nil {
		return "", fmt.Errorf("read manifest: %w", err)
	}
	return string(b), nil
}

func runPluginsInspect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "plugins inspect", "print the preview as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pluginsUsage(prog)) }
	src := addPluginSourceFlags(fs, false)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return exitCode
	}
	in, err := src.input()
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	pv, err := client.InspectPlugin(context.Background(), in)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("inspect plugin: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, pv, func() { printPluginPreview(stdout, pv) })
}

func printPluginPreview(out io.Writer, pv apiclient.PluginPreview) {
	m, v := pv.Manifest, pv.Verification
	_, _ = fmt.Fprintf(out, "Plugin:        %s %s (%s)\nPublisher:     %s\nImage:         %s\n", m.Name, m.Version, m.Kind, m.Publisher, m.Image)
	signer := "none"
	if v.Signed {
		signer = v.KeyID
		if v.KeyName != "" {
			signer = v.KeyName + " (" + v.KeyID + ")"
		}
	}
	_, _ = fmt.Fprintf(out, "Verification:  %s, digest pinned: %t, signed by: %s\n", v.Status, v.DigestPinned, signer)
	if !v.Installable {
		_, _ = fmt.Fprintf(out, "Refused:       %s\n", v.Reason)
	}
	_, _ = fmt.Fprintf(out, "Abilities:     %s\nNetwork:       %s\n", joinOrNone(m.Abilities), outboundSummary(m.OutboundHosts))
	printNetworkNote(out, m.OutboundHosts)
	if pv.Action == "upgrade" {
		_, _ = fmt.Fprintf(out, "Upgrade from:  %s (approved: %s)\nAdded:         %s abilities, %s hosts\nRemoved:       %s abilities, %s hosts\n",
			pv.InstalledVersion, joinOrNone(pv.ApprovedAbilities), joinOrNone(pv.AddedAbilities), joinOrNone(pv.AddedHosts), joinOrNone(pv.RemovedAbilities), joinOrNone(pv.RemovedHosts))
		if pv.ReapprovalRequired {
			_, _ = fmt.Fprintln(out, "Re-approval:   required, the plugin stays stopped until you approve the wider set")
		}
	}
}

func runPluginsInstall(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runPluginsApply(prog, "install", args, stdout, stderr, lookupEnv)
}

func runPluginsUpgrade(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runPluginsApply(prog, "upgrade", args, stdout, stderr, lookupEnv)
}

func runPluginsApply(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "plugins "+verb, "print the plugin as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, pluginsUsage(prog)) }
	src := addPluginSourceFlags(fs, true)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return exitCode
	}
	in, err := src.input()
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var p apiclient.Plugin
	if verb == "upgrade" {
		pv, ierr := client.InspectPlugin(ctx, in)
		if ierr != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrade plugin: %w", ierr))
		}
		p, err = client.UpgradePlugin(ctx, pv.Manifest.Name, in)
	} else {
		p, err = client.InstallPlugin(ctx, in)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s plugin: %w", verb, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, p, func() {
		printPlugin(stdout, p)
		if p.ApprovedAt != "" {
			_, _ = fmt.Fprintf(stdout, "\nAbilities approved. To start it:\n  %s plugins enable %s\n", prog, p.Name)
			return
		}
		_, _ = fmt.Fprintf(stdout, "\nInstalled but not approved or running. Review the abilities above, then:\n  %s plugins approve %s --abilities %s\n  %s plugins enable %s\n",
			prog, p.Name, strings.Join(p.RequestedAbilities, ","), prog, p.Name)
	})
}
