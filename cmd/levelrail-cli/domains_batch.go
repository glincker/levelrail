package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func domainsBatchUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains batch park (--app APP | --static-site SITE) [--env ENV] [--note TEXT] <domain>...
  %[1]s domains batch move (--from APP | --from-static-site SITE) (--to APP | --to-static-site SITE) [--from-env ENV] [--to-env ENV] [--confirm] <domain>...
  %[1]s domains batch attach (--app APP | --static-site SITE) [--env ENV] [--discard-settings] <domain>...
  %[1]s domains batch delete-parked <domain>...
  %[1]s domains batch <verb> --file items.json

Runs the same action on several domains in one request. It is not all or
nothing: each domain is checked and applied on its own, a failure does not
stop the others, and the result lists every domain. A move without --confirm
only reports which domains would change owner. --file reads a JSON object
with an "items" array (and optional "confirm" and "discard_settings"), the
same shape the API takes; the verb sets the action.
`, prog)
}

// runDomainsBatch handles "domains batch <verb> ...".
func runDomainsBatch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsBatchUsage(prog))
		return exitUsage
	}
	verb := args[0]
	action := strings.ReplaceAll(verb, "-", "_")
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsBatchUsage(prog))
		return exitOK
	case "park":
	case "move":
	case "attach":
	case "delete-parked":
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains batch action %q\n\n", prog, verb)
		_, _ = fmt.Fprint(stderr, domainsBatchUsage(prog))
		return exitUsage
	}
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "domains batch "+verb, "print the result as JSON", stderr)
	app := fs.String("app", "", "app that receives or loses the domains (park, attach)")
	site := fs.String("static-site", "", "static site instead of --app (park, attach)")
	env := fs.String("env", "", "environment domain set (default: the app's default list)")
	note := fs.String("note", "", "why the domains are parked")
	from := fs.String("from", "", "app that serves the domains now (move)")
	to := fs.String("to", "", "app that should serve them (move)")
	fromSite := fs.String("from-static-site", "", "static site that serves the domains now (move)")
	toSite := fs.String("to-static-site", "", "static site that should serve them (move)")
	fromEnv := fs.String("from-env", "", "environment set the domains leave (move)")
	toEnv := fs.String("to-env", "", "environment set the domains join (move)")
	confirm := fs.Bool("confirm", false, "apply moves after the collision check")
	discard := fs.Bool("discard-settings", false, "drop the settings kept by park instead of restoring them (attach)")
	file := fs.String("file", "", "read the request items from this JSON file")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, domainsBatchUsage(prog)+"\nFlags:\n"); fs.PrintDefaults() }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return code
	}
	req := apiclient.DomainBatchRequest{Action: action, Confirm: *confirm, DiscardSettings: *discard}
	if *file != "" {
		fileReq, err := readDomainBatchFile(*file)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
			return exitUsage
		}
		req.Items = fileReq.Items
		req.Confirm = req.Confirm || fileReq.Confirm
		req.DiscardSettings = req.DiscardSettings || fileReq.DiscardSettings
	} else {
		tpl := apiclient.DomainBatchItem{Environment: *env, Note: *note, FromEnvironment: *fromEnv, ToEnvironment: *toEnv, App: *app, From: *from, To: *to}
		if *site != "" {
			tpl.App, tpl.TargetKind = *site, "static_site"
		}
		if *fromSite != "" {
			tpl.From, tpl.FromKind = *fromSite, "static_site"
		}
		if *toSite != "" {
			tpl.To, tpl.ToKind = *toSite, "static_site"
		}
		if msg := batchFlagProblem(action, tpl, *app != "" && *site != "", (*from != "" && *fromSite != "") || (*to != "" && *toSite != "")); msg != "" {
			_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, msg)
			return exitUsage
		}
		for _, d := range fs.Args() {
			item := tpl
			item.Domain = d
			req.Items = append(req.Items, item)
		}
	}
	if len(req.Items) == 0 {
		_, _ = fmt.Fprintf(stderr, "%s: at least one domain (or --file) is required\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	res, err := client.DomainsBatch(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("domains batch %s: %w", verb, err))
	}
	code = writeScheduledTaskResult(stdout, stderr, of, res, func() { printDomainBatch(prog, stdout, res) })
	if code == exitOK && res.Summary.Failed > 0 {
		return exitAPIError
	}
	return code
}

func batchFlagProblem(action string, tpl apiclient.DomainBatchItem, appBoth, moveBoth bool) string {
	switch action {
	case "park", "attach":
		if appBoth {
			return "use either --app or --static-site"
		}
		if tpl.App == "" {
			return "--app or --static-site is required"
		}
	case "move":
		if moveBoth {
			return "use either --from or --from-static-site, and either --to or --to-static-site"
		}
		if tpl.From == "" || tpl.To == "" {
			return "a source (--from or --from-static-site) and a target (--to or --to-static-site) are required"
		}
	}
	return ""
}

func readDomainBatchFile(path string) (apiclient.DomainBatchRequest, error) {
	var req apiclient.DomainBatchRequest
	raw, err := os.ReadFile(path) //nolint:gosec // the operator names this file on purpose
	if err != nil {
		return req, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return req, fmt.Errorf("parse %s: %w", path, err)
	}
	return req, nil
}

func printDomainBatch(prog string, out io.Writer, res apiclient.DomainBatchResponse) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DOMAIN\tRESULT\tDETAIL")
	for _, r := range res.Results {
		state, detail := "ok", strings.Join(r.RestoredSettings, ", ")
		switch {
		case r.RequiresConfirmation:
			state, detail = "needs confirm", r.Message
		case !r.OK:
			state, detail = "failed", r.Error
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Domain, state, detail)
	}
	_ = tw.Flush()
	_, _ = fmt.Fprintf(out, "%d done, %d failed, %d need confirmation (batch %s)\n", res.Summary.Succeeded, res.Summary.Failed, res.Summary.RequiresConfirmation, res.BatchID)
	if res.Summary.RequiresConfirmation > 0 {
		_, _ = fmt.Fprintf(out, "rerun with --confirm to apply them: %s domains batch move ... --confirm\n", prog)
	}
}
