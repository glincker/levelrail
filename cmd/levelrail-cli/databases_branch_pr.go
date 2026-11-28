package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/glincker/levelrail/internal/apiclient"
)

const prBranchNameMax = 63

var prDBNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

var prTTLPattern = regexp.MustCompile(`^(never|[1-9][0-9]{0,3}[dh])$`)

// prBranchName is the branch database name for a pull request. Database names are global per
// server, so it includes the database; long names keep a hash of the full name to stay unique.
func prBranchName(db string, pr int) (string, error) {
	if pr < 1 {
		return "", newValidationError("--pr must be a positive pull request number")
	}
	if !prDBNamePattern.MatchString(db) {
		return "", newValidationError("database name %q is not a valid name", db)
	}
	suffix := "-pr-" + strconv.Itoa(pr)
	if len(db)+len(suffix) <= prBranchNameMax {
		return db + suffix, nil
	}
	sum := sha256.Sum256([]byte(db))
	tag := "-" + hex.EncodeToString(sum[:])[:6]
	keep := prBranchNameMax - len(suffix) - len(tag)
	if keep < 1 {
		return "", newValidationError("--pr is too large for a branch name")
	}
	return db[:keep] + tag + suffix, nil
}

var prNumberPattern = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)

// parsePRNumber reads --pr as plain decimal, so 010 or 0x10 are refused instead of reinterpreted.
func parsePRNumber(s string) (int, error) {
	if !prNumberPattern.MatchString(s) {
		return 0, newValidationError("--pr must be a pull request number such as 42")
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, newValidationError("--pr must be a pull request number such as 42")
	}
	return n, nil
}

// isBranchNotFound is true only for the "branch not found" answer, so a missing parent database is never mistaken for it.
func isBranchNotFound(err error) bool {
	var apiErr *apiclient.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound && apiErr.Message == "branch not found"
}

// prExistingVerdict decides what to do with a branch that already has the pull request name.
func prExistingVerdict(n apiclient.DatabaseBranchNode, db, mode string) (reuse, recreate bool, err error) {
	if n.Parent != db {
		return false, false, fmt.Errorf("branch %q belongs to %q, not %q", n.Name, n.Parent, db)
	}
	switch n.State {
	case "failed":
		return false, true, nil
	case "promoted", "deleting", "expiring", "deleted":
		return false, false, fmt.Errorf("branch %q is %s and cannot be reused; delete it or use another pull request number", n.Name, n.State)
	}
	if n.Mode != mode {
		return false, false, fmt.Errorf("branch %q already exists with mode %q, not %q; run pr-close first", n.Name, n.Mode, mode)
	}
	return true, false, nil
}

func validatePRTTL(ttl string) error {
	if !prTTLPattern.MatchString(ttl) {
		return newValidationError("--ttl must be never or a number of days or hours like 7d or 12h")
	}
	return nil
}

// validatePRMode maps the public mode names to the API; anonymized branches are not creatable yet.
func validatePRMode(mode string) (string, error) {
	switch mode {
	case "schema":
		return "schema", nil
	case "full", "data", "":
		return "full", nil
	case "anonymized":
		return "", newValidationError("--mode anonymized is not available for pull request branches yet, use schema or full")
	}
	return "", newValidationError("--mode must be schema or full")
}

