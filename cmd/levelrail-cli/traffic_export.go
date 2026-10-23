package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/trafficexport"
)

func trafficExportUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s traffic export sinks list [flags]
  %[1]s traffic export sinks add --kind posthog|otlp|webhook --name NAME [flags]
  %[1]s traffic export sinks set <id> [flags]
  %[1]s traffic export sinks rm <id> [flags]
  %[1]s traffic export sinks test <id> [flags]
  %[1]s traffic export sinks status [flags]

Sends one record per finished edge request to an analytics vendor. Records carry
no visitor identity by default. Client IP, user agent and visitor id are opt-in
per sink through --fields, and --ip-truncate (on by default) zeroes the last
octet of an IPv4 address or the last 80 bits of an IPv6 address. Cookies, auth
headers, query strings and request bodies are never exported.

Credentials are never passed as arguments. "add" prompts for them when stdin is a
terminal; "--secret-stdin" reads them from stdin, one per line, in this order:
  posthog          project token
  otlp, webhook    destination URL, then an optional Authorization header value
An empty line keeps the stored value on "set".

Run "%[1]s traffic export sinks <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runTrafficExport(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "sinks" {
		if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
			_, _ = fmt.Fprint(stdout, trafficExportUsage(prog))
			return exitOK
		}
		_, _ = fmt.Fprint(stderr, trafficExportUsage(prog))
		return exitUsage
	}
	args = args[1:]
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, trafficExportUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, trafficExportUsage(prog))
		return exitOK
	case "list":
		return runListCommand(prog, args[1:], stdout, stderr, lookupEnv, listCommandParams[apiclient.TrafficExportSinkList]{
			cmdLabel:  "traffic export sinks list",
			jsonUsage: "print the sinks as JSON to stdout and nothing else",
			usageText: fmt.Sprintf("Usage:\n  %s traffic export sinks list [flags]\n\nLists the export sinks. Credential values are never returned, only whether one is stored.\n\nFlags:\n", prog),
			fetch: func(c *Client, ctx context.Context) (apiclient.TrafficExportSinkList, error) {
				return c.ListTrafficExportSinks(ctx)
			},
			errVerb: "list traffic export sinks",
			print:   printTrafficExportSinks,
		})
	case "status":
		return runListCommand(prog, args[1:], stdout, stderr, lookupEnv, listCommandParams[apiclient.TrafficExportStatus]{
			cmdLabel:  "traffic export sinks status",
			jsonUsage: "print the status as JSON to stdout and nothing else",
			usageText: fmt.Sprintf("Usage:\n  %s traffic export sinks status [flags]\n\nShows last success, last error kind, queued and dropped counters per sink.\n\nFlags:\n", prog),
			fetch: func(c *Client, ctx context.Context) (apiclient.TrafficExportStatus, error) {
				return c.GetTrafficExportStatus(ctx)
			},
			errVerb: "get traffic export status",
			print:   printTrafficExportStatus,
		})
	case "add":
		return runTrafficExportWrite(prog, args[1:], stdin, stdout, stderr, lookupEnv, true)
	case "set":
		return runTrafficExportWrite(prog, args[1:], stdin, stdout, stderr, lookupEnv, false)
	case "rm":
		return runTrafficExportIDCommand(prog, "rm", args[1:], stdout, stderr, lookupEnv)
	case "test":
		return runTrafficExportIDCommand(prog, "test", args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown traffic export sinks subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, trafficExportUsage(prog))
		return exitUsage
	}
}

type trafficExportFlags struct {
	name, kind, host, fields string
	sample                   float64
	enabled, ipTruncate      bool
	secretStdin, promptSec   bool
}

func (f *trafficExportFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&f.name, "name", "", "display name for the sink")
	fs.StringVar(&f.kind, "kind", "", "posthog, otlp or webhook")
	fs.BoolVar(&f.enabled, "enabled", true, "deliver to this sink; --enabled=false keeps it configured but idle")
	fs.StringVar(&f.host, "host", "", "posthog only: us (default), eu or a self-hosted base URL")
	fs.StringVar(&f.fields, "fields", "", "comma-separated opt-in fields: "+strings.Join(trafficexport.OptionalFields(), ", ")+" (empty: no visitor identity)")
	fs.BoolVar(&f.ipTruncate, "ip-truncate", true, "zero the last octet of an IPv4 address or the last 80 bits of an IPv6 address when client_ip is exported")
	fs.Float64Var(&f.sample, "sample", 1, "fraction of traces exported, 0 to 1")
	fs.BoolVar(&f.secretStdin, "secret-stdin", false, "read the credentials from stdin, one per line")
	fs.BoolVar(&f.promptSec, "prompt-secret", false, "prompt for the credentials without echo")
}

