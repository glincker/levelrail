package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const envMigrateSourcePassword = "APP_MIGRATE_SOURCE_DB_PASSWORD" //nolint:gosec // env var name, not a credential

const copyPollInterval = 3 * time.Second

type dataFlags struct {
	apiURL, profile string
	jsonOut         bool
}

func (f *dataFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&f.apiURL, "api-url", "", "control plane API base URL")
	fs.StringVar(&f.profile, "profile", "", "named credentials profile for the control plane")
	fs.BoolVar(&f.jsonOut, "json", false, "print the result as JSON")
}

func newDataFlagSet(prog, name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(prog+" migrate "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func printJSONOrFail(stdout, stderr io.Writer, v any) int {
	if err := writeJSONValue(stdout, v); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitNetwork
	}
	return exitOK
}

// runMigrateDbCopy implements "migrate db-copy NAME".
func runMigrateDbCopy(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	var src apiclient.DataCopySource
	var passwordStdin, wait bool
	fs := newDataFlagSet(prog, "db-copy", stderr)
	f.register(fs)
	fs.StringVar(&src.Host, "host", "", "source database host (required)")
	fs.IntVar(&src.Port, "port", 0, "source database port (default per engine)")
	fs.StringVar(&src.User, "user", "", "source database user")
	fs.StringVar(&src.Database, "database", "", "source database name (required for postgres, mysql, mariadb)")
	fs.StringVar(&src.AuthDatabase, "auth-database", "", "MongoDB authentication database (default admin)")
	fs.BoolVar(&src.TLS, "tls", false, "require TLS to the source")
	fs.BoolVar(&passwordStdin, "password-stdin", false, "read the source password from the first line of stdin")
	fs.BoolVar(&wait, "wait", true, "wait for the copy and its verification to finish")
	positional, code, ok := parseInterspersed(fs, args)
	if !ok {
		return code
	}
	if len(positional) != 1 || src.Host == "" {
		_, _ = fmt.Fprintf(stderr, "usage: %s migrate db-copy NAME --host HOST --user USER --database DB [--password-stdin]\n", prog)
		return exitUsage
	}
	name := positional[0]
	switch v, has := lookupEnv(envMigrateSourcePassword); {
	case passwordStdin:
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		src.Password = strings.TrimRight(line, "\r\n")
	case has:
		src.Password = v
	}

	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	st, err := client.StartDatabaseDataCopy(ctx, name, src)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("copy data into %s: %w", name, err))
	}
	if wait {
		st, err = waitForCopy(ctx, client, name, st, stderr, f.jsonOut)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, err)
		}
	}
	if f.jsonOut {
		if c := printJSONOrFail(stdout, stderr, st); c != exitOK {
			return c
		}
	} else {
		printDataCopy(stdout, []apiclient.DataCopyStatus{st}, true)
	}
	if st.Status == "failed" {
		return exitCheckFailed
	}
	return exitOK
}

func waitForCopy(ctx context.Context, client *Client, name string, st apiclient.DataCopyStatus, stderr io.Writer, quiet bool) (apiclient.DataCopyStatus, error) {
	if !quiet {
		_, _ = fmt.Fprintf(stderr, "Copying data into %s, this can take a while. Ctrl-C stops waiting, the copy keeps running.\n", name)
	}
	for st.Status == "copying" {
		select {
		case <-ctx.Done():
			return st, fmt.Errorf("stopped waiting, check progress with: migrate db-status %s", name)
		case <-time.After(copyPollInterval):
		}
		next, err := client.GetDatabaseDataCopy(ctx, name)
		if err != nil {
			return st, fmt.Errorf("read copy status of %s: %w", name, err)
		}
		st = next
	}
	return st, nil
}

// runMigrateDbStatus implements "migrate db-status [NAME]".
func runMigrateDbStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	fs := newDataFlagSet(prog, "db-status", stderr)
	f.register(fs)
	positional, code, ok := parseInterspersed(fs, args)
	if !ok {
		return code
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	var rows []apiclient.DataCopyStatus
	if len(positional) == 1 {
		st, err := client.GetDatabaseDataCopy(context.Background(), positional[0])
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, err)
		}
		rows = []apiclient.DataCopyStatus{st}
	} else {
		var err error
		if rows, err = client.ListDatabaseDataCopies(context.Background()); err != nil {
			return reportError(stdout, stderr, f.jsonOut, err)
		}
	}
	if f.jsonOut {
		return printJSONOrFail(stdout, stderr, rows)
	}
	printDataCopy(stdout, rows, len(positional) == 1)
	return exitOK
}

