package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/migrationplan"
)

const envMigrateSourceToken = "APP_MIGRATE_SOURCE_TOKEN" //nolint:gosec // env var name, not a credential

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

// parseFixes reads "SOURCE:port=8080", "SOURCE:name=new" and "SOURCE:hold-domains".
func parseFixes(fixes []string) (map[string]apiclient.MigrationOverride, error) {
	out := map[string]apiclient.MigrationOverride{}
	for _, f := range fixes {
		id, rest, ok := strings.Cut(f, ":")
		if !ok || id == "" || rest == "" {
			return nil, fmt.Errorf("--fix %q must look like SOURCE:port=8080, SOURCE:name=NAME or SOURCE:hold-domains", f)
		}
		key, val, _ := strings.Cut(rest, "=")
		ov := out[id]
		switch key {
		case "port":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 || n > 65535 {
				return nil, fmt.Errorf("--fix %q: port must be 1 to 65535", f)
			}
			ov.Port = n
		case "name":
			if val == "" {
				return nil, fmt.Errorf("--fix %q: name is empty", f)
			}
			ov.Name = val
		case "hold-domains":
			ov.DropDomains = true
		default:
			return nil, fmt.Errorf("--fix %q: unknown fix %q", f, key)
		}
		out[id] = ov
	}
	return out, nil
}