func (f *trafficExportFlags) apply(fs *flag.FlagSet, s *apiclient.TrafficExportSink) {
	fs.Visit(func(fl *flag.Flag) {
		switch fl.Name {
		case "name":
			s.Name = f.name
		case "kind":
			s.Kind = f.kind
		case "enabled":
			v := f.enabled
			s.Enabled = &v
		case "host":
			s.Host = f.host
		case "fields":
			list := []string{}
			for _, p := range strings.Split(f.fields, ",") {
				if p = strings.TrimSpace(p); p != "" {
					list = append(list, p)
				}
			}
			s.Fields = list
		case "ip-truncate":
			v := f.ipTruncate
			s.IPTruncate = &v
		case "sample":
			v := f.sample
			s.Sample = &v
		}
	})
}

// promptTrafficSecret reads one credential without echo; a variable so tests can stub it.
var promptTrafficSecret = func(stderr io.Writer, label string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("--prompt-secret needs a terminal; use --secret-stdin to pipe the credentials")
	}
	_, _ = fmt.Fprintf(stderr, "%s (input hidden): ", label)
	b, err := term.ReadPassword(fd)
	_, _ = fmt.Fprintln(stderr)
	return string(b), err
}

func trafficSlotLabels(kind string) []string {
	if kind == trafficexport.SinkPostHog {
		return []string{"Project token"}
	}
	return []string{"Destination URL", "Authorization header value (optional)"}
}

func setTrafficSlot(s *apiclient.TrafficExportSink, idx int, v string) {
	if v == "" {
		return
	}
	switch {
	case s.Kind == trafficexport.SinkPostHog:
		s.Token = v
	case idx == 0:
		s.URL = v
	default:
		s.AuthHeader = v
	}
}

// readTrafficSecrets fills the credential fields from stdin lines or hidden prompts; it never reads argv.
func readTrafficSecrets(f *trafficExportFlags, promptByDefault bool, s *apiclient.TrafficExportSink, stdin io.Reader, stderr io.Writer) error {
	labels := trafficSlotLabels(s.Kind)
	switch {
	case f.secretStdin:
		r := bufio.NewReader(stdin)
		for i := range labels {
			line, err := r.ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return fmt.Errorf("read credential from stdin: %w", err)
			}
			setTrafficSlot(s, i, strings.TrimRight(line, "\r\n"))
			if err != nil {
				break
			}
		}
	case f.promptSec || (promptByDefault && stdinIsTerminal()):
		for i, l := range labels {
			v, err := promptTrafficSecret(stderr, l)
			if err != nil {
				return err
			}
			setTrafficSlot(s, i, v)
		}
	}
	return nil
}

func runTrafficExportWrite(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), isAdd bool) int {
	label := "traffic export sinks set"
	if isAdd {
		label = "traffic export sinks add"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the sink as JSON to stdout and nothing else", stderr)
	var f trafficExportFlags
	f.bind(fs)
	fs.Usage = func() {
		verb := "set <id>"
		if isAdd {
			verb = "add --kind KIND --name NAME"
		}
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s traffic export sinks %s [flags]\n\nOnly the flags you pass change; credentials are read from stdin or a prompt, never from arguments.\n\nFlags:\n", prog, verb)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	var id string
	var base apiclient.TrafficExportSink
	if isAdd {
		if len(fs.Args()) != 0 {
			_, _ = fmt.Fprintf(stderr, "%s: %s takes flags only\n", prog, label)
			return exitUsage
		}
	} else {
		id, ok = requireOneArg(fs, stderr, prog, label, "sink id")
		if !ok {
			return exitUsage
		}
		existing, err := client.GetTrafficExportSink(ctx, id)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get traffic export sink %q: %w", id, err))
		}
		base = existing
	}
	f.apply(fs, &base)
	if isAdd && (base.Kind == "" || base.Name == "") {
		_, _ = fmt.Fprintf(stderr, "%s: --kind and --name are required\n", prog)
		return exitValidation
	}
	if err := readTrafficSecrets(&f, isAdd, &base, stdin, stderr); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, err)
		return exitValidation
	}
	if trafficexport.HasIdentity(base.Fields) {
		_, _ = fmt.Fprintf(stderr, "%s: this sink exports visitor identity (%s) to a third party. Make sure your privacy notice covers it.\n", prog, strings.Join(base.Fields, ", "))
	}

	var saved apiclient.TrafficExportSink
	var err error
	if isAdd {
		saved, err = client.CreateTrafficExportSink(ctx, base)
	} else {
		saved, err = client.UpdateTrafficExportSink(ctx, id, base)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", label, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, saved, func() {
		printTrafficExportSinks(stdout, apiclient.TrafficExportSinkList{Sinks: []apiclient.TrafficExportSink{saved}})
	})
}