func printDataCopy(w io.Writer, rows []apiclient.DataCopyStatus, detail bool) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DATABASE\tENGINE\tSTATUS\tTABLES\tMISMATCHED")
	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\n", r.Database, r.Engine, r.Status, r.Checked, r.Mismatched)
	}
	_ = tw.Flush()
	if !detail {
		return
	}
	for _, r := range rows {
		if r.Reason != "" {
			_, _ = fmt.Fprintf(w, "\n%s: %s\n", r.Database, r.Reason)
		}
		for _, t := range r.Tables {
			mark := "ok"
			if !t.OK {
				mark = "DIFFERS"
			}
			_, _ = fmt.Fprintf(w, "  %-40s source=%d target=%d %s\n", t.Name, t.Source, t.Target, mark)
		}
		if r.NextAction != "" {
			_, _ = fmt.Fprintf(w, "  next: %s\n", r.NextAction)
		}
	}
}

// runMigrateVolumes implements "migrate volumes --source user@host".
func runMigrateVolumes(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	var source, app string
	fs := newDataFlagSet(prog, "volumes", stderr)
	f.register(fs)
	fs.StringVar(&source, "source", "", "SSH login of the source host, user@host (required)")
	fs.StringVar(&app, "app", "", "only this imported app")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if source == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --source user@host is required\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	res, err := client.MigrationVolumeGuide(context.Background(), source, app)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if f.jsonOut {
		return printJSONOrFail(stdout, stderr, res)
	}
	if len(res.Guides) == 0 {
		_, _ = fmt.Fprintln(stdout, "No imported app has a persistent volume or bind mount.")
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "%s\n\n", res.Warning)
	for _, g := range res.Guides {
		_, _ = fmt.Fprintf(stdout, "# %s: %s -> %s (%s)\n%s\n\n", g.App, g.Source, g.ContainerPath, g.Kind, g.Command)
	}
	return exitOK
}

// runMigrateCutover implements "migrate cutover [--verify]".
func runMigrateCutover(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	var app, targetIP string
	var verify bool
	fs := newDataFlagSet(prog, "cutover", stderr)
	f.register(fs)
	fs.StringVar(&app, "app", "", "only this imported app")
	fs.StringVar(&targetIP, "target-ip", "", "this node's public IP, detected when empty")
	fs.BoolVar(&verify, "verify", false, "post-switch check: resolves here, certificate issued, health path answers 200")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	rep, err := client.CutoverReport(context.Background(), app, targetIP, verify)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if f.jsonOut {
		if c := printJSONOrFail(stdout, stderr, rep); c != exitOK {
			return c
		}
	} else {
		printCutover(stdout, rep)
	}
	if rep.Verdict != "go" && rep.Verdict != "switched" {
		return exitCheckFailed
	}
	return exitOK
}

func printCutover(w io.Writer, rep apiclient.CutoverReport) {
	_, _ = fmt.Fprintf(w, "Cutover check (%s), this node: %s\nOverall: %s\n", rep.Phase, strings.Join(rep.TargetIPs, ", "), strings.ToUpper(rep.Verdict))
	if len(rep.Domains) == 0 {
		_, _ = fmt.Fprintln(w, "\nNo imported app has a domain to check.")
	}
	for _, d := range rep.Domains {
		_, _ = fmt.Fprintf(w, "\n%s (%s): %s\n", d.Domain, d.App, strings.ToUpper(d.Verdict))
		for _, c := range d.Checks {
			_, _ = fmt.Fprintf(w, "  [%s] %s: %s\n", c.Status, c.ID, c.Detail)
			if c.Fix != "" {
				_, _ = fmt.Fprintf(w, "         next: %s\n", c.Fix)
			}
		}
		if d.Change != nil && (d.Verdict == "go" || d.Verdict == "wait") {
			_, _ = fmt.Fprintf(w, "  change: set %s %s -> %s (TTL %d)", d.Change.Type, d.Change.Name, d.Change.Value, d.Change.TTL)
			if d.Change.Replaces != "" {
				_, _ = fmt.Fprintf(w, ", replacing %s", d.Change.Replaces)
			}
			_, _ = fmt.Fprintln(w)
		}
	}
	if len(rep.Guidance) > 0 {
		_, _ = fmt.Fprintln(w)
		for _, g := range rep.Guidance {
			_, _ = fmt.Fprintf(w, "- %s\n", g)
		}
	}
}

func migrateDataUsage(prog string) string {
	return fmt.Sprintf(`  %[1]s migrate db-copy NAME --host H --user U --database D   copy live data into an imported database and verify row counts
  %[1]s migrate db-status [NAME]                              per-database copy status
  %[1]s migrate volumes --source user@host                    print the commands that copy imported volumes
  %[1]s migrate cutover [--verify]                            go or no-go for the DNS switch, then verify after it

The source database password is read from %[2]s or --password-stdin.
`, prog, envMigrateSourcePassword)
}

// parseInterspersed parses flags that may follow the positional arguments.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, int, bool) {
	var positional []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, exitUsage, false
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
	return positional, exitOK, true
}