// runMigratePlan implements "migrate plan": a read-only report, or with
// --apply the selected items (--only), previewed first with --dry-run.
func runMigratePlan(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	var req apiclient.MigrationSourceRequest
	var only, fixes listFlag
	var apply bool
	fs := newDataFlagSet(prog, "plan", stderr)
	f.register(fs)
	fs.StringVar(&req.Platform, "platform", "coolify", "source platform: coolify, dokploy or caprover")
	fs.StringVar(&req.URL, "url", "", "source platform base URL (required)")
	fs.StringVar(&req.Token, "token", "", "source platform API token, or "+envMigrateSourceToken)
	fs.BoolVar(&req.InsecureTLS, "insecure-tls", false, "skip TLS verification of the source")
	fs.Var(&only, "only", "import only this source id or name, repeatable (needs --apply)")
	fs.Var(&fixes, "fix", "SOURCE:port=N, SOURCE:name=NAME or SOURCE:hold-domains, repeatable")
	fs.BoolVar(&apply, "apply", false, "create the items chosen with --only (default is read-only)")
	fs.BoolVar(&req.DryRun, "dry-run", false, "with --apply, preview what would be created and create nothing")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if req.Token == "" {
		req.Token, _ = lookupEnv(envMigrateSourceToken)
	}
	if req.URL == "" || req.Token == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --url and a source token (--token or %s) are required\n", prog, envMigrateSourceToken)
		return exitUsage
	}
	var err error
	if req.Overrides, err = parseFixes(fixes); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitUsage
	}
	req.Only = only
	if apply && len(only) == 0 && !req.DryRun {
		_, _ = fmt.Fprintf(stderr, "%s: --apply needs --only SOURCE for each item to create\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	var rep apiclient.MigrationPlanReport
	if apply {
		rep, err = client.ImportMigration(context.Background(), req)
	} else {
		rep, err = client.PlanMigration(context.Background(), req)
	}
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if f.jsonOut {
		return printJSONOrFail(stdout, stderr, rep)
	}
	printMigrationPlan(stdout, rep)
	return exitOK
}

func printMigrationPlan(w io.Writer, rep apiclient.MigrationPlanReport) {
	_, _ = fmt.Fprintf(w, "Source: %s\n", rep.Platform)
	order := []migrationplan.Verdict{migrationplan.WillImport, migrationplan.ImportsWithChanges, migrationplan.NeedsManual, migrationplan.Unsupported, migrationplan.AlreadyImported}
	for _, v := range order {
		if rep.Counts[v] > 0 {
			_, _ = fmt.Fprintf(w, "  %-22s %d\n", v, rep.Counts[v])
		}
	}
	for _, it := range rep.Items {
		_, _ = fmt.Fprintf(w, "\n[%s] %s %s (%s)", it.Verdict, it.Kind, it.Name, it.SourceID)
		if it.Target != "" && it.Target != it.Name {
			_, _ = fmt.Fprintf(w, " -> %s", it.Target)
		}
		_, _ = fmt.Fprintf(w, "\n  env %d, volumes %d, domains %d, scheduled tasks %d\n", it.EnvCount, len(it.Volumes), len(it.Domains), len(it.Crons))
		for _, r := range it.Reasons {
			_, _ = fmt.Fprintf(w, "  - %s\n", r)
		}
		for _, m := range it.Manual {
			_, _ = fmt.Fprintf(w, "  manual: %s\n", m)
		}
		for _, fx := range it.Fixes {
			if fx.Kind == migrationplan.FixAuto {
				_, _ = fmt.Fprintf(w, "  fix: --fix %s:%s\n", it.SourceID, fixFlag(fx))
			}
		}
	}
	for _, n := range rep.Notes {
		_, _ = fmt.Fprintf(w, "\n%s\n", n)
	}
}

func fixFlag(fx migrationplan.Fix) string {
	switch fx.Code {
	case migrationplan.FixSetPort:
		return "port=PORT"
	case migrationplan.FixRename:
		return "name=NAME"
	case migrationplan.FixHoldDomains:
		return "hold-domains"
	}
	return fx.Code
}

// runMigrateStatus implements "migrate status [APP]".
func runMigrateStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	fs := newDataFlagSet(prog, "status", stderr)
	f.register(fs)
	positional, code, ok := parseInterspersed(fs, args)
	if !ok {
		return code
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	var list []apiclient.MigrationChecklist
	var err error
	if len(positional) == 1 {
		var one apiclient.MigrationChecklist
		one, err = client.GetMigrationChecklist(context.Background(), positional[0])
		list = []apiclient.MigrationChecklist{one}
	} else {
		list, err = client.ListMigrationChecklists(context.Background())
	}
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if f.jsonOut {
		return printJSONOrFail(stdout, stderr, list)
	}
	if len(list) == 0 {
		_, _ = fmt.Fprintln(stdout, "No imported app to track.")
		return exitOK
	}
	for _, c := range list {
		next := c.Current
		if next == "" {
			next = "finished"
		}
		_, _ = fmt.Fprintf(stdout, "%s: %d/%d, next %s\n", c.App, c.Progress, c.Total, next)
		for _, s := range c.Steps {
			_, _ = fmt.Fprintf(stdout, "  [%s] %s\n", map[bool]string{true: "x", false: " "}[s.Status == migrationplan.StatusDone], s.ID)
		}
	}
	return exitOK
}

// runMigrateStep implements "migrate step APP STEP".
func runMigrateStep(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var f dataFlags
	var req apiclient.MigrationStepRequest
	var reset bool
	var dbs, vols listFlag
	fs := newDataFlagSet(prog, "step", stderr)
	f.register(fs)
	fs.BoolVar(&reset, "reset", false, "reopen the step and every step after it")
	fs.StringVar(&req.Note, "note", "", "note stored with the step")
	fs.BoolVar(&req.Confirm, "confirm", false, "confirm a step the control plane cannot observe (old_stopped), or accept a failed DNS check")
	fs.Var(&dbs, "database", "data_verified: a migrated database to check, repeatable")
	fs.Var(&vols, "volume", "data_verified: NAME=SRC_FILES:SRC_BYTES:DST_FILES:DST_BYTES, repeatable")
	positional, code, ok := parseInterspersed(fs, args)
	if !ok {
		return code
	}
	if len(positional) != 2 {
		_, _ = fmt.Fprintf(stderr, "usage: %s migrate step APP STEP [--reset] [--database NAME] [--volume NAME=SF:SB:TF:TB] [--confirm]\nsteps: %s\n", prog, strings.Join(migrationplan.Steps, ", "))
		return exitUsage
	}
	req.Action = "complete"
	if reset {
		req.Action = "reset"
	}
	req.Databases = dbs
	for _, v := range vols {
		st, err := parseVolumeStat(v)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
			return exitUsage
		}
		req.Volumes = append(req.Volumes, st)
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	c, err := client.StepMigrationChecklist(context.Background(), positional[0], positional[1], req)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, err)
	}
	if f.jsonOut {
		return printJSONOrFail(stdout, stderr, c)
	}
	_, _ = fmt.Fprintf(stdout, "%s: %d/%d, next %s\n", c.App, c.Progress, c.Total, c.Current)
	return exitOK
}

func parseVolumeStat(s string) (apiclient.MigrationVolumeStat, error) {
	name, nums, ok := strings.Cut(s, "=")
	parts := strings.Split(nums, ":")
	if !ok || name == "" || len(parts) != 4 {
		return apiclient.MigrationVolumeStat{}, fmt.Errorf("--volume %q must look like NAME=SRC_FILES:SRC_BYTES:DST_FILES:DST_BYTES", s)
	}
	var n [4]int64
	for i, p := range parts {
		v, err := strconv.ParseInt(p, 10, 64)
		if err != nil || v < 0 {
			return apiclient.MigrationVolumeStat{}, fmt.Errorf("--volume %q: %q is not a count", s, p)
		}
		n[i] = v
	}
	return apiclient.MigrationVolumeStat{Name: name, SourceFiles: n[0], SourceBytes: n[1], TargetFiles: n[2], TargetBytes: n[3]}, nil
}