// prBranchResult is what "databases branch pr" prints. It never holds a password.
type prBranchResult struct {
	Branch       string `json:"branch"`
	Database     string `json:"database"`
	Mode         string `json:"mode"`
	State        string `json:"state"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	Created      bool   `json:"created"`
	Host         string `json:"host,omitempty"`
	Port         int    `json:"port,omitempty"`
	DatabaseName string `json:"database_name"`
	Credentials  string `json:"credentials"`
	URL          string `json:"url,omitempty"`
}

func runDatabasesBranchPR(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	const cmd = "databases branch pr"
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cmd, "print the branch as JSON to stdout and nothing else", stderr)
	var prArg, mode, ttl string
	var protect, printURL bool
	var timeout time.Duration
	fs.StringVar(&prArg, "pr", "", "pull request number")
	fs.StringVar(&mode, "mode", "schema", "schema or full")
	fs.StringVar(&ttl, "ttl", "7d", "auto delete after this time, for example 7d, 12h or never")
	fs.BoolVar(&protect, "protect", false, "protect the branch from delete and expiry")
	fs.BoolVar(&printURL, "print-url", false, "print the connection URL with the real password (owner only, step-up, audited)")
	fs.DurationVar(&timeout, "timeout", 5*time.Minute, "how long to wait for the branch to become ready")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesBranchUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: %s needs a database name\n\n", prog, cmd)
		fs.Usage()
		return exitUsage
	}
	pr, err := parsePRNumber(prArg)
	var name string
	if err == nil {
		name, err = prBranchName(rest[0], pr)
	}
	if err == nil {
		err = validatePRTTL(ttl)
	}
	var apiMode string
	if err == nil {
		apiMode, err = validatePRMode(mode)
	}
	if err == nil && timeout <= 0 {
		err = newValidationError("--timeout must be positive")
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	db := rest[0]
	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	fail := func(err error) int { return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", cmd, err)) }

	create := func() (apiclient.DatabaseBranchNode, error) {
		n, cerr := client.CreateLiveDatabaseBranch(ctx, db, apiclient.CreateLiveDatabaseBranchRequest{Name: name, Mode: apiMode, TTL: ttl, Protected: protect})
		if cerr != nil {
			return n, fmt.Errorf("create branch %q of %q: %w", name, db, cerr)
		}
		return n, nil
	}
	node, err := client.GetDatabaseBranch(ctx, db, name)
	created := false
	switch {
	case err == nil:
		reuse, recreate, verr := prExistingVerdict(node, db, apiMode)
		if verr != nil {
			return fail(verr)
		}
		if recreate {
			if derr := client.DeleteDatabaseBranch(ctx, db, name, name); derr != nil && !isBranchNotFound(derr) {
				return fail(fmt.Errorf("delete failed branch %q: %w", name, derr))
			}
		}
		if !reuse {
			if node, err = create(); err != nil {
				return fail(err)
			}
			created = true
		}
	case isBranchNotFound(err):
		if node, err = create(); err != nil {
			return fail(err)
		}
		created = true
	default:
		return fail(err)
	}
	node, err = waitBranchReady(ctx, client, db, name, node, timeout, 2*time.Second)
	if err != nil {
		return fail(err)
	}

	res := prBranchResult{Branch: node.Name, Database: db, Mode: node.Mode, State: node.State, ExpiresAt: node.ExpiresAt, Created: created, DatabaseName: node.Name}
	res.Credentials = fmt.Sprintf("%s databases connection-string %s --reveal", prog, node.Name)
	if info, gerr := client.GetDatabase(ctx, node.Name); gerr == nil {
		res.Host = node.Name
		res.Port = defaultEnginePort(info.Engine)
		if info.PubliclyAccessible && info.PublicPort > 0 {
			res.Port = info.PublicPort
		}
		if printURL {
			cred, rerr := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.DatabaseRevealedCredential, error) {
				return client.RevealDatabaseCredentials(ctx, node.Name, p)
			})
			if rerr != nil {
				return fail(fmt.Errorf("reveal credentials of %q: %w", node.Name, rerr))
			}
			dbName := ""
			if info.Engine == "postgres" {
				dbName = "postgres"
			}
			u, okURL := buildConnectionURL(connStringTarget{engine: info.Engine, user: defaultConnectUser(info.Engine), password: cred.Password, host: res.Host, port: res.Port, database: dbName, tls: info.TLSEnabled})
			if okURL {
				res.URL = u
			}
		}
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { printPRBranch(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printPRBranch(w io.Writer, r prBranchResult) {
	verb := "reused"
	if r.Created {
		verb = "created"
	}
	exp := r.ExpiresAt
	if exp == "" {
		exp = "never"
	}
	_, _ = fmt.Fprintf(w, "branch %s %s: mode=%s state=%s expires=%s\n", safeTerminalText(r.Branch), verb, safeTerminalText(r.Mode), safeTerminalText(r.State), safeTerminalText(exp))
	if r.Host != "" {
		_, _ = fmt.Fprintf(w, "host=%s port=%s database=%s\n", safeTerminalText(r.Host), strconv.Itoa(r.Port), safeTerminalText(r.DatabaseName))
	}
	if r.URL != "" {
		_, _ = fmt.Fprintf(w, "url=%s\n", r.URL)
		return
	}
	_, _ = fmt.Fprintf(w, "credentials: %s\n", r.Credentials)
}

// waitBranchReady polls until the branch is ready, fails on a failed state, and stops at the timeout.
func waitBranchReady(ctx context.Context, client *apiclient.Client, db, name string, node apiclient.DatabaseBranchNode, timeout, every time.Duration) (apiclient.DatabaseBranchNode, error) {
	deadline := time.Now().Add(timeout)
	for {
		switch node.State {
		case "ready":
			return node, nil
		case "failed":
			return node, fmt.Errorf("branch %q failed: %s", name, safeTerminalText(node.StateReason))
		case "promoted", "deleting", "expiring", "deleted":
			return node, fmt.Errorf("branch %q is %s and will not become ready", name, node.State)
		}
		if time.Now().After(deadline) {
			return node, fmt.Errorf("branch %q is still %q after %s", name, node.State, timeout)
		}
		time.Sleep(every)
		var err error
		if node, err = client.GetDatabaseBranch(ctx, db, name); err != nil {
			return node, err
		}
	}
}

func runDatabasesBranchPRClose(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	const cmd = "databases branch pr-close"
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, cmd, "print the result as JSON to stdout and nothing else", stderr)
	var prArg string
	var yes, force bool
	fs.StringVar(&prArg, "pr", "", "pull request number")
	fs.BoolVar(&yes, "yes", false, "delete without asking")
	fs.BoolVar(&force, "force", false, "also delete a protected branch")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesBranchUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: %s needs a database name\n\n", prog, cmd)
		fs.Usage()
		return exitUsage
	}
	db := rest[0]
	pr, err := parsePRNumber(prArg)
	var name string
	if err == nil {
		name, err = prBranchName(db, pr)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if !yes {
		return reportError(stdout, stderr, jsonOut, newValidationError("%s deletes branch %s and its data, pass --yes to confirm", cmd, name))
	}
	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	fail := func(err error) int { return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", cmd, err)) }
	// A wrong database name or a token without access must fail, never look like a clean close.
	if _, err := client.GetDatabase(ctx, db); err != nil {
		return fail(fmt.Errorf("database %q: %w", db, err))
	}
	node, err := client.GetDatabaseBranch(ctx, db, name)
	deleted := false
	switch {
	case isBranchNotFound(err):
	case err != nil:
		return fail(err)
	default:
		if node.Parent != db {
			return fail(fmt.Errorf("branch %q belongs to %q, not %q", name, node.Parent, db))
		}
		if node.State != "ready" && node.State != "failed" {
			return fail(fmt.Errorf("branch %q is %s, close it when it is ready or failed", name, node.State))
		}
		if node.Protected && !force {
			return fail(fmt.Errorf("branch %q is protected, pass --force to delete it", name))
		}
		if node.Protected {
			off := false
			if _, err := client.PatchDatabaseBranch(ctx, db, name, apiclient.PatchDatabaseBranchRequest{Protected: &off}); err != nil {
				return fail(err)
			}
		}
		if err := client.DeleteDatabaseBranch(ctx, db, name, name); err != nil && !isBranchNotFound(err) {
			if node.Protected {
				on := true
				_, _ = client.PatchDatabaseBranch(ctx, db, name, apiclient.PatchDatabaseBranchRequest{Protected: &on})
			}
			return fail(err)
		}
		deleted = true
	}
	if jsonOut {
		_, _ = fmt.Fprintf(stdout, "{\"branch\":%q,\"deleted\":%t}\n", name, deleted)
		return exitOK
	}
	if deleted {
		_, _ = fmt.Fprintf(stdout, "branch %q deleted\n", name)
	} else {
		_, _ = fmt.Fprintf(stdout, "branch %q not found, nothing to delete\n", name)
	}
	return exitOK
}
