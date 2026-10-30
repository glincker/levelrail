package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func marketplaceUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s plugins marketplace search [QUERY] [--category C] [flags]      list plugins from the signed marketplace indexes
  %[1]s plugins marketplace info ID [--source SRC] [flags]             show what installing would do: publisher key, abilities, digest
  %[1]s plugins marketplace install ID --approve-abilities LIST --yes [--trust-publisher] [--source SRC] [flags]
                                                                      trust, install, approve exactly LIST and enable in one step
  %[1]s plugins marketplace update (NAME | --all) --approve-abilities LIST --yes [flags]
                                                                      upgrade installed plugins that have a newer signed listing
  %[1]s plugins marketplace refresh [--source SRC] [flags]            refetch and re-verify the indexes
  %[1]s plugins marketplace sources list [flags]                      show index sources and their last status
  %[1]s plugins marketplace sources add --url URL --root-key B64 [--name NAME] [flags]
  %[1]s plugins marketplace sources remove ID [flags]

--approve-abilities must list exactly the abilities that info shows, never fewer or more.
Nothing is installed without --yes, and an untrusted publisher key needs --trust-publisher.
`, prog)
}

func runPluginsMarketplace(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, marketplaceUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, marketplaceUsage(prog))
		return exitOK
	case "search":
		return runMarketplaceSearch(prog, rest, stdout, stderr, lookupEnv)
	case "info":
		return runMarketplaceInfo(prog, rest, stdout, stderr, lookupEnv)
	case "install":
		return runMarketplaceInstall(prog, rest, stdout, stderr, lookupEnv)
	case "update":
		return runMarketplaceUpdate(prog, rest, stdout, stderr, lookupEnv)
	case "refresh":
		return runMarketplaceRefresh(prog, rest, stdout, stderr, lookupEnv)
	case "sources":
		return runMarketplaceSources(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown plugins marketplace subcommand %q\n\n%s", prog, args[0], marketplaceUsage(prog))
		return exitUsage
	}
}

type marketplaceRun struct {
	fs      *flag.FlagSet
	client  *apiclient.Client
	jsonOut bool
	of      outputFlags
}

func newMarketplaceRun(prog, label string, args []string, stderr io.Writer, lookupEnv func(string) (string, bool), bind func(fs *flag.FlagSet)) (*marketplaceRun, int, bool) {
	fs, tokenP, apiP, profP, jsonP, outP, qP := apiFlagSet(prog, "plugins marketplace "+label, "print the result as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, marketplaceUsage(prog)) }
	if bind != nil {
		bind(fs)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profP, jsonP, outP, qP}, prog, stderr)
	if !ok {
		return nil, exitCode, false
	}
	return &marketplaceRun{fs: fs, client: apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv), jsonOut: jsonOut, of: of}, exitOK, true
}

func (r *marketplaceRun) fail(stdout, stderr io.Writer, what string, err error) int {
	return reportError(stdout, stderr, r.jsonOut, fmt.Errorf("%s: %w", what, err))
}

func runMarketplaceSearch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var category *string
	r, code, ok := newMarketplaceRun(prog, "search", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		category = fs.String("category", "", "only plugins in this category")
	})
	if !ok {
		return code
	}
	cat, err := r.client.MarketplaceCatalog(context.Background())
	if err != nil {
		return r.fail(stdout, stderr, "marketplace search", err)
	}
	query := strings.ToLower(strings.Join(r.fs.Args(), " "))
	hits := []apiclient.MarketplaceEntry{}
	for _, e := range cat.Entries {
		hay := strings.ToLower(e.ID + " " + e.Name + " " + e.Summary + " " + e.Publisher)
		if (query == "" || strings.Contains(hay, query)) && (*category == "" || slices.Contains(e.Categories, *category)) {
			hits = append(hits, e)
		}
	}
	result := apiclient.MarketplaceCatalog{Entries: hits, Sources: cat.Sources}
	return writeScheduledTaskResult(stdout, stderr, r.of, result, func() {
		printMarketplaceSources(stderr, cat.Sources, true)
		if len(hits) == 0 {
			_, _ = fmt.Fprintln(stdout, "no plugins match")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tVERSION\tPUBLISHER\tABILITIES\tSTATE")
		for _, e := range hits {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", e.ID, e.Version, e.Publisher, strings.Join(e.Abilities, ","), marketplaceState(e))
		}
		_ = tw.Flush()
	})
}

func marketplaceState(e apiclient.MarketplaceEntry) string {
	switch {
	case !e.Compatible:
		return "needs newer control plane"
	case e.UpdateAvailable:
		return "update available (installed " + e.InstalledVersion + ")"
	case e.Installed:
		return "installed"
	default:
		return "available"
	}
}

func printMarketplaceSources(out io.Writer, sources []apiclient.MarketplaceSource, onlyProblems bool) {
	for _, s := range sources {
		if onlyProblems && s.Status == "ok" {
			continue
		}
		_, _ = fmt.Fprintf(out, "source %s (%s): %s %s\n", s.Name, s.ID, s.Status, s.Error)
	}
}

func findSourceFor(sourceFlag string, entries []apiclient.MarketplaceEntry, id string) (string, error) {
	var found []string
	for _, e := range entries {
		if e.ID == id && (sourceFlag == "" || e.SourceID == sourceFlag) {
			found = append(found, e.SourceID)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("plugin %q is not listed (try: plugins marketplace refresh)", id)
	case 1:
		return found[0], nil
	default:
		return "", fmt.Errorf("plugin %q is listed by several sources (%s), pick one with --source", id, strings.Join(found, ", "))
	}
}

func printMarketplacePreview(out io.Writer, pv apiclient.MarketplacePreview) {
	e := pv.Entry
	_, _ = fmt.Fprintf(out, "%s %s by %s (source %s)\n", e.ID, e.Version, e.Publisher, pv.SourceName)
	_, _ = fmt.Fprintf(out, "  action:     %s\n  publisher key: %s (%s), trusted: %v\n", pv.Action, pv.KeyID, pv.KeyName, pv.KeyTrusted)
	_, _ = fmt.Fprintf(out, "  digest:     %s\n  abilities:  %s\n  hosts:      %s (declared by the plugin, not restricted)\n", e.Digest, strings.Join(e.Abilities, ", "), joinOrNone(e.OutboundHosts))
	printNetworkNote(out, e.OutboundHosts)
	if pv.Action == "upgrade" {
		_, _ = fmt.Fprintf(out, "  upgrade from %s, new abilities: %s, new hosts (declared): %s\n", pv.InstalledVersion, joinOrNone(pv.AddedAbilities), joinOrNone(pv.AddedHosts))
	}
}

func parseAbilityList(s string) []string {
	out := []string{}
	for _, a := range strings.Split(s, ",") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	slices.Sort(out)
	return out
}

func runMarketplaceInfo(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var source *string
	r, code, ok := newMarketplaceRun(prog, "info", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		source = fs.String("source", "", "index source id when several list the same plugin")
	})
	if !ok {
		return code
	}
	id, ok := requireOneArg(r.fs, stderr, prog, "plugins marketplace info", "plugin id")
	if !ok {
		return exitUsage
	}
	ctx := context.Background()
	cat, err := r.client.MarketplaceCatalog(ctx)
	if err != nil {
		return r.fail(stdout, stderr, "marketplace info", err)
	}
	src, err := findSourceFor(*source, cat.Entries, id)
	if err != nil {
		return reportError(stdout, stderr, r.jsonOut, err)
	}
	pv, err := r.client.PreviewMarketplacePlugin(ctx, src, id)
	if err != nil {
		return r.fail(stdout, stderr, "marketplace info", err)
	}
	return writeScheduledTaskResult(stdout, stderr, r.of, pv, func() { printMarketplacePreview(stdout, pv) })
}

type installFlags struct {
	source, approve *string
	yes, trust      *bool
}

func bindInstallFlags(fs *flag.FlagSet, withSource bool) installFlags {
	f := installFlags{
		approve: fs.String("approve-abilities", "", "comma separated abilities, must equal the listed set exactly"),
		yes:     fs.Bool("yes", false, "proceed without asking, for scripts"),
		trust:   fs.Bool("trust-publisher", false, "also trust the publisher key (labelled added from marketplace)"),
		source:  new(string),
	}
	if withSource {
		f.source = fs.String("source", "", "index source id when several list the same plugin")
	}
	return f
}

// confirmInstall applies the same server-side contract the dashboard sheet uses: the caller restates the abilities it saw.
func confirmInstall(pv apiclient.MarketplacePreview, f installFlags) (apiclient.MarketplaceInstallInput, error) {
	in := apiclient.MarketplaceInstallInput{SourceID: pv.SourceID, ID: pv.Entry.ID, Version: pv.Entry.Version, Digest: pv.Entry.Digest, Abilities: parseAbilityList(*f.approve)}
	if !slices.Equal(in.Abilities, pv.Entry.Abilities) {
		return in, fmt.Errorf("--approve-abilities must be exactly %s, as listed above", strings.Join(pv.Entry.Abilities, ","))
	}
	if !*f.yes {
		return in, fmt.Errorf("nothing installed: re-run with --yes to confirm")
	}
	if !pv.KeyTrusted {
		if !*f.trust {
			return in, fmt.Errorf("the publisher key %s is not trusted: re-run with --trust-publisher to trust it", pv.KeyID)
		}
		in.TrustKeyID = pv.KeyID
	}
	return in, nil
}

func printMarketplaceResult(out io.Writer, res apiclient.MarketplaceInstallResult) {
	p := res.Plugin
	_, _ = fmt.Fprintf(out, "%s %s: %s, abilities approved: %s, state: %s\n", res.Action, p.Name, p.Version, strings.Join(p.ApprovedAbilities, ","), p.State)
	if res.TrustAdded {
		_, _ = fmt.Fprintln(out, "publisher key added to the trust list")
	}
}

func runMarketplaceInstall(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f installFlags
	r, code, ok := newMarketplaceRun(prog, "install", args, stderr, lookupEnv, func(fs *flag.FlagSet) { f = bindInstallFlags(fs, true) })
	if !ok {
		return code
	}
	id, ok := requireOneArg(r.fs, stderr, prog, "plugins marketplace install", "plugin id")
	if !ok {
		return exitUsage
	}
	ctx := context.Background()
	cat, err := r.client.MarketplaceCatalog(ctx)
	if err != nil {
		return r.fail(stdout, stderr, "marketplace install", err)
	}
	src, err := findSourceFor(*f.source, cat.Entries, id)
	if err != nil {
		return reportError(stdout, stderr, r.jsonOut, err)
	}
	pv, err := r.client.PreviewMarketplacePlugin(ctx, src, id)
	if err != nil {
		return r.fail(stdout, stderr, "marketplace install", err)
	}
	in, err := confirmInstall(pv, f)
	if err != nil {
		if !r.jsonOut {
			printMarketplacePreview(stderr, pv)
		}
		return reportError(stdout, stderr, r.jsonOut, err)
	}
	res, err := r.client.InstallMarketplacePlugin(ctx, in)
	if err != nil {
		return r.fail(stdout, stderr, "marketplace install", err)
	}
	return writeScheduledTaskResult(stdout, stderr, r.of, res, func() { printMarketplaceResult(stdout, res) })
}

func runMarketplaceUpdate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f installFlags
	var all *bool
	r, code, ok := newMarketplaceRun(prog, "update", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		f = bindInstallFlags(fs, false)
		all = fs.Bool("all", false, "update every installed plugin that has a newer listing")
	})
	if !ok {
		return code
	}
	names := r.fs.Args()
	if (*all && len(names) > 0) || (!*all && len(names) != 1) {
		_, _ = fmt.Fprintf(stderr, "%s: plugins marketplace update needs one plugin name or --all\n\n%s", prog, marketplaceUsage(prog))
		return exitUsage
	}
	ctx := context.Background()
	cat, err := r.client.MarketplaceCatalog(ctx)
	if err != nil {
		return r.fail(stdout, stderr, "marketplace update", err)
	}
	results := []apiclient.MarketplaceInstallResult{}
	for _, e := range cat.Entries {
		if !e.UpdateAvailable || (!*all && e.ID != names[0]) {
			continue
		}
		pv, perr := r.client.PreviewMarketplacePlugin(ctx, e.SourceID, e.ID)
		if perr != nil {
			return r.fail(stdout, stderr, "marketplace update "+e.ID, perr)
		}
		in, cerr := confirmInstall(pv, f)
		if cerr != nil {
			if !r.jsonOut {
				printMarketplacePreview(stderr, pv)
			}
			return reportError(stdout, stderr, r.jsonOut, fmt.Errorf("%s: %w", e.ID, cerr))
		}
		res, ierr := r.client.InstallMarketplacePlugin(ctx, in)
		if ierr != nil {
			return r.fail(stdout, stderr, "marketplace update "+e.ID, ierr)
		}
		results = append(results, res)
	}
	return writeScheduledTaskResult(stdout, stderr, r.of, results, func() {
		if len(results) == 0 {
			_, _ = fmt.Fprintln(stdout, "nothing to update")
		}
		for _, res := range results {
			printMarketplaceResult(stdout, res)
		}
	})
}

func runMarketplaceRefresh(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var source *string
	r, code, ok := newMarketplaceRun(prog, "refresh", args, stderr, lookupEnv, func(fs *flag.FlagSet) {
		source = fs.String("source", "", "refresh only this source id")
	})
	if !ok {
		return code
	}
	cat, err := r.client.RefreshMarketplace(context.Background(), *source)
	if err != nil {
		return r.fail(stdout, stderr, "marketplace refresh", err)
	}
	return writeScheduledTaskResult(stdout, stderr, r.of, cat, func() { printMarketplaceSourceTable(stdout, cat.Sources) })
}

func printMarketplaceSourceTable(out io.Writer, sources []apiclient.MarketplaceSource) {
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tPLUGINS\tSEQUENCE\tDETAIL")
	for _, s := range sources {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\n", s.ID, s.Name, s.Status, s.PluginCount, s.Sequence, s.Error)
	}
	_ = tw.Flush()
}

func runMarketplaceSources(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, marketplaceUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "list":
		r, code, ok := newMarketplaceRun(prog, "sources list", rest, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		cat, err := r.client.MarketplaceCatalog(context.Background())
		if err != nil {
			return r.fail(stdout, stderr, "marketplace sources", err)
		}
		return writeScheduledTaskResult(stdout, stderr, r.of, cat.Sources, func() { printMarketplaceSourceTable(stdout, cat.Sources) })
	case "add":
		var name, url, key *string
		r, code, ok := newMarketplaceRun(prog, "sources add", rest, stderr, lookupEnv, func(fs *flag.FlagSet) {
			name = fs.String("name", "", "a label for the source (defaults to its host)")
			url = fs.String("url", "", "https URL of the signed index (required)")
			key = fs.String("root-key", "", "base64 ed25519 public key that signs the index (required)")
		})
		if !ok {
			return code
		}
		if *url == "" || *key == "" {
			_, _ = fmt.Fprintf(stderr, "%s: plugins marketplace sources add needs --url and --root-key\n\n%s", prog, marketplaceUsage(prog))
			return exitUsage
		}
		s, err := r.client.AddMarketplaceSource(context.Background(), *name, *url, *key)
		if err != nil {
			return r.fail(stdout, stderr, "add marketplace source", err)
		}
		return writeScheduledTaskResult(stdout, stderr, r.of, s, func() {
			_, _ = fmt.Fprintf(stdout, "added source %s (%s), root key %s\n", s.Name, s.ID, s.RootKeyID)
		})
	case "remove":
		r, code, ok := newMarketplaceRun(prog, "sources remove", rest, stderr, lookupEnv, nil)
		if !ok {
			return code
		}
		id, ok := requireOneArg(r.fs, stderr, prog, "plugins marketplace sources remove", "source id")
		if !ok {
			return exitUsage
		}
		if err := r.client.RemoveMarketplaceSource(context.Background(), id); err != nil {
			return r.fail(stdout, stderr, "remove marketplace source", err)
		}
		return writeScheduledTaskResult(stdout, stderr, r.of, map[string]string{"id": id, "status": "removed"}, func() {
			_, _ = fmt.Fprintf(stdout, "source %s removed\n", id)
		})
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown plugins marketplace sources subcommand %q\n\n%s", prog, args[0], marketplaceUsage(prog))
		return exitUsage
	}
}