func runTrafficExportIDCommand(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "traffic export sinks " + verb
	jsonHelp := "print the result as JSON to stdout and nothing else"
	if verb == "rm" {
		jsonHelp = "print {\"deleted\": true} as JSON to stdout on success and nothing else"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, jsonHelp, stderr)
	fs.Usage = func() {
		desc := "Removes a sink and its stored credentials."
		if verb == "test" {
			desc = "Sends one synthetic record through that sink alone, enabled or not."
		}
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s <id> [flags]\n\n%s\n\nFlags:\n", prog, label, desc)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, label, "sink id")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	if verb == "rm" {
		if err := client.DeleteTrafficExportSink(ctx, id); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("remove traffic export sink %q: %w", id, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, map[string]bool{"deleted": true}, func() {
			_, _ = fmt.Fprintf(stdout, "traffic export sink %q removed\n", id)
		})
	}
	res, err := client.TestTrafficExportSink(ctx, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test traffic export sink %q: %w", id, err))
	}
	code := writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if res.OK {
			_, _ = fmt.Fprintf(stdout, "delivered one test record to %s in %.0f ms\n", id, res.DurationMS)
			return
		}
		_, _ = fmt.Fprintf(stdout, "delivery to %s failed: %s\n", id, res.ErrorKind)
	})
	if code == exitOK && !res.OK {
		return exitValidation
	}
	return code
}

func printTrafficExportSinks(out io.Writer, l apiclient.TrafficExportSinkList) {
	if len(l.Sinks) == 0 {
		_, _ = fmt.Fprintln(out, "no traffic export sinks configured")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tKIND\tENABLED\tTARGET\tFIELDS\tCREDENTIALS")
	for _, s := range l.Sinks {
		creds := "missing"
		if (s.Kind == trafficexport.SinkPostHog && s.TokenSet) || (s.Kind != trafficexport.SinkPostHog && s.URLSet) {
			creds = "set"
		}
		fields := "-"
		if len(s.Fields) > 0 {
			fields = strings.Join(s.Fields, ",")
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%v\t%s\t%s\t%s\n", s.ID, s.Name, s.Kind, s.Enabled == nil || *s.Enabled, orDash(s.TargetHost), fields, creds)
	}
	_ = tw.Flush()
}

func printTrafficExportStatus(out io.Writer, st apiclient.TrafficExportStatus) {
	if len(st.Sinks) == 0 {
		_, _ = fmt.Fprintln(out, "no traffic export sinks configured")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tKIND\tSTATE\tLAST OK\tLAST ERROR\tQUEUED\tSENT\tDROPPED\tFAILED")
	for _, s := range st.Sinks {
		state := "idle"
		switch {
		case !s.Enabled:
			state = "disabled"
		case s.Active:
			state = "active"
		case s.LastErrorKind != "":
			state = "invalid"
		}
		lastOK := "-"
		if !s.LastSuccess.IsZero() {
			lastOK = s.LastSuccess.UTC().Format("2006-01-02 15:04")
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\t%d\t%d\t%d\n", s.Name, s.Kind, state, lastOK, orDash(s.LastErrorKind), s.Queued, s.Sent, s.Dropped, s.Failed)
	}
	_ = tw.Flush()
}
