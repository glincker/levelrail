package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func formsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s forms list <app>
  %[1]s forms create <app> <name> [--redirect-url URL] [--honeypot FIELD] [--notify CHANNEL_ID,...]
  %[1]s forms update <app> <name> [--redirect-url URL] [--honeypot FIELD] [--notify CHANNEL_ID,...]
  %[1]s forms snippet <app> <name>
  %[1]s forms fields get <app> <name>
  %[1]s forms fields set <app> <name> --file schema.json
  %[1]s forms test <app> <name>
  %[1]s forms delete <app> <name>
  %[1]s forms submissions <app> <name> [--search TEXT] [--spam true|false] [--limit N]
  %[1]s forms spam|unspam <app> <name> <submission-id>
  %[1]s forms remove <app> <name> <submission-id>
  %[1]s forms export <app> <name> [--search TEXT] [--spam true|false]

schema.json holds {"fields":[{"name","type","label","required","placeholder","min_length","max_length","options"}],
"success_message":"","allowed_origins":["https://example.com"]}. Types: text, email, textarea, select,
checkbox, number, url, hidden. With fields set, undeclared submitted names are dropped. No fields keeps
the default name, email and message.

A form is a public endpoint a static site posts to (form-urlencoded, multipart or JSON).
Submissions keep a salted hash of the sender address, never the address itself, and are
deleted after FORMS_RETENTION_DAYS. Deleting a form deletes its submissions.
`, prog)
}

type formsCall struct {
	client *Client
	args   []string
	jsonO  bool
	of     outputFlags
	// set tells update which optional flags were passed, so unset ones keep their value.
	set                    map[string]bool
	redirect, honeypot     string
	notify, search, spamIn string
	limit                  int
	file                   string
}

func parseFormsCall(prog, label string, args []string, stderr io.Writer, lookupEnv func(string) (string, bool), wantArgs int) (formsCall, int, bool) {
	fs, tokenP, apiP, profileP, jsonP, outP, queryP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	var c formsCall
	fs.StringVar(&c.redirect, "redirect-url", "", "send browser posts here after a submission")
	fs.StringVar(&c.honeypot, "honeypot", "", "name of the hidden spam-trap field")
	fs.StringVar(&c.notify, "notify", "", "comma-separated notification channel ids to alert on new submissions")
	fs.StringVar(&c.search, "search", "", "only submissions containing this text")
	fs.StringVar(&c.spamIn, "spam", "", "only spam (true) or only non-spam (false) submissions")
	fs.StringVar(&c.file, "file", "", "path to a schema JSON file (forms fields set)")
	fs.IntVar(&c.limit, "limit", 0, "maximum submissions to list")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, formsUsage(prog)) }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiP, profileP, jsonP, outP, queryP}, prog, stderr)
	if !ok {
		return formsCall{}, code, false
	}
	if len(fs.Args()) != wantArgs {
		_, _ = fmt.Fprintf(stderr, "%s: %s takes %d argument(s)\n\n%s", prog, label, wantArgs, formsUsage(prog))
		return formsCall{}, exitUsage, false
	}
	c.set = map[string]bool{}
	fs.Visit(func(f *flag.Flag) { c.set[f.Name] = true })
	c.client = apiClientFromFlags(prog, apiURL, token, profile, lookupEnv)
	c.args, c.jsonO, c.of = fs.Args(), jsonOut, of
	return c, exitOK, true
}

func (c formsCall) request() apiclient.FormRequest {
	var req apiclient.FormRequest
	if c.set["redirect-url"] {
		req.RedirectURL = &c.redirect
	}
	if c.set["honeypot"] {
		req.HoneypotField = &c.honeypot
	}
	if c.set["notify"] {
		req.NotifyChannelIDs = []string{}
		for _, id := range strings.Split(c.notify, ",") {
			if id = strings.TrimSpace(id); id != "" {
				req.NotifyChannelIDs = append(req.NotifyChannelIDs, id)
			}
		}
	}
	return req
}

func (c formsCall) filter() apiclient.FormSubmissionFilter {
	return apiclient.FormSubmissionFilter{Query: c.search, Spam: c.spamIn, Limit: c.limit}
}

// runForms implements "forms": per-app form capture endpoints and their inbox.
func runForms(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, formsUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, formsUsage(prog))
		return exitOK
	case "list":
		return runFormsList(prog, rest, stdout, stderr, lookupEnv)
	case "create", "update":
		return runFormsSave(prog, args[0], rest, stdout, stderr, lookupEnv)
	case "snippet":
		return runFormsSnippet(prog, rest, stdout, stderr, lookupEnv)
	case "fields":
		return runFormsFields(prog, rest, stdout, stderr, lookupEnv)
	case "test":
		return runFormsTest(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		return runFormsDelete(prog, rest, stdout, stderr, lookupEnv)
	case "submissions", "export":
		return runFormsSubmissions(prog, args[0], rest, stdout, stderr, lookupEnv)
	case "spam", "unspam", "remove":
		return runFormsSubmissionAction(prog, args[0], rest, stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown forms subcommand %q\n\n%s", prog, args[0], formsUsage(prog))
	return exitUsage
}

func runFormsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseFormsCall(prog, "forms list", args, stderr, lookupEnv, 1)
	if !ok {
		return code
	}
	res, err := c.client.ListForms(bg(), c.args[0])
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("list forms: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		if len(res.Forms) == 0 {
			_, _ = fmt.Fprintln(stdout, "no forms yet; create one with: forms create <app> <name>")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "NAME\tSUBMISSIONS\tSPAM\tENDPOINT")
		for _, f := range res.Forms {
			_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%s\n", f.Name, f.Submissions, f.SpamSubmissions, f.Endpoint)
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintf(stdout, "submissions are kept %d days\n", res.RetentionDays)
	})
}

func runFormsSave(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseFormsCall(prog, "forms "+verb, args, stderr, lookupEnv, 2)
	if !ok {
		return code
	}
	app, name := c.args[0], c.args[1]
	req := c.request()
	var f apiclient.Form
	var err error
	if verb == "create" {
		req.Name = name
		f, err = c.client.CreateForm(bg(), app, req)
	} else {
		f, err = c.client.UpdateForm(bg(), app, name, req)
	}
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("%s form: %w", verb, err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, f, func() {
		_, _ = fmt.Fprintf(stdout, "form %s saved\nendpoint: %s\n\n%s\n", f.Name, f.Endpoint, f.Snippet)
	})
}

func runFormsSnippet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseFormsCall(prog, "forms snippet", args, stderr, lookupEnv, 2)
	if !ok {
		return code
	}
	res, err := c.client.ListForms(bg(), c.args[0])
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("forms snippet: %w", err))
	}
	for _, f := range res.Forms {
		if f.Name == c.args[1] {
			return writeScheduledTaskResult(stdout, stderr, c.of, f, func() { _, _ = fmt.Fprintln(stdout, f.Snippet) })
		}
	}
	return reportError(stdout, stderr, c.jsonO, newValidationError("app %q has no form %q", c.args[0], c.args[1]))
}

func runFormsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseFormsCall(prog, "forms delete", args, stderr, lookupEnv, 2)
	if !ok {
		return code
	}
	if err := c.client.DeleteForm(bg(), c.args[0], c.args[1]); err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("delete form: %w", err))
	}
	_, _ = fmt.Fprintf(stdout, "form %s and its submissions deleted\n", c.args[1])
	return exitOK
}

func runFormsSubmissions(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseFormsCall(prog, "forms "+verb, args, stderr, lookupEnv, 2)
	if !ok {
		return code
	}
	app, name := c.args[0], c.args[1]
	if verb == "export" {
		data, err := c.client.ExportFormSubmissions(bg(), app, name, c.filter())
		if err != nil {
			return reportError(stdout, stderr, c.jsonO, fmt.Errorf("export submissions: %w", err))
		}
		_, _ = stdout.Write(data)
		return exitOK
	}
	page, err := c.client.ListFormSubmissions(bg(), app, name, c.filter())
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("list submissions: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, page, func() {
		for _, s := range page.Submissions {
			tag := ""
			if s.Spam {
				tag = " [spam]"
			}
			_, _ = fmt.Fprintf(stdout, "#%d %s%s\n", s.ID, s.CreatedAt.Format("2006-01-02 15:04"), tag)
			for _, k := range sortedFieldKeys(s.Fields) {
				_, _ = fmt.Fprintf(stdout, "  %s: %s\n", k, s.Fields[k])
			}
		}
		if page.NextBefore != 0 {
			_, _ = fmt.Fprintf(stdout, "more available; raise --limit\n")
		}
	})
}

func runFormsSubmissionAction(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseFormsCall(prog, "forms "+verb, args, stderr, lookupEnv, 3)
	if !ok {
		return code
	}
	id, err := strconv.ParseInt(c.args[2], 10, 64)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, newValidationError("submission id must be a number"))
	}
	app, name := c.args[0], c.args[1]
	switch verb {
	case "spam":
		err = c.client.MarkFormSubmissionSpam(bg(), app, name, id, true)
	case "unspam":
		err = c.client.MarkFormSubmissionSpam(bg(), app, name, id, false)
	default:
		err = c.client.DeleteFormSubmission(bg(), app, name, id)
	}
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("forms %s: %w", verb, err))
	}
	_, _ = fmt.Fprintf(stdout, "submission %d: %s done\n", id, verb)
	return exitOK
}

func sortedFieldKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func runFormsFields(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "get" && args[0] != "set") {
		_, _ = fmt.Fprint(stderr, formsUsage(prog))
		return exitUsage
	}
	verb := args[0]
	c, code, ok := parseFormsCall(prog, "forms fields "+verb, args[1:], stderr, lookupEnv, 2)
	if !ok {
		return code
	}
	app, name := c.args[0], c.args[1]
	if verb == "get" {
		raw, err := c.client.GetFormFields(bg(), app, name)
		if err != nil {
			return reportError(stdout, stderr, c.jsonO, fmt.Errorf("get form fields: %w", err))
		}
		var pretty bytes.Buffer
		if json.Indent(&pretty, raw, "", "  ") != nil {
			pretty.Write(raw)
		}
		_, _ = fmt.Fprintln(stdout, pretty.String())
		return exitOK
	}
	if c.file == "" {
		return reportError(stdout, stderr, c.jsonO, newValidationError("forms fields set needs --file schema.json"))
	}
	data, err := os.ReadFile(c.file)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("read schema file: %w", err))
	}
	if !json.Valid(data) {
		return reportError(stdout, stderr, c.jsonO, newValidationError("%s is not valid JSON", c.file))
	}
	f, err := c.client.SetFormFields(bg(), app, name, data)
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("set form fields: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, f, func() {
		_, _ = fmt.Fprintf(stdout, "form %s fields saved\n\n%s\n", f.Name, f.Snippet)
	})
}

func runFormsTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseFormsCall(prog, "forms test", args, stderr, lookupEnv, 2)
	if !ok {
		return code
	}
	res, err := c.client.SendFormTest(bg(), c.args[0], c.args[1])
	if err != nil {
		return reportError(stdout, stderr, c.jsonO, fmt.Errorf("forms test: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, c.of, res, func() {
		_, _ = fmt.Fprintf(stdout, "test submission %d stored (marked as a test)\n", res.SubmissionID)
	})
}
