package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type envApplyOutput struct {
	App        string                    `json:"app"`
	DryRun     bool                      `json:"dry_run"`
	Changed    bool                      `json:"changed"`
	EnvVersion int                       `json:"env_version"`
	Plan       []apiclient.EnvPlanChange `json:"plan"`
}

// runEnvApply implements "env apply <app>": plain, secret and vault changes in one versioned transaction.
func runEnvApply(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "env apply", "print the plan as JSON to stdout and nothing else", stderr)
	var file string
	var sets, secretSets, unsets, secretDeletes, converts, vaultClears stringListFlag
	var expected int
	var dryRun, forceLocked bool
	fs.StringVar(&file, "f", "", "JSON plan file (the apply request body); flags below add to it")
	fs.Var(&sets, "set", "plain KEY=VALUE, repeatable")
	fs.Var(&unsets, "unset", "plain KEY to remove, repeatable")
	fs.Var(&secretSets, "secret-set", "secret KEY=VALUE, repeatable; the value is never printed back")
	fs.Var(&secretDeletes, "secret-delete", "secret KEY to delete, repeatable")
	fs.Var(&converts, "convert", "plain KEY to move into a secret, repeatable")
	fs.Var(&vaultClears, "vault-clear", "vault reference KEY to clear, repeatable")
	fs.IntVar(&expected, "expected-version", -1, "env version the plan was made against; a newer env exits with code 8 and changes nothing")
	fs.BoolVar(&dryRun, "dry-run", false, "print the plan and change nothing")
	fs.BoolVar(&forceLocked, "force-locked", false, "replace or delete locked secrets in the plan")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, envApplyUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: env apply requires exactly one app name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := fs.Arg(0)
	req, err := buildEnvApplyRequest(file, sets, secretSets)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	req.Unset = append(req.Unset, unsets...)
	req.SecretDelete = append(req.SecretDelete, secretDeletes...)
	req.Convert = append(req.Convert, converts...)
	req.VaultClear = append(req.VaultClear, vaultClears...)
	req.DryRun = req.DryRun || dryRun
	req.OverwriteLocked = req.OverwriteLocked || forceLocked
	if expected >= 0 {
		req.ExpectedVersion = &expected
	}

	if err := checkEnvValueSizes(req); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.ApplyAppEnv(context.Background(), name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, wrapEnvConflict(prog, name, fmt.Errorf("apply env for app %q: %w", name, readableNetError(err))))
	}
	out := envApplyOutput{App: name, DryRun: res.DryRun, Changed: res.Changed, EnvVersion: res.EnvVersion, Plan: res.Plan}
	if out.Plan == nil {
		out.Plan = []apiclient.EnvPlanChange{}
	}
	return writeScheduledTaskResult(stdout, stderr, of, out, func() { printEnvApplyHuman(stdout, out) })
}

// buildEnvApplyRequest merges the plan file with the KEY=VALUE flags; flags win per key.
func buildEnvApplyRequest(file string, sets, secretSets []string) (apiclient.EnvApplyRequest, error) {
	var req apiclient.EnvApplyRequest
	if file != "" {
		data, err := os.ReadFile(file) //nolint:gosec // operator-supplied plan path, same trust boundary as every --file flag
		if err != nil {
			return req, newValidationError("read plan file %q: %v", file, err)
		}
		if err := json.Unmarshal(data, &req); err != nil {
			return req, newValidationError("plan file %q is not valid JSON for env apply", file)
		}
	}
	plain, err := kvPairs(sets, "set")
	if err != nil {
		return req, newValidationError("%s", kvKeyOnlyError(err, sets))
	}
	secret, err := kvPairs(secretSets, "secret-set")
	if err != nil {
		return req, newValidationError("%s", kvKeyOnlyError(err, secretSets))
	}
	if len(plain) > 0 && req.Set == nil {
		req.Set = map[string]string{}
	}
	for k, v := range plain {
		req.Set[k] = v
	}
	if len(secret) > 0 && req.SecretSet == nil {
		req.SecretSet = map[string]string{}
	}
	for k, v := range secret {
		req.SecretSet[k] = v
	}
	return req, nil
}

// kvKeyOnlyError rewrites a KEY=VALUE parse error without the argument, which may carry a value.
func kvKeyOnlyError(err error, items []string) string {
	for _, it := range items {
		if k, _, ok := strings.Cut(it, "="); !ok || k == "" {
			return "a --set or --secret-set argument is not KEY=VALUE"
		}
	}
	return err.Error()
}

// printEnvApplyHuman lists key names and actions only, never a value.
func printEnvApplyHuman(w io.Writer, o envApplyOutput) {
	for _, c := range o.Plan {
		_, _ = fmt.Fprintf(w, "%s %s\n", c.Action, c.Key)
	}
	switch {
	case o.DryRun:
		_, _ = fmt.Fprintf(w, "dry run for app %q: %d change(s), nothing saved (env version %d)\n", o.App, len(o.Plan), o.EnvVersion)
	case o.Changed:
		_, _ = fmt.Fprintf(w, "applied %d change(s) to app %q (env version %d)\n", len(o.Plan), o.App, o.EnvVersion)
	default:
		_, _ = fmt.Fprintf(w, "nothing to change for app %q (env version %d)\n", o.App, o.EnvVersion)
	}
}

func envApplyUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s env apply <app> [-f plan.json] [--set K=V] [--unset K] [--secret-set K=V] [--secret-delete K]
                    [--convert K] [--vault-clear K] [--expected-version N] [--dry-run] [--force-locked] [flags]

Applies plain, secret and vault changes in one transaction: all of them land or none do.
Without --expected-version the plan applies to the current env. With it, a newer env
changes nothing and the command exits with code 8 (read the version with "%[1]s apps get <app> --json", field env_version).
Output lists key names and actions only, never a value. -f takes the API request body as JSON;
flags are added on top. Locked secrets need --force-locked.

Exit codes: 0 applied or dry run, 1 usage, 2 invalid plan, 3 network, 4 API error, 8 env changed since expected version.
`, prog)
}
